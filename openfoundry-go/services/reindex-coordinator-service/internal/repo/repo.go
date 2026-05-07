// Postgres-backed JobRepo + processed_events idempotency store for the
// reindex coordinator (1:1 port of services/reindex-coordinator-service
// `src/state.rs::repo` and `src/main.rs::PROCESSED_EVENTS_TABLE`).
//
// Every write funnels through SQL that gates on the previous status in
// the WHERE clause, so two coordinator replicas racing on the same row
// produce at most one successful UPDATE; the loser sees rows_affected
// == 0 and surfaces a typed *state.IllegalTransitionError.
package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/idempotency"
	"github.com/openfoundry/openfoundry-go/services/reindex-coordinator-service/internal/state"
)

// ProcessedEventsTable is the fully-qualified name of the per-batch
// dedup table. Mirrors `PROCESSED_EVENTS_TABLE` in the Rust main.rs.
const ProcessedEventsTable = "reindex_coordinator.processed_events"

// NewProcessedEventsStore wires an idempotency.PgStore to the
// reindex-coordinator processed_events table. The Kafka consumer
// records the deterministic batch event id (uuid_v5 of
// tenant||type||token) BEFORE producing each batch, so a crash between
// "produce batch" and "advance resume_token" replays safely without
// double-publishing.
func NewProcessedEventsStore(pool *pgxpool.Pool) *idempotency.PgStore {
	return idempotency.NewPgStore(pool, ProcessedEventsTable)
}

// JobRecord is one row of reindex_coordinator.reindex_jobs.
//
// TypeID is the empty string in Postgres for the all-types path;
// surfaced here as "" rather than nil to match the Go convention.
// The Rust mirror exposes Option<String>; consumers that need the
// "absent" boundary form can call HasTypeID().
type JobRecord struct {
	ID          uuid.UUID
	TenantID    string
	TypeID      string
	Status      state.JobStatus
	ResumeToken *string
	PageSize    int32
	Scanned     int64
	Published   int64
	Error       *string
	StartedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

// HasTypeID reports whether the row is a per-type scan. Empty TypeID
// means the all-types ALLOW FILTERING path.
func (r JobRecord) HasTypeID() bool { return r.TypeID != "" }

// JobRepo is the Postgres-backed repository for reindex_jobs.
type JobRepo struct {
	pool *pgxpool.Pool
}

// NewJobRepo returns a JobRepo bound to pool.
func NewJobRepo(pool *pgxpool.Pool) *JobRepo { return &JobRepo{pool: pool} }

// Pool returns the underlying connection pool.
func (r *JobRepo) Pool() *pgxpool.Pool { return r.pool }

// ErrJobNotFound is returned by Load when the row does not exist.
var ErrJobNotFound = errors.New("repo: reindex job not found")

// UpsertQueued is the idempotent claim path. It inserts the row at
// status 'queued' (or no-ops if a sibling has already inserted it),
// then loads and returns the current snapshot. Callers decide what
// to do with non-queued returns (e.g. ignore a duplicate
// `requested.v1` for a still-running job).
//
// typeID may be the empty string to mean "all-types"; that maps to
// the same empty-string convention the SQL DEFAULT uses, so the
// (tenant_id, type_id) UNIQUE index treats it as a single row per
// tenant.
func (r *JobRepo) UpsertQueued(
	ctx context.Context,
	id uuid.UUID,
	tenantID string,
	typeID string,
	pageSize int32,
) (JobRecord, error) {
	const sql = `
		INSERT INTO reindex_coordinator.reindex_jobs
			(id, tenant_id, type_id, status, page_size)
		VALUES ($1, $2, $3, 'queued', $4)
		ON CONFLICT (id) DO NOTHING
	`
	if _, err := r.pool.Exec(ctx, sql, id, tenantID, typeID, pageSize); err != nil {
		return JobRecord{}, fmt.Errorf("repo: upsert queued: %w", err)
	}
	return r.Load(ctx, id)
}

// Load fetches one job by id. Returns ErrJobNotFound for a missing row.
func (r *JobRepo) Load(ctx context.Context, id uuid.UUID) (JobRecord, error) {
	const sql = `
		SELECT id, tenant_id, type_id, status, resume_token, page_size,
		       scanned, published, error, started_at, updated_at, completed_at
		  FROM reindex_coordinator.reindex_jobs
		 WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, sql, id)
	rec, err := scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobRecord{}, ErrJobNotFound
	}
	return rec, err
}

// ListResumable returns every job whose status is queued or running,
// oldest first. The coordinator drives the post-restart catch-up loop
// off this list instead of replaying Kafka.
func (r *JobRepo) ListResumable(ctx context.Context) ([]JobRecord, error) {
	const sql = `
		SELECT id, tenant_id, type_id, status, resume_token, page_size,
		       scanned, published, error, started_at, updated_at, completed_at
		  FROM reindex_coordinator.reindex_jobs
		 WHERE status IN ('queued', 'running')
		 ORDER BY started_at ASC
	`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("repo: list resumable: %w", err)
	}
	defer rows.Close()

	var out []JobRecord
	for rows.Next() {
		rec, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list resumable: %w", err)
	}
	return out, nil
}

// MarkRunning flips the row to 'running' (or self-loops for an
// already-running row) and clears any leftover error. Returns
// *state.IllegalTransitionError if the current status is terminal
// or if a concurrent writer has already moved the row out from
// underneath us.
func (r *JobRepo) MarkRunning(ctx context.Context, id uuid.UUID) error {
	current, err := r.Load(ctx, id)
	if err != nil {
		return err
	}
	if err := current.Status.EnsureTransition(state.StatusRunning); err != nil {
		return err
	}
	const sql = `
		UPDATE reindex_coordinator.reindex_jobs
		   SET status = 'running',
		       error = NULL,
		       updated_at = now()
		 WHERE id = $1
		   AND status IN ('queued', 'running')
	`
	tag, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("repo: mark running: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.lostRace(ctx, id, state.StatusRunning)
	}
	return nil
}

// Advance persists a successful page: bumps cumulative counters and
// moves the resume cursor. Pass nextResumeToken == nil to clear the
// cursor (the final page). Gated on status='running' so a concurrent
// cancel wins.
func (r *JobRepo) Advance(
	ctx context.Context,
	id uuid.UUID,
	nextResumeToken *string,
	scannedDelta int64,
	publishedDelta int64,
) error {
	const sql = `
		UPDATE reindex_coordinator.reindex_jobs
		   SET resume_token = $2,
		       scanned   = scanned   + $3,
		       published = published + $4,
		       updated_at = now()
		 WHERE id = $1
		   AND status = 'running'
	`
	tag, err := r.pool.Exec(ctx, sql, id, nextResumeToken, scannedDelta, publishedDelta)
	if err != nil {
		return fmt.Errorf("repo: advance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.lostRace(ctx, id, state.StatusRunning)
	}
	return nil
}

// MarkTerminal flips the row to a terminal status (completed, failed,
// or cancelled) and stamps completed_at. errMessage is required iff
// next == StatusFailed; pass nil otherwise.
//
// Idempotent self-loop: re-marking an already-terminal row with the
// same status returns the existing snapshot unchanged so a Kafka
// redelivery does not raise. Cross-terminal flips
// (e.g. failed → completed) return *state.IllegalTransitionError.
func (r *JobRepo) MarkTerminal(
	ctx context.Context,
	id uuid.UUID,
	next state.JobStatus,
	errMessage *string,
) (JobRecord, error) {
	if !next.IsTerminal() {
		return JobRecord{}, &state.IllegalTransitionError{From: state.StatusRunning, To: next}
	}
	current, err := r.Load(ctx, id)
	if err != nil {
		return JobRecord{}, err
	}
	if current.Status == next {
		// Idempotent self-loop: redelivery of the same terminal event.
		return current, nil
	}
	if err := current.Status.EnsureTransition(next); err != nil {
		return JobRecord{}, err
	}
	const sql = `
		UPDATE reindex_coordinator.reindex_jobs
		   SET status = $2,
		       error = $3,
		       updated_at = now(),
		       completed_at = now()
		 WHERE id = $1
		   AND status IN ('queued', 'running')
	`
	tag, err := r.pool.Exec(ctx, sql, id, next.String(), errMessage)
	if err != nil {
		return JobRecord{}, fmt.Errorf("repo: mark terminal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return JobRecord{}, r.lostRace(ctx, id, next)
	}
	return r.Load(ctx, id)
}

// lostRace reloads the row and synthesises an IllegalTransition error
// describing the (current → attempted) flip that lost the race.
func (r *JobRepo) lostRace(ctx context.Context, id uuid.UUID, attempted state.JobStatus) error {
	now, err := r.Load(ctx, id)
	if err != nil {
		return err
	}
	return &state.IllegalTransitionError{From: now.Status, To: attempted}
}

// scanRow decodes one pgx row into a JobRecord. Accepts pgx.Row
// (single-row QueryRow) and pgx.Rows (Query loop) via the Scan-only
// interface they both implement.
func scanRow(row pgx.Row) (JobRecord, error) {
	var (
		rec         JobRecord
		statusStr   string
		typeIDDB    string
		resumeToken *string
		errMessage  *string
		completedAt *time.Time
	)
	if err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&typeIDDB,
		&statusStr,
		&resumeToken,
		&rec.PageSize,
		&rec.Scanned,
		&rec.Published,
		&errMessage,
		&rec.StartedAt,
		&rec.UpdatedAt,
		&completedAt,
	); err != nil {
		return JobRecord{}, err
	}
	st, err := state.ParseStatus(statusStr)
	if err != nil {
		return JobRecord{}, err
	}
	rec.TypeID = typeIDDB
	rec.Status = st
	rec.ResumeToken = resumeToken
	rec.Error = errMessage
	rec.CompletedAt = completedAt
	return rec, nil
}
