# connector-management-service Rust → Go parity inventory

Date: 2026-05-07

Scope:

- Rust source: `services/connector-management-service/`
- Go target: `openfoundry-go/services/connector-management-service/`
- Rust route root: `services/connector-management-service/src/main.rs`
- Go route root: `openfoundry-go/services/connector-management-service/internal/server/server.go`
- Go foundation inspected: `internal/handlers/handlers.go`, `internal/repo/repo.go`, `internal/handlers/media_runtime.go`

Generated route baseline:

```sh
cd openfoundry-go && go run ./tools/route-audit -services connector-management-service
```

Current route-audit result after the route-surface slice: 47 Rust routes and 58 Go routes, with **0 Rust routes reported as `missing`**. The extra Go routes are existing foundation/read-update helpers (`PATCH /connections/{id}`, `GET/PATCH /data-connection/syncs/{id}`, media-set get/update/run, and the Go virtual-table primitive surface). The audit canonicalizes connector-management Rust routes mounted inside Rust's `/api/v1` closure so the comparison reflects the externally effective HTTP surface.

## Status vocabulary

- `implemented`: the effective Go route and handler exist and persist/read data with a real repository implementation.
- `partial`: the Go route exists but does not yet preserve the full Rust contract, response shape, side effects, or runtime dispatch semantics.
- `501`: route is mounted for Rust HTTP parity and returns a machine-readable pending error with HTTP 501.
- `503` / `config-gated`: route is mounted but depends on optional runtime/config wiring.
- `runtime-pending`: persistence route exists but external runtime/bridge/catalog side effects from Rust are not implemented yet.

## Shared tables and migrations

| Area | Tables | Migrations |
| --- | --- | --- |
| Connection CRUD/catalog ownership | `connections`, legacy `sync_jobs` | `20260419100002_initial_connectors.sql`, `20260424201000_sync_jobs_runtime.sql`, `20260503120000_drop_sync_jobs_runtime.sql` |
| Enterprise connectivity/agents/registrations | `connector_agents`, `connection_registrations` | `20260425153000_enterprise_connectivity.sql` |
| Credentials, egress bindings, batch syncs/runs | `source_credentials`, `source_policy_bindings`, `batch_sync_defs`, `sync_runs` | `20260430120000_data_connection_mvp.sql`, `20260430140000_sync_runs_ingest_job_id.sql`, `20260501100000_sync_runs_dataset_version.sql` |
| Outbox | `outbox.events`, `outbox.heartbeat` | `20260503010000_outbox.sql` |
| Virtual tables | `virtual_table_sources_link`, `virtual_tables`, `virtual_table_imports`, `virtual_table_audit` | `20260504000120_virtual_tables_init.sql` |
| Auto registration | `virtual_table_sources_link` auto-register columns, `auto_register_runs` | `20260504000121_auto_registration.sql` |
| Update detection | `update_detection_polls`, `virtual_tables` update-detection columns | `20260504000122_update_detection.sql` |
| Media-set syncs | `media_set_syncs`, `batch_sync_defs.sync_kind` | `20260505100000_media_set_syncs.sql` |

## Auth and temporary-handler policy

Go now mirrors Rust's global optional-auth shape for `/api/v1` and `/iceberg/v1`: anonymous requests pass through middleware, and handlers that require claims enforce auth internally. Catalog/read bring-up routes that are open in Rust remain open in Go and return 501 until implemented. Mutating or user-scoped pending handlers require claims first, then return 501. Media-set runtime execution remains `503`/`config-gated` when `MediaSetRuntime` is not wired. Dev-auth routes mount only when `OPENFOUNDRY_DEV_AUTH=1`.

Machine-readable pending errors use this shape:

```json
{"error":"<code>","code":"<code>","message":"route mounted for Rust parity; implementation pending"}
```

## Rust test corpus inspected

- Connector/runtime integration: `tests/kafka_real_broker.rs`, `tests/postgres_cdc_e2e.rs`, `tests/s3_minio.rs`, `tests/schema_registry_compat.rs`.
- Media-set filters: `tests/media_set_sync_filters.rs`.
- Metrics: `src/metrics.rs` tests.
- Credentials: `src/credential_crypto.rs`, `src/handlers/credentials_vending.rs` tests.
- Egress/domain: `src/domain/egress.rs` tests.
- Dataset versioning/runtime dispatch: `src/domain/dataset_versioning.rs`, `src/ingestion_bridge.rs` tests.
- Virtual table/domain: tests under `src/virtual_table/domain/*`, `src/virtual_table/models/*`, and mirrored tests in `src/domain/*` where present.
- Connector adapters: unit tests in connector modules such as `src/connectors/parquet.rs`, `src/connectors/kafka.rs`, `src/connectors/bigquery.rs`, `src/connectors/postgres.rs`, and virtual-table connector modules.

## Route parity by domain

### health/metrics

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/health` | inline `|| async { "ok" }` | `/health` | inline health handler | implemented | none | none found |
| GET | `/healthz` | n/a | `/healthz` | inline healthz handler | implemented (Go extra) | none | Go router tests |
| GET | `/metrics` | `metrics_handler` | `/metrics` | `m.Handler()` | implemented | none | `src/metrics.rs` |

### Data Connection catalog/contracts/streaming sources

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/catalog` | `handlers::catalog::get_connector_catalog` | `/api/v1/data-connection/catalog` | `h.GetConnectorCatalog` | 501 | static connector catalog, connector modules | connector module tests |
| GET | `/api/v1/data-connection/catalog/contracts` | `handlers::catalog::get_connector_contracts` | `/api/v1/data-connection/catalog/contracts` | `h.GetConnectorContracts` | 501 | static connector contracts | connector/contract fixture expectations |
| GET | `/api/v1/data-connection/streaming-sources` | `handlers::streaming_syncs::list_streaming_sources` | `/api/v1/data-connection/streaming-sources` | `h.ListStreamingSources` | 501 | static streaming-source contracts | Kafka/schema-registry tests |

### sources/connections CRUD/test/capabilities

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources` | `handlers::connections::list_connections` | `/api/v1/data-connection/sources` | `h.ListConnections` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/data-connection/sources` | `handlers::connections::create_connection` | `/api/v1/data-connection/sources` | `h.CreateConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| GET | `/api/v1/data-connection/sources/{id}` | `handlers::connections::get_connection` | `/api/v1/data-connection/sources/{id}` | `h.GetConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| DELETE | `/api/v1/data-connection/sources/{id}` | `handlers::connections::delete_connection` | `/api/v1/data-connection/sources/{id}` | `h.DeleteConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/data-connection/sources/{id}/test-connection` | `handlers::connections::test_connection` | `/api/v1/data-connection/sources/{id}/test-connection` | `h.TestConnection` | 501 | `connections`; connector adapter modules | connector adapter tests, real-broker/minio/e2e tests |
| GET | `/api/v1/data-connection/sources/{id}/capabilities` | `handlers::catalog::get_connection_capabilities` | `/api/v1/data-connection/sources/{id}/capabilities` | `h.GetConnectionCapabilities` | 501 | `connections`, connector catalog | connector/domain capability tests |

### credentials vending/storage

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/credentials` | `handlers::data_connection::list_credentials` | `/api/v1/data-connection/sources/{id}/credentials` | `h.ListCredentials` | 501 | `source_credentials`; `20260430120000_data_connection_mvp.sql` | `src/credential_crypto.rs`, `src/handlers/credentials_vending.rs` |
| POST | `/api/v1/data-connection/sources/{id}/credentials` | `handlers::data_connection::set_credential` | `/api/v1/data-connection/sources/{id}/credentials` | `h.SetCredential` | 501 | `source_credentials`; `20260430120000_data_connection_mvp.sql` | `src/credential_crypto.rs`, `src/handlers/credentials_vending.rs` |

### egress policies/network boundary

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/egress-policies` | `handlers::data_connection::list_source_policies` | `/api/v1/data-connection/sources/{id}/egress-policies` | `h.ListSourcePolicies` | 501 | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |
| POST | `/api/v1/data-connection/sources/{id}/egress-policies` | `handlers::data_connection::attach_policy` | `/api/v1/data-connection/sources/{id}/egress-policies` | `h.AttachPolicy` | 501 | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |
| DELETE | `/api/v1/data-connection/sources/{source_id}/egress-policies/{policy_id}` | `handlers::data_connection::detach_policy` | `/api/v1/data-connection/sources/{source_id}/egress-policies/{policy_id}` | `h.DetachPolicy` | 501 | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |

### sync jobs/runs/runtime dispatch

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/syncs` | `handlers::data_connection::list_syncs` | `/api/v1/data-connection/sources/{id}/syncs` | `h.ListSyncJobs` | partial | `batch_sync_defs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |
| POST | `/api/v1/data-connection/syncs` | `handlers::data_connection::create_sync` | `/api/v1/data-connection/syncs` | `h.CreateSyncJob` | partial | `batch_sync_defs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |
| POST | `/api/v1/data-connection/syncs/{id}/run` | `handlers::data_connection::run_sync` | `/api/v1/data-connection/syncs/{id}/run` | `h.RunSyncJob` | runtime-pending | `batch_sync_defs`, `sync_runs`; `20260430120000_data_connection_mvp.sql`, `20260430140000_sync_runs_ingest_job_id.sql`, `20260501100000_sync_runs_dataset_version.sql` | `src/domain/dataset_versioning.rs`, `src/ingestion_bridge.rs`, connector integration tests |
| GET | `/api/v1/data-connection/syncs/{id}/runs` | `handlers::data_connection::list_runs` | `/api/v1/data-connection/syncs/{id}/runs` | `h.ListRuns` | 501 | `sync_runs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |

### media-set syncs

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `handlers::media_set_syncs::list_media_set_syncs` | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `h.ListMediaSetSyncs` | partial | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | `tests/media_set_sync_filters.rs`, `src/domain/media_set_sync.rs` |
| POST | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `handlers::media_set_syncs::create_media_set_sync` | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `h.CreateMediaSetSync` | partial | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | `tests/media_set_sync_filters.rs`, `src/domain/media_set_sync.rs` |
| GET/PATCH/POST | n/a | n/a | `/api/v1/data-connection/media-set-syncs/{id}` and `/run` | `h.GetMediaSetSync`, `h.UpdateMediaSetSync`, `h.RunMediaSetSync` | Go extra; run is config-gated | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | Go media runtime tests |

### virtual table registrations/discovery/bulk/auto/status/query/Arrow

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/registrations` | `handlers::registrations::list_registrations` | same | `h.ListRegistrations` | 501 | `connection_registrations`, `virtual_tables` | virtual-table domain/model tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/discover` | `handlers::registrations::discover` | same | `h.DiscoverRegistrations` | 501 | connector adapters, `connection_registrations` | discovery/schema inference tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/bulk` | `handlers::registrations::bulk_register` | same | `h.BulkRegister` | 501 | `connection_registrations`, `virtual_tables`, `virtual_table_audit` | registration tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/bulk/preview` | `handlers::registrations::bulk_register_preview` | same | `h.BulkRegisterPreview` | 501 | connector adapters | preview tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/auto` | `handlers::registrations::auto_register` | same | `h.AutoRegister` | 501 | `virtual_table_sources_link`, `auto_register_runs`, `virtual_table_audit` | auto-registration tests |
| PUT | `/api/v1/data-connection/sources/{id}/registrations/auto` | `handlers::registrations::update_auto_registration` | same | `h.UpdateAutoRegistration` | 501 | `virtual_table_sources_link` | auto-registration tests |
| GET | `/api/v1/data-connection/sources/{id}/registrations/auto/status` | `handlers::registrations::auto_register_status` | same | `h.AutoRegisterStatus` | 501 | `auto_register_runs`, `virtual_table_sources_link` | auto-registration tests |
| DELETE | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}` | `handlers::registrations::delete_registration` | same | `h.DeleteRegistration` | 501 | `connection_registrations`, `virtual_tables`, `virtual_table_audit` | registration tests |
| POST | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}/query` | `handlers::registrations::query_registration` | same | `h.QueryRegistration` | 501 | `connection_registrations`, connector adapters | query tests |
| POST | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}/query/arrow` | `handlers::registrations::query_registration_arrow` | same | `h.QueryRegistrationArrow` | 501 | `connection_registrations`, connector adapters, Arrow IPC | Arrow/materialization tests |

### virtual table source enable/list/get/create

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| n/a | no Rust route in `main.rs` | n/a | `/api/v1/virtual-table/sources/{source_rid}/enable` | `h.EnableVirtualTableSource` | implemented (Go extra) | `virtual_table_sources_link`; `20260504000120_virtual_tables_init.sql` | virtual-table source/model tests |
| n/a | no Rust route in `main.rs` | n/a | `/api/v1/virtual-table/sources/{source_rid}/virtual-tables` | `h.CreateVirtualTable` | implemented (Go extra) | `virtual_tables`, `virtual_table_audit`; `20260504000120_virtual_tables_init.sql` | virtual-table source/model tests |
| n/a | no Rust route in `main.rs` | n/a | `/api/v1/virtual-tables` | `h.ListVirtualTables` | implemented (Go extra) | `virtual_tables`; `20260504000120_virtual_tables_init.sql` | virtual-table source/model tests |
| n/a | no Rust route in `main.rs` | n/a | `/api/v1/virtual-tables/{rid}` | `h.GetVirtualTable` | implemented (Go extra) | `virtual_tables`; `20260504000120_virtual_tables_init.sql` | virtual-table source/model tests |

### Iceberg REST Catalog

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/iceberg/v1/config` | `handlers::iceberg_catalog::get_config` | `/iceberg/v1/config` | `h.IcebergGetConfig` | 501 | `virtual_tables`, `virtual_table_sources_link` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces` | `handlers::iceberg_catalog::list_namespaces` | `/iceberg/v1/namespaces` | `h.IcebergListNamespaces` | 501 | `virtual_tables` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}` | `handlers::iceberg_catalog::get_namespace` | `/iceberg/v1/namespaces/{namespace}` | `h.IcebergGetNamespace` | 501 | `virtual_tables` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}/tables` | `handlers::iceberg_catalog::list_tables` | `/iceberg/v1/namespaces/{namespace}/tables` | `h.IcebergListTables` | 501 | `virtual_tables`, `connection_registrations` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}/tables/{table}` | `handlers::iceberg_catalog::load_table` | `/iceberg/v1/namespaces/{namespace}/tables/{table}` | `h.IcebergLoadTable` | 501 | `virtual_tables`, `connection_registrations` | Iceberg catalog/domain tests |

### legacy `/connections` aliases

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/connections` | `handlers::connections::list_connections` | `/api/v1/connections` | `h.ListConnections` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/connections` | `handlers::connections::create_connection` | `/api/v1/connections` | `h.CreateConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| GET | `/api/v1/connections/{id}` | `handlers::connections::get_connection` | `/api/v1/connections/{id}` | `h.GetConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| DELETE | `/api/v1/connections/{id}` | `handlers::connections::delete_connection` | `/api/v1/connections/{id}` | `h.DeleteConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/connections/{id}/test` | `handlers::connections::test_connection` | `/api/v1/connections/{id}/test` | `h.TestConnection` | 501 | `connections`, connector adapters | connector adapter tests |

### webhooks

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/webhooks/{id}/invoke` | `handlers::webhooks::invoke_webhook` | `/api/v1/webhooks/{id}/invoke` | `h.InvokeWebhook` | 501 | `connections`, sync/runtime target tables depending webhook definition | webhook handler/domain expectations |

### dev-auth shim

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/auth/login` | `handlers::dev_auth::login` | `/api/v1/auth/login` | `h.DevAuthLogin` | config-gated + 501 | none | dev auth handler expectations |
| POST | `/api/v1/auth/refresh` | `handlers::dev_auth::refresh` | `/api/v1/auth/refresh` | `h.DevAuthRefresh` | config-gated + 501 | none | dev auth handler expectations |
| GET | `/api/v1/auth/bootstrap-status` | `handlers::dev_auth::bootstrap_status` | `/api/v1/auth/bootstrap-status` | `h.DevAuthBootstrapStatus` | config-gated + 501 | none | dev auth handler expectations |
| GET | `/api/v1/users/me` | `handlers::dev_auth::me` | `/api/v1/users/me` | `h.DevAuthMe` | config-gated + 501 | none | dev auth handler expectations |

### connector adapters

No Rust routes are mounted directly under adapter modules, but Rust request handlers delegate to adapters for catalog, capabilities, connection testing, discovery, virtual-table query, Arrow materialization, sync payloads, and credentials vending.

| Adapter area | Rust implementation | Go parity state | Related routes | Rust tests |
| --- | --- | --- | --- | --- |
| Object/file sources | `s3`, `gcs`, `azure_blob`, `onelake`, `sftp`, `parquet`, `csv`, `json`, `excel` | routes mounted; adapter logic pending | catalog, test-connection, syncs, registrations/query | minio/media-set/filter and connector tests |
| Databases/warehouses | `postgres`, `mysql`, `mssql`, `oracle`, `jdbc`, `odbc`, `bigquery`, `snowflake`, `databricks` | routes mounted; adapter logic pending | catalog, test-connection, discovery, query, Iceberg | Postgres CDC/e2e and connector tests |
| Streaming | `kafka`, `kinesis`, schema registry support | routes mounted; adapter logic pending | streaming-sources, test-connection, sync runtime | Kafka real broker/schema registry tests |
| SaaS/BI/API | `salesforce`, `sap`, `rest_api`, `graphql`, `power_bi`, `tableau`, `iot`, `ldap`, `generic` | routes mounted; adapter logic pending | catalog, contracts, test-connection, discovery/query | connector tests |
| Runtime bridges | `http_runtime`, `catalog_bridge`, `open_table_catalog` | routes mounted; runtime bridge logic pending | discovery/query, Iceberg, runtime dispatch | virtual-table/iceberg tests |

### outbox

| Area | Rust implementation | Go parity state | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- |
| Transactional events | `src/outbox.rs` | route surface mounted where relevant; outbox emission pending | `outbox.events`, `outbox.heartbeat`; `20260503010000_outbox.sql` | `src/outbox.rs` |

### background workers

| Worker | Rust implementation | Trigger/config | Go parity state | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- |
| Auto registration scheduler | `domain::auto_registration::run/tick` | `OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS` > 0 | routes mounted; worker pending | `virtual_table_sources_link`, `auto_register_runs`; `20260504000121_auto_registration.sql` | auto-registration tests |
| Sync scheduler/runtime | `domain::scheduler::run_scheduler`, `domain::sync_engine::run_due_jobs` | service/runtime config | route surface partial/runtime-pending | `batch_sync_defs`, `sync_runs` | sync/dataset versioning tests |
| Update detection | `domain::update_detection` and virtual-table counterpart | virtual table polling settings | pending | `update_detection_polls`, `virtual_tables`; `20260504000122_update_detection.sql` | update-detection tests |
| Agent registry resolution | `domain::agent_registry` | connector agent config | pending | `connector_agents`; `20260425153000_enterprise_connectivity.sql` | enterprise connector tests |

### conformance/tests

| Conformance area | Rust source of truth | Go coverage today | Gap |
| --- | --- | --- | --- |
| Route parity | `src/main.rs` | router tests and `tools/route-audit`; audit reports no Rust `missing` routes | Keep route-audit in CI while replacing 501s. |
| HTTP handler contracts | `src/handlers/*.rs` | router tests cover mounted routes, auth-required behavior, and dev-auth env gating | Implement catalog, credentials, egress, test-connection, capabilities, registrations, Iceberg, webhook, dev-auth behavior. |
| Persistence migrations | Rust migrations | Go migrations mirror filenames | Need repo methods for all carried tables and outbox writes. |
| Connector behavior | `src/connectors/*.rs`, `src/virtual_table/connectors/*.rs` | contract fixture test and mounted pending endpoints | Need adapter-level unit/integration parity. |
| Runtime dispatch | `ingestion_bridge`, `dataset_versioning`, media-set runtime | Go DB-only sync run plus media-set HTTP runtime | Need ingestion-replication dispatch, dataset version recording, run listing/status semantics. |
| Background workers | `domain/*` schedulers | none found | Need worker ports, tests, config gates. |

## Prioritized PR/slices to close migration

1. **Catalog/contracts/streaming-source slice**: replace 501 handlers with Rust static catalog/contracts plus streaming source contract response shapes and tests.
2. **Connection test/capabilities slice**: port connector capability matrix, `test_connection` dispatch, core validation, and adapter unit tests for highest-use connectors.
3. **Credential storage/vending slice**: port encrypted `source_credentials` CRUD and vended credential helpers, including key derivation/encryption compatibility tests.
4. **Egress policy slice**: port source policy binding handlers and domain URL/allowlist/private-network validation; keep network-boundary ownership external.
5. **Sync runtime slice**: complete `run_sync` parity by dispatching to ingestion-replication, materializing payloads, recording dataset versions/content hashes, and implementing `GET /syncs/{id}/runs`.
6. **Media-set parity slice**: reconcile Rust-only create/list vs Go extended run/get/update API, then wire runtime config and filter/classification parity tests.
7. **Virtual registrations slice**: replace registration 501s with list/discover/bulk/preview/delete/query/Arrow endpoints and repo methods over `connection_registrations`/`virtual_tables`/audit tables.
8. **Auto-registration/update-detection workers slice**: replace status/update 501s plus scheduler/update-detection workers and config gates.
9. **Iceberg REST Catalog slice**: replace `/iceberg/v1/*` 501s with config/namespaces/table-loading semantics, including foundry-vended vs upstream metadata behavior.
10. **Webhooks slice**: port webhook lookup/invoke flow and side-effect tests.
11. **Dev-auth shim slice**: implement `OPENFOUNDRY_DEV_AUTH=1` gated local web-app compatibility behavior.
12. **Connector adapter breadth slice**: port remaining adapters in batches (object/file, DB/warehouse, streaming, SaaS/BI/API, runtime bridges) with integration tests where Rust has real-service coverage.
13. **Outbox/conformance hardening slice**: add transactional outbox emission, route-audit CI assertions, golden JSON fixtures, and end-to-end conformance tests across Rust-compatible paths.
