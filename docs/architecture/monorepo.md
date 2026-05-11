# Monorepo Structure

The root Go module is the primary organizational unit of OpenFoundry. It
groups shared libraries, developer tooling, and runtime services under one
repository. The frontend and docs site live beside that Go module so the
contracts, UI, generated SDKs, and services version together.

## Top-Level Layout

| Path | Role |
| --- | --- |
| `apps/web` | React + Vite frontend for the platform UI |
| `libs/` | Shared Go packages used across services |
| `services/` | Go runtime services, normally with `cmd/<service>` and `internal/*` packages |
| `tools/of-cli` | Internal CLI for generation, smoke, mock providers, and benchmarks |
| `proto/` | Protobuf and code-generation inputs |
| `sdks/` | Generated TypeScript, Python, and Java SDKs |
| `infra/` | Docker Compose, Helm charts, Terraform provider schema, and deployment overlays |
| `smoke/` | Scenario definitions for end-to-end smoke validation |
| `benchmarks/` | Benchmark scenarios and result outputs |
| `docs/` | VitePress technical documentation site |

## Workspace Composition

The Go module currently includes:

- shared packages under `libs/`
- backend services under `services/`
- primary developer tooling under `tools/`, including `tools/of-cli`
- generated Go protobuf output under `libs/proto-gen/`

This structure makes it possible to share:

- authentication and claims middleware
- storage adapters
- vector and geospatial primitives
- audit and event abstractions
- testing helpers

## Frontend and Contract Placement

The frontend sits outside the Go module's package graph but inside the
monorepo so that it can directly consume:

- generated OpenAPI JSON
- generated Terraform schema
- generated TypeScript SDK outputs

That keeps the UI and contract surfaces versioned alongside the backend services that produce them.

## Service Ownership Model

Each service typically contains:

- `cmd/<service>/main.go`
- `internal/server`, `internal/handlers`, `internal/domain`, and
  `internal/repo` packages when that split is needed
- `internal/repo/migrations/` for service-owned PostgreSQL schema
- a service-specific `Dockerfile`

The codebase follows a service-owned database model in local development and CI smoke flows, rather than forcing all services into a single shared migration stream.
