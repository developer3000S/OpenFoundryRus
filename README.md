# OpenFoundry

OpenFoundry is an open-source operational data platform — a Go monorepo
of microservices plus a React frontend, organized around the same
capability model as Palantir Foundry's documentation (datasets,
ontology, applications, AI/ML, governance, observability).

> **Working with this codebase as an AI agent?** Start at
> [`CLAUDE.md`](CLAUDE.md). It's the canonical onboarding for agents
> (commands, conventions, security-critical zones, what NOT to read)
> and is kept tighter than this README on purpose.

## Repository layout

```
openfoundry/
├── apps/web/         React 19 + Vite + TypeScript frontend
├── services/         41 Go microservices (one binary per dir, copy services/template/)
├── libs/             32 shared Go packages (auth, observability, kernels, …)
├── proto/            Protobuf source of truth (Go generated to libs/proto-gen/)
├── sdks/             Generated SDKs (TS/Python/Java)
├── infra/            Helm, ArgoCD, Terraform, runbooks
├── docs/             VitePress capability-oriented docs site
├── docs/archive/     Historical migration logs (do not load by default)
├── tools/            CLIs (of-cli, route-audit, lint helpers)
├── go.mod            Single module for the whole monorepo
├── Makefile          Canonical task runner
└── .golangci.yml     Lint config (schema v2)
```

Per-service shape (uniform — copy `services/template/`):

```
services/<svc>/
  cmd/<svc>/main.go          entrypoint
  internal/server/           chi router (/healthz, /metrics, /api)
  internal/handlers/         HTTP handlers
  internal/domain/           pure logic
  internal/repo/             data access (sqlc-generated when relevant)
  internal/repo/migrations/  goose-style SQL migrations
  internal/models/           wire types
  internal/config/           koanf-backed config
```

## Single-module decision

The repository is intentionally a **single Go module** (`go.mod` at the
root) rather than a `go.work` multi-module setup:

- Mirrors the way Kubernetes, Grafana, and CockroachDB monorepos are
  organised.
- Avoids version drift between `libs/` and `services/`.
- Faster builds (one module cache, one resolution graph).

Splitting individual services into their own modules is reversible.

## Day-to-day commands

Run from the repo root. The Makefile is canonical; a `justfile` is
provided as a thin shim over `make` for muscle memory.

```sh
make tools             # one-off: install buf, golangci-lint, sqlc, gofumpt to ./bin
make ci                # tidy + vet + lint + contract drift checks + test
make test              # fast unit tests (-race + coverage, no Docker)
make test-integration  # tests behind //go:build integration (needs Docker)
make gen               # regen proto Go + sqlc + OpenAPI + SDKs
make contracts-check   # verify OpenAPI + TypeScript/Python/Java SDK drift
make build             # compile every package
make build-services    # one binary per service into ./bin/
make lint              # golangci-lint
make fmt               # gofumpt + gci
```

Frontend (`apps/web/`):

```sh
pnpm --filter @open-foundry/web dev    # vite dev server
pnpm --filter @open-foundry/web check  # tsc -b --noEmit
pnpm --filter @open-foundry/web test   # vitest
```

## Conventions

- **Errors:** `errors.Is`-style sentinels at package scope
  (`ErrNotFound`, `ErrPreconditionFailed`, …); HTTP layer maps them.
- **Wire types:** generic envelopes `models.Page[T]` and
  `models.ListResponse[T]`. Cursor pagination uses `next_cursor`.
- **Auth:** every protected route goes through `libs/auth-middleware`.
  Read claims from `r.Context()` via the lib helpers; never parse JWT
  in handlers.
- **Observability:** `libs/observability` for slog + OTel + Prometheus.
  Each service exposes `/metrics`; do not re-register globals.
- **Testing:** unit tests next to source; anything needing
  Postgres/Cassandra/Kafka uses the `integration` build tag and the
  helpers in [`libs/testing/`](libs/testing/) (testcontainers).
- **Migrations:** once shipped, immutable — add a new file rather than
  editing.
- **DI:** state is held on a struct (`*Handlers`, `*AppState`); avoid
  package-level globals.

## Wire-compatibility invariants

These contracts are pinned by golden tests in
`libs/core-models/**/*_test.go` and must not drift:

- `/healthz` payload shape (`status`, `service`, `version`, `timestamp`).
- JWT claims field names and JSON tags.
- Dataset RID format `ri.foundry.main.dataset.<uuid-v7>`.
- Transaction state and type tokens (`open|committed|aborted`,
  `snapshot|append|update|delete`).
- Marking source discriminator (`{"kind": "direct"}` / `{"kind": "inherited_from_upstream", ...}`).
- Media reference camelCase keys (`mediaSetRid`, `mediaItemRid`, `branch`, `schema`).
- Schema field type discriminator (`{"type": "DECIMAL", "precision": …, "scale": …}`).

## Documentation

- [`CLAUDE.md`](CLAUDE.md) — agent-facing onboarding (concise).
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — high-level architecture overview.
- [`docs/`](docs/) — capability-oriented technical documentation
  (VitePress); start at [`docs/index.md`](docs/index.md).
- [`docs/architecture/adr/`](docs/architecture/adr/) — numbered, dated
  architectural decisions with supersession tracking.
- [`docs/archive/`](docs/archive/) — historical migration logs and
  superseded design docs. Do not load these by default; they exist for
  audit only.
- Per-module `CLAUDE.md` files inside large libs and services (e.g.
  `libs/ontology-kernel/CLAUDE.md`).

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the PR process, RFC
requirements, and DCO policy. Security reports follow
[`SECURITY.md`](SECURITY.md).

## License

OpenFoundry is licensed under **AGPL-3.0-only** (see [`LICENSE`](LICENSE)).

