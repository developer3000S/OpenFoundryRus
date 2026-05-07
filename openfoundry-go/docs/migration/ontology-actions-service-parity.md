# Ontology Actions Service Go Parity Inventory

Date: 2026-05-07

This is a manual parity audit for the 1:1 Rust → Go migration of `ontology-actions-service`. The automated route audit was first corrected and re-run so chi nested routes mounted with `r.Route("/api/v1/ontology", ...)` are compared as their effective `/api/v1/ontology/...` paths instead of false unprefixed paths.

## Sources inspected

- Rust router: `services/ontology-actions-service/src/lib.rs`.
- Go router: `openfoundry-go/services/ontology-actions-service/internal/server/server.go`.
- Go kernel handlers: `openfoundry-go/libs/ontology-kernel/handlers/actions`, `funnel`, `functions`, `rules`, plus `storage` for `/storage/insights`.
- Runtime wiring: `openfoundry-go/services/ontology-actions-service/cmd/ontology-actions-service/main.go`.
- Route audit command: `cd openfoundry-go && go run ./tools/route-audit -services ontology-actions-service`.

## Status vocabulary

- `implemented`: effective route exists and the state-backed Go kernel handler contains real behavior.
- `partial`: effective route exists and substantial behavior is present, but at least one Rust behavior is deferred or incomplete.
- `config-gated`: effective route exists but runtime behavior depends on optional service configuration such as `DATABASE_URL`, Python sidecar, or external service URLs.
- `missing`: Rust route has no effective Go route.

## Route audit result after nested-prefix fix

`tools/route-audit` now reports `Rust routes: 51. Go routes: 52. State counts: implemented: 52.` for `ontology-actions-service`. The extra Go route is `GET /healthz`. There are no Rust routes reported as missing after propagating `/api/v1/ontology` through chi nested router callbacks and router-parameter mount helper calls.

> Note: the route-audit status is intentionally route-shape oriented. The inventory below is the source-of-truth for real behavioral status (`partial` and `config-gated` distinctions).

## Runtime wiring

| Concern | Rust | Go | Status | Data source / runtime | Associated tests |
| --- | --- | --- | --- | --- | --- |
| Protected ontology surface | `Router::new().nest("/api/v1/ontology", protected).layer(auth_layer).with_state(state)` | `r.Route("/api/v1/ontology", func(api chi.Router) { api.Use(authmw.Middleware(jwt)); mount... })` | implemented | JWT middleware, HTTP router | Rust integration tests under `services/ontology-actions-service/tests`; Go `internal/server/server_test.go` |
| Public probes | `GET /health`, `GET /metrics` in `main.rs` | `GET /health`, `GET /healthz`, optional `GET /metrics` with action execution collectors registered into the service registry | implemented | in-process JSON health and Prometheus handler plus ontology action counters/histograms | Go `internal/server/server_test.go`, `libs/ontology-kernel/metrics` tests |
| State-backed handlers | Rust always receives `AppState` | Go now requires a non-nil `AppState` and always mounts ontology-kernel handlers; missing `DATABASE_URL` or Cassandra runtime store config fails startup unless explicit local/test mode is enabled | config-gated | PG via `DATABASE_URL`; Cassandra/Scylla via `CASSANDRA_CONTACT_POINTS` + `CASSANDRA_KEYSPACE`; explicit local/test in-memory state via `OF_DEV_STUB_MODE=true` | Go service tests create an explicit in-memory `AppState` |
| Store wiring | Rust kernel state | `buildState` requires `DATABASE_URL` by default, creates a PG pool, and `buildStores` wires Cassandra object/link/action/read-model stores plus PG definitions; in-memory stores are allowed only in explicit dev/test mode | implemented | PG + Cassandra/Scylla; optional Vespa/OpenSearch search backend | Go `cmd/ontology-actions-service/main_test.go`, `libs/cassandra-kernel` tests, kernel store tests |
| Python runtime | Rust inline Python path | Go wires `python-sidecar` only when `PYTHON_SIDECAR_BINARY` is set | config-gated | Python sidecar | Go `cmd/.../main_test.go`, actions/domain python sidecar integration tests |

## Actions

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /api/v1/ontology/actions` | `list_action_types` | `GET /api/v1/ontology/actions` | `actions.ListActionTypes(state)` | implemented | in-memory `Stores.Definitions` in current service wiring; repository abstraction can back PG/Cassandra when wired | Rust `actions` tests; Go `handlers/actions/actions_test.go` |
| `POST /api/v1/ontology/actions` | `create_action_type` | same | `actions.CreateActionType(state)` | implemented | in-memory `Stores.Definitions`; auth claims | Rust `actions` tests; Go `handlers/actions/actions_test.go` |
| `GET /api/v1/ontology/actions/{id}` | `get_action_type` | same | `actions.GetActionType(state)` | implemented | in-memory `Stores.Definitions` | Rust `actions` tests; Go `handlers/actions/actions_test.go` |
| `PUT /api/v1/ontology/actions/{id}` | `update_action_type` | same | `actions.UpdateActionType(state)` | implemented | in-memory `Stores.Definitions` | Rust `actions` tests; Go `handlers/actions/actions_test.go` |
| `DELETE /api/v1/ontology/actions/{id}` | `delete_action_type` | same | `actions.DeleteActionType(state)` | implemented | in-memory `Stores.Definitions` | Rust `actions` tests; Go `handlers/actions/actions_test.go` |
| `POST /api/v1/ontology/actions/{id}/validate` | `validate_action` | same | `actions.ValidateAction(state)` | full; config-gated for inline Python | definitions/types + object/action/link stores; plans update_object, delete_object, create_link, invoke_webhook, invoke_function HTTP/inline Python, and interface-typed operations without mutating | Rust `actions` tests; Go `handlers/actions/execute_test.go` |
| `POST /api/v1/ontology/actions/{id}/execute` | `execute_action` | same | `actions.ExecuteAction(state)` | full; config-gated for inline Python | object/action/link stores, PG outbox for object writeback, HTTP external for webhook/function actions, Python sidecar for inline Python, interface resolver, action-log attempts and Prometheus counters/histograms for success, validation, authorization, writeback, and execution failures, structured audit POSTs, notification fan-out, and webhook side-effects | Rust `actions` tests; Go `handlers/actions/execute_test.go`, `side_effects_test.go`, `python_sidecar_integration_test.go` |
| `GET /api/v1/ontology/actions/{id}/metrics` | `get_action_metrics` | same | `actions.GetActionMetrics(state)` | implemented | in-memory action log | Rust metrics tests; Go `handlers/actions/actions_test.go` / `metrics.go` coverage |
| `POST /api/v1/ontology/actions/{id}/execute-batch` | `execute_action_batch` | same | `actions.ExecuteActionBatchHandler(state)` | full | same as single execute; per-target partial failures, scale limits, batched HTTP function invocation, side-effects, and action-log attempts present | Rust batch tests; Go `handlers/actions/batch_test.go` |
| `GET /api/v1/ontology/types/{type_id}/applicable-actions` | `list_applicable_actions` | same | `actions.ListApplicableActions(state)` | implemented | in-memory `Stores.Definitions` | Rust applicable-actions tests; Go `handlers/actions/actions_test.go` |

## Inline-edit

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `POST /api/v1/ontology/types/{type_id}/properties/{property_id}/objects/{obj_id}/inline-edit` | `execute_inline_edit` | same | `actions.ExecuteInlineEditHandler(state)` | full | property/action resolution, parameter back-fill, object/action stores, PG/writeback substrate, HTTP external/webhook, Python sidecar when configured, action-log attempts | Rust inline-edit tests; Go `handlers/actions/inline_edit_test.go` |
| `POST /api/v1/ontology/types/{type_id}/inline-edit-batch` | `execute_inline_edit_batch` | same | `actions.ExecuteInlineEditBatchHandler(state)` | full | same as single inline-edit; duplicate-target rejection, per-item validation, partial failures, scale limits, and per-item action-log attempts | Rust inline-edit batch tests; Go `handlers/actions/inline_edit_test.go` |

## What-if

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /api/v1/ontology/actions/{id}/what-if` | `list_action_what_if_branches` | same | `actions.ListActionWhatIfBranches(state)` | implemented | in-memory `Stores.ReadModels` | Rust what-if tests; Go `handlers/actions/actions_test.go` / `whatif.go` coverage |
| `POST /api/v1/ontology/actions/{id}/what-if` | `create_action_what_if_branch` | same | `actions.CreateActionWhatIfBranch(state)` | partial | in-memory `Stores.ReadModels` + definitions; simulation-free branch creation only | Rust what-if tests; Go `handlers/actions/actions_test.go` / `whatif.go` coverage |
| `DELETE /api/v1/ontology/actions/{id}/what-if/{branch_id}` | `delete_action_what_if_branch` | same | `actions.DeleteActionWhatIfBranch(state)` | implemented | in-memory `Stores.ReadModels` | Rust what-if tests; Go `handlers/actions/actions_test.go` / `whatif.go` coverage |

## Uploads

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `POST /api/v1/ontology/actions/uploads` | `upload_action_attachment` | same | `actions.UploadActionAttachment(state)` | implemented | in-memory attachment/upload store abstraction in current service wiring; attachment RID returned | Rust upload tests; Go `handlers/actions/actions_test.go` / `upload.go` coverage |

## Funnel/storage

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /api/v1/ontology/funnel/health` | `funnel::get_funnel_health` | same | `funnel.GetFunnelHealth(state)` | full | PG or DefinitionStore-backed sources plus action-log run metrics | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `GET /api/v1/ontology/storage/insights` | `storage::get_storage_insights` | same | `storage.GetStorageInsights(state)` | full | real object/link/funnel/search metrics from runtime stores, PG metadata when configured, DefinitionStore fallback in explicit in-memory mode, Cassandra index catalogue | Rust storage tests; Go `handlers/storage/storage_test.go` coverage |
| `GET /api/v1/ontology/funnel/sources` | `funnel::list_funnel_sources` | same | `funnel.ListFunnelSources(state)` | full | PG or DefinitionStore fallback; owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `POST /api/v1/ontology/funnel/sources` | `funnel::create_funnel_source` | same | `funnel.CreateFunnelSource(state)` | full | PG or DefinitionStore fallback; source validation | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `GET /api/v1/ontology/funnel/sources/{id}` | `funnel::get_funnel_source` | same | `funnel.GetFunnelSource(state)` | full | PG or DefinitionStore fallback; owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `PATCH /api/v1/ontology/funnel/sources/{id}` | `funnel::update_funnel_source` | same | `funnel.UpdateFunnelSource(state)` | full | PG or DefinitionStore fallback; owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `DELETE /api/v1/ontology/funnel/sources/{id}` | `funnel::delete_funnel_source` | same | `funnel.DeleteFunnelSource(state)` | full | PG or DefinitionStore fallback; owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `GET /api/v1/ontology/funnel/sources/{id}/health` | `funnel::get_funnel_source_health` | same | `funnel.GetFunnelSourceHealth(state)` | full | PG/DefinitionStore source plus action-log run metrics | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `POST /api/v1/ontology/funnel/sources/{id}/run` | `funnel::trigger_funnel_run` | same | `funnel.TriggerFunnelRun(state)` | full | run ledger, optional pipeline trigger, dataset preview, row validation, object writeback/upsert, revision append, source last_run_at | Rust funnel run tests; Go `handlers/funnel/funnel_test.go` |
| `GET /api/v1/ontology/funnel/sources/{id}/runs` | `funnel::list_funnel_runs` | same | `funnel.ListFunnelRuns(state)` | full | action-log run ledger plus source owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |
| `GET /api/v1/ontology/funnel/sources/{source_id}/runs/{run_id}` | `funnel::get_funnel_run` | same | `funnel.GetFunnelRun(state)` | full | action-log run ledger plus source owner/admin filtering | Rust funnel tests; Go `handlers/funnel/funnel_test.go` |

## Functions/media functions

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /api/v1/ontology/functions` | `functions::list_function_packages` | same | `functions.ListFunctionPackages(state)` | implemented | PG | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `POST /api/v1/ontology/functions` | `functions::create_function_package` | same | `functions.CreateFunctionPackage(state)` | implemented | PG | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `GET /api/v1/ontology/functions/authoring-surface` | `functions::get_function_authoring_surface` | same | `functions.GetFunctionAuthoringSurface()` | implemented | static in-process catalog | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `GET /api/v1/ontology/functions/{id}` | `functions::get_function_package` | same | `functions.GetFunctionPackage(state)` | implemented | PG | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `PATCH /api/v1/ontology/functions/{id}` | `functions::update_function_package` | same | `functions.UpdateFunctionPackage(state)` | implemented | PG | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `DELETE /api/v1/ontology/functions/{id}` | `functions::delete_function_package` | same | `functions.DeleteFunctionPackage(state)` | implemented | PG | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `POST /api/v1/ontology/functions/{id}/validate` | `functions::validate_function_package` | same | `functions.ValidateFunctionPackage(state)` | implemented | PG + validator | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `POST /api/v1/ontology/functions/{id}/simulate` | `functions::simulate_function_package` | same | `functions.SimulateFunctionPackage(state)` | config-gated | PG package/object lookup, in-memory object store, Python sidecar for Python inline execution, run ledger in PG | Rust functions tests; Go `handlers/functions/functions_test.go`, domain/function runtime tests |
| `GET /api/v1/ontology/functions/{id}/runs` | `functions::list_function_package_runs` | same | `functions.ListFunctionPackageRuns(state)` | implemented | PG run ledger | Rust functions tests; Go `handlers/functions/functions_test.go` |
| `GET /api/v1/ontology/functions/{id}/metrics` | `functions::get_function_package_metrics` | same | `functions.GetFunctionPackageMetrics(state)` | implemented | PG run ledger aggregation | Rust functions tests; Go `handlers/functions/functions_test.go`, `domain/function_metrics_test.go` |
| No HTTP route (library API) | `media_functions::{read_raw, ocr, extract_text, transcribe_audio, read_metadata}` | No HTTP route (library API) | `internal/mediafunctions` package | implemented | injected media runtime / mock runtime; no PG | Rust `tests/functions_on_media.rs`; Go `internal/mediafunctions/media_test.go` |

## Rules/machinery

| Rust route | Rust handler | Go effective route | Go effective handler | Status | Data source used | Tests |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /api/v1/ontology/rules` | `rules::list_rules` | same | `rules.ListRules(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` and `domain/rules_test.go` |
| `POST /api/v1/ontology/rules` | `rules::create_rule` | same | `rules.CreateRule(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` and `domain/rules_test.go` |
| `GET /api/v1/ontology/rules/insights` | `rules::get_machinery_insights` | same | `rules.GetMachineryInsights(state)` | implemented | PG | Rust machinery tests; Go `handlers/rules` and `domain/rules_test.go` |
| `GET /api/v1/ontology/rules/machinery/queue` | `rules::get_machinery_queue` | same | `rules.GetMachineryQueue(state)` | implemented | PG | Rust machinery tests; Go `handlers/rules` and `domain/rules_test.go` |
| `PATCH /api/v1/ontology/rules/machinery/queue/{id}` | `rules::update_machinery_queue_item` | same | `rules.UpdateMachineryQueueItem(state)` | implemented | PG | Rust machinery tests; Go `handlers/rules` and `domain/rules_test.go` |
| `GET /api/v1/ontology/rules/{id}` | `rules::get_rule` | same | `rules.GetRule(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` and `domain/rules_test.go` |
| `PATCH /api/v1/ontology/rules/{id}` | `rules::update_rule` | same | `rules.UpdateRule(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` and `domain/rules_test.go` |
| `DELETE /api/v1/ontology/rules/{id}` | `rules::delete_rule` | same | `rules.DeleteRule(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` and `domain/rules_test.go` |
| `POST /api/v1/ontology/rules/{id}/simulate` | `rules::simulate_rule` | same | `rules.SimulateRule(state)` | implemented | PG rule catalog + in-memory object/read stores | Rust simulate tests; Go `domain/rules_test.go` |
| `POST /api/v1/ontology/rules/{id}/apply` | `rules::apply_rule` | same | `rules.ApplyRule(state)` | partial | PG rule catalog/queue + object writeback/outbox path; effect writeback depends on object writeback substrate | Rust apply tests; Go `handlers/rules` / `domain/rules_test.go` |
| `GET /api/v1/ontology/types/{type_id}/rules` | `rules::list_rules_for_object_type` | same | `rules.ListRulesForObjectType(state)` | implemented | PG | Rust rules tests; Go `handlers/rules` / `domain/rules_test.go` |
| `GET /api/v1/ontology/objects/{obj_id}/rule-runs` | `rules::list_object_rule_runs` | same | `rules.ListObjectRuleRuns(state)` | implemented | PG | Rust rule-run tests; Go `handlers/rules` / `domain/rules_test.go` |

## Writeback/outbox

| Area | Rust behavior | Go parity assessment | Status | Data source / runtime | Tests |
| --- | --- | --- | --- | --- | --- |
| Object writeback | Apply object mutations and append revisions/outbox entries | Go `handlers/objects` delegates to `domain.ApplyObjectWithOutbox`; update_object, interface create/modify, and function object_patch effects use this path, while delete operations use the ObjectStore delete contract as in Rust | full for action object effects | PG outbox + configured object store | Go `handlers/objects/objects_test.go`, actions execute tests |
| Action side-effects | Webhook writeback, webhook side-effects, notifications, batched execution | Go action side-effects include synchronous webhook writeback, success-only webhook fan-out, notification fan-out, structured audit POSTs, Prometheus/action-log attempt recording for success and all classified failure paths, and side-effect action-log rows | full for action operations | HTTP external, action log, optional notification/audit URLs | Go `handlers/actions/side_effects_test.go`, `execute_test.go` |
| Funnel run writeback | Dataset rows → object writes + revisions | `TriggerFunnelRun` executes source validation, optional pipeline trigger, dataset preview, row validation, object upsert/writeback, revision append, terminal run ledger, and source `last_run_at` update | full | PG/DefinitionStore source metadata, HTTP dataset/pipeline services, object store/writeback, action-log run ledger | Go `handlers/funnel/funnel_test.go` |
| Cassandra writeback/indexes | Rust deployment may rely on Cassandra-backed stores | Go production startup wires Cassandra/Scylla object/link/action/read-model stores when configured and explicit dev mode uses in-memory stores | implemented | Cassandra/Scylla in production; in-memory only for `OF_DEV_STUB_MODE=true` tests/dev | Go `cmd/.../main.go`; `libs/cassandra-kernel` tests |

## Conformance/tests

| Suite | Rust tests | Go tests | Current assessment |
| --- | --- | --- | --- |
| Route shape | `services/ontology-actions-service/src/lib.rs` plus integration tests under `services/ontology-actions-service/tests` | `go test ./tools/route-audit`; `go run ./tools/route-audit -services ontology-actions-service`; `internal/server/server_test.go` | No route-shape gaps after nested-prefix fix. |
| Actions | Rust action handler tests in the service/kernel source | `go test ./libs/ontology-kernel/handlers/actions/...` files: `actions_test.go`, `execute_test.go`, `batch_test.go`, `inline_edit_test.go`, `side_effects_test.go`, Python sidecar integration | Real kernel handlers are mounted; action validate/execute, execute-batch, inline-edit, observability, and best-effort side-effect behavior are complete for object/link/function/interface operation kinds. |
| Funnel/storage | Rust funnel/storage tests | `handlers/funnel/funnel_test.go`, `handlers/storage/storage_test.go`, storage handler/domain tests | Funnel CRUD/run/health and storage insights use real stores and have no remaining 501 gap. |
| Functions/media | Rust function package tests and `tests/functions_on_media.rs` | `handlers/functions/functions_test.go`, `internal/mediafunctions/media_test.go`, function runtime/domain metrics tests | Function CRUD/validate/run reads use persistent package/run records; simulation uses the Python sidecar for Python packages, records success/failure run history, and exposes real run metrics. |
| Rules/machinery | Rust rule/machinery tests | `domain/rules_test.go` and rules handler package tests | Rule catalog and machinery routes are present; apply depends on writeback substrate behavior. |
| Service runtime | Rust binary/integration setup | `go test ./services/ontology-actions-service/...` | Tests create explicit in-memory `AppState`; production startup requires `DATABASE_URL` and optional sidecar/external URLs. |

## Real gaps (not nested-prefix false positives)

1. Cassandra/Scylla runtime stores are required for production startup; operators must provide `CASSANDRA_CONTACT_POINTS` and `CASSANDRA_KEYSPACE` and provision the keyspace replication policy.
2. Inline/function execution is guarded by `PYTHON_PACKAGES_ENABLED`: production startup requires `PYTHON_SIDECAR_BINARY` when Python packages are enabled, while explicit dev/test mode may omit it to preserve the Rust-compatible `python_runtime_not_wired` response.
3. `state == nil` is rejected by the Go router; local/test execution must create an explicit in-memory `AppState`.
