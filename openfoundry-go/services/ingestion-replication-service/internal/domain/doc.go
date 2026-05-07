// Package domain holds the typed projections + interfaces that the
// in-process streaming engine operates on. The wire-shape DTOs live in
// internal/models; this package converts them into the strongly-typed
// projections the engine needs (parsed schemas, parsed connector bindings,
// concrete window/topology metadata).
//
// Mirrors event_streaming::domain in the Rust source — same module
// boundary so 1:1 ports of the helpers (backpressure, engine processor,
// runtime store) read the same way in Go and Rust.
package domain
