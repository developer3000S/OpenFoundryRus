# CLAUDE.md — OpenFoundry (Go monorepo)

Onboarding for AI agents. Humans should also read `README.md` for the
project narrative and `CONTRIBUTING.md` for the PR / RFC process — both
are kept current with this repo. This file is the agent-facing summary:
tighter, with the gotchas and security boundaries surfaced. If a
disagreement ever appears, **this file wins** for agent purposes.

## What this repo is

Single Go module (`github.com/openfoundry/openfoundry-go`) plus a React
frontend. Originated as a port of a Rust workspace; the Rust side is gone
from this tree but its vocabulary still leaks into docs.

```
apps/web/        React 19 + Vite + TypeScript frontend
services/        41 Go microservices (one binary per dir, see template/)
libs/            32 shared Go packages (kernels, observability, auth, …)
proto/           Source-of-truth .proto files (Go code generated to libs/proto-gen/)
sdks/            Generated client SDKs (TS/Python/Java)
infra/           Helm charts, ArgoCD, Terraform, runbooks
docs/            VitePress docs site (capability-oriented)
docs/archive/    Historical migration logs — DO NOT READ unless asked
tools/           CLIs (of-cli, route-audit, lint helpers)
```

Per-service shape (uniform — copy from `services/template/`):

```
services/<svc>/
  cmd/<svc>/main.go         entrypoint
  internal/server/          chi router wiring (/healthz /metrics /api)
  internal/handlers/        HTTP handlers
  internal/domain/          pure logic
  internal/repo/            data access (sqlc-generated when relevant)
  internal/repo/migrations/ goose-style SQL migrations
  internal/models/          wire types
  internal/config/          koanf-backed config
```

## Canonical commands

Run from repo root. The Makefile is authoritative. Ignore `justfile`.

```sh
make tools             # one-off: install buf, golangci-lint, sqlc, gofumpt to ./bin
make ci                # tidy + vet + lint + test  (the gate to pass before pushing)
make test              # unit tests, -race + coverage, fast (no Docker)
make test-integration  # integration (testcontainers, NEEDS DOCKER)
make gen               # regen proto Go + sqlc (run after touching proto/ or *.sql)
make build-services    # one binary per service into ./bin/
```

Frontend (`apps/web/`):

```sh
pnpm --filter @open-foundry/web dev    # vite dev server
pnpm --filter @open-foundry/web check  # tsc -b --noEmit
pnpm --filter @open-foundry/web test   # vitest
```

## Gotchas (real, not theoretical)

- **`justfile` is a thin shim over `make`.** Every recipe just calls the
  matching Make target; the Makefile is canonical. (Until recently the
  justfile was full of `cargo` recipes pointing at a Rust workspace
  that no longer exists in this tree. If you see `just <recipe>` in
  legacy docs, mentally translate to `make <recipe>`.)
- **`make lint` baselines pre-existing issues.** `.golangci.yml` is
  configured with `new-from-rev: HEAD`, so `make lint` only flags
  issues introduced *after* the latest commit. To audit the full
  backlog: `golangci-lint run --new-from-rev= ./...` (mostly spelling
  + staticcheck style nits, tracked as tech debt rather than a feature
  gate).
- **Go CI lives in `.github/workflows/openfoundry-go.yml`.** Jobs:
  `lint` (golangci-lint), `vet`, `tidy` (drift check on go.mod/go.sum),
  `proto` (`buf lint` + `buf generate` drift check on `libs/proto-gen`),
  `test` (unit, race + coverage), `integration` (build tag
  `integration`, runs after lint+test, uses GH runner Docker for
  testcontainers). It mirrors `make ci` plus a proto-generation drift
  check; **sqlc drift is not yet checked in CI**. The legacy `ci.yml`
  (cargo-based, Rust era) has been removed.
- **Other workflows still reference the Rust era.** `proto-check.yml`,
  `chaos-smoke.yml`, `iceberg-integration.yml`,
  `integration-foundry-pattern.yml` and `security-audit.yml` either
  have `Cargo.*` in their path filters or run cargo / sqlx steps that
  no longer match this tree. The Python tooling under
  `tools/bus-lint/` and `tools/data-residency/` also walks Cargo.toml
  / sqlx and will not work against the Go monorepo. Treat any green
  signal from those as "did not run" until they are migrated.
- **Single Go module, root `go.mod`.** Don't create per-service modules.
- **`libs/proto-gen/` is generated.** Don't edit by hand — re-run `make gen`.

## Conventions

- **Errors:** `errors.Is`-style sentinels at package scope (`ErrNotFound`,
  `ErrPreconditionFailed`, …). HTTP layer maps them.
- **Wire types:** generic envelopes `models.Page[T]` and
  `models.ListResponse[T]`. Cursor-pagination uses `next_cursor`.
- **Auth:** every protected route goes through `libs/auth-middleware`.
  Claims live in `r.Context()` — fetch via the lib helpers, never parse
  JWT in handlers.
- **Observability:** use `libs/observability` for slog logger + OTel +
  Prometheus. Each service exposes `/metrics`; do not re-register globals.
- **Testing:** unit tests next to source as `*_test.go`. Anything needing
  Postgres/Cassandra/Kafka must use the `integration` build tag and the
  helpers in `libs/testing` (testcontainers).
- **DI:** state is held on a struct (`*Handlers`, `*AppState`). Avoid
  package-level globals; only 3 `init()` functions exist in the entire
  service tree — keep it that way.

## Security-critical zones

Changes here need extra care and explicit human review:

- `services/identity-federation-service/` — OIDC, SAML, MFA, WebAuthn,
  SCIM, JWKS rotation, Cedar admin policies.
- `services/authorization-policy-service/` — Cedar engine, ABAC/RBAC
  evaluation, restricted views.
- `libs/auth-middleware/` — JWT validation chain.
- `services/*/internal/repo/migrations/` — destructive DDL once shipped.
- `proto/auth/`, `proto/audit/` — wire-format breakage hits every
  consumer.

When touching these, surface the change in the PR description and
prefer additive changes over rewrites.

## What NOT to read

These exist for human historical context only. Loading them into your
context window wastes tokens and may give you obsolete instructions:

- `docs/archive/**` — Rust→Go migration logs, route audits, evaluations,
  inventories, and prompt programs. Superseded by the live code or by
  ADRs in `docs/architecture/adr/`. Don't load these by default; only
  read a specific section if an ADR cites it.
- `docs_original_palantir_foundry/` — third-party reference material,
  not OpenFoundry's own docs.

For runtime architecture, prefer:

1. `docs/architecture/index.md`
2. `docs/architecture/adr/` (decisions; numbered, dated, supersession-tracked)
3. The per-module `CLAUDE.md` in the directory you're editing

## Adding a new service

Copy `services/template/`, register it in:

- `infra/helm/apps/<chart>/` if it ships in a release
- `services/edge-gateway-service/internal/proxy/router_table.go` if it
  receives external HTTP traffic
- `infra/argocd/apps/` for GitOps deploy
