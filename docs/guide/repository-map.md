# Repository Map

OpenFoundry is organized as a platform monorepo with clear directory-level ownership boundaries.

## Top-Level Layout

| Path | Role |
| --- | --- |
| `apps/web` | React + Vite frontend and product UI routes. |
| `services/*` | Go HTTP services, one directory per bounded service, normally with `cmd/<service>` and `internal/*` packages. |
| `libs/*` | Shared Go libraries for contracts, domain kernels, middleware, authz, observability, tests, and service support. |
| `proto/*` | Protobuf contracts grouped by domain, plus Buf configuration. |
| `tools/of-cli` | Go CLI for smoke execution, benchmarks, OpenAPI validation, SDK generation, and Terraform schema export. |
| `infra/*` | Docker Compose, Helm, Terraform, backup scripts, and operational runbooks. |
| `sdks/*` | Generated SDKs for TypeScript, Python, and Java. |
| `smoke/*` | Critical-path end-to-end scenarios used to validate real platform flows. |
| `benchmarks/*` | Reproducible benchmark scenarios and results. |
| `.github/workflows/*` | CI, release, packaging, security, and docs automation. |

## Workspace Control Files

| File | Purpose |
| --- | --- |
| `go.mod` | Root Go module for services, libraries, and tooling. |
| `go.sum` | Locked Go module checksums used by CI and local builds. |
| `Makefile` | Canonical workspace task runner for tools, generation, build, test, lint, and local CI. |
| `package.json` | Root Node scripts that delegate to the web app. |
| `pnpm-workspace.yaml` | Current pnpm workspace definition for `apps/*`. |
| `justfile` | Compatibility shim over `make`; it should not introduce commands that do not exist in the Makefile. |
| `.gitignore` | Keeps generated local artifacts out of version control while preserving checked-in generated specs. |

## Delivery Surfaces

The repository produces more than one artifact:

- frontend bundles from `apps/web`
- Go binaries from `services/*` and `tools/of-cli`
- Docker images from service-specific Dockerfiles
- generated OpenAPI, SDK, and Terraform schema artifacts
- Helm templates and Terraform modules
- GitHub Pages output from `docs/`

## Where To Look First

- If the change is product UI or navigation related, start in `apps/web/src/routes`.
- If it is API or service behavior, start in the matching folder under `services/`.
- If it affects a shared concern, inspect `libs/` before duplicating logic.
- If it changes public contract shape, inspect `proto/`, generated OpenAPI, and SDK flows together.
- If it changes deployability, inspect `infra/` and the relevant workflow under `.github/workflows/`.
