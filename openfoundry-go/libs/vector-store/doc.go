// Package vectorstore mirrors libs/vector-store from the Rust
// workspace: a small backend-agnostic abstraction
// ([VectorBackend]) plus one or more concrete implementations.
//
// Subpackages:
//
//   - This package — the [VectorBackend] interface and the
//     [PgVectorBackend] skeleton (returns ErrUnimplemented until
//     the pgx + pgvector wiring lands).
//   - [vespa] — hybrid search (BM25 + ANN HNSW + phased ranking)
//     backed by Vespa over its HTTP/JSON Document v1 and Search
//     APIs. Importable on its own; the package never forces a
//     dependency on the others.
//
// Adding a new backend is purely additive and does not change the
// existing surface.
package vectorstore
