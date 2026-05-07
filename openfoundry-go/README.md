# openfoundry-go

Go re-implementation of the OpenFoundry platform, mirroring the Rust workspace
1:1 at the **functional / contract** level (proto, OpenAPI, SQL schemas, Kafka
topics) so a service in either language can interoperate with the other during
the migration window.

> **Status (2026-05-06):** Phases 0–6 complete. The Rust workspace at the
> repo root remains the production source of truth, but the Go re-implementation
> covers the foundational libs, edge gateway, audit/AI Kafka sinks, the
> identity stack (federation incl. SAML 5b + SCIM 2.0 + Cedar authz + JWKS
> rotation with Vault Transit), Phase 4 data libs (cassandra-kernel,
> ontology-kernel, scheduling-cron, saga, search-abstraction, state-machine),
> and Phase 5 ai/ml libs (ai-kernel-go llm runtime + agents/executor,
> ml-kernel-go interop + training/runner). See `INVENTORY-PHASE6.md` for the
> per-service port matrix. Phase 5 pyo3 sidecars
> (notebook-runtime, pipeline-build, ontology-actions) are deferred pending
> a go/no-go decision on the sidecar architecture.

## Repository layout

```
openfoundry-go/
├── libs/                       # shared packages (mirrors Rust libs/)
│   ├── core-models/            # typed IDs, errors, schemas, markings ✅
│   ├── observability/          # slog + OTel + Prometheus ✅
│   ├── auth-middleware/        # JWT + chi middleware ✅
│   ├── db-pool/                # pgxpool DualPool (writer + reader) ✅
│   ├── event-bus-control/      # NATS JetStream publisher + streams ✅
│   ├── event-bus-data/         # Kafka publisher + subscriber + OL headers ✅
│   ├── audit-trail/            # 13 events + envelope + outbox bridge ✅
│   ├── idempotency/            # Store interface + Pg + Mem backends ✅
│   ├── outbox/                 # transactional outbox INSERT+DELETE helper ✅
│   ├── testing/                # testcontainers-go + JWT/SQL fixtures ✅
│   ├── …                       # 18 tier-2 libs deferred until consumed
│   └── proto-gen/              # generated from ../proto via `make gen`
│
├── services/                   # one Go binary entrypoint per microservice
│   ├── edge-gateway-service/   # ✅ Phase 2 — HTTP edge (first cutover)
│   ├── audit-sink/             # ✅ Phase 2 — Kafka → Iceberg (audit.events.v1)
│   ├── ai-sink/                # ✅ Phase 2 — Kafka → Iceberg (ai.events.v1, 4 tables)
│   ├── identity-federation-service/ # ✅ Phase 6 — full auth (OIDC + SAML + SCIM + MFA + WebAuthn + Cedar + JWKS)
│   ├── tenancy-organizations-service/ # ✅ Phase 6
│   ├── authorization-policy-service/  # ✅ Phase 6 (Cedar)
│   ├── notification-alerting-service/ # ✅ Phase 2 cluster
│   ├── sdk-generation-service/        # ✅ Phase 2 cluster
│   ├── telemetry-governance-service/  # ✅ Phase 2 cluster
│   ├── …                              # Phase 4 / 5 services landing per inventory
│   └── template/               # reference layout (copy + rename)
│
├── proto/                      # reserved (canonical .proto live at ../proto)
├── tools/of-cli                # admin CLI (port of tools/of-cli)
│
├── go.mod                      # single module for the whole monorepo
├── Makefile                    # build / test / lint / gen / ci
├── buf.gen.yaml                # Go codegen pipeline
├── sqlc.yaml                   # type-safe DB code generation
└── .golangci.yml               # lint config (mirror of clippy strictness)
```

CI is wired at the repo root: `.github/workflows/openfoundry-go.yml`.

## Single-module decision

This repository is intentionally a **single Go module** (`go.mod` at the root)
rather than a `go.work` multi-module setup. Rationale:

- Mirrors the way Kubernetes, Grafana, CockroachDB monorepos are organised.
- Avoids version drift between `libs/` and `services/`.
- Faster builds (one module cache, one resolution graph).

Splitting individual `services/` into their own modules is reversible — none of
the current code depends on the single-module shape.

## Day-to-day commands

```sh
make tools          # one-time: install buf / golangci-lint / sqlc / gofumpt into ./bin
make gen            # regenerate proto (Go) + sqlc (per-service)
make build          # compile everything
make build-services # produce one binary per service in ./bin
make test           # unit tests with -race + coverage
make test-integration  # tests tagged "integration" — needs Docker
make lint           # golangci-lint (CI gate)
make ci             # full local CI gate (tidy + vet + lint + test)
```

## Phase 0 deliverables (this commit)

- ✅ Repository scaffolding (`libs/`, `services/`, `proto`, `tools/`, `build/`).
- ✅ Single Go module rooted at `github.com/openfoundry/openfoundry-go`.
- ✅ `buf` pipeline generating Go from `../proto` into `libs/proto-gen/`.
- ✅ `sqlc.yaml` skeleton ready for per-service registration.
- ✅ Service template (`services/template/`) with:
  - `cmd/template/main.go` — config → observability → server boot.
  - koanf-backed `internal/config` mirroring the Rust `config-rs` precedence.
  - chi-based `internal/server` with `/healthz`, `/metrics`, JWT-protected `/api`.
  - Multi-stage distroless `Dockerfile`.
- ✅ Foundational libs migrated:
  - `libs/core-models/` — IDs, errors, health, pagination, timestamps, dataset
    schema, security markings, media references. **Wire format byte-compatible
    with Rust.**
  - `libs/observability/` — slog logging, OTel tracing, Prometheus registry.
  - `libs/auth-middleware/` — Claims, JWT (HS256+RS256), chi middleware.
- ✅ CI workflow (`lint`, `proto-drift`, `test`, `integration`).
- ✅ Strict `golangci-lint` config matching the Rust workspace's clippy posture.

## Tools parity status (2026-05-07)

- ✅ `tools/of-cli/` is now ported to Go as `go run ./tools/of-cli -- ...`.
  It closes the Rust `tools/of-cli` gap for command/flag parsing and the
  principal tool surfaces: OpenAPI generation/validation from proto files,
  TypeScript/Python/Java SDK scaffold generation/validation, scenario-driven
  smoke runs, sequential benchmark runs, and the local AI mock provider.

## Phase 1 deliverables (this commit)

- ✅ `libs/db-pool/` — pgxpool-backed DualPool (writer + optional reader)
  with `from-env` precedence + ping-on-create matching Rust sqlx semantics.
- ✅ `libs/event-bus-control/` — NATS JetStream `Publisher`,
  `EnsureStream`/`CreateConsumer`, well-known subjects/streams constants,
  `Event` envelope wire-compat.
- ✅ `libs/event-bus-data/` — Kafka `Publisher`/`Subscriber` over
  `segmentio/kafka-go` (no CGO), at-least-once + explicit commits,
  `OpenLineageHeaders` round-trip, SCRAM-SHA-512 / PLAINTEXT principals.
- ✅ `libs/audit-trail/` — 13 audit event variants, 7 categories,
  `AuditEnvelope`, **deterministic v5 UUID byte-identical to Rust**
  (cross-language golden test), outbox bridge `EmitToOutbox`.
- ✅ `libs/idempotency/` — `Store` interface + `PgStore` (INSERT … ON
  CONFLICT DO NOTHING RETURNING) + `MemStore` + `Idempotent` wrapper.
- ✅ `libs/outbox/` — transactional INSERT+DELETE helper compatible with
  Debezium EventRouter SMT.
- ✅ `libs/testing/` — `BootPostgres` (testcontainers-go, integration
  build tag), `JWTConfig`, `DevToken`, `SeedDataset`.

## Phase 1 — tier 2 (status as of 2026-05-06)

These were originally listed as "migrate on first consumer." Most have
landed:

- ✅ `cassandra-kernel` — 5 stores ported (Object/Link/Schema/Session/ActionLog)
  via gocql; `~3500` LOC + 60+ unit tests.
- ✅ `authz-cedar` — wired through cedar-go (`cedarauthz.Service` +
  `AdminGuard` middleware) inside identity-federation-service.
- ✅ `state-machine`, `scheduling-cron`, `saga`, `search-abstraction` —
  full ports with parser/evaluator/runner-style sub-modules where the
  Rust crate had them.
- ✅ `ontology-kernel` — domain layer foundation + handlers (in progress).
- 🟡 `storage-abstraction` — search trait surface ported; HTTP backends
  (vespa, opensearch) deferred to first consumer.
- ⏸ `query-engine`, `vector-store`, `geospatial-core`, `geospatial-tiles`,
  `media-scanner`, `pipeline-expression`, `plugin-sdk`, `analytical-logic`
  — placeholder dirs only; consumed-on-demand.

## Phase 2 deliverables (this commit)

- ✅ `services/edge-gateway-service/` — full reverse-proxy port:
  - 80+ path-prefix routing rules (`internal/proxy/router_table.go`)
    matching the Rust crate's service map.
  - Path rewriting (`/api/v1/datasets/...` → `/v1/datasets/...`,
    filesystem alias, catalog facets).
  - Tenant + auth header injection (`x-openfoundry-*`) — header set
    byte-identical to the Rust gateway.
  - Token-bucket rate-limit middleware (in-memory backend +
    Lua-backed Redis backend behind the same `Store` interface).
  - Zero-trust scope enforcement (403 with stable error codes).
  - Audit fire-and-forget to NATS `OF_AUDIT.gateway`.
  - Canonical error envelope `{"error":{"code":"...","message":"..."}}`.
  - `/healthz` + `/metrics` outside the proxy chain so they stay
    reachable when upstreams or rate-limit backends fail.
- ✅ `libs/auth-middleware/tenant.go` — `TenantContext`,
  `QuotaStandard/Team/Enterprise`, per-claim quota overrides.
- ✅ Test coverage:
  - Router-table golden test covers ~60 distinct routes.
  - Path-rewrite tests for the Foundry compatibility surface.
  - Proxy integration tests via `httptest`: routing, path rewriting,
    zero-trust scope, header injection, body limits, error envelope.
  - Rate-limit token-bucket math (burst exhaustion, key separation,
    limit=0 deny-all).

## Phase 2 — Kafka sinks (this commit)

- ✅ `services/audit-sink/` — `audit.events.v1` → `lakekeeper/of_audit/events`.
  Batch policy 100k OR 60s, at-least-once with single-call
  `CommitMessages` per batch, poison-pill handling moves the offset
  forward so the partition can't wedge.
- ✅ `services/ai-sink/` — `ai.events.v1` → 4 Iceberg tables
  (`prompts`/`responses`/`evaluations`/`traces`) routed by `kind`.
  Per-table batching + per-table metrics labels.
- ✅ Both ship a `Writer` interface with two implementations:
  `JSONLWriter` (production-suitable for staging / observability) and
  `IcebergWriter` (stub — fails loud until `apache/iceberg-go`'s
  write API stabilises).
- ✅ `libs/event-bus-data.Subscriber` gained `CommitMessages([]*DataMessage)`
  for batch-commit semantics; matches segmentio/kafka-go's reader API
  and makes the runtimes trivially testable with stubs.

### Phase 2 — remaining services (all landed)

- ✅ `notification-alerting-service`
- ✅ `sdk-generation-service`
- ✅ `telemetry-governance-service`

## Phase 6 — Identity (this commit)

The identity stack is now Go-native end to end:

- ✅ `services/identity-federation-service/` — full re-implementation:
  - **Slice 5a** OIDC SSO via `coreos/go-oidc` (Google, Microsoft,
    GitHub, GitLab) + state row in oauth_state.
  - **Slice 5b** SAML 2.0 via hand-rolled domain layer +
    `russellhaering/goxmldsig` + `beevik/etree`. Covers: AuthnRequest
    construction, IdP metadata parsing, response signature
    verification (RSA-SHA1 supported for legacy IdP fixtures),
    full RFC 7522 validation chain (status, destination,
    in-response-to, conditions, audience, subject confirmation,
    expected issuer), AttributeStatement extraction, byte-exact
    OneLogin sample fixtures. POST /api/v1/auth/sso/{provider}/acs
    endpoint for the HTTP-POST binding.
  - **Slice 6** RBAC CRUD (users, roles, groups, permissions, api-keys).
  - **Slice 7a** Restricted views.
  - **Slice 8 (Cedar)** `internal/cedarauthz` — Cedar policy
    evaluation + `AdminGuard` middleware emitting Group/Role parent
    entities for hierarchy lookups.
  - **Slice 8 (JWKS rotation)** `internal/jwksrotation` —
    Service orchestrator + Postgres key store + Vault Transit signer
    (Token + Kubernetes-role auth) + HTTP handlers
    (PublishJwks/RotateJwks/RollbackJwks) + Hash/Sign/Verify
    helpers. ~3520 LOC + ~75 tests.
  - **Slice 8 (SCIM 2.0)** `internal/scim` — RFC 7643/7644 endpoints:
    discovery (ServiceProviderConfig / Schemas / ResourceTypes),
    User CRUD + Patch + Delete, Group CRUD + member operations +
    Patch. PostgresUserStore + PostgresGroupStore for production +
    in-memory stores for tests. ~5170 LOC + ~97 tests.
  - **MFA TOTP + WebAuthn** ports.
  - **Sessions in Cassandra** for the slice-2b cutover.
- ✅ `services/tenancy-organizations-service/` and
  `services/authorization-policy-service/`.

## Wire-compat invariants (do not break)

These are the contracts every Go service inherits from the Rust source of truth
and must not drift from while both implementations coexist:

- `/healthz` payload shape (`status`, `service`, `version`, `timestamp`).
- JWT claims field names + JSON tags (see `libs/auth-middleware/claims.go`).
- Dataset RID format `ri.foundry.main.dataset.<uuid-v7>`.
- Transaction state / type tokens (`open|committed|aborted`, `snapshot|append|update|delete`).
- Marking source discriminator (`{"kind": "direct"}` / `{"kind": "inherited_from_upstream", ...}`).
- Media reference camelCase keys (`mediaSetRid`, `mediaItemRid`, `branch`, `schema`).
- Schema field type discriminator (`{"type": "DECIMAL", "precision": ..., "scale": ...}`).

The test suites in `libs/core-models/**/*_test.go` already lock these.

## Where to read more

- `Cargo.toml` (repo root) — authoritative inventory of Rust services + libs.
- `proto/` — canonical RPC contracts shared between Rust and Go.
- ADRs under `docs/architecture/` — the decisions Phase 1+ must respect.
