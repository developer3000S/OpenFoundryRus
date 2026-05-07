package repo

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestJobRecordHasTypeID — empty TypeID means the all-types scan path.
// Pinned because the empty-string boundary representation differs
// from the Rust Option<String> form and the runtime depends on it.
func TestJobRecordHasTypeID(t *testing.T) {
	t.Parallel()
	assert.False(t, JobRecord{}.HasTypeID())
	assert.True(t, JobRecord{TypeID: "person"}.HasTypeID())
}

// ProcessedEventsTable mirrors the constant Rust pins in main.rs.
// Drift here would silently disable per-batch idempotency.
func TestProcessedEventsTableConstant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "reindex_coordinator.processed_events", ProcessedEventsTable)
}

// NewProcessedEventsStore returns a non-nil store wired to the
// canonical table. Smoke-only — the actual CheckAndRecord round-trip
// is exercised by the integration test build tag.
func TestNewProcessedEventsStoreUsesCanonicalTable(t *testing.T) {
	t.Parallel()
	store := NewProcessedEventsStore(nil)
	assert.NotNil(t, store)
	assert.Equal(t, ProcessedEventsTable, store.Table())
}

// Sanity check: uuid.New produces distinct ids on consecutive calls.
// Guards against an accidental swap to a deterministic generator that
// would break the (id, status) idempotency wiring.
func TestUUIDDistinctness(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	assert.NotEqual(t, a, b)
}
