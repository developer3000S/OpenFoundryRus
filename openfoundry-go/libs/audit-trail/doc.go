// Package audittrail is the Go counterpart of Rust's `audit-trail` crate.
//
// What this package owns
//
//   - The 13 canonical audit event variants (media-set + media-item).
//   - The 7 Foundry audit categories (camelCase JSON tokens).
//   - AuditContext for request-side metadata (actor, IP, request-id, …).
//   - AuditEnvelope, the wire format published on Kafka topic
//     `audit.events.v1` (TopicAuditEvents constant).
//   - DeriveEventID, a deterministic v5 UUID derivation that ensures
//     retried handlers collapse to the same outbox row.
//
// Wire-format invariants
//
// JSON byte-identical to the Rust crate so audit-sink does not care
// which language emitted the event:
//
//   - Top-level: `event_id`, `at` (epoch microseconds), `kind`,
//     `categories`, `resource_rid`, `project_rid`, `markings_at_event`,
//     `occurred_at` (RFC3339), `payload`.
//   - Optional request-side fields are omitted when unset.
//   - `payload.kind` is the variant discriminator (snake_case +
//     dotted, e.g. "media_set.created").
//
// Outbox publisher
//
// EmitToOutbox composes audit envelope + outbox.Enqueue inside a
// caller-owned pgx transaction so the SQL mutation and the audit
// emission land atomically (ADR-0022).
package audittrail
