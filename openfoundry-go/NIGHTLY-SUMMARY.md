# Nightly summary — Rust → Go autonomous run

**Date:** 2026-05-06
**Stop reason:** Hard architectural decision required — Cedar policy
engine strategy for `authorization-policy-service` and the cedar_authz
piece of `identity-federation-service` slice 8. See
[INVENTORY-authorization-policy-service.md](INVENTORY-authorization-policy-service.md).

**Resolution (2026-05-06):** User picked **Option A — adopt cedar-go**.
Loop is unblocked. Next iteration starts with `libs/authz-cedar-go` as
the de-risking step before porting the service itself.

## What landed

15 commits across the autonomous run, all on
`frontend/settings-mfa-apikeys-sso`, **never pushed to remote**.

| Iter | Commit    | Service / slice                                                  |
|------|-----------|------------------------------------------------------------------|
| 1    | d7daad3c  | Phase 2 — notification-alerting-service                           |
| 2    | 6165cbe8  | Phase 2 — sdk-generation-service                                  |
| 3    | 4a0e3087  | Phase 2 — telemetry-governance-service (CRUD baseline)            |
| 4    | c92e8866  | Phase 3 prep — identity-federation-service inventory              |
| 5    | 9a333f80  | Phase 3 / identity-federation slice 1 — register/login/token      |
| 6    | b29cd226  | Phase 3 / identity-federation slice 2 — cassandra-kernel scaffold |
| 7    | 0e141b83  | Phase 3 / identity-federation slice 3 — MFA TOTP                  |
| 8    | 8cebd686  | Phase 3 / identity-federation slice 4 — WebAuthn                  |
| 9    | ecbd5c65  | Phase 3 / identity-federation slice 5a — OIDC SSO                 |
| 10   | 5ab352a3  | Phase 3 / identity-federation slice 6 — RBAC CRUD                 |
| 11   | 073ae61c  | Phase 3 / identity-federation slice 7a — restricted views CBAC   |
| 12   | 3e22f6b3  | Phase 3 / tenancy-organizations slice 1 — organizations + enrollments |
| 13   | 13eba464  | Phase 2 follow-up — telemetry-governance streaming-monitors      |
| 14   | 81a1b7b0  | Phase 3 / tenancy-organizations slice 2 — workspace + favorites + recents |
| 15   | 1b259f38  | Phase 3 / tenancy-organizations slice 3 — sharing                 |

**Total LOC delta inside `openfoundry-go/`:** +12 599 / −28 across 118
files.

## Phase status

| Phase | Status |
|-------|--------|
| 0 — Foundations (scaffolding, libs/core-models, observability, auth-middleware, service template, CI) | ✅ done |
| 1 — Core libs (db-pool, event-bus-control, event-bus-data, audit-trail, idempotency, outbox, testing) | ✅ done |
| 1.5 — Tier-2 libs | partial — cassandra-kernel scaffold landed alongside identity-federation slice 2 |
| 2 — Stateless edge services | ✅ all 6 services migrated; streaming-monitor follow-up closed |
| 3 — Identity & authz | 🟡 in progress — see breakdown below |
| 4 — Data & ontology | pending |
| 5 — pyo3 sidecars | pending |
| 6 — ML/AI/apps & retire Rust | pending |

### Phase 3 breakdown

- **identity-federation-service** — slices 1, 2, 3, 4, 5a, 6, 7a ✅
  - 5b (SAML 2.0 + XML signing) — pending follow-up
  - 7b (control panel + ABAC + scoped sessions admin) — pending follow-up
  - 8 (Cedar + JWKS rotation + Vault + SCIM) — **STOP-and-ask** on Cedar
- **tenancy-organizations-service** — slices 1, 2, 3 ✅; full active
  surface complete. Spaces / projects / trash / resource_resolve are
  RETIRED upstream (verified via Rust `src/main.rs`) and deferred unless
  upstream re-introduces them.
- **authorization-policy-service** — INVENTORY written; **STOP-and-ask**.
  Rust binary is currently `fn main() {}` with all 5 203 LOC of handlers
  as dead-code library namespaces. See INVENTORY for Cedar A/B/C
  options.
- **audit-compliance-service** — pending; clean port (no flagged risks).

## Tests added

Every committed slice ships unit tests pinning the wire format. Notable:

- `libs/auth-middleware`: JWT + tenant context middleware tests.
- `libs/cassandra-kernel`: gocql cluster builder + migration ledger tests.
- `services/identity-federation-service/internal/{oidc,webauthn,rbac,...}`:
  per-slice tests covering register/login flows, MFA TOTP RFC 6238
  conformance, WebAuthn attestation/assertion, OIDC PKCE + nonce, RBAC
  permission wildcards, restricted-view CBAC.
- `services/telemetry-governance-service/internal/streamingmonitors`:
  enum SCREAMING_SNAKE_CASE pinning, comparator FP-tolerant EQ semantics,
  `{"data": [...]}` envelope.
- `services/tenancy-organizations-service/internal/{handlers,workspace}`:
  Organization/Enrollment JSON shape, `{"items": [...]}` envelope,
  ResourceKind legacy aliases (project → ontology_project), workspace
  `{"data": [...]}` envelope (different from org/enrollment), AccessLevel
  enum, share principal split (exactly one of user/group), share
  validation paths.

All tests run with `go test -race -count=1 ./...` after every iteration.

## Decisions deferred for human review

### 1. Cedar strategy — RESOLVED 2026-05-06: Option A (cedar-go)

User signed off on **Option A**: adopt `github.com/cedar-policy/cedar-go`.
The dead-code Rust binary (`fn main() {}`) collapsed the argument for
Option B (sidecar) — there's no live production system to preserve
byte-identical evaluation with, so conformance becomes "Cedar spec ↔ Go
impl" (AWS's problem). Pre-1.0 risk managed by pinning a cedar-go tag
and mirroring AWS's conformance test suite in CI.

**De-risking step:** port `libs/authz-cedar` → `libs/authz-cedar-go`
(~1 671 LOC) before the service itself. Validates cedar-go in a bounded
scope and ships a reusable lib. Only after the lib passes its
conformance suite do we start porting handlers/domain in slices.

### 2. Workspace inventory finding — retired upstream surfaces

Audit of `services/tenancy-organizations-service/src/main.rs` confirmed
that spaces / projects / trash / tenant_resolution / resource_resolve /
resource_ops are all unmounted upstream ("Cross-bounded-context project
/ space / trash / resource-operation handlers are intentionally not
wired here anymore"). Scope was re-scoped: ~2 150 LOC of Rust handlers
won't be ported unless upstream re-introduces them. Worth confirming
this matches the project's strategic intent.

### 3. Iceberg writer for audit-sink + ai-sink (Phase 2 follow-up, non-blocking)

Both sinks ship `JSONLWriter` (production-suitable) and an
`IcebergWriter` stub that fails loudly. iceberg-go's write API was
unstable when ports landed; revisit when iceberg-go ≥1.0 publishes
stable writes.

### 4. SAML 2.0 (identity-federation slice 5b, non-blocking)

XML signing infrastructure (crewjam/saml + russellhaering/goxmldsig in
Go) ports cleanly but needs IdP test certs + metadata fixtures to
validate end-to-end. Pending until dev infra ships SAML test rig.

### 5. Sessions Cassandra wiring (identity-federation slice 2b, non-blocking)

`libs/cassandra-kernel` and the `sessionscassandra` adapter are
scaffolded. The active backend remains Postgres; flipping the switch is
a one-line config change, gated on Scylla being in dev infrastructure.

### 6. Ontology actions (Phase 4, non-blocking)

`ontology-actions-service` uses pyo3 → Python sidecar pattern. Plan to
treat this as a polyglot service per the migration doc Phase 5 strategy.
Flag for explicit go/no-go before starting that port.

## Build warnings worth flagging

None. `go build ./... && go vet ./... && go test -race ./...` clean
across the workspace at HEAD (1b259f38).

## Resume protocol

When the human signs off on Cedar strategy:

1. Update todos: pick a Cedar option (A/B/C) and unblock either
   authorization-policy-service migration or the cedar_authz slice 8.
2. If Option A (cedar-go): port `libs/authz-cedar-go` first, mirror
   AWS's cedar conformance tests, only then port handlers/domain in
   slices.
3. If Option B (sidecar): write the 50-LOC tonic Rust sidecar + Go gRPC
   client; flip authorization-policy-service to call out.
4. If Option C (wait): mark the todo deferred to Phase 6 and continue
   with audit-compliance-service (the next non-Cedar Phase 3 service).

Other unblocked work that doesn't need Cedar:

- `audit-compliance-service` migration.
- Phase 4 services (data + ontology).
- identity-federation slice 5b (SAML XML signing) once dev infra exists.
- identity-federation slice 7b (control panel + ABAC) — note ABAC
  evaluator is the Cedar piece; control_panel pages + scoped sessions
  admin are independent.

---

# Run 2 — 2026-05-06

**Stop reason:** All initial Phase 6 services ported. Remaining todos
are all (a) deferred follow-up slices for already-ported services,
(b) ai-kernel-go / ml-kernel-go domain sub-slices that need a
multi-iteration build-out, or (c) Phase 5 pyo3 sidecars under the
STOP-and-ask guardrail. Per the loop's STOP PROTOCOL the loop ends
here.

## Services migrated this run

Phase 6 (this run): **16 commits** plus the libs/ai-kernel-go
skeleton. All on `frontend/settings-mfa-apikeys-sso`, never pushed.

| Iter | Commit    | Service / lib                                                |
|------|-----------|--------------------------------------------------------------|
| 1    | de1b6aa0  | object-database-service — full HTTP + InMemory stores         |
| 2    | 3a43356c  | code-repository-review-service — global-branching + subscriber |
| 3    | 58ee8e15  | media-transform-runtime-service — runtime + 33-entry catalog  |
| 4    | 5003dbc7  | entity-resolution-service — architecture + rules slice        |
| 5    | 36b14afb  | ontology-exploratory-analysis-service — substrate + scaffolding |
| 6    | 7d82a40d  | reindex-coordinator-service — arch + pure-logic slice         |
| 7    | 0d764ab5  | lineage-service — HTTP-health + Iceberg schema constants      |
| 8    | 9b6046fb  | workflow-automation-service — arch + 13 topic constants       |
| 9    | 871e1ad7  | federation-product-exchange-service — substrate + 8 migrations |
| 10   | 2ad92ba8  | libs/ai-kernel-go — skeleton + 5 models + AiPlatformOverview  |
| 11   | 1afeed63  | llm-catalog-service — substrate + ai-kernel-go round-trip test |
| 12   | cd396f1a  | solution-design-service — full foundation (CRUD)              |
| 13   | 8dbc107e  | application-composition-service — full foundation (CRUD)      |
| 14   | f8a474fb  | agent-runtime-service — full foundation + ai-events constants |
| 15   | 50ddba43  | ai-evaluation-service — substrate (true ai-kernel-bound)      |
| 16   | 99e0a8a7  | model-catalog-service — adapter + lifecycle CRUD              |
| 17   | 13b00356  | retrieval-context-service — substrate (true ai-kernel shell)  |

## Wire-compat invariants pinned this run

- object-database: BackendMode token (`in_memory` | `cassandra`),
  /health plain "ok", full Object/Link JSON shape.
- code-repo-review: PromoteTopic (`foundry.global.branch.promote.requested.v1`),
  PromoteEventType, FindingsTopic (`code.security.findings`), Subscriber
  Topic (`foundry.branch.events.v1`), GlobalRIDPrefix
  (`ri.foundry.main.globalbranch.`), event-type → status map verbatim.
- media-transform: TransformStatus SCREAMING_SNAKE_CASE, HandlerStatus
  tag="kind" snake_case, MEDIA_TRANSFORM_* error codes, full 33-entry
  catalog in Rust source order with all external-binary annotations.
- entity-resolution: BlockingStrategy defaults (key-based, [email/phone/
  display_name], 5, 24), thresholds 0.76/0.9, default_strategy=
  longest_non_empty, ListResponse.data envelope.
- reindex-coordinator: ReindexNamespace bytes pinned verbatim
  (6f-82-4d-6e-...-88-10), 3 topic constants, JobStatus tokens, page-size
  [1, 10000] default 1000, partition_key="tenant/id".
- lineage: SourceTopic="lineage.events.v1", ConsumerGroup="lineage-service",
  Iceberg catalog="lakekeeper", namespace="of_lineage", tables (runs/events/
  datasets_io), partition transform "day(event_time)", every Iceberg field
  ID pinned (1..7 / 1..7 / 1..6).
- workflow-automation: WorkflowAutomationNamespace bytes pinned
  (4e-21-9b-1a-...-d1-40), 13 topic constants spanning automate/saga/
  approval planes, SagaConsumerGroup="automation-operations-service",
  ProcessedEventsTable, DeriveRunID + DeriveConditionEventID + TenantUUIDFromStr.
- federation: ListResponse `{"items":[…]}` envelope, SyncStatus +
  NexusOverview JSON shape.
- ai-kernel-go models: provider/agent/tool/prompt/kb defaults pinned to
  Rust source values (provider_type=openai, model_name=gpt-4.1-mini,
  endpoint_url=https://api.openai.com/v1, weight=100,
  max_context_tokens=32000, network_scope=public, supported_modalities=
  [text], agent planning_strategy=plan-act-observe, tool category=
  analysis/execution_mode=simulated, prompt category=copilot, kb
  embedding_provider=deterministic-hash, chunking_strategy=balanced,
  search top_k=5/min_score=0.55).
- agent-runtime: ai.events.v1 topic, "agent-runtime-" txn prefix,
  AiEventKind enum (lowercase JSON: prompt/response/evaluation/trace),
  TargetTable routing (prompts/responses/evaluations/traces), Producer
  name canonicalised.

## Tests added

Every port has wire-compat tests pinning the invariants above.
Highlights:
- 13 distinct topic constants with explicit verbatim assertions.
- 2 UUID-v5 namespace byte arrays pinned (reindex + workflow-automation).
- 33-entry media-transform catalog test enforcing Rust order +
  external-binary annotations.
- Lineage Iceberg field IDs locked (table-by-table, field-by-field).
- llm-catalog round-trip test against libs/ai-kernel-go/models.LlmProvider —
  proves the kernel skeleton integrates end-to-end before the handlers
  slice lands.
- Object-database HTTP smoke (PUT → GET → version_conflict → DELETE → 404)
  drove a real binary boot on PORT=51999.
- Media-transform binary smoke verified `/healthz`, `/status` (`backend:
  "in_memory"`), `/health` "ok" — binary boots on PORT=51998.

## Inventory corrections

The Phase 6 inventory misclassified several services as
"ai-kernel-bound" when they actually have own handlers + models:
- **solution-design-service**: own handlers (CRUD on solution_diagrams +
  solution_references). Ported as full foundation.
- **application-composition-service**: own handlers (CRUD on
  composition_views + composition_bindings). Ported as full foundation.
- **agent-runtime-service**: own handlers (agents CRUD + runs + steps +
  human-approval + chat-completion stub + copilot stub). Tools surface
  IS ai-kernel-bound — deferred. Ported as full foundation.
- **model-catalog-service**: SPLIT — adapter + lifecycle handlers are
  local (ported); models + experiments are ml-kernel-bound (deferred).
- **model-deployment-service**: classified as ai-kernel-bound; in fact
  it's **ml-kernel-bound** (different kernel — needs libs/ml-kernel-go).

True ai-kernel-bound shells (substrate-only ports, awaiting kernel):
- **llm-catalog-service** (`#[path]` re-exports of handlers/models/domain).
- **ai-evaluation-service** (domain/llm/{gateway, guardrails, runtime}
  re-exports).
- **retrieval-context-service** (handlers/models/domain all re-exports).

## Decisions deferred for human review

1. **ai-kernel-bound shells need libs/ai-kernel-go to ship handlers.**
   The kernel's models sub-domain is in (commit 2ad92ba8); domain/{llm,
   agents, rag} + handlers each need their own iteration. Conversation
   models specifically were deferred because they cross-reference
   GuardrailVerdict / SemanticCacheMetadata / LlmUsageSummary /
   ChatRoutingMetadata / KnowledgeSearchResult — those land with the
   relevant domain sub-slice.
2. **libs/ml-kernel-go is unstarted.** Required for `model-deployment-service`
   foundation and the ml-kernel-bound surfaces of `model-catalog-service`.
3. **Pre-ADR-0030 retired services pinned in code paths.** Two cases hit
   this iteration:
   - `application-composition-service` widgets endpoint `#[path]`s into
     `app-builder-service/src/models/widget_type.rs` which has been
     retired upstream (commit 7fc037c4). Catalog data must be re-ported
     as its own slice.
   - `agent-runtime-service` tools handler `#[path]`s into
     libs/ai-kernel/src/handlers/tools.rs (still alive) — wires with the
     libs/ai-kernel-go/handlers slice.
4. **Cassandra-backed services are still on InMemory fakes.** object-database
   (committed) and ontology-query (Phase 4) both ship with pgx + InMemory
   fallbacks; the Cassandra wiring lands when libs/cassandra-kernel-go
   gets a real gocql implementation.
5. **Kafka producers are stubbed for the ai/lineage/audit feeds.**
   Topics, transactional-id prefixes, envelope shapes, and consumer
   groups are all pinned in Go constants/tests, but the actual
   `kafka-go` producer wiring lands with libs/event-bus-data-go +
   the per-service runtime slice.
6. **ontology-actions-service (pyo3) and Phase 5 pyo3 sidecars stay
   STOP-and-ask** per the existing guardrail.

## Build warnings worth flagging

None. `go build ./... && go vet ./... && go test -race -count=1 ./...`
runs clean across the whole workspace at the end of every commit.

## Still pending (deferred follow-up slices)

Per-service runtime slices queued in TodoList:
- media-transform-runtime image handlers (golang.org/x/image port).
- entity-resolution: jobs + clusters + golden-records + engine domain.
- ontology-exploratory-analysis: views/maps/writeback/scenarios/timeseries/
  geospatial slices (all 4 absorbed sub-domains pending consolidation).
- reindex-coordinator: JobRepo + idempotency + Kafka consumer + Cassandra
  scanner + publisher.
- lineage: Kafka subscriber + Iceberg writer + lineage graph domain +
  query router + handlers.
- workflow-automation: handlers + condition consumer + saga consumer +
  approvals state machine + timeout sweep + NATS subscriber.
- federation-product-exchange: marketplace + marketplace-catalog +
  product-distribution sub-domain handlers.
- application-composition: widget-catalog data re-port.
- agent-runtime: tools handlers + Kafka producer.
- ai-evaluation: handlers + domain/llm.
- model-catalog: ml-kernel models + experiments handlers.

Library slices queued:
- libs/ai-kernel-go: domain/llm + conversation models, domain/agents,
  domain/rag, handlers.
- libs/ml-kernel-go: skeleton + models sub-domain.

Service ports queued (after libs):
- model-deployment-service (after libs/ml-kernel-go).

Phase 3 follow-ups deferred by user choice:
- identity-federation slices 2b (Cassandra sessions), 5b (SAML),
  7b (control panel + ABAC), 8 (Cedar + JWKS rotation + Vault + SCIM).
- tenancy-organizations RETIRED follow-ups.
- libs/authz-cedar-go AWS Cedar conformance suite mirror.

Phase 4: ontology-actions-service (pyo3 STOP-and-ask).
Phase 5: pyo3 sidecars (notebook-runtime, pipeline-build,
ontology-actions) — STOP-and-ask, gRPC sidecar pattern.
