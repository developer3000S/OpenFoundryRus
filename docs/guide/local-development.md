# Local Development

This page describes the fastest reliable paths for working in the OpenFoundry monorepo.

## Required Tooling

- Go version compatible with the root `go.mod`
- Node.js `20+`
- pnpm `9+`
- Docker and Docker Compose
- `make`; `just` is optional and only delegates to the Makefile

Optional but useful for specialized flows:

- Buf for protobuf linting and breaking-change checks
- Helm for chart validation
- Terraform for module validation

## Common Workflows

### Bootstrap

Install pinned Go-side tools into `./bin` and install frontend dependencies:

```bash
make tools
pnpm install
```

### Infrastructure Only

Use Docker Compose directly when you only need backing services. The active Compose files live under `infra/compose/`:

```bash
docker compose -f infra/compose/docker-compose.yml up -d
docker compose -f infra/compose/docker-compose.yml down
```

### Backend Iteration

Build the whole Go module:

```bash
make build
```

Build all service binaries into `./bin`:

```bash
make build-services
```

Run tests:

```bash
make test
go test ./services/<service>/...
```

Use `make test-integration` for integration tests; it expects Docker and any service-specific dependencies required by the tested package.

### Frontend Iteration

The root Node scripts already proxy into `apps/web`:

```bash
pnpm dev
pnpm lint
pnpm test:unit
pnpm build
```

If you prefer to work directly in the app package:

```bash
pnpm --filter @open-foundry/web dev
pnpm --filter @open-foundry/web check
```

### Docs Iteration

Docs live under `docs/`. If a docs package is present, work from that directory and use its local scripts:

```bash
cd docs
npm ci
npm run docs:dev
```

There is no current `just docs-build` recipe in the root `justfile`; check the docs package scripts before copying older commands.

## Operational Assumptions

The repo is designed around service isolation rather than a single shared database. The smoke workflow creates separate Postgres databases for multiple services such as auth, datasets, pipelines, reports, geospatial, ontology, AI, and ML. That is a good mental model for local development too.

Several services also assume supporting infrastructure:

- Redis for gateway caching and stateful coordination
- NATS for async messaging
- MinIO or another object-store-compatible backend
- Vespa (single-node container in dev, multi-node Helm chart in production)
  for hybrid BM25 + vector + filter + ranking search
  (see [ADR-0007](../architecture/adr/ADR-0007-search-engine-choice.md))

## Helpful Commands From `Makefile`

| Goal | Command |
| --- | --- |
| Install pinned Go tools | `make tools` |
| Build all Go packages | `make build` |
| Build all service binaries | `make build-services` |
| Run Go tests | `make test` |
| Run integration tests | `make test-integration` |
| Run Go lint | `make lint` |
| Format Go code | `make fmt` |
| Generate proto and sqlc output | `make gen` |
| Validate capabilities snapshot | `make capabilities-check` |
| Run local Go CI gate | `make ci` |

`just` remains available for users who have that muscle memory, but it is a shim. If a command is not visible in `make help` or `just --list`, do not assume it exists.
