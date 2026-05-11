# Quality Gates

OpenFoundry uses CI as an executable compatibility contract across Go services, the React frontend, generated artifacts, infra packaging, and SDK outputs.

## Workflow Inventory

| Workflow | Purpose | Typical Local Entry Point |
| --- | --- | --- |
| `openfoundry-go.yml` | Go hygiene, generated proto/sqlc drift, capabilities drift, unit tests, and integration tests. | `make tools`, `make ci`, `make capabilities-check`, `make test-integration` |
| `ci-frontend.yml` | React/Vite lint, TypeScript checks, unit tests, E2E, and production build. | `pnpm --filter @open-foundry/web lint`, `pnpm --filter @open-foundry/web check`, `pnpm --filter @open-foundry/web test:unit`, `pnpm --filter @open-foundry/web build` |
| `proto-check.yml` | Buf lint plus OpenAPI and SDK validation. | `buf lint proto`, `make contracts-check` |
| `helm-check.yml` / `helm-lint.yml` | Helm lint and render validation across deployment overlays. | Check the workflow path before running locally; active Helm sources live under `infra/helm/`. |
| `terraform-check.yml` | Terraform format plus module and schema validation. | `terraform fmt -check -recursive infra/terraform` |
| `sdk-smoke.yml` | Compiles and imports generated SDKs outside the main generation workflow. | Follow the per-language commands in the workflow and SDK folders. |
| `security-audit.yml` | Go vulnerability scanning. | `govulncheck ./...` when `govulncheck` is installed. |
| `docker-publish.yml` | Builds and pushes selected service images to GHCR. | Verify the service has a `services/<name>/Dockerfile` before mirroring the matrix locally. |
| `release.yml` | Generates tagged GitHub releases and changelog entries. | Git tag push flow |
| `deploy-docs.yml` | Builds VitePress docs and deploys them to GitHub Pages. | `cd docs && npm ci && npm run docs:build` |

## Executable Architecture Through Smoke Tests

The smoke suites are especially important because they validate feature chains rather than isolated units:

- `p2-runtime-critical-path.json` covers connection, dataset, pipeline, query, streaming, report, and geospatial runtime flows.
- `p3-semantic-governance-critical-path.json` covers ontology and governance-oriented semantics.
- `p4-developer-platform-critical-path.json` covers code repository and platform-builder flows.
- `p5-ai-ml-critical-path.json` covers provider-backed AI and ML paths.
- `p6-analytics-enterprise-critical-path.json` covers enterprise analytics and geospatial scenarios.

When you modify cross-cutting behavior, smoke scenarios and generated contract checks are often the first places that will tell you whether the overall platform contract still holds.

## What To Watch During Review

- Contract changes can require OpenAPI, SDK, or frontend updates.
- Infra changes can break Helm, Terraform, or smoke setup even when unit tests pass.
- Service changes may need database, environment, or Compose updates outside the service folder itself.
- Docs changes should keep navigation, edit links, and Pages deployment in sync.
