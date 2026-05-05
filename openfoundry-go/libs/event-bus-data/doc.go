// Package databus is the OpenFoundry data-plane event bus (Kafka) —
// the Go counterpart of Rust's `event-bus-data`.
//
// Why split it from the control plane (controlbus / NATS)
//
//	| Plane     | Transport      | Latency | Retention  | Use cases                            |
//	|-----------|----------------|---------|------------|--------------------------------------|
//	| controlbus| NATS JetStream | µs–ms   | hours/days | RPC events, signals, fan-out         |
//	| databus   | Apache Kafka   | ms      | weeks–PB   | CDC, ingestion, lineage, analytics   |
//
// Delivery semantics
//
// Publisher and Subscriber expose at-least-once delivery with explicit
// commits. Consumers MUST call DataMessage.Commit() (or
// Subscriber.CommitOffsets()) after a record has been durably
// processed. Auto-commit is disabled in the default consumer config.
//
// OpenLineage headers
//
// Records carry the small, well-known set of Kafka headers
// (`ol-namespace`, `ol-job-name`, `ol-run-id`, `ol-event-time`,
// `ol-producer`, `ol-schema-url`) so any consumer (Rust, Go, Python,
// Java) can extract them without a schema registry lookup. See
// OpenLineageHeaders.
//
// Auto-creation and ACLs
//
// Topic auto-creation is disabled. Topic provisioning is handled out
// of band by the platform's topic registry, and every service
// authenticates with its own SASL principal — see ServicePrincipal.
//
// Implementation note
//
// The Rust crate uses rdkafka (librdkafka). The Go side uses
// segmentio/kafka-go (pure Go, no CGO) — the wire-level Kafka protocol
// is identical so brokers cannot tell which client is connected.
// Producer settings (acks=all, idempotent writes, compression) match
// the Rust defaults.
package databus
