// Package idempotency is the consumer-side deduplication helper for
// at-least-once delivery (Kafka topics, NATS work queues).
//
// Every consumer calls Store.CheckAndRecord(eventID) BEFORE processing.
// At most one caller across the cluster ever sees OutcomeFirstSeen for
// a given eventID; everyone else sees OutcomeAlreadyProcessed. The
// Idempotent wrapper composes the store with a closure for the common
// "run f exactly once per eventID" pattern.
//
// Backends:
//
//   - PgStore  — INSERT ... ON CONFLICT DO NOTHING RETURNING event_id
//     (one round-trip, atomic).
//   - MemStore — process-local map for unit tests.
//
// Cassandra LWT-backed store is deferred until cassandra-kernel lands.
//
// # Record-before-process semantics
//
// CheckAndRecord records the eventID before the closure runs. A failed
// closure leaves the eventID marked as processed; the next redelivery
// will skip. Wrap side effects in an outbox / saga so this corner case
// is not silent data loss. Same trade-off the Rust crate makes — see
// the package doc on the Rust side.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Outcome is the result of CheckAndRecord.
type Outcome int

const (
	// OutcomeFirstSeen — caller is the unique owner of this eventID
	// and should run side effects.
	OutcomeFirstSeen Outcome = iota
	// OutcomeAlreadyProcessed — eventID was recorded by an earlier
	// delivery (this consumer or a sibling). Caller MUST skip.
	OutcomeAlreadyProcessed
)

// IsFirstSeen reports whether the caller should run the work.
func (o Outcome) IsFirstSeen() bool { return o == OutcomeFirstSeen }

// IsAlreadyProcessed reports whether the caller should skip the work.
func (o Outcome) IsAlreadyProcessed() bool { return o == OutcomeAlreadyProcessed }

// Store is the atomic "is this event new?" primitive.
type Store interface {
	CheckAndRecord(ctx context.Context, eventID uuid.UUID) (Outcome, error)
}

// ErrBackend wraps a backend storage failure (Postgres, etc.) so the
// trait stays driver-agnostic.
type ErrBackend struct{ Cause error }

func (e *ErrBackend) Error() string { return "idempotency backend: " + e.Cause.Error() }
func (e *ErrBackend) Unwrap() error { return e.Cause }

// ─── Wrapper ────────────────────────────────────────────────────────────

// Idempotent runs f exactly once per eventID.
//
// First delivery: records eventID, runs f, returns (T, true, nil).
// Redelivery:    skips f, returns (zero, false, nil).
// Backend error: returns (zero, false, *ErrBackend).
// Closure error: returns (zero, true, error from f) — eventID stays recorded.
func Idempotent[T any](
	ctx context.Context,
	store Store,
	eventID uuid.UUID,
	f func(context.Context) (T, error),
) (value T, ran bool, err error) {
	outcome, berr := store.CheckAndRecord(ctx, eventID)
	if berr != nil {
		return value, false, berr
	}
	if outcome.IsAlreadyProcessed() {
		slog.Debug("skipped duplicate delivery", slog.String("event_id", eventID.String()))
		return value, false, nil
	}
	value, err = f(ctx)
	return value, true, err
}

// ─── In-memory store ────────────────────────────────────────────────────

// MemStore is a process-local Store backed by a map. Tests + dev only —
// state evaporates on restart.
type MemStore struct {
	mu   sync.Mutex
	seen map[uuid.UUID]struct{}
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore { return &MemStore{seen: make(map[uuid.UUID]struct{})} }

// CheckAndRecord implements Store.
func (m *MemStore) CheckAndRecord(_ context.Context, eventID uuid.UUID) (Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.seen[eventID]; dup {
		return OutcomeAlreadyProcessed, nil
	}
	m.seen[eventID] = struct{}{}
	return OutcomeFirstSeen, nil
}

// Len reports how many distinct eventIDs are currently recorded.
func (m *MemStore) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.seen)
}

// ─── Postgres store ─────────────────────────────────────────────────────

// PgStore is a Postgres-backed Store.
//
// Table is the fully-qualified `schema.table` of the dedup table; it
// MUST already exist with the canonical shape:
//
//	event_id     uuid PRIMARY KEY
//	processed_at timestamptz NOT NULL DEFAULT now()
//
// The table name is a Go string but the recommended pattern is to keep
// it in a const inside the calling service so it cannot be flexed at
// runtime. Postgres does not allow parameterising table names so the
// SQL is built via fmt.Sprintf — operator-controlled, never user input.
type PgStore struct {
	pool  *pgxpool.Pool
	table string
}

// NewPgStore wires a PgStore to `table` (e.g. `idem.processed_events`).
func NewPgStore(pool *pgxpool.Pool, table string) *PgStore {
	if table == "" {
		panic("idempotency: table must not be empty")
	}
	return &PgStore{pool: pool, table: table}
}

// Pool returns the underlying connection pool (tests, shutdown).
func (s *PgStore) Pool() *pgxpool.Pool { return s.pool }

// Table returns the fully-qualified table name this store writes to.
func (s *PgStore) Table() string { return s.table }

// CheckAndRecord implements Store via INSERT ... ON CONFLICT DO NOTHING RETURNING.
func (s *PgStore) CheckAndRecord(ctx context.Context, eventID uuid.UUID) (Outcome, error) {
	sql := fmt.Sprintf(
		`INSERT INTO %s (event_id) VALUES ($1)
		 ON CONFLICT (event_id) DO NOTHING
		 RETURNING event_id`, s.table,
	)
	var returned uuid.UUID
	err := s.pool.QueryRow(ctx, sql, eventID).Scan(&returned)
	switch {
	case err == nil:
		return OutcomeFirstSeen, nil
	case errors.Is(err, pgx.ErrNoRows):
		// RETURNING emitted zero rows — the duplicate path.
		return OutcomeAlreadyProcessed, nil
	default:
		return OutcomeAlreadyProcessed, &ErrBackend{Cause: err}
	}
}
