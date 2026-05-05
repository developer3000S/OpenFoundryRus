// Package outbox is the transactional outbox helper for ADR-0022.
//
// One function: Enqueue(ctx, tx, event). The event is appended to
// `outbox.events` and immediately deleted in the same transaction.
// Both records land in the WAL when the transaction commits; the
// Debezium Postgres connector captures the INSERT (relayed to Kafka
// by EventRouter SMT) and discards the DELETE
// (`tombstones.on.delete=false`). Net effect: the row is gone before
// commit, the WAL still carries the full payload, and the table stays
// empty in steady state without a janitor.
//
// # OpenLineage headers
//
// OutboxEvent.Headers is a free-form map[string]string that the
// EventRouter SMT copies onto Kafka record headers. Producers should
// populate `ol-run-id`, `ol-parent-run-id`, `ol-namespace` and `ol-job`
// whenever a lineage context is in scope.
//
// # Schema
//
//	CREATE SCHEMA IF NOT EXISTS outbox;
//	CREATE TABLE outbox.events (
//	  event_id     uuid PRIMARY KEY,
//	  aggregate    text NOT NULL,
//	  aggregate_id text NOT NULL,
//	  payload      jsonb NOT NULL,
//	  headers      jsonb NOT NULL,
//	  topic        text NOT NULL,
//	  created_at   timestamptz NOT NULL DEFAULT now()
//	);
//	ALTER TABLE outbox.events REPLICA IDENTITY FULL;
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OutboxEvent is one domain event ready to be appended to outbox.events.
//
// EventID should be deterministic (typically a v5 UUID derived from
// aggregate || aggregate_id || version) so retries stay idempotent
// under the table's primary key.
type OutboxEvent struct {
	EventID     uuid.UUID
	Aggregate   string
	AggregateID string
	Topic       string
	Payload     json.RawMessage
	Headers     map[string]string
}

// New builds a minimal OutboxEvent with no headers.
func New(eventID uuid.UUID, aggregate, aggregateID, topic string, payload json.RawMessage) *OutboxEvent {
	return &OutboxEvent{
		EventID:     eventID,
		Aggregate:   aggregate,
		AggregateID: aggregateID,
		Topic:       topic,
		Payload:     payload,
		Headers:     make(map[string]string),
	}
}

// WithHeader returns the event with `key=value` set on Headers.
func (e *OutboxEvent) WithHeader(key, value string) *OutboxEvent {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
	return e
}

// Enqueue appends `event` to outbox.events inside `tx` and immediately
// deletes the row in the same transaction. Returns nil on success;
// duplicate event_id is silently treated as a no-op (idempotent retry).
//
// The caller owns the transaction lifecycle — this helper never commits
// or rolls back. Pair it with the application's primary write so a
// single tx.Commit() atomically publishes both.
func Enqueue(ctx context.Context, tx pgx.Tx, event *OutboxEvent) error {
	headersJSON, err := json.Marshal(event.Headers)
	if err != nil {
		return fmt.Errorf("serialize outbox headers: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO outbox.events
		   (event_id, aggregate, aggregate_id, payload, headers, topic)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, event.Aggregate, event.AggregateID,
		event.Payload, headersJSON, event.Topic,
	)
	if err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Duplicate event_id — another transaction already emitted
		// this event. Nothing more to do; the WAL record from the
		// earlier transaction has been (or will be) captured by Debezium.
		slog.Debug("outbox enqueue no-op for duplicate event_id",
			slog.String("event_id", event.EventID.String()),
			slog.String("topic", event.Topic),
		)
		return nil
	}

	// DELETE in the same transaction. The WAL still carries the full
	// INSERT (REPLICA IDENTITY FULL is set on the table), Debezium emits
	// it via EventRouter SMT, and the DELETE event is dropped via the
	// connector's tombstones.on.delete=false.
	if _, err := tx.Exec(ctx,
		`DELETE FROM outbox.events WHERE event_id = $1`,
		event.EventID,
	); err != nil {
		return fmt.Errorf("outbox tombstone delete: %w", err)
	}
	return nil
}
