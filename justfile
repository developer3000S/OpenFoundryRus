# OpenFoundry — `just` shim over the Makefile.
#
# The Makefile is the canonical task runner for this Go monorepo. This
# justfile exists only so users with `just` muscle memory don't get
# stuck. Every recipe delegates to `make <target>`.
#
# Previously this file ran `cargo` against the deleted Rust workspace.
# If you need that history, see git log before commit 2ec24d3.
#
# Run `just` with no args to list recipes; `just --evaluate` to inspect
# variables.

set dotenv-load := true

# ── Default ──────────────────────────────────────────────────────────

default:
    @just --list

# ── Bootstrap ────────────────────────────────────────────────────────

# Install pinned dev tools (buf, golangci-lint, sqlc, gofumpt) into ./bin.
tools:
    make tools

# ── Build ────────────────────────────────────────────────────────────

# Build all Go packages.
build:
    make build

# Produce one binary per service into ./bin/.
build-services:
    make build-services

# ── Test ─────────────────────────────────────────────────────────────

# Fast unit tests (no Docker, race detector + coverage).
test:
    make test

# Integration tests (requires Docker for testcontainers).
test-integration:
    make test-integration

# Open the HTML coverage report from the last `just test` run.
cover:
    make cover

# ── Code generation ──────────────────────────────────────────────────

# Run all generators (proto + sqlc).
gen:
    make gen

gen-proto:
    make gen-proto

gen-sqlc:
    make gen-sqlc

# ── Lint / format / hygiene ──────────────────────────────────────────

# Run golangci-lint with the project config.
lint:
    make lint

# Apply gofumpt + gci.
fmt:
    make fmt

# `go mod tidy`.
tidy:
    make tidy

# `go vet`.
vet:
    make vet

# ── Composite gates ──────────────────────────────────────────────────

# Full local CI gate (tidy + vet + lint + test). Run before pushing.
ci:
    make ci

clean:
    make clean

# ── GitOps (ArgoCD) ──────────────────────────────────────────────────

gitops-bootstrap:
    make gitops-bootstrap

gitops-status:
    make gitops-status

gitops-sync:
    make gitops-sync

gitops-ui:
    make gitops-ui

gitops-uninstall:
    make gitops-uninstall
