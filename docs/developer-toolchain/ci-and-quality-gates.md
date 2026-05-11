# CI and quality gates

OpenFoundry’s toolchain is backed by strong CI expectations rather than by convention alone.

## Main workflow layers

- Go correctness, generated-code drift, capabilities drift, tests, and dependency policy in `openfoundry-go.yml`
- React/Vite frontend lint, typecheck, unit, E2E, and build in `ci-frontend.yml`
- proto and generated artifact drift in `proto-check.yml`
- Helm, Terraform, SDK, docs, release, and container publication in specialized workflows

## What stands out

The Go CI pipeline does more than compile:

- it runs `go vet ./...`
- it checks `go.mod` / `go.sum` drift after `go mod tidy`
- it lints with `golangci-lint`
- it regenerates proto and sqlc outputs and fails on drift
- it checks the stable capabilities snapshot
- it runs unit and integration tests

That behavior is defined in `.github/workflows/openfoundry-go.yml`. Older references to `ci.yml`, Cargo, or Rust workspace checks are historical and should not be copied into new automation.

## Why this matters

These checks turn architecture assumptions into executable tests:

- database-per-service boundaries
- gateway-to-service wiring
- ontology and workflow readiness
- AI/ML and analytics critical paths
