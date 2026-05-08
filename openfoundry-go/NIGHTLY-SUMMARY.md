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

- **identity-federation-service** — active runtime wires auth, MFA, WebAuthn, OIDC, RBAC, restricted views ✅
  - 2b (Cassandra sessions) — adapter exists; active backend remains Postgres
  - 5b (SAML 2.0 + XML signing) — helper package/tests exist; runtime registry wiring pending
  - 7b (control panel + ABAC + scoped sessions admin) — pending follow-up
  - 8 (Cedar + JWKS rotation + Vault + SCIM) — Cedar/JWKS/Vault/SCIM packages exist and are tested; route/binary wiring pending
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

Current audit (2026-05-08): `internal/saml` includes AuthnRequest,
metadata parsing, response/signature validation, registry, and test
fixtures. `handlers.SSO` has SAML start/ACS methods and the ACS route is
mounted, but `cmd/.../main.go` does not construct a `saml.Registry`; the
default binary remains OIDC-only. Pending work is runtime provider
config/loading and true end-to-end SAML login wiring.

### 5. Sessions Cassandra wiring (identity-federation slice 2b, non-blocking)

Current audit (2026-05-08): `libs/cassandra-kernel` exists and
`internal/sessionscassandra` has DDL + adapter methods for
`auth_runtime.user_session` and `auth_runtime.refresh_token`. The active
`Issuer` still uses Postgres refresh tokens. Flipping the backend remains
gated on Scylla/CQL dev infrastructure and runtime wiring.

### 6. Ontology actions (Phase 4, non-blocking)

`ontology-actions-service` uses pyo3 → Python sidecar pattern. Plan to
treat this as a polyglot service per the migration doc Phase 5 strategy.
Flag for explicit go/no-go before starting that port.

## Build warnings worth flagging

None. `go build ./... && go vet ./... && go test -race ./...` clean
across the workspace at HEAD (1b259f38).

## Resume protocol

Cedar strategy is already signed off (Option A: cedar-go). Next steps:

1. Continue extending `libs/authz-cedar-go` conformance fixtures before
   cedar-go upgrades.
2. Wire identity `internal/cedarauthz.AdminGuard` onto live JWKS/SCIM
   routes when those routes are mounted.
3. Continue authorization-policy-service migration against the Go Cedar
   wrapper.

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
- identity-federation Cassandra active wiring, SAML runtime registry,
  control panel + ABAC, and JWKS/Vault/SCIM route wiring.
- tenancy-organizations RETIRED follow-ups.
- Continue extending the libs/authz-cedar-go conformance mirror.

Phase 4: ontology-actions-service (pyo3 STOP-and-ask).
Phase 5: pyo3 sidecars (notebook-runtime, pipeline-build,
ontology-actions) — STOP-and-ask, gRPC sidecar pattern.

---

# Run 3 — 2026-05-06 (continuation)

User requested continuation after Run 2's stop point. Worked through
the kernel-library backlog that was blocking the ai-kernel- and
ml-kernel-bound shells.

## What landed

5 new commits, all on `frontend/settings-mfa-apikeys-sso`, never pushed:

| # | Commit    | Library / service                                              |
|---|-----------|----------------------------------------------------------------|
| 1 | `67b25a35`| libs/ml-kernel-go — skeleton + full models sub-domain (10 files) |
| 2 | `70340389`| model-deployment-service — substrate + ml-kernel-go proof-point |
| 3 | `fec8710b`| libs/ai-kernel-go — domain/llm slice + conversation models     |
| 4 | `91a67a07`| libs/ml-kernel-go — domain/drift slice                         |
| 5 | `4f522a62`| libs/ai-kernel-go — domain/agents + domain/rag                 |

## Wire-compat invariants pinned this run

ml-kernel-go models: defaults pinned (batch_schedule, freshness_sla,
problem_type, deployment strategy/window, experiment task_type/
primary_metric, objective status); HasSignal() methods on the 3
descriptors mirror Rust trim().is_empty().

ml-kernel-go drift: metricStatus thresholds; GenerateDriftReport
defaults (baseline=10000, observed=1.12×baseline, volume_shift cap
1.5, dataset_score base 0.12 / threshold 0.25, concept_score base
0.09 / threshold 0.18, recommend_retraining trigger).

ai-kernel-go conversation: DefaultGuardrailVerdict, all attachment/
benchmark/fallback/sql/temperature/max_tokens defaults pinned.

ai-kernel-go domain/llm: NormalizeText collapses whitespace runs;
Fingerprint unit-magnitude 16-dim; CosineSimilarity handles zero
magnitude; EvaluateText flag matrix (email/phone medium/non-blocking,
hate/ignore-instructions blocked, clean=passed); InterpolateTemplate
strict-vs-non-strict elision; RouteProviders filter+sort+preferred
override; ProviderUsesPrivateNetwork case-insensitive scope match.

ai-kernel-go domain/agents: UpdateMemory short_term Vec::truncate(6)
semantics (drops trailing on overflow — verbatim Rust); long_term
dedup+truncate(8); last_run_summary truncate(180); BuildPlan ordering
(analyze→retrieve?→tool*→synthesize) with id case+space rules.

ai-kernel-go domain/rag: EmbedText unit-magnitude 12-dim deterministic;
ChunkText paragraph-then-sentence with buffer flush; Search ranks +
truncates to max(top_k, 1); IndexDocument fine→320 / balanced→520
chunk_chars + chunk.id format.

## Tests added this run

40+ new test cases across 5 commits, all green at every commit.

## Decisions deferred for human review

1. **agents/executor (1307 LOC)**: own iteration. Agent runtime's
   plan-act-observe loop is too big for one commit.
2. **domain/llm/runtime (581 LOC)**: own iteration. Mixes runtime/
   driver concerns; depends on the primitives ported this run.
3. **handlers (ml-kernel 3334 LOC, ai-kernel similar)**: each file
   warrants its own iteration. Recommend smallest first
   (overview 81 → predictions 301 → training 321 → features 378 →
   deployments 405 → models 548 → experiments 1225). Same shape on
   the ai-kernel side (chat/agents/knowledge/prompts/tools).
4. **Service runtime slices** (Kafka subscribers, Iceberg writers,
   Cassandra scanners) for already-ported services — each per service
   is its own iteration.

## Status snapshot after Run 3

- ai-kernel-go: 7/7 models, domain/{llm pure-logic, agents memory+planner,
  rag full}. Pending: llm/runtime, agents/executor, handlers.
- ml-kernel-go: 10/10 models, domain/drift. Pending: domain/{serving,
  training, monitoring, feature_store, predictions, interop}, handlers.
- 1 new service (model-deployment-service) ported as the ml-kernel-go
  consumer proof-point.

The ai-kernel-go handlers slice is the next big unblock for
llm-catalog, ai-evaluation, retrieval-context, and the tools surface
of agent-runtime.

---

# Run 4 — 2026-05-06 (continuation)

User said "puedes continuar por favor ?" — kept going on the
kernel-library handler + domain backlog Run 3 left behind. This run
finished the ai-kernel-go and ml-kernel-go HTTP handler surfaces and
landed every remaining pure-logic domain slice except the four heavy
runtime / executor / interop / training-runner ports.

## What landed

14 commits, all on `frontend/settings-mfa-apikeys-sso`, never pushed:

| # | Commit    | Slice                                                          |
|---|-----------|----------------------------------------------------------------|
| 1 | `945d0e7e`| libs/ai-kernel-go — handlers/prompts (CRUD + render)          |
| 2 | `7f3a1dc6`| libs/ai-kernel-go — handlers/knowledge + domain/llm runtime placeholder |
| 3 | `b9cf66ed`| libs/ai-kernel-go — handlers/agents (CRUD + execute stub)     |
| 4 | `8a6cb8ec`| libs/ai-kernel-go — handlers/chat slice 1 + domain/evaluation |
| 5 | `3cd17370`| libs/ml-kernel-go — domain/predictions (record-prediction)    |
| 6 | `916e6c9d`| libs/ml-kernel-go — domain/training/hyperparameter            |
| 7 | `3a83a0cf`| libs/ai-kernel-go — domain/copilot (deterministic draft)      |
| 8 | `2da114c7`| libs/ml-kernel-go — handlers/overview                         |
| 9 | `b076a516`| libs/ml-kernel-go — handlers/predictions (realtime + batch)   |
| 10| `b50a5835`| libs/ml-kernel-go — handlers/features (feature-store CRUD)    |
| 11| `7592f503`| libs/ml-kernel-go — handlers/deployments + drift report       |
| 12| `21ab73f0`| libs/ml-kernel-go — handlers/training (list + create stub)    |
| 13| `6b1ac389`| libs/ml-kernel-go — handlers/models (model + version slice)   |
| 14| `e8a11216`| libs/ml-kernel-go — handlers/experiments (CRUD + run stubs)   |

## Wire-compat invariants pinned this run

**ai-kernel-go handlers**:
- ErrorResponse {"error": "..."} envelope; tools/prompts/knowledge/
  agents/chat each pin their empty-name / bad-JSON 400 messages.
- jsonOrEmptyObject + jsonOrFallback canonicalise null/empty JSONB
  to "{}" matching the Rust serde defaults.
- knowledge: resolveEmbeddingProvider parses "provider:<uuid>"
  prefixes, returns 404 on unknown IDs, and falls through to nil for
  non-prefix references — handler then picks rag.IndexDocument /
  rag.EmbedText (offline) vs llm.EmbedText (provider runtime).
- chat: ConversationSummary uses last-message-preview from final
  ChatMessage with summarizeTitle's 60-rune limit; "New conversation"
  fallback. Provider create/update flips health_state.status on
  enable/disable transitions.
- chat / training: chat completion + copilot + benchmark + create
  training job stub at 501 with input-validation envelope preserved
  (the runtime port lands those).

**ml-kernel-go handlers**:
- overview pins 10 SQL counters verbatim (incl. drift_report->>
  'recommend_retraining' boolean cast and cache_hit_rate via the
  evaluation helper).
- predictions: realtime path persists every batch into ml_batch_
  predictions with status='realtime', completed_at=created_at,
  output_destination=NULL — write is fire-and-forget (response stays
  200 even if the audit insert fails). runPredictions filter_map
  drops records without matching split or runtime.
- deployments: normaliseTrafficSplit replicates the Rust round-and-
  remainder allocation rebalance (single → 100; ab_test sums to
  exactly 100; last entry absorbs rounding remainder).
  generate_drift_report queues a 'drift_recovery' training job when
  report.recommend_retraining + body.auto_retrain.
- models: production-singleton invariant (existing production
  version flips to staging on transition), refreshModelRollup picks
  current_stage from production > staging > candidate > none.
- features: last_materialized_at = Some(now) only when samples
  non-empty; last_online_sync_at gated by online_enabled AND samples
  non-empty.

**ai-kernel-go domain**:
- evaluation: cache_hit_rate (capped at 100), risk_score (1.0 when
  blocked, len/5.0 capped at 0.95 otherwise), safety_score = 1.0 -
  risk_score clamped, estimated_cost_usd = (pt/1k)·in_rate +
  (ct/1k)·out_rate (zero on cache_hit), quality_score (rubric
  fraction or word_count/120 clamped 0.35–0.9 when no rubric),
  normalized_score (clamped + inverse), overall_benchmark_score
  (0.45·quality + 0.25·safety + 0.15·latency + 0.15·cost).
- copilot: dataset-id present + include_sql → 30-day SELECT against
  dataset_<simple-uuid>; "sql"/"query" keyword → 7-day fallback;
  pipeline_plan toggle emits fixed 3-row plan; ontology_type_id +
  "ontology"/"object" keyword combine into hint list.

**ml-kernel-go domain**:
- predictions: RouteVariant deterministic with bucket=(ord*37)%100;
  PredictRecord has model_state branch (sigmoid + clamp 0.001–0.999)
  + sin-wave fallback (clamp 0.02–0.98); both produce
  "record-N" IDs and truncate explain contributions to top 3.
  scalarScore covers numbers/strings (len/100 cap)/bools (0.65/0.35).
- training/hyperparameter: candidate_sets defaults (lr 0.05/0.08/
  0.12, epochs 250/350/500, l2 0.0/0.001/0.01); ValueAsFloat64 /
  ValueAsUint64 with negative-int fallback semantics.

## Tests added this run

90+ new test cases across 14 commits, all green at every commit.
Notable nailed-down edges:
- normaliseTrafficSplit ab_test rebalance to exactly 100 with three
  30-allocation entries (rounding remainder → last entry).
- runPredictions filter_map shape (records without matching runtime
  drop silently rather than erroring).
- evaluation float32 numerics pinned to Rust assertions (cost
  estimate within 0.0001, quality_score within 0.02 for rubric).
- conversation_summary fallback ("No messages yet" when empty,
  "..." suffix when content > 60 runes).
- summarizeTitle rune semantics (60-rune limit; empty string →
  "New conversation").

## Decisions deferred for human review

1. **libs/ai-kernel-go/domain/llm/runtime (581 LOC)** — full per-
   provider HTTP request/response shapes, retries, credential
   injection. Placeholder shim returns the offline 12-dim embedding
   so the surrounding handlers stay wire-compatible. Lands in its
   own iteration.
2. **libs/ai-kernel-go/domain/agents/executor (1307 LOC)** — agent
   plan-act-observe runtime. Own iteration (was already deferred in
   Run 3). The handlers/agents.ExecuteAgent stub at 501 documents
   the dependency.
3. **libs/ml-kernel-go/domain/interop (769 LOC)** — model-version
   schema normalisation + tracking-source merging + framework /
   adapter inference. Used by training, models, experiments
   handlers. Each handler embeds shallow JSON-extraction shims
   (modelAdapterFromSchema, registrySourceFromSchema,
   trackingSourceFromSchema, trackingSourceFromTrainingConfig) that
   pluck the typed object + filter on HasSignal but skip the
   whitespace + casing normalisation. Full port lands in its own
   slice.
4. **libs/ml-kernel-go/domain/training/runner (418 LOC)** + train
   execution path (training/mod 191) — depends on interop. Lands
   alongside or after the interop port.
5. **handlers/runs slice** — list_runs / create_run / update_run /
   compare_runs (5×stubs in handlers/experiments.go). Independent
   of interop; needs the ml_runs row scaffolding (params/metrics/
   artifacts JSONB, external_tracking via interop, evaluation.
   QualityScore for compare_runs).
6. **handlers/experiments asset-lineage builder (~459 LOC)** —
   builds the 6-tier graph (experiment → runs → training jobs →
   model versions → models → deployments) with every neighbour
   edge. Pure logic but heavy enough for its own iteration.
7. **handlers/chat slice 2** — chat completion + ask copilot +
   benchmark providers (~530 LOC of Rust + the runtime port). Lands
   when the runtime is in.
8. **handlers/models create_model_version** — chains interop heavily
   for normalize_model_version_schema + merge_metrics +
   preferred_artifact_uri. Lands with interop.

## Status snapshot after Run 4

- **libs/ai-kernel-go**: 7/7 models, 5/5 handler files
  (tools/prompts/knowledge/agents/chat) — chat slice 2 stubbed;
  domain/{llm pure-logic + runtime placeholder, agents memory+
  planner, rag full, evaluation, copilot}. Pending: llm/runtime
  full, agents/executor.
- **libs/ml-kernel-go**: 10/10 models, 7/7 handler files
  (overview/predictions/training/features/deployments/models/
  experiments) — runs + asset-lineage + create_model_version +
  create_training_job stubbed at 501 with shape-stable envelopes;
  domain/{drift, predictions, training/hyperparameter}. Pending:
  domain/{interop, training/runner, training/execute_training}.
- **Handlers slice complete enough to wire** every consuming
  service today: llm-catalog, ai-evaluation, retrieval-context,
  agent-runtime (tools / prompts / knowledge / agents-CRUD /
  guardrails), model-deployment, model-catalog, experiment-tracking,
  feature-store. The 501 stubs are intentional and documented.
- All builds + vets + tests green workspace-wide at every commit.

## Build warnings worth flagging

None. `go build ./...` and `go test ./libs/...` both clean.

The next iteration's natural starting point is libs/ml-kernel-go/
domain/interop (769 LOC pure logic; unblocks 4 handler stubs). The
runtime port can run in parallel since it's a different lib.

---

# Run 5 — 2026-05-06 (continuation)

User asked "puedes continuar con la migración? hay algo pendiente
de migrar 1:1 de rust a go?". Confirmed there were 4 big pure-logic
modules still pending and ported all of them.

## What landed

5 new commits, all on `frontend/settings-mfa-apikeys-sso`, never pushed:

| # | Commit    | Slice                                                          |
|---|-----------|----------------------------------------------------------------|
| 1 | `2541be78`| libs/ml-kernel-go — domain/interop full port + wire into handlers/{training,models} |
| 2 | `069c3e9a`| libs/ml-kernel-go — domain/training/{runner,execute} + wire CreateTrainingJob |
| 3 | `fa09208e`| libs/ai-kernel-go — domain/llm/runtime full port (replaces placeholder) |
| 4 | `ed46de5b`| libs/ai-kernel-go — domain/agents/executor + wire ExecuteAgent |
| 5 | (this)    | docs(NIGHTLY-SUMMARY,migration-rust-to-go) — Run 5 close-out  |

## Pure-logic modules ported this run

### libs/ml-kernel-go/domain/interop (769 LOC of Rust)

23 functions — every byte the Rust source uses. Wired into the
matching handlers so the previously-shallow JSON shims could be
deleted:

- **Normalisation**: NormalizeTrackingSource, NormalizeModelAdapterDescriptor,
  NormalizeRegistrySource. Each trims every string field and routes
  system / framework / flavor through their alias tables (wandb,
  mlflow, sagemaker, azureml, vertexai, comet, neptune; scikit-learn,
  pytorch, tensorflow, xgboost, lightgbm, catboost, onnx,
  huggingface; pyfunc, joblib, torchscript, savedmodel, pickle).
- **Pluck-from-JSON**: TrackingSourceFromParams /
  TrackingSourceFromTrainingConfig / TrackingSourceFromSchema /
  ModelAdapterFromSchema / RegistrySourceFromSchema. Filter on
  HasSignal, normalise.
- **Merging**: MergeTrainingConfigWithExternal (default engine /
  framework / artifact_uri when missing, fold inferred adapter +
  registry); MergeRunParams (fold without overwriting + tag
  framework + tracking_system); MergeRunArtifacts (dedupe by URI,
  append synthetic Model URI / Artifact Bundle / Tracking Run
  references); MergeMetrics (dedupe by name, primary precedence).
- **Inference**: EffectiveFramework (candidate ordering with
  tabular-logistic fallback); InferModelAdapter (full kind /
  framework / flavor / runtime / loader / artifact_uri matrix);
  InferRegistrySource (existing-over-tracking merge, nil when no
  signal).
- **Schema folding**: NormalizeModelVersionSchema (fold artifact_uri
  / model_adapter / registry_source / external_tracking, default
  engine + signature); PreferredArtifactURI (model_uri →
  artifact_uri → run_uri → config.artifact_uri).
- **Decision tables**: runtimeForAdapter (in-process for native;
  onnxruntime / torchserve / tensorflow-serving / transformers /
  external-serving / python-remote based on framework × flavor);
  loaderForAdapter (mlflow always wins for mlflow tracking; onnx /
  torch / tensorflow / joblib / xgboost / lightgbm / artifact-
  reference defaults).

Tests pin all 4 Rust assertions plus 13 new edges (priority orders,
flavor defaults per framework, registry source pull-from-tracking +
existing-priority + nil-when-empty, schema signature selection
tabular vs external-model + folding, all alias matrices, normalize
ObjectOrNull paths, dedupeArtifacts first-URI-wins).

### libs/ml-kernel-go/domain/training/{runner, execute} (609 LOC of Rust)

- **runner.go (TrainTrial)**: full logistic-regression training
  trial. parseDataset → standardizeRows → fitLogisticRegression →
  evaluateMetrics. feature_names auto-derive (alphabetically sorted,
  deduped, label_field excluded). scalarFeature: number / bool /
  parsable string / hash-fallback string. binaryLabel: bool / >=0.5
  number / "positive" / case-insensitive "true" / "1". evaluateMetrics
  computes accuracy + precision + recall + f1 + log_loss with
  denominator clamps + 4-decimal rounding. Schema build folds
  interop.{EffectiveFramework, InferModelAdapter} into the
  model_state object.
- **execute.go (ExecuteTraining)**: 3 branches:
  1. external_training detected → synthesise single
     "imported-<run_id>" trial + reproducibility schema metadata.
  2. no inline records → synthetic_trials with deterministic 0.5 +
     0.05·i objectives.
  3. inline records → TrainTrial per candidate set, sort by
     objective desc, pick best.
- **handlers/training.CreateTrainingJob now fully implemented**:
  merges external_training into config via interop, runs
  ExecuteTraining, optionally registers ml_model_versions row via
  the auto_register_model_version path (inserting "autotune-vN"
  label and "ml://models/<id>/versions/<n>" fallback URI),
  refreshes ml_models.{latest_version_number, current_stage='candidate'},
  inserts ml_training_jobs row. The 501 stub is gone.

Tests pin the Rust assertion (logistic trial achieves f1≥0.8 on the
4-record fixture) plus 14 new edges covering all 3 ExecuteTraining
branches, scalarFeature / binaryLabel coercion matrices, 4-decimal
roundMetric, evaluateMetrics empty + perfectly-separable, isJSONObject
detection.

### libs/ai-kernel-go/domain/llm/runtime (581 LOC of Rust)

Replaces the offline placeholder shim with the full HTTP runtime.
CompleteText routes per-provider api_mode to one of three protocols
verbatim:

- **chat_completions**: OpenAI-compatible /chat/completions with
  Bearer auth from CredentialReference env var; multimodal user-
  content (text + image_url + image_base64 → data: URLs); reads
  /choices/0/message/content with /choices/0/text fallback; usage
  parsed from {prompt,completion,total}_tokens and clamped to
  prompt+completion when total missing.
- **messages**: Anthropic /messages with anthropic-version header,
  x-api-key auth, usage from {input,output}_tokens.
- **chat**: Ollama /chat with stream=false, message.content reply,
  usage from prompt_eval_count / eval_count.

EmbedText / EmbedTextWith — chat_completions posts to /embeddings
reading /data/0/embedding; chat (Ollama) posts to /embeddings
reading top-level "embedding". Empty content short-circuits to
empty slice. Nil provider falls back to the offline 12-dim
deterministic embedding (rag.EmbedText algorithm inlined to avoid
the rag → llm cosine cycle).

Helper ports: providerToken (env var resolution + trim + nil/empty
guards); endpointURL (drops trailing/leading slashes; passes through
when suffix already present); buildOpenAIUserContent /
buildAnthropicUserContent / buildOllamaUserPayload (full attachment-
to-payload conversion matrix mirroring Rust including image_url
required-url / image_base64 required-data error messages and the
unsupported-kind rejection); parseEmbedding / valueArrayToFloat32 /
valueAsText / usageTokens / jsonPointer / stringFromPointer.

Tests pin all 3 Rust assertions plus 21 new edges via httptest
servers (chat_completions / messages / chat happy paths; non-2xx
error wrapping; Bearer auth attachment from env-resolved
CredentialReference; multimodal rejection envelopes; embedding
shapes; endpointURL join semantics including suffix-already-present;
jsonPointer for maps + arrays + invalid index; usageTokens across
float64/int64 shapes; empty content / unsupported api_mode envelopes;
providerToken env / nil / empty cases; nil-provider offline fallback).

### libs/ai-kernel-go/domain/agents/executor (1307 LOC of Rust)

Full port — every byte. Each Rust function has a 1:1 Go counterpart
in the executor.go file. Wired into handlers/agents.ExecuteAgent.

ExecutePlan iterates AgentPlanStep entries, dispatches the tool steps
to ExecuteTool, synthesises observations for the built-in
retrieve-context / synthesize-answer steps. Counts successful
invocations into the final synthesise observation.

ExecuteTool routes by execution_mode across all 11 modes:
**simulated** (mock_response passthrough or skipped); **knowledge_search**
(0.6·retrieval_score + 0.4·lexical mix, min_score filter, top_k
truncation); **native_sql** / **native_dataset** /
**native_ontology** / **native_pipeline** / **native_report** /
**native_workflow** / **native_code_repo** (deterministic JSON
observations from inferTimeWindowDays / inferObjectTypes /
inferActionHints / inferDataset{Operation,Governance} /
inferPipelineTrigger / inferReport{Channels,Schedule} /
inferWorkflowProposal / inferRepoBranchSlug / inferRepoMRTitle —
all verbatim Rust ports); **http_json** / **openfoundry_api** (real
HTTP request with optional Authorization forward via auth_mode=
forward_bearer, query string for GET / JSON body for POST,
extra-headers from config, base_url + path resolution including
OPENFOUNDRY_API_BASE_URL env fallback).

resolveToolInputs reads context.tool_inputs[name|UUID] then falls
back to {question,user_message,objective}. enforceToolPolicy emits
status="blocked" + approval_required when sensitivity ∈
{high,mutating,admin} or requires_approval=true AND tool_policy
doesn't approve.

handlers/agents.ExecuteAgent now fully wired: loads agent + tools
(UUIDs in agent.tool_ids), runs rag.Search over the optional
knowledge_base_id documents, resolves objective (body → agent →
user_message), calls agents.BuildPlan + ExecutePlan, picks an
LlmProvider via llm.RouteProviders + SelectProvider for "agents"
use_case, runs llm.CompleteText with the synthesised system+user
prompt, updates agent.memory via agents.UpdateMemory and persists
the bumped memory + last_execution_at column. Falls back to last-
trace observation when no provider is configured.

Tests pin 18 edges: ExecutePlan synthesise-counter / retrieve-context
shape / generic-step fallback / missing-tool envelope / simulated
with-and-without mock / sensitive blocking + context-approval
bypass / knowledge_search ranking + top_k + empty-query path / HTTP
happy path + missing-URL envelopes for both modes / unsupported-mode
skip / native_sql lookback inference / 5 inference helpers
(time window, object types, repo branch slug, report channels,
lexical score) / template rendering.

## Decisions deferred for human review

The big-pure-logic backlog from Run 4 is now empty. The remaining
501 stubs in handlers are blocked on integrations rather than
pure-logic ports:

1. **handlers/chat slice 2** (CreateChatCompletion / AskCopilot /
   BenchmarkProviders) — needs ai_semantic_cache + ai_conversations +
   ai_llm_usage_events DB-side-effect helpers (~200 LOC of Rust)
   plus the auth-middleware-go purpose-checkpoint integration.
2. **handlers/runs slice** — list_runs / create_run / update_run /
   compare_runs in handlers/experiments.go. Independent of the
   deferred items above; needs the ml_runs row scaffolding.
3. **handlers/experiments asset-lineage** — ~459 LOC graph builder.
   Pure logic but heavy enough for its own iteration.
4. **auth-middleware-go purpose-checkpoint** — blocking the chat slice
   2 and the agent execute_with_sensitive_tools path. Hits the
   `/api/v1/checkpoints/purpose/enforce` endpoint of the identity-
   federation service.

## Status snapshot after Run 5

- **libs/ai-kernel-go**: 7/7 models, 5/5 handler files (chat slice 2
  still 501-stubbed pending DB-side-effect helpers); domain/{llm
  full incl. runtime, agents memory+planner+executor, rag full,
  evaluation, copilot}. **Zero pure-logic modules pending.**
- **libs/ml-kernel-go**: 10/10 models, 7/7 handler files (runs +
  asset-lineage 501-stubbed pending separate slices); domain/{drift,
  predictions, training/{hyperparameter, runner, execute}, interop
  full}. **Zero pure-logic modules pending.**
- All 16 substrate-only / full-foundation service shells continue
  to bind the kernel handlers cleanly — agent-runtime now wires the
  full ExecuteAgent end-to-end since the executor is in.
- All builds + vets + tests green workspace-wide at every commit.

## Build warnings worth flagging

None. `go build ./...` and `go test ./libs/...` both clean across
the workspace at every commit.

The remaining migration backlog is now entirely DB-side-effect
helpers + auth-middleware integration + isolated ml_runs handler
scaffolding. There are no large pure-logic ports left between the
Rust source and the Go workspace for libs/ai-kernel and
libs/ml-kernel.

---

# Run 6 — 2026-05-06 (continuation)

User asked "continua por favor". Cleared the remaining DB-side-effect
+ handler-scaffolding stubs in the ml-kernel-go handler surface.

## What landed

3 new commits, all on `frontend/settings-mfa-apikeys-sso`, never pushed:

| # | Commit    | Slice                                                          |
|---|-----------|----------------------------------------------------------------|
| 1 | `638942b4`| libs/ml-kernel-go — handlers/runs slice (full ml_runs CRUD + compare) |
| 2 | `c50679eb`| libs/ml-kernel-go — handlers/asset_lineage (full 6-tier graph builder) |
| 3 | (this)    | docs(NIGHTLY-SUMMARY,migration-rust-to-go) — Run 6 close-out  |

## Slices completed this run

### libs/ml-kernel-go/handlers/runs (~260 LOC of Rust)

Replaces 4 501-stubs in handlers/experiments.go. ListRuns(experimentID)
returns ml_runs filtered by experiment_id ordered by created_at DESC.
CreateRun defaults status="completed", auto-fills started_at to now
and finished_at to now when status=completed and the body left them
nil; folds external_tracking through interop.MergeRunParams +
MergeMetrics + MergeRunArtifacts; refreshes experiment rollup
afterwards. UpdateRun uses the per-field unwrap_or pattern; external
tracking re-folds; refreshes rollup using the run's experiment_id.
CompareRuns loads each run (404 on first miss), returns the union of
metric names sorted alphabetically (matches Rust BTreeSet via
sort.Strings).

scanRun rebuilds ExperimentRun including the ExternalTracking re-
derivation via interop.TrackingSourceFromParams so the wire shape
stays identical to the Rust source.

### libs/ml-kernel-go/handlers/asset_lineage (~459 LOC of Rust)

Replaces the 501-stub on GetExperimentAssetLineage with the full
6-tier graph builder mirroring fn get_experiment_asset_lineage
verbatim. Walks runs + training_jobs + experiment.objective_spec to
collect dataset_ids / model_ids / version_ids / frameworks; loads
model_versions / models / deployments by ID array; emits nodes for
each entity and edges for every neighbour pair using the verbatim
Rust label taxonomy (targets_dataset, tracks_run,
orchestrates_training, targets_model, consumes_dataset,
logged_model_version, trains_on, produces_for_model, best_candidate,
belongs_to_model, serves_model, monitors_against_dataset).

Adds a narrower lineageDeploymentRow scanner (10 columns vs the 12
needed by handlers/deployments.go), and uuidSet / stringSet helpers
that mirror Rust BTreeSet sorted iteration via uuid.String() lex
order / sort.Strings (stable test output across runs).

The framework set folds 5 sources: run.external_tracking.framework,
training_job.external_training.framework,
training_job.training_config.engine, model_version.schema.engine,
model_version.schema.model_adapter.framework.

## Tests added this run

13 new test cases:
  - CreateRun rejects empty / whitespace name + bad JSON.
  - CompareRuns rejects empty run_ids array (verbatim Rust message
    "at least one run is required" — not "run id" as the prior
    stub had).
  - assetNodeID format roundtrip.
  - stringFieldFromJSON + nestedStringFieldFromJSON path matrix
    (nil / empty / mismatch / hit / malformed JSON).
  - rawOrNullValue malformed-JSON tolerance.
  - uuidSet / stringSet sorted iteration with dedupe.

## Status snapshot after Run 6

- **libs/ai-kernel-go**: 7/7 models, 5/5 handler files, every
  pure-logic domain. ExecuteAgent fully wired through executor +
  runtime. (Chat completion / copilot / benchmark are also wired
  end-to-end via chat_runtime.go — the chat_runtime + chat_test +
  chat changes in the working tree are user-authored and pending
  the user's own commit.)
- **libs/ml-kernel-go**: 10/10 models, 7/7 handler files, every
  pure-logic domain. **Zero 501 stubs anywhere in the kernel
  handler surfaces** — every endpoint defined by the Rust source is
  wired to the same DB shape and same wire envelope.
- 16 substrate-only or full-foundation service shells continue to
  bind the kernel handlers cleanly. Workspace `go build ./...` and
  `go test ./libs/...` both green.

## Decisions deferred for human review

The kernel-library backlog is now empty for handler / domain ports.
What remains are integration items:

1. **auth-middleware-go purpose-checkpoint** — `/api/v1/checkpoints/
   purpose/enforce` integration in identity-federation-service.
   Blocks the sensitive-tool agent path and the chat-completion
   sensitive-PII path from doing operator-approval enforcement.
   Both currently rely on per-tool policy / private-network routing
   instead.
2. **pyo3 sidecars** (notebook-runtime, pipeline-build,
   ontology-actions) — STOP-and-ask, gRPC sidecar pattern.
3. **Phase 3 follow-ups** — identity-federation Cassandra active
   wiring, SAML runtime registry, 7b control panel + ABAC, and JWKS/
   Vault/SCIM route wiring; tenancy-organizations RETIRED spaces /
   projects / trash / resource_resolve; continue extending the
   authz-cedar-go conformance mirror.
4. **Phase 6 per-service runtime slices** — media-transform image
   decoding, entity-resolution engine, ontology-exploratory
   views/maps, reindex/lineage/workflow-automation runtime,
   federation sub-domains, app-composition widgets, agent-runtime
   tools+Kafka, ai-evaluation handlers, model-catalog ml-kernel.
5. **Iceberg writers** in telemetry-governance / audit-sink / ai-sink
   — pending iceberg-go writes.

## Build warnings worth flagging

None. Workspace clean at every commit.
