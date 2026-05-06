// Package queryengine is the placeholder for libs/query-engine in
// the Go workspace. The Rust crate is a thin wrapper around
// [Apache DataFusion] (its `SessionContext`, `DataFrame`,
// `LogicalPlan`, plus an Arrow Flight SQL `TableProvider`) and is
// not 1:1 portable to Go — the apache/arrow-go module ships only
// the Arrow data layer; there is no Go port of the DataFusion SQL
// engine.
//
// Resolving this lib is therefore blocked on a human decision (see
// MIGRATION-LOOP-STATUS.md, item P4.8). The viable options are:
//
//  1. Sidecar / FFI to the Rust crate (mirrors the pyo3 sidecar
//     decision pending for notebook-runtime, pipeline-build,
//     ontology-actions).
//  2. Replace the engine with a different Go-native SQL surface
//     (e.g. embedded DuckDB, or federate to an external Trino /
//     Presto / Flight SQL endpoint). This is *not* a 1:1 port.
//  3. Keep the Rust crate as the engine and have Go services consume
//     it over gRPC / Flight SQL.
//
// Until that decision is made, this package stays empty. The Rust
// source under libs/query-engine/src/ is the source of truth.
//
// [Apache DataFusion]: https://datafusion.apache.org/
package queryengine
