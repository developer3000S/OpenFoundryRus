//go:build integration

// Integration tests for the Postgres-backed JobRepo + processed_events
// idempotency store. Boots a real postgres:16-alpine testcontainer via
// libs/testing.BootPostgres, applies the embedded reindex_jobs migration,
// and exercises the lifecycle the way the coordinator runtime will:
//
//   - upsert is idempotent (a second `requested.v1` redelivery returns
//     the same row and does not reset counters);
//   - state machine matches Rust: queued → running → completed | failed
//     | cancelled, terminal-self-loops are no-ops, cross-terminal flips
//     and resurrection attempts return *state.IllegalTransitionError;
//   - Advance bumps cumulative scanned/published and rotates the
//     resume cursor;
//   - ListResumable returns queued+running rows oldest-first;
//   - processed_events dedup: the same event_id never returns
//     OutcomeFirstSeen twice, even across PgStore instances on the
//     same pool.
//
// Opt-in via `go test -tags=integration ./...`.
package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/idempotency"
	testingx "github.com/openfoundry/openfoundry-go/libs/testing"
	"github.com/openfoundry/openfoundry-go/services/reindex-coordinator-service/internal/state"
)

func bootRepo(t *testing.T) *JobRepo {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	h := testingx.BootPostgres(ctx, t)
	require.NoError(t, Migrate(ctx, h.Pool))
	return NewJobRepo(h.Pool)
}

// ptr returns &v. Tiny helper because TestUpsertQueued and friends
// pass *string fields straight to the repo.
func ptr[T any](v T) *T { return &v }

// TestUpsertQueuedIsIdempotent — second insert of the same id returns
// the original row unchanged. Mirrors the Rust JobRepo::upsert_queued
// invariant that a duplicate `requested.v1` does not reset counters.
func TestUpsertQueuedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	first, err := r.UpsertQueued(ctx, id, "tenant-a", "person", 500)
	require.NoError(t, err)
	assert.Equal(t, state.StatusQueued, first.Status)
	assert.Equal(t, int32(500), first.PageSize)
	assert.Equal(t, "person", first.TypeID)

	// Bump some counters to prove the upsert no-ops, not overwrites.
	require.NoError(t, r.MarkRunning(ctx, id))
	require.NoError(t, r.Advance(ctx, id, ptr("token-1"), 42, 41))

	second, err := r.UpsertQueued(ctx, id, "tenant-a", "person", 999)
	require.NoError(t, err)
	assert.Equal(t, state.StatusRunning, second.Status, "upsert must not reset status")
	assert.Equal(t, int32(500), second.PageSize, "upsert must not overwrite page_size")
	assert.Equal(t, int64(42), second.Scanned)
	assert.Equal(t, int64(41), second.Published)
	require.NotNil(t, second.ResumeToken)
	assert.Equal(t, "token-1", *second.ResumeToken)
}

// TestUpsertQueuedAllTypes pins the "empty type_id == all-types"
// boundary form. Stored as empty string in Postgres so the
// (tenant_id, type_id) UNIQUE index treats it as one row per tenant.
func TestUpsertQueuedAllTypes(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	rec, err := r.UpsertQueued(ctx, id, "tenant-b", "", 1000)
	require.NoError(t, err)
	assert.Equal(t, "", rec.TypeID)
	assert.False(t, rec.HasTypeID())
}

// TestLoadMissingReturnsErrJobNotFound — Load distinguishes the
// missing-row case so callers do not have to type-assert pgx errors.
func TestLoadMissingReturnsErrJobNotFound(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	_, err := r.Load(ctx, uuid.New())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrJobNotFound))
}

// TestMarkRunningHappyPath — queued → running clears resume_token
// gating and any prior error.
func TestMarkRunningHappyPath(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-c", "asset", 100)
	require.NoError(t, err)

	require.NoError(t, r.MarkRunning(ctx, id))

	rec, err := r.Load(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, state.StatusRunning, rec.Status)
	assert.Nil(t, rec.Error)
}

// TestMarkRunningFromTerminalRejected — a coordinator must never
// resurrect a terminal job. Mirrors the Rust state machine's
// "no resurrection" rule.
func TestMarkRunningFromTerminalRejected(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-d", "asset", 100)
	require.NoError(t, err)
	_, err = r.MarkTerminal(ctx, id, state.StatusCancelled, nil)
	require.NoError(t, err)

	err = r.MarkRunning(ctx, id)
	require.Error(t, err)
	var ill *state.IllegalTransitionError
	require.True(t, errors.As(err, &ill))
	assert.Equal(t, state.StatusCancelled, ill.From)
	assert.Equal(t, state.StatusRunning, ill.To)
}

// TestAdvanceCumulativeCounters — counters accumulate across pages,
// resume_token rotates on every call, and the final page can clear
// the cursor by passing a nil token.
func TestAdvanceCumulativeCounters(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-e", "asset", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, id))

	require.NoError(t, r.Advance(ctx, id, ptr("page-1"), 100, 95))
	require.NoError(t, r.Advance(ctx, id, ptr("page-2"), 100, 100))
	require.NoError(t, r.Advance(ctx, id, nil, 50, 50))

	rec, err := r.Load(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, int64(250), rec.Scanned)
	assert.Equal(t, int64(245), rec.Published)
	assert.Nil(t, rec.ResumeToken, "final page clears the cursor")
}

// TestAdvanceRejectsNonRunning — Advance is gated on status='running'.
// A concurrent cancel that lands first must win.
func TestAdvanceRejectsNonRunning(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-f", "asset", 100)
	require.NoError(t, err)
	// Skip MarkRunning: row is still 'queued'.
	err = r.Advance(ctx, id, ptr("page-1"), 1, 1)
	require.Error(t, err)
	var ill *state.IllegalTransitionError
	require.True(t, errors.As(err, &ill))
	assert.Equal(t, state.StatusQueued, ill.From)
	assert.Equal(t, state.StatusRunning, ill.To)
}

// TestMarkTerminalCompleted — running → completed stamps completed_at
// and clears resume_token gating.
func TestMarkTerminalCompleted(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-g", "asset", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, id))

	rec, err := r.MarkTerminal(ctx, id, state.StatusCompleted, nil)
	require.NoError(t, err)
	assert.Equal(t, state.StatusCompleted, rec.Status)
	require.NotNil(t, rec.CompletedAt)
	assert.WithinDuration(t, time.Now(), *rec.CompletedAt, time.Minute)
}

// TestMarkTerminalFailedRecordsError — failed transition writes the
// error column, stamps completed_at.
func TestMarkTerminalFailedRecordsError(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-h", "asset", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, id))

	rec, err := r.MarkTerminal(ctx, id, state.StatusFailed, ptr("scan exploded"))
	require.NoError(t, err)
	assert.Equal(t, state.StatusFailed, rec.Status)
	require.NotNil(t, rec.Error)
	assert.Equal(t, "scan exploded", *rec.Error)
}

// TestMarkTerminalIdempotentSelfLoop — replaying a terminal event must
// not raise. Coordinator runtime relies on this for safe Kafka
// redeliveries of the completed.v1 publish step.
func TestMarkTerminalIdempotentSelfLoop(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-i", "asset", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, id))
	first, err := r.MarkTerminal(ctx, id, state.StatusCompleted, nil)
	require.NoError(t, err)
	second, err := r.MarkTerminal(ctx, id, state.StatusCompleted, nil)
	require.NoError(t, err)
	assert.Equal(t, first.UpdatedAt, second.UpdatedAt, "self-loop must not bump updated_at")
}

// TestMarkTerminalCrossTerminalRejected — failed → completed (or any
// other cross-terminal flip) must surface IllegalTransition.
func TestMarkTerminalCrossTerminalRejected(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-j", "asset", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, id))
	_, err = r.MarkTerminal(ctx, id, state.StatusFailed, ptr("boom"))
	require.NoError(t, err)

	_, err = r.MarkTerminal(ctx, id, state.StatusCompleted, nil)
	require.Error(t, err)
	var ill *state.IllegalTransitionError
	require.True(t, errors.As(err, &ill))
	assert.Equal(t, state.StatusFailed, ill.From)
	assert.Equal(t, state.StatusCompleted, ill.To)
}

// TestMarkTerminalRejectsNonTerminal — passing a non-terminal status
// is a programmer error and must fail before touching SQL.
func TestMarkTerminalRejectsNonTerminal(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	id := uuid.New()
	_, err := r.UpsertQueued(ctx, id, "tenant-k", "asset", 100)
	require.NoError(t, err)

	_, err = r.MarkTerminal(ctx, id, state.StatusRunning, nil)
	require.Error(t, err)
	var ill *state.IllegalTransitionError
	require.True(t, errors.As(err, &ill))
	assert.Equal(t, state.StatusRunning, ill.To)
}

// TestListResumableReturnsQueuedAndRunning — restart-time recovery
// query: queued + running rows, oldest first; terminal rows excluded.
func TestListResumableReturnsQueuedAndRunning(t *testing.T) {
	ctx := context.Background()
	r := bootRepo(t)

	queuedID := uuid.New()
	_, err := r.UpsertQueued(ctx, queuedID, "tenant-l", "first", 100)
	require.NoError(t, err)

	// Force a small gap so the started_at ordering is deterministic.
	time.Sleep(10 * time.Millisecond)

	runningID := uuid.New()
	_, err = r.UpsertQueued(ctx, runningID, "tenant-l", "second", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, runningID))

	completedID := uuid.New()
	_, err = r.UpsertQueued(ctx, completedID, "tenant-l", "third", 100)
	require.NoError(t, err)
	require.NoError(t, r.MarkRunning(ctx, completedID))
	_, err = r.MarkTerminal(ctx, completedID, state.StatusCompleted, nil)
	require.NoError(t, err)

	rows, err := r.ListResumable(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2, "completed row must be filtered out")
	assert.Equal(t, queuedID, rows[0].ID, "oldest first")
	assert.Equal(t, runningID, rows[1].ID)
}

// TestProcessedEventsCheckAndRecordDedup — the per-batch dedup table
// returns OutcomeFirstSeen exactly once per event_id, regardless of
// PgStore instance churn. Coordinator guarantees the (tenant, type,
// token) → uuid_v5 mapping; this test pins the SQL primitive.
func TestProcessedEventsCheckAndRecordDedup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	h := testingx.BootPostgres(ctx, t)
	require.NoError(t, Migrate(ctx, h.Pool))

	store := NewProcessedEventsStore(h.Pool)
	eventID := uuid.New()

	out, err := store.CheckAndRecord(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, out.IsFirstSeen())

	// Same event id, same store: dedup wins.
	out, err = store.CheckAndRecord(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, out.IsAlreadyProcessed())

	// Same event id, brand-new store on the same pool: still deduped
	// because the dedup is in the DB, not the in-memory client.
	other := NewProcessedEventsStore(h.Pool)
	out, err = other.CheckAndRecord(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, out.IsAlreadyProcessed())

	// A different event id sails through.
	out, err = store.CheckAndRecord(ctx, uuid.New())
	require.NoError(t, err)
	assert.True(t, out.IsFirstSeen())
}

// TestProcessedEventsStoreSatisfiesInterface — compile-time guard:
// NewProcessedEventsStore returns idempotency.Store. Catches accidental
// signature drift that would break wiring in cmd/main.go (the
// consumer-loop slice depends on this).
func TestProcessedEventsStoreSatisfiesInterface(t *testing.T) {
	var _ idempotency.Store = NewProcessedEventsStore(nil)
}
