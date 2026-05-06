# Migration plan: Rust → Go (openfoundry-go)

This document is the operating model for the multi-quarter Rust-to-Go
re-implementation of OpenFoundry. The Go workspace lives at the
sibling directory [`openfoundry-go/`](../../openfoundry-go/). The Rust
workspace at the repo root remains the production source of truth
until each service flips over.

> Goal: **functional 1:1 paridad** (same proto, OpenAPI, SQL schema,
> Kafka topics, JWT shape, /healthz payload). This is *not* a literal
> 1:1 port — see "What does NOT migrate literally" below.

---

## Phases

| Phase | Scope | Status |
|-------|-------|--------|
| 0 | Foundations: scaffolding, libs/core-models, observability, auth-middleware, service template, CI | ✅ done |
| 1 | Core libs (db-pool, event-bus-control, event-bus-data, audit-trail, idempotency, outbox, testing) | ✅ done |
| 1.5 | Tier-2 libs (cassandra-kernel, authz-cedar, saga, state-machine, scheduling, search/storage/vector abstractions, geospatial, media-scanner, ontology-kernel, pipeline-expression, plugin-sdk, analytical-logic, ai-kernel, ml-kernel) | 🟡 in progress — `libs/authz-cedar-go` ✅ feature-complete; `libs/ai-kernel-go` ✅ models (7/7 incl. conversation) + domain/{llm cache+gateway+guardrails+provider, agents memory+planner, rag embedder+chunker+retriever+indexer}; `libs/ml-kernel-go` ✅ full models (10 files) + domain/drift. Pending in ai-kernel-go: domain/llm/runtime (581 LOC), domain/agents/executor (1307 LOC), handlers. Pending in ml-kernel-go: handlers. Other tier-2 libs migrate alongside first consuming service. |
| 2 | Stateless edge services: edge-gateway, notification-alerting, sdk-generation, telemetry-governance, audit-sink, ai-sink | ✅ done — all 6 services migrated. telemetry-governance streaming-monitor surface (Bloque P4) ✅ ported (typed enums, monitoring views/rules/evaluations CRUD, RBAC). Open follow-up: Iceberg writer for sinks (pending iceberg-go writes). |
| 3 | Identity & authz: identity-federation, authorization-policy, tenancy-organizations, audit-compliance | 🟡 in progress — `identity-federation-service` slices 1–4 ✅, 5a ✅, 6 ✅, 7a ✅ (restricted views CBAC CRUD); 5b + 7b (control panel + ABAC) + 8 pending. `tenancy-organizations-service` ✅ active surface complete (slice 1 organizations+enrollments, slice 2 favorites+recents, slice 3 sharing). spaces / projects / trash / resource_resolve are RETIRED upstream and deferred. **Cedar strategy decided 2026-05-06: Option A (cedar-go)**; `libs/authz-cedar-go` ✅ feature-complete (core + pg + nats + kafka + chi + iceberg/schedule generators), conformance mirror pending. `authorization-policy-service` slice 1 ✅ (cedar policy CRUD with strict schema validation; service is canonical Go impl since Rust binary is `fn main(){}`). audit-compliance pending. |
| 4 | Data & ontology: dataset-versioning, media-sets, iceberg-catalog, ontology-{definition,query,actions,indexer}, connector-management, ingestion-replication, object-database | 🟡 in progress — `dataset-versioning-service` ✅ foundation, `iceberg-catalog-service` ✅ foundation, `connector-management-service` ✅ foundation, `ingestion-replication-service` ✅ foundation, `media-sets-service` ✅ foundation, `ontology-definition-service` ✅ foundation (object_types CRUD), `ontology-query-service` ✅ foundation (501 stubs until Cassandra port lands), `ontology-indexer` ✅ foundation (Kafka→Vespa/OpenSearch worker — stub runtime; consumer + SearchBackend ports follow), `object-database-service` ✅ foundation (full HTTP surface + InMemory ObjectStore/LinkStore mirroring Rust noop fakes; Cassandra wiring follows in libs/cassandra-kernel-go slice). ontology-actions deferred (pyo3). Phase 4 closes pending Cassandra-kernel ports. |
| 5 | pyo3-bound services as Python sidecars: notebook-runtime, pipeline-build, lineage, agent-runtime | pending |
| 6 | ML/AI/apps & retire Rust | 🟡 in progress — all 8 architecture+slice / foundation / substrate service ports done: `code-repository-review-service` ✅, `media-transform-runtime-service` ✅, `entity-resolution-service` ✅, `ontology-exploratory-analysis-service` ✅, `reindex-coordinator-service` ✅, `lineage-service` ✅, `workflow-automation-service` ✅, `federation-product-exchange-service` ✅. **`libs/ai-kernel-go`** ✅ models sub-domain (provider, agent, tool, prompt_template, knowledge_base + AiPlatformOverview — 6 of 7 model files; conversation cross-references domain/llm types and lands with the next slice). Inventory at openfoundry-go/INVENTORY-PHASE6.md. Remaining: libs/ai-kernel-go {handlers, domain/{llm,agents,rag}, conversation models} + 8 ai-kernel-bound shells (each blocked on the relevant ai-kernel sub-domain); pyo3 sidecars STOP-and-ask. |

`sql-bi-gateway-service` and `query-engine` are excepted: they keep
Datafusion/Arrow in Rust because Go has no equivalent push-down query
engine. Treated as permanent exceptions, communicated to Go services
via gRPC.

---

## What does NOT migrate literally

Five places where the Go re-implementation deliberately diverges from
the Rust shape:

1. **`pyo3` (5 services)** → Python sidecars over gRPC on loopback.
   The Go service owns lifecycle (subprocess) and the protocol; the
   Python child reuses existing Rust-side python modules without
   change.
2. **`datafusion` push-down** → kept in Rust (`sql-bi-gateway-service`,
   `query-engine`).
3. **`authz-cedar` Cargo features** → split into separate Go packages
   (`cedar/postgres`, `cedar/nats`, …). No `#[cfg(feature)]` equivalent.
4. **`sqlx::query!` compile-time check** → `sqlc` in CI (same level of
   safety, different moment in the build).
5. **Async traits with generics** → flat interfaces per entity. Do not
   port the `Repository<T: Entity>` hierarchy literally.

---

## Wire-compat invariants

The Go side MUST keep these byte-identical to the Rust side while both
implementations coexist:

- `/healthz` payload shape.
- JWT claims field names + JSON tags.
- Dataset RID format `ri.foundry.main.dataset.<uuid-v7>`.
- Transaction state/type tokens (`open|committed|aborted`,
  `snapshot|append|update|delete`).
- Marking source discriminator (`{"kind":"direct"}` /
  `{"kind":"inherited_from_upstream","upstream_rid":"..."}`).
- Media reference camelCase keys (`mediaSetRid`, `mediaItemRid`,
  `branch`, `schema`).
- Schema field type discriminator (`{"type":"DECIMAL","precision":...
,"scale":...}`).

The test suites under `openfoundry-go/libs/core-models/**/*_test.go`
already lock these.

---

## Cutover protocol (per service)

Each service follows the same pattern:

1. **Ship Go binary alongside Rust** in the same Helm release with the
   same SecretMount, Postgres user and Kafka principal.
2. **Header-based routing** at `edge-gateway-service`:
   `X-Of-Migration: go-canary` → Go pod, default → Rust pod.
3. **Contract diff suite** runs against both implementations in CI;
   mismatches block the cutover.
4. **Traffic ramp**: 1 % → 10 % → 50 % → 100 %, each step ≥ 24 h with
   error-budget gates.
5. **Soak**: 100 % Go for ≥ 14 days before removing the Rust pod.
6. **Decommission**: remove the Rust crate from the workspace, delete
   the Helm subchart entry.

---

## Operating cadence

- One service migration in flight at a time per pair of engineers.
- One PR per service migration, never bundling multiple.
- The `openfoundry-go/` Makefile + `.github/workflows/ci.yml` are the
  authoritative gates. Local `make ci` must pass before review.
- Memory-only state: every change to behavior is gated by a contract
  test, never by hidden state.

---

## How to start

```sh
cd openfoundry-go
make tools          # one-time
make gen            # populate libs/proto-gen
make ci             # green = ready to migrate the next service
```

When you migrate a new service, **copy `services/_template/` and rename
`template` → `<your-service>` everywhere** (filename, package, ldflags
target, Dockerfile ARG). Add the OpenAPI spec under `api/`, the SQL
schema + queries under `internal/repo/`, and a sqlc block in
`sqlc.yaml`. CI handles the rest.
