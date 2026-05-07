# openfoundry-go stub / TODO audit

Date: 2026-05-07

## Method

Primary search command:

```sh
rg -n "TODO|FIXME|stub|not implemented|NotImplemented|http\.StatusNotImplemented|Status: stub" openfoundry-go --glob '*.go' --glob '!*_test.go'
```

The scan returned 131 matches. This report groups adjacent matches that are the
same stub surface, then classifies every match into one of:

- **test double acceptable** — intentional mocking, generated unimplemented
  gRPC defaults, injectable test seams, or the standard gRPC term "stub".
- **fallback dev-only acceptable** — explicit no-DB / no-external-binary / local
  smoke-test behavior that should not be selected for production traffic.
- **stub productivo pendiente** — a production route, backend, or catalog entry
  that returns 501 / `ErrNotImplemented` / placeholder data when selected.
- **comentario obsoleto** — comment no longer matched the code and was updated
  in this audit.

## Executive summary

| Priority | Area | Classification | Suggested owner | Action |
| --- | --- | --- | --- | --- |
| P0 | `pipeline-build-service` handlers + runtime dispatch | stub productivo pendiente | Pipeline Builder / data platform | Continue the planned large-service slice: resolver, DAG executor, log streaming, Spark / Iceberg orchestration. |
| P0 | `notebook-runtime-service` execute + notepad CRUD | stub productivo pendiente | Notebook Runtime + Python sidecar | Wire runtime-backed cell execution and a notepad repository slice. |
| P0 | `ontology-query-service` read API | stub productivo pendiente | Ontology Query / ontology-kernel | Replace skeleton 501 handlers with query-backed reads. |
| P0 | `ontology-kernel` functions/object-sets + PostgreSQL stores | stub productivo pendiente | Ontology Kernel | Finish Phase 5/6 object-set evaluation, materialization, and storage adapters. |
| P0 | `agent-runtime-service` chat/copilot | stub productivo pendiente | AI Kernel / Agent Runtime | Wire OpenAI-compatible chat completions and copilot execution into `libs/ai-kernel-go`. |
| P1 | `ai-sink` + `audit-sink` Iceberg writers | stub productivo pendiente | Observability / AI events / Audit platform | Replace failing Iceberg writer stubs when the Go Iceberg write path is selected; JSONL remains the safe dev mode. |
| P1 | `media-transform-runtime-service` NotImplemented catalog entries | stub productivo pendiente | Media Transform + AI Kernel + Geospatial | Wire external binary workers and AI/geospatial transforms. Native image transforms are already implemented. |
| P1 | `ontology-indexer` Kafka runtime | stub productivo pendiente | Ontology Indexer | Replace foundation runtime with Kafka consumer + indexer loop. |
| P1 | `workflow-automation-service` retention sweep step | stub productivo pendiente | Workflow Automation + Audit Compliance | Replace echo step with audit-compliance-service call. |
| P2 | storage/vector/query/geospatial doc-only or optional unimplemented backends | fallback/dev acceptable or documented pending surface | Shared libraries | Keep as explicit optional-backend errors unless a production backend selects them. |

## Cross-check with inventories

- `INVENTORY-PHASE6.md` still correctly predicts the highest-risk remaining
  families: pyo3/sidecar services (`notebook-runtime-service`,
  `pipeline-build-service`, `ontology-actions-service`), AI/ML-kernel-bound
  shells, and large architecture-slice services.
- The service inventories for `tenancy-organizations-service`,
  `authorization-policy-service`, and `identity-federation-service` do not
  reveal productive stubs in this scan. Their matches are injectable test seams
  (`tests can stub...`, `httpClient ... stubbed transport`) rather than product
  gaps.
- One clearly obsolete comment was found and updated: the
  `media-transform-runtime-service` command comment still said Go-native image
  handlers were NotImplemented. The catalog now marks thumbnail / resize /
  rotate / crop / grayscale transforms as Native, so the comment now reflects
  the current runtime behavior.

## Detailed classification

### Test double acceptable

| Matches | Classification | Rationale / owner |
| --- | --- | --- |
| `libs/testing/{mocks.go,doc.go}` | test double acceptable | Dedicated integration-test mock helpers. Owner: platform test tooling. |
| `libs/proto-gen/media_set/media_set_service_grpc.pb.go` `UnimplementedMediaSetServiceServer` methods | test double acceptable | Generated-style gRPC default server methods used for embedding and tests; not product handlers. Owner: proto generation. |
| `ontology-actions-service/internal/mediafunctions` `MockRuntime` comments | test double acceptable | Public mock runtime for cross-package tests. Owner: ontology actions. |
| `media-sets-service/internal/repo/media_items.go` | test double acceptable | Method seam exists so tests can stub a repository behavior. Owner: media sets. |
| `ontology-kernel/stores/mock.go` | test double acceptable | Intentional mock store; not mounted as a production PostgreSQL adapter. Owner: ontology kernel. |
| `audit-compliance-service/internal/lineagedeletion/deletion.go` | test double acceptable | Injectable HTTP client seam for unit tests. Owner: audit compliance. |
| `identity-federation-service/internal/saml/metadata.go` | test double acceptable | Injectable HTTP transport for SAML metadata tests. Owner: identity federation. |
| `tenancy-organizations-service/internal/workspace/repo.go` | test double acceptable | Package-level helper is intentionally overridable by tests. Owner: tenancy organizations. |
| `libs/python-sidecar/manager.go` | test double acceptable | Uses the standard gRPC term "stub" for the generated client, not a placeholder implementation. Owner: python sidecar. |

### Fallback dev-only acceptable

| Matches | Classification | Rationale / owner |
| --- | --- | --- |
| `ontology-actions-service/cmd` and `internal/server` substrate fallbacks | fallback dev-only acceptable | Explicit no-`DATABASE_URL` smoke-test behavior; production should configure the DB-backed kernel handlers. Owner: ontology actions. |
| `sql-bi-gateway-service/internal/{server,handler}` seed-only saved-query stubs | fallback dev-only acceptable | Only used when the repository is nil; DB-backed mode should be used for production. Owner: SQL BI gateway. |
| `pipeline-runner/internal/runner/run.go` `OF_PIPELINE_RUNNER_SPARK_MODE=stub` | fallback dev-only acceptable | Hermetic local test mode; production defaults to `spark-submit`. Owner: pipeline runner. |
| `storage-abstraction/search.go`, `storage-abstraction/repositories.go`, `vector-store/backend.go` | fallback dev-only acceptable | Optional backend methods fail explicitly with NotImplemented/unsupported instead of silently degrading. Owner: shared storage/vector libraries. |
| `query-engine/{udf,optimizer_rules,datasource}.go`, `geospatial-core/doc.go` | fallback dev-only acceptable | Doc-only layout placeholders; no production endpoint is mounted from these files. Owner: shared query/geospatial libraries. |

### Stub productivo pendiente

| Matches | Priority | Suggested owner | Actionable next step |
| --- | --- | --- | --- |
| `notebook-runtime-service/cmd` and `internal/handler` execute + notepad CRUD stubs | P0 | Notebook Runtime + Python sidecar | Keep export/workspace CRUD as-is; separately wire `ExecuteCell` / `ExecuteAllCells` through the Python runtime sidecar and add a notepad repository slice. |
| `agent-runtime-service/cmd` and `internal/handlers` chat/copilot stubs | P0 | AI Kernel / Agent Runtime | Connect `CreateChatCompletion` and `AskCopilot` to `libs/ai-kernel-go` agent/LLM execution rather than returning placeholder content. |
| `ontology-query-service/internal/handlers` skeleton 501 read endpoints | P0 | Ontology Query / ontology-kernel | Implement read endpoints against the ontology stores or route through the completed ontology-kernel query layer. |
| `pipeline-build-service/cmd`, `internal/domain/engine`, and `internal/handler` stubs | P0 | Pipeline Builder / data platform | Continue the large-service migration: build resolution, DAG execution, logs SSE, SparkApplication submission, Iceberg output client. |
| `ontology-kernel/handlers/functions/functions.go` missing execution path | P0 | Ontology Kernel / Actions | Replace function execution 501s once Phase 5 Python/runtime integration is complete. |
| `ontology-kernel/handlers/objectsets/objectsets.go` evaluation/materialization 501s | P0 | Ontology Kernel | Port object-set evaluation/materialization semantics and storage. |
| `ontology-kernel/stores/pg.go` `Status: stub` stores | P0 | Ontology Kernel / storage-abstraction | Implement PostgreSQL object/link/action-log adapters or remove them from production wiring. |
| `libs/ai-kernel-go/handlers/chat.go` 501-preserving stubs | P0 | AI Kernel | Finish chat handler execution path while preserving current validation contracts. |
| `libs/ml-kernel-go/handlers/{training,models,experiments}.go` 501 notes | P1 | ML Kernel | Replace create-training-job/model-version/run/compare endpoints after runner and lineage slices are ready. |
| `ontology-indexer/internal/runtime` foundation runtime stub | P1 | Ontology Indexer | Wire Kafka topic consumption and indexing loop. |
| `workflow-automation-service/internal/automationoperations/steps/retention_sweep.go` TODO echo step | P1 | Workflow Automation + Audit Compliance | Replace with an HTTP call to audit-compliance-service. |
| `ai-sink/internal/writer` Iceberg writer stub | P1 | AI events platform | Keep JSONL for dev; implement Iceberg writer before selecting Iceberg mode in production. |
| `audit-sink/internal/writer` Iceberg writer stub | P1 | Audit platform | Same as AI sink; fail-loud behavior is safer than silently dropping events. |
| `media-transform-runtime-service/internal/catalog` NotImplemented entries | P1 | Media Transform + AI Kernel + Geospatial | Wire geospatial tile pyramids, AI embeddings/transcription/layout/VLM extraction, and spreadsheet rendering. |
| `ontology-actions-service/internal/mediafunctions` `KindNotImplemented` mappings | P1 | Ontology Actions + Media Transform | This is the caller-side error classification for media transforms not available from the runtime; resolve by implementing the matching media-transform entries. |
| `media-sets-service/internal/{accesspatterns,models,transformclient}` NotImplemented passthrough | P1 | Media Sets + Media Transform | Acceptable as a transparent worker status today; clear it by implementing the media-transform runtime entries above. |
| `lineage-service/internal/lineage/executor.go` Rust-source-compatible stub | P2 | Lineage | Decide whether to keep a compatibility no-op or replace with actual query execution in the next lineage slice. |

### Comentario obsoleto actualizado

| File | Before | After |
| --- | --- | --- |
| `services/media-transform-runtime-service/cmd/media-transform-runtime-service/main.go` | Said image entries marked Native in Rust were NotImplemented in Go until a future image-handler slice. | Now states Native image handlers are executed in Go and only external/AI/geospatial/spreadsheet/cost-metering work remains. |

## Recommended sequencing

1. **P0 compile-facing/product-facing stubs first**: `pipeline-build-service`,
   `ontology-query-service`, `notebook-runtime-service` execution, and
   `agent-runtime-service` chat/copilot.
2. **P0 shared-kernel blockers next**: `ontology-kernel` object sets/functions
   and PostgreSQL stores. These unblock several services without duplicating
   service logic.
3. **P1 operational gaps**: Iceberg sink writers, media-transform external/AI
   catalog entries, ontology-indexer Kafka runtime, workflow retention sweep.
4. **P2 doc-only/optional backends**: keep as explicit NotImplemented errors
   unless a production service begins selecting them.
