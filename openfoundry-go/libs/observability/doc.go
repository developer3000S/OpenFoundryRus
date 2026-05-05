// Package observability is the OpenFoundry replacement for Rust's
// `libs/observability` + the parts of `core-models::observability`
// concerned with tracing initialisation.
//
// It bundles three concerns every service needs at boot:
//
//   - Structured logging (Init): slog with JSON or text format.
//   - OTel tracing (InitTracing): span pipeline → OTLP/gRPC exporter.
//   - Prometheus registry (NewMetrics): default + service-scoped metrics.
//
// Subpackages (forthcoming during Phase 1 migration):
//
//   - costmodel — Foundry compute-seconds cost table for media transforms.
package observability
