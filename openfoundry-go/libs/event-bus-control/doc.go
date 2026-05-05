// Package controlbus is the OpenFoundry control-plane event bus
// (NATS JetStream) — the Go counterpart of Rust's `event-bus-control`.
//
// What this package owns
//
//   - Connect          — open a NATS connection + JetStream context.
//   - Publisher        — typed event publishing with the canonical
//     Event<T> envelope.
//   - EnsureStream     — idempotent stream creation with the same
//     defaults the Rust crate uses (limits retention, 1M msgs, 7d).
//   - CreateConsumer   — durable pull consumer.
//   - Subjects/Streams — well-known constants (`of.auth`, `of.datasets`,
//     `OF_EVENTS`, …) shared with Rust.
//
// Wire compatibility: Event<T> serialises with the same JSON shape as
// the Rust side so a Rust publisher and a Go consumer (or vice versa)
// round-trip the envelope unchanged.
package controlbus
