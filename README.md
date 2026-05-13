<div align="center">
  <a href="https://github.com/openfoundry/openfoundry-go">
    <img src="images/logo.png" alt="OpenFoundry" width="240" />
  </a>


  **The Open-Source Data Operating System**

  An open, cloud-native operational data platform for building data products with datasets, ontologies, applications, AI/ML, governance, and observability from one monorepo.

  [![Go CI](https://github.com/openfoundry/openfoundry-go/actions/workflows/openfoundry-go.yml/badge.svg)](https://github.com/openfoundry/openfoundry-go/actions/workflows/openfoundry-go.yml)
  [![Frontend CI](https://github.com/openfoundry/openfoundry-go/actions/workflows/ci-frontend.yml/badge.svg)](https://github.com/openfoundry/openfoundry-go/actions/workflows/ci-frontend.yml)
  [![Proto Check](https://github.com/openfoundry/openfoundry-go/actions/workflows/proto-check.yml/badge.svg)](https://github.com/openfoundry/openfoundry-go/actions/workflows/proto-check.yml)
  [![License: AGPL-3.0-only](https://img.shields.io/badge/License-AGPL--3.0--only-blue.svg)](LICENSE)

  [Documentation](docs/) · [Architecture](ARCHITECTURE.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Issues](https://github.com/openfoundry/openfoundry-go/issues)
</div>

---

OpenFoundry is an open-source operational data platform inspired by the capability model of Palantir Foundry, implemented as auditable, extensible software. It combines **42 Go microservices**, **33 shared libraries**, Protobuf/OpenAPI contracts, generated SDKs, a **React 19 + Vite + TypeScript** web console, and declarative infrastructure for Kubernetes.

The goal is to provide a reproducible foundation for teams that need to connect sources, version datasets, model an ontology, expose APIs, automate workflows, govern access, and operate analytical or AI workloads with end-to-end traceability.

> **Working with this codebase as an AI agent?** Start at [`CLAUDE.md`](CLAUDE.md). It is the canonical onboarding guide for commands, conventions, security-critical zones, and what not to read by default.

## Features & Status

- **Cloud-native architecture:** small Go services, one entrypoint per service, and delivery through Helm, ArgoCD, and Terraform.
- **Ontology at the core:** object types, actions, functions, object views, lineage, and stable contracts for building applications on operational data.
- **Contracts first:** Protobuf as the source of truth, generated OpenAPI, and synchronized TypeScript, Python, and Java SDKs.
- **Integrated governance:** authentication, authorization, Cedar policies, audit, tenancy, SSO/MFA, and egress controls.
- **Observability by default:** `/healthz`, `/metrics`, Prometheus, Grafana, Mimir, structured logs, and OTel traces.
- **Developer platform:** CLI tooling, SDK generation, service templates, VitePress docs, and unit/integration test paths.

| Capability | Status | Capability | Status |
| :-- | :-- | :-- | :-- |
| **Datasets & versioning** | ✅ Available | **Ontology services** | ✅ Available |
| **React web console** | ✅ Available | **Generated SDKs** | ✅ Available |
| **Protobuf/OpenAPI contracts** | ✅ Available | **AuthN/AuthZ foundations** | ✅ Available |
| **Observability stack** | ✅ Available | **Helm/ArgoCD delivery** | ✅ Available |
| **Kafka/NATS integrations** | ✅ Available | **Lakehouse/Iceberg paths** | Under active development |
| **AI/agent runtime services** | Under active development | **Production hardening** | In progress |

## OpenFoundry vs closed data platforms

| Area | OpenFoundry | Closed platforms |
| :-- | :-- | :-- |
| **Control** | Auditable code, contracts, and infrastructure in one monorepo. | Strong provider dependency and less implementation visibility. |
| **Extensibility** | Services, libraries, SDKs, and docs can evolve with your needs. | Extensions are limited by external APIs and vendor roadmaps. |
| **Deployment** | Kubernetes, Helm, ArgoCD, Terraform, and Compose for reproducible environments. | Usually SaaS or managed deployments with less operational control. |
| **Governance** | Policies, audit, and tenancy live beside the platform code. | Governance is coupled to the product and its commercial boundaries. |
| **Developer workflow** | Standard Go, TypeScript, Python, Java, Protobuf, and Makefile workflows. | Proprietary tooling or local workflows that are harder to automate. |

## Quickstart

### 1. Clone the repository

```sh
git clone https://github.com/openfoundry/openfoundry-go.git
cd openfoundry-go
```

### 2. Install development tools

```sh
make tools
```

This installs the Go tools used by the monorepo into `./bin`, including `buf`, `golangci-lint`, `sqlc`, and `gofumpt`.

### 3. Run the main local gate

```sh
make ci
```

`make ci` runs tidy, vet, lint, contract checks, and unit tests. For faster iteration, use:

```sh
make test
make build
make contracts-check
```

### 4. Start the frontend

```sh
pnpm install
pnpm --filter @open-foundry/web dev
```

The web application lives in [`apps/web/`](apps/web/) and uses React 19, Vite, and TypeScript.

### 5. Start local dependencies with Compose

```sh
docker compose -f infra/compose/docker-compose.yml up -d
```

For development, there is also:

```sh
docker compose -f infra/compose/docker-compose.dev.yml up -d
```

### 6. Deploy to Kubernetes

```sh
make gitops-bootstrap
make gitops-status
```

Delivery assets live in [`infra/`](infra/): Helm charts, ArgoCD apps, Terraform, Compose, and operational runbooks.

## Repository layout

```text
openfoundry-go/
├── apps/web/         React 19 + Vite + TypeScript frontend
├── services/         Go microservices; copy services/template/ for new services
├── libs/             Shared Go packages for auth, observability, kernels and more
├── proto/            Protobuf source of truth; Go generated into libs/proto-gen/
├── sdks/             Generated TypeScript, Python and Java SDKs
├── infra/            Helm, ArgoCD, Terraform, Compose and operational runbooks
├── docs/             VitePress capability-oriented documentation site
├── tools/            CLIs and lint/helper tools
├── images/           Project branding assets, including this README logo
├── go.mod            Single Go module for the entire monorepo
└── Makefile          Canonical local task runner
```

Per-service shape:

```text
services/<svc>/
  cmd/<svc>/main.go          entrypoint
  internal/server/           chi router (/healthz, /metrics, /api)
  internal/handlers/         HTTP handlers
  internal/domain/           pure logic
  internal/repo/             data access, sqlc-generated when relevant
  internal/repo/migrations/  goose-style SQL migrations
  internal/models/           wire types
  internal/config/           koanf-backed config
```

## Why a single Go module?

OpenFoundry uses a single Go module (`go.mod` at the root) instead of a multi-module `go.work` setup because it:

- Keeps `libs/` and `services/` synchronized without version drift.
- Simplifies dependency resolution, caching, and builds.
- Makes contract generation and compatibility tests more direct.
- Follows a familiar pattern for large infrastructure monorepos.

Splitting specific services into their own modules remains possible if the project needs it.

## Day-to-day commands

Run these commands from the repository root:

```sh
make help              # list available targets
make tools             # install tools into ./bin
make ci                # tidy + vet + lint + contracts-check + test
make test              # unit tests with race detector and coverage
make test-integration  # tests with the integration build tag; requires Docker
make gen               # regenerate proto Go, sqlc, OpenAPI, and SDKs
make contracts-check   # verify OpenAPI and SDK drift
make build             # compile all packages
make build-services    # compile one binary per service into ./bin/
make lint              # golangci-lint
make fmt               # gofumpt + gci
```

Frontend:

```sh
pnpm --filter @open-foundry/web dev
pnpm --filter @open-foundry/web check
pnpm --filter @open-foundry/web test
```

## Documentation

- [`docs/`](docs/) — capability-oriented technical documentation.
- [`docs/index.md`](docs/index.md) — VitePress site entrypoint.
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — high-level architecture overview.
- [`docs/architecture/adr/`](docs/architecture/adr/) — dated architectural decisions.
- [`CLAUDE.md`](CLAUDE.md) — concise onboarding for AI agents.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — PR process, RFC requirements, and DCO policy.
- [`SECURITY.md`](SECURITY.md) — how to report vulnerabilities.

## Wire-compatibility invariants

Some contracts are pinned by golden tests and must not change without an explicit migration:

- `/healthz` payload shape (`status`, `service`, `version`, `timestamp`).
- JWT claim names and JSON tags.
- Dataset RID format: `ri.foundry.main.dataset.<uuid-v7>`.
- Transaction state/type tokens: `open|committed|aborted` and `snapshot|append|update|delete`.
- Marking source and schema field type discriminators.
- Media reference camelCase keys (`mediaSetRid`, `mediaItemRid`, `branch`, `schema`).

## Getting help

- Open a bug report or feature request in [GitHub Issues](https://github.com/openfoundry/openfoundry-go/issues).
- Review the documentation in [`docs/`](docs/) before changing services or contracts.
- For new capabilities, start from [`services/template/`](services/template/) and the existing ADRs.

## Contributing

Contributions are welcome. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the PR process, RFC requirements, and DCO policy. Security reports follow [`SECURITY.md`](SECURITY.md).

## License

OpenFoundry is licensed under **AGPL-3.0-only**. See [`LICENSE`](LICENSE).


