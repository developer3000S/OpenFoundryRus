# Contributing to OpenFoundry

OpenFoundry is a Go monorepo (with a React frontend, generated SDKs and
infra packaging). This document covers how to propose changes; AI agents
working on the codebase should also read [`CLAUDE.md`](CLAUDE.md) for
the canonical commands, conventions and gotchas.

> **TL;DR**
>
> 1. Open an issue for anything non-trivial.
> 2. Branch from `main`, follow [Conventional Commits](https://www.conventionalcommits.org/).
> 3. Run `make ci` locally before pushing.
> 4. Keep PRs small (< ~400 lines diff), focused, and with tests.
> 5. Changes to `libs/core-models`, `libs/auth-middleware`, `proto/**`
>    or any public SDK require an **RFC** (see below).

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Ways to contribute](#ways-to-contribute)
- [Project layout](#project-layout)
- [Development environment](#development-environment)
- [Workflow](#workflow)
- [Branch and commit conventions](#branch-and-commit-conventions)
- [Pull request checklist](#pull-request-checklist)
- [Review process](#review-process)
- [Tests and quality gates](#tests-and-quality-gates)
- [RFCs and breaking changes](#rfcs-and-breaking-changes)
- [Adding a new service](#adding-a-new-service)
- [Documentation contributions](#documentation-contributions)
- [Security issues](#security-issues)
- [Licensing and DCO](#licensing-and-dco)
- [Getting help](#getting-help)

## Code of conduct

Participation is governed by our Code of Conduct (Contributor Covenant
2.1). Report issues to `conduct@openfoundry.dev`.

## Ways to contribute

- **Bug reports** — use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml).
- **Feature proposals** — use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml).
- **New service proposals** — use the [new service template](.github/ISSUE_TEMPLATE/new_service.yml).
- **Documentation** improvements under [`docs/`](docs/).
- **SDK examples** in [`sdks/`](sdks/).
- **Plugins** built on top of [`libs/plugin-sdk`](libs/plugin-sdk).
- **Triage**: reproducing bugs, labelling issues, reviewing PRs.

Filter issues by [`good first issue`](../../labels/good%20first%20issue) or
[`help wanted`](../../labels/help%20wanted) to find a starting point.

## Project layout

| Path | Purpose |
|------|---------|
| [`services/`](services/) | One Go binary per microservice (`cmd/<svc>/main.go`). Copy from [`services/template/`](services/template/) when adding a new one. |
| [`libs/`](libs/) | Shared Go packages (auth, observability, kernels, storage abstractions). |
| [`apps/web/`](apps/web/) | React 19 + Vite + TypeScript frontend. |
| [`proto/`](proto/) | Protobuf contracts; source of truth for RPC and SDKs. |
| [`sdks/`](sdks/) | Generated SDKs (TypeScript, Python, Java). |
| [`infra/`](infra/) | Helm charts, ArgoCD, Terraform, runbooks. |
| [`docs/`](docs/) | VitePress documentation site. |
| [`docs/archive/`](docs/archive/) | Historical migration logs — **do not load by default**. |
| [`tools/`](tools/) | CLIs (`of-cli`, `route-audit`, lint helpers). |
| [`benchmarks/`](benchmarks/), [`smoke/`](smoke/) | Performance and end-to-end scenarios. |

Repository map: [`docs/guide/repository-map.md`](docs/guide/repository-map.md).
Architecture overview: [`docs/architecture/index.md`](docs/architecture/index.md).

## Development environment

**Required tooling**

- Go (version pinned by `go.mod`).
- Node 20+ and `pnpm` 9+ (`corepack enable`) for the frontend.
- Docker / Docker Compose (only needed for integration tests via testcontainers).
- `buf` (installed by `make tools`).

**First-time setup**

```bash
git clone https://github.com/diocrafts/openfoundry.git
cd openfoundry
make tools          # installs buf, golangci-lint, sqlc, gofumpt to ./bin
make build          # compile every Go package
make test           # fast unit tests (no Docker needed)
```

The local-development guide lives in
[`docs/getting-started/`](docs/getting-started/).

**Useful targets** (see [`Makefile`](Makefile) for the full list)

```bash
make build-services    # one binary per service into ./bin/
make test              # unit tests with race detector + coverage
make test-integration  # tests behind //go:build integration (needs Docker)
make lint              # golangci-lint with the project config
make fmt               # gofumpt + gci
make gen               # regen proto Go + sqlc
make ci                # tidy + vet + lint + test  (full local CI gate)
```

A `justfile` is provided as a thin shim over `make` for users with
`just` muscle memory, but the Makefile is canonical.

## Workflow

1. **Search existing issues / discussions** to avoid duplication.
2. **Open an issue** for anything non-trivial. Get rough agreement on
   the approach before writing code.
3. **Fork** and create a branch from `main`:
   `git checkout -b feat/ontology-bulk-import`.
4. **Implement** the change, adding tests and updating docs.
5. **Run quality gates locally**: `make ci`.
6. **Push** and open a pull request against `main`.
7. **Iterate** with reviewers; keep the branch rebased on `main`.
8. A maintainer **squash-merges** the PR once approved and CI is green.

We do **not** force-push to shared branches and we do **not** rebase
merge commits in `main`.

## Branch and commit conventions

- **Branch names**: `<type>/<short-description>` (e.g.
  `feat/nexus-bulk-export`, `fix/auth-jwt-leeway`,
  `docs/contributing-guide`).
- **PR title / final squash commit**:
  [Conventional Commits](https://www.conventionalcommits.org/),
  enforced by the `contribution-policy` CI workflow; drives the changelog.

  ```
  feat(ontology): add bulk import endpoint
  fix(auth-middleware): tolerate clock skew up to 60s
  docs(getting-started): document make tools
  refactor(core-models): split dataset types into own module
  chore(deps): bump opentelemetry to v1.39
  ```

  Allowed types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`,
  `build`, `ci`, `chore`, `revert`. A `!` after the scope
  (`feat(api)!: ...`) or a `BREAKING CHANGE:` footer flags a breaking
  change.

## Pull request checklist

- [ ] Linked to an issue (`Closes #123`) when applicable.
- [ ] Diff stays focused; unrelated changes split into separate PRs.
- [ ] `make ci` passes locally.
- [ ] New / changed behaviour covered by tests (unit, integration, or smoke).
- [ ] Public APIs (proto, SDK, REST) updated together with their generated
      artefacts (`make gen`, plus the SDK generators in `tools/of-cli`).
- [ ] Docs in [`docs/`](docs/) updated when behaviour or interfaces change.
- [ ] An entry added to the **Unreleased** section of [`CHANGELOG.md`](CHANGELOG.md)
      for any user-visible change.
- [ ] Migrations are **forward-only**; once a migration ships it is
      immutable, add a new one rather than editing the old.
- [ ] No secrets, credentials, or customer data committed.

PRs that fail any of the above will be sent back without a review round.

## Review process

- **Routing**: GitHub auto-assigns reviewers based on
  [`.github/CODEOWNERS`](.github/CODEOWNERS). At least **one CODEOWNER
  approval** is required for the touched paths.
- **SLA**: triage within 3 business days; first review within 7 business
  days. Ping the PR if you have not heard back after that window.
- **Merge strategy**: squash-merge with the PR title used as the commit
  message (must follow Conventional Commits).
- **Stale PRs** with no contributor activity for 30 days are auto-labelled
  `stale` and closed after 14 more days. Reopen freely when you have time.

## Tests and quality gates

CI runs on every PR (see [`.github/workflows/`](.github/workflows/)):

- `make ci` — `go mod tidy`, `go vet`, `golangci-lint`, unit tests with
  race detector and coverage.
- `buf lint` and `buf breaking` against `main`.
- `make test-integration` for changes that touch DB or message-bus paths
  (uses testcontainers; requires Docker on the runner).
- Frontend lint, typecheck, unit and E2E tests for `apps/web` changes.
- Helm and Terraform validation for `infra/` changes.

A PR cannot be merged with red CI. If a check is genuinely flaky,
document it in the PR and ping a maintainer; do **not** disable it.

`golangci-lint` is configured with `new-from-rev: HEAD` so it only
flags issues introduced by your commits — the existing baseline is
silenced. To audit the full backlog locally:
`golangci-lint run --new-from-rev= ./...`.

## RFCs and breaking changes

Some changes need a written design before implementation:

- Any change to the **public API surface** (`proto/**`, generated SDKs,
  REST routes documented in OpenAPI).
- Any change to **`libs/core-models`** or **`libs/auth-middleware`**
  types re-exported by services.
- Introducing a new **cross-cutting library** under [`libs/`](libs/).
- Adding or removing a **service** under [`services/`](services/).
- Changes to the **storage schema** that require coordinated migrations
  across more than one service.

Process:

1. Open an issue using the `RFC: <title>` prefix and the `kind/rfc`
   label.
2. Fill the RFC template (context, decision, alternatives,
   consequences, migration plan).
3. Allow **7 days minimum** for community comments.
4. A maintainer marks the RFC as **accepted**, **rejected**, or
   **needs-revision**. Accepted RFCs land as a numbered ADR under
   [`docs/architecture/adr/`](docs/architecture/adr/) and can then be
   implemented.

Breaking proto changes must additionally:

- Bump the package version (`open_foundry.<domain>.v1` → `v2`).
- Keep the previous version compiling for at least **one minor release**.
- Be flagged with `!` and a `BREAKING CHANGE:` footer.

## Adding a new service

We try to keep services consistent. Before adding a new one:

1. Open a `new service` issue and get approval from a Platform CODEOWNER.
2. Copy [`services/template/`](services/template/) as the starting
   point — it ships the `cmd/<svc>/main.go`, `internal/server`,
   `internal/config` and `internal/handler/health` skeletons plus a
   distroless `Dockerfile`.
3. Register the service in:
   - [`infra/helm/apps/`](infra/helm/apps/) — chart that ships it.
   - [`infra/argocd/apps/`](infra/argocd/apps/) — GitOps app.
   - [`services/edge-gateway-service/internal/proxy/router_table.go`](services/edge-gateway-service/internal/proxy/router_table.go) —
     external HTTP routing (only if the service receives external traffic).
   - [`.github/CODEOWNERS`](.github/CODEOWNERS) — ownership.
4. Add proto definitions under [`proto/<domain>/v1/`](proto/) and run
   `make gen`.
5. Wire `/healthz`, `/metrics` and structured logging via
   [`libs/observability`](libs/observability/), and a smoke scenario under
   [`smoke/scenarios/`](smoke/scenarios/).

## Documentation contributions

- The public site lives in [`docs/`](docs/) and is built with VitePress.
- Reference docs for SDKs are generated; **edit the proto / Go
  doc-comments**, not the generated output.
- Per-module agent-facing notes go in `CLAUDE.md` files inside the
  module directory.

## Security issues

**Do not file security vulnerabilities as public issues.** Follow the
disclosure process in [`SECURITY.md`](SECURITY.md).

## Licensing and DCO

- OpenFoundry is licensed under **AGPL-3.0-only** (see [`LICENSE`](LICENSE)).
- By contributing you agree your contribution is licensed under the
  same terms.
- All non-merge commits must be **signed off** with the
  [Developer Certificate of Origin](https://developercertificate.org/):

  ```bash
  git commit -s -m "feat(scope): your message"
  ```

  The `contribution-policy` CI workflow rejects PRs whose commits are not
  signed off.

## Getting help

- **Documentation**: <https://diocrafts.github.io/OpenFoundry/>
- **Discussions / Q&A**: GitHub Discussions on this repository.
- **Maintainer ping**: mention `@open-foundry/maintainers` on your
  issue or PR.
