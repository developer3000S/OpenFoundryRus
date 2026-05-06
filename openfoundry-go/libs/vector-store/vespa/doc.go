// Package vespa is the [Vespa] backend for hybrid (BM25 + ANN HNSW
// + phased ranking) search. Mirrors libs/vector-store/src/vespa
// from the Rust workspace.
//
// Importing this package is purely additive: the
// [vectorstore.PgVectorBackend] skeleton and the
// [vectorstore.VectorBackend] surface stay unchanged.
//
// # Why a hand-rolled HTTP client?
//
// As of writing there is no widely-adopted, maintained Vespa client
// for Go that supports both the Document v1 CRUD API and the Search
// API with custom YQL + tensor query parameters. Vespa's public APIs
// are plain HTTP/JSON, so we wrap them with the standard library's
// `net/http` and `encoding/json`.
//
// # Endpoints used
//
//   - POST   /document/v1/<namespace>/<doctype>/docid/<id> — upsert
//   - DELETE /document/v1/<namespace>/<doctype>/docid/<id> — delete
//   - POST   /search/                                       — YQL +
//     `input.query(q_embedding)` tensor
//
// See the [Vespa HTTP API reference] and the [Search API
// reference].
//
// # Recommended schema (deploy this in your application package)
//
//	schema doc {
//	  document doc {
//	    field text type string {
//	      indexing: index | summary
//	      index: enable-bm25
//	    }
//	    field tenant_id type string {
//	      indexing: attribute | summary
//	    }
//	    field embedding type tensor<float>(x[N]) {
//	      indexing: attribute | index
//	      attribute { distance-metric: angular }
//	      index {
//	        hnsw {
//	          max-links-per-node: 16
//	          neighbors-to-explore-at-insert: 200
//	        }
//	      }
//	    }
//	  }
//
//	  rank-profile hybrid {
//	    inputs { query(q_embedding) tensor<float>(x[N]) }
//	    first-phase  { expression: bm25(text) + closeness(field, embedding) }
//	    second-phase { expression: firstPhase }      // tune as needed
//	  }
//	}
//
// See [Backend] for usage.
//
// [Vespa]: https://vespa.ai
// [Vespa HTTP API reference]: https://docs.vespa.ai/en/reference/document-v1-api-reference.html
// [Search API reference]: https://docs.vespa.ai/en/reference/query-api-reference.html
package vespa
