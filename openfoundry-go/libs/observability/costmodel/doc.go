// Package costmodel will host the Foundry compute-seconds cost table
// for media-set transformations once `media-transform-runtime-service`
// is migrated.
//
// The Rust source of truth lives at:
//
//	libs/observability/src/cost_model.rs
//
// Migration policy: do NOT port the table speculatively — rates are
// pinned to the public Foundry doc and a snapshot test guards drift.
// Port the constants together with the consumer service in Phase 3.
package costmodel
