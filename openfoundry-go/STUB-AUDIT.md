# openfoundry-go stub / TODO audit

Date: 2026-05-07

## Method

Primary search command run from the repository root:

```sh
rg -n "TODO|FIXME|stub|not implemented|NotImplemented|http\.StatusNotImplemented|Status: stub|ErrNotImplemented|NOT_IMPLEMENTED" openfoundry-go --glob '*.go' --glob '!*_test.go'
```

The scan returned **105 production-Go matches** after cleaning obsolete
comments in this pass. The table below classifies every match by file/surface;
adjacent matches in the same implementation are grouped so owners can act on a
single backlog item instead of on individual string hits.

Classification values used here:

- **test fake aceptable** — test seam, mock helper, scanner/tooling literal, or
  standard gRPC term that is not a production placeholder.
- **generated code aceptable** — generated gRPC defaults that intentionally
  return `codes.Unimplemented` until a server embeds/implements them.
- **external unsupported compatible con Rust** — explicit unsupported backend,
  external transform, or worker `NOT_IMPLEMENTED` status that is part of the
  compatibility contract and should pass through clearly.
- **productivo pendiente** — production path, runtime, writer, or catalog entry
  still returning 501 / placeholder data / fail-loud `ErrNotImplemented` when
  selected.
- **comentario obsoleto** — documentation/comment claimed a stub that the code
  no longer has; updated in this pass.

## Executive summary

| Priority | Area | Classification | Suggested owner | Action |
| --- | --- | --- | --- | --- |
| P0 | `notebook-runtime-service` notepad CRUD + unconfigured Python runtime | productivo pendiente | Notebook Runtime + Python sidecar | Keep notebook/cell/session CRUD status as wired; add a notepad repository slice and require/operate the Python sidecar for production Python execution. |
| P0 | `agent-runtime-service` `AskCopilot` + chat-completion placeholder | productivo pendiente | AI Kernel / Agent Runtime | Wire service handlers into `libs/ai-kernel-go` agent/LLM execution; do not leave placeholder answers in production. |
| P0 | `pipeline-build-service` handlers + runtime dispatch | productivo pendiente | Pipeline Builder / data platform | Continue resolver, DAG executor, logs SSE, Spark route parity, and Iceberg output work; route gaps are listed in `docs/migration/route-parity-audit.md`. |
| P0 | `ontology-kernel` action/function execution 501s | productivo pendiente | Ontology Kernel / Actions | Replace 501 branches once Python/runtime integration is complete. |
| P1 | `ontology-indexer` Kafka runtime | productivo pendiente | Ontology Indexer | Replace foundation runtime with Kafka consumer + search indexing loop. |
| P1 | `ai-sink` + `audit-sink` Iceberg writers | productivo pendiente | AI events + Audit platform | Keep JSONL for dev; implement Iceberg writers before selecting Iceberg mode in production. |
| P1 | `media-transform-runtime-service` catalog `NotImplemented` entries | external unsupported compatible con Rust | Media Transform + AI Kernel + Geospatial | Wire external binary/AI/geospatial/spreadsheet transforms; until then pass through `NOT_IMPLEMENTED` explicitly. |
| P1 | `ontology-actions-service` no-DB substrate fallback + media transform passthrough | productivo pendiente / external unsupported compatible con Rust | Ontology Actions | Configure DB-backed kernel handlers for production and clear media runtime gaps through media-transform entries. |
| P2 | doc-only shared-library placeholders and optional unsupported backends | external unsupported compatible con Rust | Shared libraries | Keep explicit unsupported errors unless a production service starts selecting the backend. |

## Detailed classification of current scan

### Test fake aceptable

| Matches | Classification | Rationale / owner |
| --- | --- | --- |
| `tools/route-audit/main.go` scanner literal for `http.StatusNotImplemented` | test fake aceptable | Tooling searches for placeholders; it is not itself a product stub. Owner: migration tooling. |
| `libs/testing/{doc.go,mocks.go}` | test fake aceptable | Dedicated integration-test mock helpers. Owner: platform test tooling. |
| `libs/ontology-kernel/stores/mock.go` | test fake aceptable | Intentional mock store because Go has no mockall codegen equivalent here. Owner: ontology kernel. |
| `services/identity-federation-service/internal/saml/metadata.go` | test fake aceptable | Injectable HTTP transport seam for tests. Owner: identity federation. |
| `services/audit-compliance-service/internal/lineagedeletion/deletion.go` | test fake aceptable | Injectable HTTP client seam. Owner: audit compliance. |
| `services/tenancy-organizations-service/internal/workspace/repo.go` | test fake aceptable | Package-level function seam so tests can stub DB behavior. Owner: tenancy organizations. |
| `services/media-sets-service/internal/repo/media_items.go` | test fake aceptable | Method seam exists so tests can stub repository behavior. Owner: media sets. |
| `libs/python-sidecar/manager.go` | test fake aceptable | Uses the standard gRPC term “stub” for the generated client. Owner: python sidecar. |
| `services/code-repository-review-service/internal/codesecurity/scanner.go` `TODO_SECURITY` | test fake aceptable | Scanner intentionally detects security TODO markers in reviewed repositories; not an implementation TODO. Owner: code review service. |

### Generated code aceptable

| Matches | Classification | Rationale / owner |
| --- | --- | --- |
| `libs/proto-gen/media_set/media_set_service_grpc.pb.go` `UnimplementedMediaSetServiceServer` methods | generated code aceptable | Generated gRPC default methods return `codes.Unimplemented` by design for forward-compatible embedding. Owner: proto generation. |

### External unsupported compatible con Rust

| Matches | Classification | Rationale / owner |
| --- | --- | --- |
| `libs/storage-abstraction/{search.go,repositories.go}` | external unsupported compatible con Rust | Optional backends/methods surface NotImplemented/unsupported explicitly instead of silently degrading. Owner: storage abstraction. |
| `libs/vector-store/backend.go` | external unsupported compatible con Rust | Optional backend methods return a typed unsupported error. Owner: vector store. |
| `libs/query-engine/{datasource.go,optimizer_rules.go,udf.go}` | external unsupported compatible con Rust | Doc-only layout placeholders parallel Rust modules; no production endpoint is mounted from these files. Owner: query engine. |
| `libs/geospatial-core/doc.go` | external unsupported compatible con Rust | Reserved package marker for a future geospatial core. Owner: geospatial. |
| `services/media-transform-runtime-service/{cmd,internal/runtime,internal/catalog,internal/handlers/image_ops.go}` | external unsupported compatible con Rust | The runtime contract includes `NOT_IMPLEMENTED` for catalog entries that need external/AI/geospatial/spreadsheet workers; native image ops are executed. Owner: media transform. |
| `services/media-sets-service/internal/{accesspatterns,models,transformclient}` | external unsupported compatible con Rust | Media Sets transparently propagates worker `NOT_IMPLEMENTED` status/reason; fix is to implement the matching media-transform catalog entries. Owner: media sets + media transform. |
| `services/ontology-actions-service/internal/mediafunctions/media.go` | external unsupported compatible con Rust | Caller-side classification for media transforms not available from media-transform runtime. Owner: ontology actions + media transform. |
| `services/lineage-service/internal/lineage/executor.go` | external unsupported compatible con Rust | Explicit compatibility no-op because the Rust source is also a stub; decide separately whether to add real query execution. Owner: lineage. |
| `services/pipeline-runner/internal/runner/run.go` `OF_PIPELINE_RUNNER_SPARK_MODE=stub` | external unsupported compatible con Rust | Hermetic local/CI mode; production default is `spark-submit`. Owner: pipeline runner. |
| `services/sql-bi-gateway-service/internal/{server,handler}` nil-repository saved-query stubs | external unsupported compatible con Rust | Seed-only fallback used when no repository is provided; production should use DB-backed mode. Owner: SQL BI gateway. |

### Productivo pendiente

| Matches | Priority | Suggested owner | Actionable next step |
| --- | --- | --- | --- |
| `services/notebook-runtime-service/internal/handler/handlers.go` notepad CRUD `notImplemented` + empty envelopes | P0 | Notebook Runtime | Implement document/presence repository-backed CRUD. Export is already wired through `domain/notepad`. |
| `services/notebook-runtime-service/cmd/notebook-runtime-service/main.go` Python sidecar gating | P0 | Notebook Runtime + Python sidecar | Production Python execution requires `PYTHON_SIDECAR_BINARY`; unset config currently returns explicit sidecar-not-configured errors. SQL/R/LLM kernels remain unsupported. |
| `services/agent-runtime-service/{cmd,internal/handlers}` `AskCopilot` and chat-completion placeholder text | P0 | AI Kernel / Agent Runtime | Connect service handlers to the AI kernel/LLM runtime instead of returning placeholder content. |
| `services/pipeline-build-service/{internal/handler,internal/domain/engine}` | P0 | Pipeline Builder / data platform | Replace empty envelopes/501s and runtime dispatch placeholders; route parity report lists missing Rust paths. |
| `libs/ontology-kernel/handlers/actions/execute.go` | P0 | Ontology Kernel / Actions | Replace Phase 5A action execution 501 when runtime integration lands. |
| `libs/ontology-kernel/handlers/functions/functions.go` | P0 | Ontology Kernel / Functions | Replace function execution 501 when Python/runtime integration lands. |
| `services/ontology-actions-service/{cmd,internal/server,internal/handler/envelope.go}` substrate fallback | P0 | Ontology Actions | The no-DB fallback returns empty envelopes/501s; production must configure DB-backed kernel handlers and then retire fallback routes if no longer needed. |
| `services/ontology-indexer/{cmd,internal/runtime}` | P1 | Ontology Indexer | Wire Kafka consumption from `ontology.objects.changed.v1` and index projection. |
| `services/audit-sink/{cmd,internal/config,internal/writer}` Iceberg writer | P1 | Audit platform | Implement Iceberg writer before selecting non-JSONL mode in production; current writer fails loudly. |
| `services/ai-sink/{internal/config,internal/writer}` Iceberg writer | P1 | AI events platform | Same as audit sink; JSONL remains safe dev mode. |

### Comentario obsoleto actualizado en este pase

| File | Before | After |
| --- | --- | --- |
| `services/pipeline-build-service/README.md` and `cmd/pipeline-build-service/main.go` | Claimed the URL grid was 1:1 with Rust and all unported handlers were 501. | Now points to the route-parity audit and documents missing paths, 501s, empty envelopes, and config-gated behavior. |
| `services/authorization-policy-service/{README.md,cmd/main.go,internal/models/models.go,internal/server/server.go}` | Described the service as the Cedar-policy foundation slice with RBAC/ABAC/submodules deferred. | Now documents the consolidated authorization surface and records that the production scan has no productive stub matches. |
| `libs/ml-kernel-go/handlers/{experiments.go,models.go,training.go,asset_lineage.go}` | Comments still described run/model-version/training/lineage handlers as 501 stubs. | Comments now match the implemented handlers that call `domain/interop`, `domain/training`, `runs.go`, and `asset_lineage.go`. |
| `services/notebook-runtime-service/{README.md,cmd/main.go}` | Claimed notebook/cell/session CRUD were stubs. | Now distinguishes implemented pgx CRUD + smoke empty-envelope fallback from real pending notepad CRUD and Python sidecar gating. |

## Authorization-policy-service note

`authorization-policy-service` had obsolete comments, not productive stubs.
The current Go router wires Cedar policies, ABAC policies/evaluation,
roles/groups/permissions, governance/project/structural-security resources,
checkpoints/purpose records, cipher catalogs, and network-boundary resources.
The production scan has no `StatusNotImplemented`, `ErrNotImplemented`, or
placeholder handler match in this service.

## Recommended sequencing

1. **P0 product-facing placeholders**: notebook notepad CRUD + Python runtime
   operations, agent-runtime copilot/chat, pipeline-build, ontology-kernel
   actions/functions, and ontology-actions DB-backed kernel wiring.
2. **P1 operational runtimes**: ontology-indexer Kafka loop and Iceberg writers
   for audit/AI sinks.
3. **P1/P2 external unsupported surfaces**: media-transform catalog entries and
   any optional storage/vector backend that becomes selected by production
   configuration.
