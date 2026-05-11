# Local workflows

The local developer experience in OpenFoundry is intentionally centered around a small number of repeatable commands.

## Primary entry points

- `make` for workspace-wide Go tasks
- `go` for direct package work
- `pnpm` for the frontend
- `npm` inside `docs/` for the isolated VitePress docs site
- Docker Compose for local infrastructure
- `just` only as a compatibility shim over the Makefile

## Core commands

The main contributor surface is defined in `Makefile`. Use `make help` before copying commands from older documents.

Important flows include:

- `make tools`
- `make build`
- `make test`
- `make test-integration`
- `make lint`
- `make fmt`
- `make gen`
- `make capabilities-check`
- `make ci`
- `pnpm --filter @open-foundry/web check`
- `pnpm --filter @open-foundry/web test:unit`
- `cd docs && npm ci && npm run docs:build`

There is no current root `just dev-stack`, `just smoke`, `just docs-build`, or `just ci-frontend` recipe. If a page recommends one of those names, treat that page as stale and prefer `Makefile`, `package.json`, and the relevant workflow file.

## Why this matters

The toolchain is not only about convenience. It is also how the repo keeps backend, frontend, generated contracts, smoke scenarios, and docs synchronized in one monorepo.
