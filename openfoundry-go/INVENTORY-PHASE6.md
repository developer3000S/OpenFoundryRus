# Phase 6 cherry-pick inventory

State: **2026-05-06**, after Phase 4 closed (object-database foundation
landed) and the identity-federation autonomous run wrapped. All
Phase 0–4 services have foundation ports (modulo pyo3 sidecars that
are STOP-and-ask). This document scopes what is left.

**Recent additions (post-original-inventory):**

- `services/identity-federation-service/` is now **feature-complete**
  on the Go side: slice 5a OIDC + slice 5b SAML 2.0 + slice 6 RBAC +
  slice 7a restricted views + slice 8 (Cedar wiring + JWKS rotation
  with Vault Transit + SCIM 2.0 endpoints + Postgres stores).
  ~10k LOC of Go across `internal/{oidc,saml,scim,cedarauthz,
  jwksrotation,handlers,repo,server}` plus tests.
- Phase-1 tier-2 libs that were "deferred until consumed" mostly
  landed during this run: `cassandra-kernel` (5 stores, ~3500 LOC),
  `state-machine`, `scheduling-cron` (parser+evaluator+DST handling),
  `saga`, `search-abstraction` (in-memory + trait surface).
- Phase-5 ai/ml kernel partial port: `libs/ai-kernel-go/domain/llm/runtime`,
  `libs/ai-kernel-go/domain/agents/executor`,
  `libs/ml-kernel-go/domain/interop`, training/runner — unblocks the
  8 ai/ml shell services at the handler layer.

## Services NOT yet ported

| Service | Rust LOC | Pattern | Strategy |
|---|---|---|---|
| `agent-runtime-service` | 495 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `ai-evaluation-service` | 724 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `application-composition-service` | 194 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `code-repository-review-service` | 672 | own `main.rs`, full Postgres impl, outbox | **PORT-READY** (foundation slice) |
| `entity-resolution-service` | 2937 | own main, large | architecture+slice (>1500 LOC) |
| `federation-product-exchange-service` | 8248 | own main, very large | architecture+slice (>1500 LOC) |
| `lineage-service` | 5100 | own main, pyo3 dep declared but unused | architecture+slice |
| `llm-catalog-service` | 67 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `media-transform-runtime-service` | 800 | own main | medium foundation |
| `model-catalog-service` | 465 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `model-deployment-service` | 122 | `fn main(){}` shell + **ml-kernel** re-exports | blocked on **ml-kernel-go** (not ai-kernel) — inventory correction 2026-05-06 |
| `notebook-runtime-service` | 2065 | **pyo3 used** | Phase 5 sidecar — STOP and ask |
| `ontology-actions-service` | — | **pyo3 used** | STOP and ask (existing guardrail) |
| `ontology-exploratory-analysis-service` | 2125 | own main | architecture+slice |
| `pipeline-build-service` | 29286 | **pyo3 used** | Phase 5 sidecar — STOP and ask |
| `reindex-coordinator-service` | 2561 | own main | architecture+slice |
| `retrieval-context-service` | 269 | shell + ai-kernel + own document_intelligence | partial — ai-kernel-bound |
| `solution-design-service` | 148 | `fn main(){}` shell + ai-kernel re-exports | blocked on ai-kernel-go |
| `workflow-automation-service` | 7188 | own main | architecture+slice (>1500 LOC) |

Excluded (permanent Rust): `sql-bi-gateway-service` (datafusion).
Excluded (Scala, not Rust): `pipeline-runner` (Spark job runner).

## Categorization

### A. Port-ready standalone services (own `main.rs`, no kernel shell)

1. **`code-repository-review-service`** (672 LOC src) — owns global-branching
   HTTP surface + code-security scan/finding stores (S8/ADR-0030 absorbed
   from `global-branch-service` and `code-security-scanning-service`).
   Postgres CRUD + outbox-emit on `foundry.global.branch.promote.requested.v1`.
   Kafka subscriber for `foundry.branch.events.v1` is exposed as a port,
   not directly linked. **Recommended next port.**
2. `media-transform-runtime-service` (800 LOC) — own main. Medium foundation.
3. `entity-resolution-service` (2937 LOC) — architecture+slice.
4. `ontology-exploratory-analysis-service` (2125 LOC) — architecture+slice.
5. `reindex-coordinator-service` (2561 LOC) — architecture+slice.
6. `lineage-service` (5100 LOC) — architecture+slice; pyo3 dep declared
   but `grep -rln "use pyo3\|Python::"` returns no hits, so the binary
   is portable; the dep is residual.
7. `workflow-automation-service` (7188 LOC) — architecture+slice.
8. `federation-product-exchange-service` (8248 LOC) — architecture+slice.
   This is the consolidated S8 marketplace + product-distribution
   service per recent commits.

### B. ai-kernel-bound shell services (`fn main(){}` + `#[path]` re-export)

`agent-runtime-service`, `ai-evaluation-service`,
`application-composition-service`, `llm-catalog-service`,
`model-catalog-service`, `model-deployment-service`,
`solution-design-service`, `retrieval-context-service` (partial).

These are blocked on **`libs/ai-kernel-go`** (Rust `libs/ai-kernel` is
~7515 LOC; well over the 1500 LOC iteration budget). Suggested follow-up
plan:

1. Inventory + carve up `ai-kernel` into ports (handlers / models / domain
   / domain/llm / domain/agents / domain/rag) — each is its own slice.
2. Port `libs/ai-kernel-go` skeleton + the simplest sub-domain first
   (likely `models` since they're plain DTOs).
3. Once a sub-domain is in place, port one of the shell services that
   only uses that sub-domain end-to-end as a proof point.
4. Iterate per sub-domain until the kernel surface is complete.

This is multiple iterations of work; tracked as separate todos.

### C. pyo3 STOP-and-ask sidecars (Phase 5)

- `ontology-actions-service` (existing guardrail)
- `notebook-runtime-service` (uses pyo3 in src)
- `pipeline-build-service` (uses pyo3 in src)

Per `docs/architecture/migration-rust-to-go.md` §"What does NOT migrate
literally", these become Python sidecars over gRPC with the Go service
owning lifecycle. Out of scope for this autonomous loop.

### D. Architecture+slice candidates (>1500 LOC)

`entity-resolution-service`, `lineage-service`,
`ontology-exploratory-analysis-service`, `reindex-coordinator-service`,
`workflow-automation-service`, `federation-product-exchange-service`.

Each gets a foundation iteration that ports the architecture +
1 vertical slice; subsequent slices follow.

## Priority for the autonomous loop

1. **`code-repository-review-service` foundation** (this run, next iter)
2. `media-transform-runtime-service` foundation
3. Architecture+slice ports of `entity-resolution-service`,
   `ontology-exploratory-analysis-service`, `reindex-coordinator-service`
4. `libs/ai-kernel-go` skeleton + first sub-domain (deferred — large
   multi-iteration block)
5. Architecture+slice of the very large services
   (`workflow-automation-service`, `federation-product-exchange-service`,
   `lineage-service`)

`pipeline-runner` (Scala) and `sql-bi-gateway-service` (Datafusion) are
permanent exclusions and stay in their current language.
