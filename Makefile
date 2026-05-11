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
OPENAPI_SPEC   := apps/web/public/generated/openapi/openfoundry.json
TS_SDK_DIR     := sdks/typescript/openfoundry-sdk
PY_SDK_DIR     := sdks/python/openfoundry-sdk
JAVA_SDK_DIR   := sdks/java/openfoundry-sdk

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
gen: gen-proto gen-sqlc contracts-gen ## Run all code generators.

.PHONY: gen-proto
gen-proto: ## Generate Go from .proto via buf.
	@command -v buf >/dev/null 2>&1 || { echo "buf not found — run 'make tools'"; exit 1; }
	buf generate

.PHONY: gen-sqlc
gen-sqlc: ## Generate type-safe DB code via sqlc.
	@command -v sqlc >/dev/null 2>&1 || { echo "sqlc not found — run 'make tools'"; exit 1; }
	sqlc generate

.PHONY: contracts-gen
contracts-gen: openapi-gen sdk-gen ## Regenerate OpenAPI and SDK contract artifacts.

.PHONY: contracts-check
contracts-check: openapi-check sdk-check ## Fail if generated OpenAPI or SDK artifacts are out of date.

.PHONY: openapi-gen
openapi-gen: ## Generate the committed OpenAPI JSON from proto contracts.
	$(GO) run ./tools/of-cli docs generate-openapi --proto-dir proto --output $(OPENAPI_SPEC)

.PHONY: openapi-check
openapi-check: ## Fail if the committed OpenAPI JSON is out of date.
	$(GO) run ./tools/of-cli docs validate-openapi --proto-dir proto --expected $(OPENAPI_SPEC)

.PHONY: sdk-gen
sdk-gen: sdk-typescript-gen sdk-python-gen sdk-java-gen ## Regenerate every generated SDK.

.PHONY: sdk-check
sdk-check: sdk-typescript-check sdk-python-check sdk-java-check ## Fail if any generated SDK is out of date.

.PHONY: sdk-typescript-gen
sdk-typescript-gen: ## Generate the TypeScript SDK from the committed OpenAPI JSON.
	$(GO) run ./tools/of-cli docs generate-sdk-typescript --input $(OPENAPI_SPEC) --output $(TS_SDK_DIR)

.PHONY: sdk-typescript-check
sdk-typescript-check: ## Fail if the TypeScript SDK is out of date.
	$(GO) run ./tools/of-cli docs validate-sdk-typescript --input $(OPENAPI_SPEC) --output $(TS_SDK_DIR)

.PHONY: sdk-python-gen
sdk-python-gen: ## Generate the Python SDK from the committed OpenAPI JSON.
	$(GO) run ./tools/of-cli docs generate-sdk-python --input $(OPENAPI_SPEC) --output $(PY_SDK_DIR)

.PHONY: sdk-python-check
sdk-python-check: ## Fail if the Python SDK is out of date.
	$(GO) run ./tools/of-cli docs validate-sdk-python --input $(OPENAPI_SPEC) --output $(PY_SDK_DIR)

.PHONY: sdk-java-gen
sdk-java-gen: ## Generate the Java SDK from the committed OpenAPI JSON.
	$(GO) run ./tools/of-cli docs generate-sdk-java --input $(OPENAPI_SPEC) --output $(JAVA_SDK_DIR)

.PHONY: sdk-java-check
sdk-java-check: ## Fail if the Java SDK is out of date.
	$(GO) run ./tools/of-cli docs validate-sdk-java --input $(OPENAPI_SPEC) --output $(JAVA_SDK_DIR)

.PHONY: capabilities-snapshot
capabilities-snapshot: ## Regenerate docs/agent-automation/stable-capabilities.json.
	$(GO) run ./tools/capabilities-snapshot

.PHONY: capabilities-check
capabilities-check: ## Fail if the stable-capabilities snapshot is out of date.
	$(GO) run ./tools/capabilities-snapshot -check

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
ci: tidy vet lint contracts-check test ## Run the full CI gate locally.

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf $(BIN_DIR) $(COVERAGE_FILE) coverage.html

# ---------------------------------------------------------------------------
# GitOps (ArgoCD)
# ---------------------------------------------------------------------------
GITOPS_ENV ?= dev

.PHONY: gitops-bootstrap
gitops-bootstrap: ## Install ArgoCD + register the app-of-apps for $$GITOPS_ENV (default dev).
	./infra/scripts/argocd-bootstrap.sh $(GITOPS_ENV)

.PHONY: gitops-status
gitops-status: ## Show every Application + ApplicationSet managed by ArgoCD.
	@command -v kubectl >/dev/null 2>&1 || { echo "kubectl not found"; exit 1; }
	@echo ">>> Applications:"
	@kubectl -n argocd get applications -o wide || true
	@echo
	@echo ">>> ApplicationSets:"
	@kubectl -n argocd get applicationsets -o wide || true

.PHONY: gitops-sync
gitops-sync: ## Force-sync every Application (refresh + sync). Useful after a Git push.
	@command -v kubectl >/dev/null 2>&1 || { echo "kubectl not found"; exit 1; }
	@for app in $$(kubectl -n argocd get applications -o name); do \
		echo ">>> refreshing $$app"; \
		kubectl -n argocd annotate $$app argocd.argoproj.io/refresh=hard --overwrite >/dev/null; \
	done
	@echo "Refresh requested. Watch progress with: make gitops-status"

.PHONY: gitops-ui
gitops-ui: ## Port-forward the ArgoCD UI to https://localhost:8080.
	@echo ">>> ArgoCD UI: https://localhost:8080"
	@echo ">>> admin password: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
	kubectl -n argocd port-forward svc/argocd-server 8080:443

.PHONY: gitops-uninstall
gitops-uninstall: ## Remove ArgoCD + the app-of-apps. Does NOT delete the workloads ArgoCD synced.
	-kubectl -n argocd delete application openfoundry-root --wait=false 2>/dev/null
	-kubectl -n argocd delete applicationsets --all --wait=false 2>/dev/null
	-kubectl -n argocd delete applications --all --wait=false 2>/dev/null
	-kubectl -n argocd delete appproject openfoundry --wait=false 2>/dev/null
	-helm -n argocd uninstall argocd 2>/dev/null
	-kubectl delete namespace argocd --wait=false 2>/dev/null
	@echo "ArgoCD removed. Workloads it deployed are still in the cluster (use helm/kubectl to clean them)."
