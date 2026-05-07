# openfoundry-go — root Makefile.
# All commands run from this directory. Targets that require a tool
# verify it via `command -v` and emit a hint if missing.

SHELL          := /usr/bin/env bash
.SHELLFLAGS    := -eu -o pipefail -c
.DEFAULT_GOAL  := help

GO             ?= go
GOFLAGS        ?= -trimpath
PKG            := ./...
BIN_DIR        := bin
export PATH    := $(PWD)/$(BIN_DIR):$(PATH)
COVERAGE_FILE  := coverage.out

SERVICES       := $(notdir $(wildcard services/*))
LIBS           := $(notdir $(wildcard libs/*))

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---------------------------------------------------------------------------
# Toolchain bootstrap
# ---------------------------------------------------------------------------
.PHONY: tools
tools: ## Install pinned dev tools (buf, golangci-lint, sqlc, etc.) into ./bin.
	@mkdir -p $(BIN_DIR)
	GOBIN=$(PWD)/$(BIN_DIR) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.47.2
	GOBIN=$(PWD)/$(BIN_DIR) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2
	GOBIN=$(PWD)/$(BIN_DIR) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0
	GOBIN=$(PWD)/$(BIN_DIR) $(GO) install mvdan.cc/gofumpt@latest
	@echo "Tools installed to $(PWD)/$(BIN_DIR). Add it to your PATH."

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
.PHONY: gen
gen: gen-proto gen-sqlc ## Run all code generators.

.PHONY: gen-proto
gen-proto: ## Generate Go from .proto via buf.
	@command -v buf >/dev/null 2>&1 || { echo "buf not found — run 'make tools'"; exit 1; }
	buf generate

.PHONY: gen-sqlc
gen-sqlc: ## Generate type-safe DB code via sqlc.
	@command -v sqlc >/dev/null 2>&1 || { echo "sqlc not found — run 'make tools'"; exit 1; }
	sqlc generate

# ---------------------------------------------------------------------------
# Build / test / lint
# ---------------------------------------------------------------------------
.PHONY: build
build: ## Build all packages.
	$(GO) build $(GOFLAGS) $(PKG)

.PHONY: build-services
build-services: ## Build every service binary into ./bin.
	@mkdir -p $(BIN_DIR)
	@for s in $(SERVICES); do \
		echo ">>> building $$s"; \
		$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$$s ./services/$$s/cmd/$$s || exit 1; \
	done

.PHONY: test
test: ## Run unit tests with race detector + coverage.
	$(GO) test -race -count=1 -coverprofile=$(COVERAGE_FILE) -covermode=atomic $(PKG)

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker for testcontainers).
	$(GO) test -race -count=1 -tags=integration $(PKG)

.PHONY: cover
cover: test ## Open the HTML coverage report.
	$(GO) tool cover -html=$(COVERAGE_FILE)

.PHONY: lint
lint: ## Run golangci-lint.
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found — run 'make tools'"; exit 1; }
	golangci-lint run --timeout 5m

.PHONY: fmt
fmt: ## Apply gofumpt + gci.
	@command -v gofumpt >/dev/null 2>&1 || { echo "gofumpt not found — run 'make tools'"; exit 1; }
	gofumpt -w .
	$(GO) run github.com/daixiang0/gci@v0.13.5 write -s standard -s default \
		-s 'prefix(github.com/openfoundry/openfoundry-go)' .

.PHONY: tidy
tidy: ## Run `go mod tidy`.
	$(GO) mod tidy

.PHONY: vet
vet: ## Run `go vet`.
	$(GO) vet $(PKG)

# ---------------------------------------------------------------------------
# Composite gates
# ---------------------------------------------------------------------------
.PHONY: ci
ci: tidy vet lint test ## Run the full CI gate locally.

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf $(BIN_DIR) $(COVERAGE_FILE) coverage.html
