# connector-management-service Rust → Go parity inventory

Date: 2026-05-08

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

Current route-audit result after this parity slice: 47 Rust routes and 59 Go routes, with **0 Rust routes reported as `missing`**. The audit classifies 48 routes as implemented; remaining 501/config-gated/empty-envelope items are outside this sync-runtime parity slice (dev-auth, optional media/webhook runtime depth, and heuristic Iceberg empty-envelope classification for the synthetic table metadata response). The extra Go routes are existing foundation/read-update helpers (`PATCH /connections/{id}`, `GET/PATCH /data-connection/syncs/{id}`, media-set get/update/run, and the Go virtual-table primitive surface). The audit canonicalizes connector-management Rust routes mounted inside Rust's `/api/v1` closure so the comparison reflects the externally effective HTTP surface.

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




## 2026-05-08 TestConnection adapter dispatch update

This slice closes the `partial` state for the connection test route shape without touching ingestion-replication. Go now carries a shared `ConnectionTestResult` adapter contract and `h.TestConnection` dispatches to a registered adapter when it implements `TestConnection`. Adapter successes and failures both return HTTP 200 with the Rust-compatible `success`, `message`, `latency_ms`, and `details` envelope, and the stored connection status is updated to `connected` or `error`. The production binary wires the Kafka adapter, whose catalog-backed and live broker probes were already ported; connectors without a registered test adapter now produce an explicit failed test result instead of the previous optimistic stub.

## 2026-05-08 background workers update

This slice ports the Rust background-worker primitives for connector-management-service:

- Auto-registration now has a Go `RunOnce`/`RunLoop` worker with Clock/Store interfaces, per-connection `config.auto_registration` parsing, update-detection skip semantics, in-memory last-run status, and the same service-level opt-in gate as Rust: `OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS > 0`.
- Update detection now exposes Rust-compatible `first_seen`, `unknown`, `unchanged`, and `changed` outcomes for discovered-source signatures and records the latest signature after successful auto-registration upserts.
- The legacy sync scheduler has a Clock/Store-backed `RunOnce`/`RunLoop`; the SQL store is intentionally a no-op because the local sync runtime remains disabled pending the ingestion-replication runtime slice. It is gated by `OPENFOUNDRY_SYNC_SCHEDULER_INTERVAL_SECS > 0` and is disabled by default.
- Agent registry resolution now mirrors Rust precedence and validation: inline `agent_url`, then `agent_id` lookup, online status enforcement, and stale-heartbeat rejection using the configured stale-after duration.

## 2026-05-08 encrypted credentials + vending update

This slice ports the Rust credential storage/vending semantics that sit behind the existing route surface:

- `source_credentials` list/upsert now encrypts plaintext values with the Rust-compatible AES-256-GCM envelope: byte `1` version prefix, 12-byte random nonce, then ciphertext+tag. The key derivation remains compatible with Rust: base64 `CREDENTIAL_ENCRYPTION_KEY` must decode to 32 bytes; otherwise the dev fallback is SHA-256 over `openfoundry/credential-encryption/v1\0` plus `JWT_SECRET`.
- Go includes decrypt support and golden tests for the Rust blob layout, malformed-blob errors, unsupported-version errors, and dev-key derivation.
- Iceberg `LoadTable` config now applies the credential-vending helper used by Rust: `expires-at-ms` is always emitted; S3 static/passthrough entries include region, endpoint, path-style, access key, secret key, and session token; Azure/ADLS/OneLake emits account name plus either container-scoped service SAS, account SAS, or a static SAS fallback; GCS emits static OAuth token/project entries.
- Azure SAS generation mirrors Rust's canonical strings, signed version `2022-11-02`, HMAC-SHA256 signing, and conservative percent encoding; tests pin account-SAS and service-SAS golden query strings.

## 2026-05-07 parity close update

This slice replaced the remaining connector-management route-parity placeholders for the requested groups:

- Registration routes now have Go handlers plus repository methods over `connection_registrations`: list, discovery, bulk register, dry-run preview, one-shot auto register, auto-registration config/status, delete, JSON query, and Arrow-stream response. Discovery/query semantics are intentionally adapter-light for now: Go derives selectors from inline `connections.config.tables` or a default source entry until the full Rust connector adapter matrix is ported.
- `test-connection` now returns the Rust-style success/message/latency/details response and updates connection status; it does not yet dispatch into every Rust connector runtime.
- `/api/v1/data-connection/streaming-sources` serves the static streaming-source contract catalog.
- `/api/v1/webhooks/{id}/invoke` loads webhook definitions from connection config, forwards the HTTP call, and returns `status`, `response`, and `output_parameters`.
- `/iceberg/v1/config`, namespaces, tables, and table loading are backed by zero-copy registrations. Load-table returns upstream `metadata_location` when registration metadata carries it, otherwise a Foundry-vended synthetic metadata/config response; in both cases the config map now includes Rust-compatible credential-vending entries.
- Handler tests now cover registration, auto-registration, connection test, webhook invoke, streaming/catalog golden surfaces, and the Iceberg REST catalog group.

Remaining non-parity gaps are connector-runtime depth rather than HTTP route availability: dev-auth remains pending, optional media/webhook runtime depth depends on configured services, and full per-adapter discovery/query/Arrow IPC fidelity remains future work.

## 2026-05-07 CMA-0 update — adapter registry + 4-capability interface

- New package [internal/adapters](../../services/connector-management-service/internal/adapters) defines the per-connector contract that the Rust dispatcher in `services/connector-management-service/src/domain/discovery.rs` plus the `connectors::*` modules expose. The Go interface collapses the four Rust capabilities — `discover_sources`, `query_virtual_table`, `stream_arrow_ipc`, `build_ingest_spec` — onto a single `ConnectorAdapter` interface in [adapter.go](../../services/connector-management-service/internal/adapters/adapter.go).
- `adapters.Registry` ([registry.go](../../services/connector-management-service/internal/adapters/registry.go)) replaces the implicit `match connection.connector_type` arms with a first-class `Register(name, factory) / Get(name) / Lookup(name)` table, so per-connector slices (CMA-2 … CMA-14) can self-register without the dispatcher having to know about them. Factories (`Factory`, `FactoryFunc`, `SingletonFactory`) let stateful adapters (Snowflake driver pools, BigQuery HTTP clients) hold per-instance state without leaking it to callers.
- Sentinels live in [errors.go](../../services/connector-management-service/internal/adapters/errors.go): `ErrNotImplemented` (returned by skeleton stubs and partial adapters; future dispatcher translates it to the existing `"discover is not supported for connector type: …"` / `"zero-copy is not supported for connector type: …"` envelopes Rust emits), `ErrAdapterNotFound`, and `ErrAlreadyRegistered`. `IngestSpec` is defined as a JSON-flavoured envelope to avoid an import cycle with `handlers.IngestJobSpec`; future per-connector slices will marshal through it.
- Tests in [registry_test.go](../../services/connector-management-service/internal/adapters/registry_test.go) cover register/lookup/has/unregister, duplicate rejection, sorted Names, MustRegister panic semantics, FactoryFunc per-call freshness, EmptyArrowStream EOF behaviour, and concurrent register/get under `-race`.
- Nothing on the read or write path is wired to the new registry yet: the existing dispatcher in `internal/domain/discovery/discovery.go` (Discover + QueryVirtualTable) and the `handlers.BuildIngestSpec` shim continue to serve traffic. CMA-2 through CMA-14 swap consumers over.

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
| GET | `/api/v1/data-connection/catalog` | `handlers::catalog::get_connector_catalog` | `/api/v1/data-connection/catalog` | `h.GetConnectorCatalog` | implemented | static connector catalog, connector modules | connector module tests |
| GET | `/api/v1/data-connection/catalog/contracts` | `handlers::catalog::get_connector_contracts` | `/api/v1/data-connection/catalog/contracts` | `h.GetConnectorContracts` | implemented | static connector contracts | connector/contract fixture expectations |
| GET | `/api/v1/data-connection/streaming-sources` | `handlers::streaming_syncs::list_streaming_sources` | `/api/v1/data-connection/streaming-sources` | `h.ListStreamingSources` | implemented | static streaming-source contracts | Kafka/schema-registry tests |

### sources/connections CRUD/test/capabilities

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources` | `handlers::connections::list_connections` | `/api/v1/data-connection/sources` | `h.ListConnections` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/data-connection/sources` | `handlers::connections::create_connection` | `/api/v1/data-connection/sources` | `h.CreateConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| GET | `/api/v1/data-connection/sources/{id}` | `handlers::connections::get_connection` | `/api/v1/data-connection/sources/{id}` | `h.GetConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| DELETE | `/api/v1/data-connection/sources/{id}` | `handlers::connections::delete_connection` | `/api/v1/data-connection/sources/{id}` | `h.DeleteConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/data-connection/sources/{id}/test-connection` | `handlers::connections::test_connection` | `/api/v1/data-connection/sources/{id}/test-connection` | `h.TestConnection` | implemented (adapter-backed for registered test adapters; unsupported connectors return Rust-compatible failed test result) | `connections`; connector adapter modules | connector adapter tests, real-broker/minio/e2e tests |
| GET | `/api/v1/data-connection/sources/{id}/capabilities` | `handlers::catalog::get_connection_capabilities` | `/api/v1/data-connection/sources/{id}/capabilities` | `h.GetConnectionCapabilities` | implemented | `connections`, connector catalog | connector/domain capability tests |

### credentials vending/storage

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/credentials` | `handlers::data_connection::list_credentials` | `/api/v1/data-connection/sources/{id}/credentials` | `h.ListCredentials` | implemented | `source_credentials`; `20260430120000_data_connection_mvp.sql` | Go golden tests mirror `src/credential_crypto.rs` |
| POST | `/api/v1/data-connection/sources/{id}/credentials` | `handlers::data_connection::set_credential` | `/api/v1/data-connection/sources/{id}/credentials` | `h.SetCredential` | implemented | `source_credentials`; `20260430120000_data_connection_mvp.sql` | AES-GCM envelope/key-derivation golden tests; vending golden tests mirror `src/handlers/credentials_vending.rs` |

### egress policies/network boundary

Go now carries Rust-parity source policy binding handlers over `source_policy_bindings` plus a dedicated `internal/domain/egress` helper that mirrors Rust `EgressPolicy::from_state_and_config`, `validate_url`, host allow/block matching, insecure-HTTP blocking, and private-network classification. As in Rust, this helper is a validation gate only: DNS resolution, socket-level controls, connector-agent proxying, and policy catalog ownership remain external network-boundary/runtime responsibilities rather than connector-management enforcement.

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/egress-policies` | `handlers::data_connection::list_source_policies` | `/api/v1/data-connection/sources/{id}/egress-policies` | `h.ListSourcePolicies` | implemented | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |
| POST | `/api/v1/data-connection/sources/{id}/egress-policies` | `handlers::data_connection::attach_policy` | `/api/v1/data-connection/sources/{id}/egress-policies` | `h.AttachPolicy` | implemented | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |
| DELETE | `/api/v1/data-connection/sources/{source_id}/egress-policies/{policy_id}` | `handlers::data_connection::detach_policy` | `/api/v1/data-connection/sources/{source_id}/egress-policies/{policy_id}` | `h.DetachPolicy` | implemented | `source_policy_bindings`; `20260430120000_data_connection_mvp.sql` | `src/domain/egress.rs` |

### sync jobs/runs/runtime dispatch

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/syncs` | `handlers::data_connection::list_syncs` | `/api/v1/data-connection/sources/{id}/syncs` | `h.ListSyncJobs` | partial | `batch_sync_defs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |
| POST | `/api/v1/data-connection/syncs` | `handlers::data_connection::create_sync` | `/api/v1/data-connection/syncs` | `h.CreateSyncJob` | partial | `batch_sync_defs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |
| POST | `/api/v1/data-connection/syncs/{id}/run` | `handlers::data_connection::run_sync` | `/api/v1/data-connection/syncs/{id}/run` | `h.RunSyncJob` | implemented (port-backed dispatch) | `batch_sync_defs`, `sync_runs`; `20260430120000_data_connection_mvp.sql`, `20260430140000_sync_runs_ingest_job_id.sql`, `20260501100000_sync_runs_dataset_version.sql` | `src/domain/dataset_versioning.rs`, `src/ingestion_bridge.rs`, connector integration tests |
| GET | `/api/v1/data-connection/syncs/{id}/runs` | `handlers::data_connection::list_runs` | `/api/v1/data-connection/syncs/{id}/runs` | `h.ListRuns` | implemented (local run ledger) | `sync_runs`; `20260430120000_data_connection_mvp.sql` | dataset versioning/sync tests |

### media-set syncs

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `handlers::media_set_syncs::list_media_set_syncs` | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `h.ListMediaSetSyncs` | partial | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | `tests/media_set_sync_filters.rs`, `src/domain/media_set_sync.rs` |
| POST | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `handlers::media_set_syncs::create_media_set_sync` | `/api/v1/data-connection/sources/{id}/media-set-syncs` | `h.CreateMediaSetSync` | partial | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | `tests/media_set_sync_filters.rs`, `src/domain/media_set_sync.rs` |
| GET/PATCH/POST | n/a | n/a | `/api/v1/data-connection/media-set-syncs/{id}` and `/run` | `h.GetMediaSetSync`, `h.UpdateMediaSetSync`, `h.RunMediaSetSync` | Go extra; run is config-gated | `media_set_syncs`; `20260505100000_media_set_syncs.sql` | Go media runtime tests |

### virtual table registrations/discovery/bulk/auto/status/query/Arrow

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/data-connection/sources/{id}/registrations` | `handlers::registrations::list_registrations` | same | `h.ListRegistrations` | implemented | `connection_registrations`, `virtual_tables` | virtual-table domain/model tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/discover` | `handlers::registrations::discover` | same | `h.DiscoverRegistrations` | partial (inline adapter parity for Rust-supported config catalogs) | connector adapters, `connection_registrations` | discovery/schema inference tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/bulk` | `handlers::registrations::bulk_register` | same | `h.BulkRegister` | implemented | `connection_registrations`, `virtual_tables`, `virtual_table_audit` | registration tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/bulk/preview` | `handlers::registrations::bulk_register_preview` | same | `h.BulkRegisterPreview` | implemented | connector adapters | preview tests |
| POST | `/api/v1/data-connection/sources/{id}/registrations/auto` | `handlers::registrations::auto_register` | same | `h.AutoRegister` | partial | `virtual_table_sources_link`, `auto_register_runs`, `virtual_table_audit` | auto-registration tests |
| PUT | `/api/v1/data-connection/sources/{id}/registrations/auto` | `handlers::registrations::update_auto_registration` | same | `h.UpdateAutoRegistration` | partial | `virtual_table_sources_link` | auto-registration tests |
| GET | `/api/v1/data-connection/sources/{id}/registrations/auto/status` | `handlers::registrations::auto_register_status` | same | `h.AutoRegisterStatus` | partial | `auto_register_runs`, `virtual_table_sources_link` | auto-registration tests |
| DELETE | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}` | `handlers::registrations::delete_registration` | same | `h.DeleteRegistration` | implemented | `connection_registrations`, `virtual_tables`, `virtual_table_audit` | registration tests |
| POST | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}/query` | `handlers::registrations::query_registration` | same | `h.QueryRegistration` | partial (inline sample rows + signatures) | `connection_registrations`, connector adapters | query tests |
| POST | `/api/v1/data-connection/sources/{source_id}/registrations/{registration_id}/query/arrow` | `handlers::registrations::query_registration_arrow` | same | `h.QueryRegistrationArrow` | partial (real Arrow IPC stream, Utf8 columns) | `connection_registrations`, connector adapters, Arrow IPC | Arrow/materialization tests |

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
| GET | `/iceberg/v1/config` | `handlers::iceberg_catalog::get_config` | `/iceberg/v1/config` | `h.IcebergGetConfig` | implemented | `virtual_tables`, `virtual_table_sources_link` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces` | `handlers::iceberg_catalog::list_namespaces` | `/iceberg/v1/namespaces` | `h.IcebergListNamespaces` | implemented | `virtual_tables` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}` | `handlers::iceberg_catalog::get_namespace` | `/iceberg/v1/namespaces/{namespace}` | `h.IcebergGetNamespace` | implemented | `virtual_tables` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}/tables` | `handlers::iceberg_catalog::list_tables` | `/iceberg/v1/namespaces/{namespace}/tables` | `h.IcebergListTables` | implemented | `virtual_tables`, `connection_registrations` | Iceberg catalog/domain tests |
| GET | `/iceberg/v1/namespaces/{namespace}/tables/{table}` | `handlers::iceberg_catalog::load_table` | `/iceberg/v1/namespaces/{namespace}/tables/{table}` | `h.IcebergLoadTable` | partial | `virtual_tables`, `connection_registrations` | Iceberg catalog/domain tests |

### legacy `/connections` aliases

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/connections` | `handlers::connections::list_connections` | `/api/v1/connections` | `h.ListConnections` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/connections` | `handlers::connections::create_connection` | `/api/v1/connections` | `h.CreateConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| GET | `/api/v1/connections/{id}` | `handlers::connections::get_connection` | `/api/v1/connections/{id}` | `h.GetConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| DELETE | `/api/v1/connections/{id}` | `handlers::connections::delete_connection` | `/api/v1/connections/{id}` | `h.DeleteConnection` | implemented | `connections`; `20260419100002_initial_connectors.sql` | connection handler tests |
| POST | `/api/v1/connections/{id}/test` | `handlers::connections::test_connection` | `/api/v1/connections/{id}/test` | `h.TestConnection` | implemented (adapter-backed for registered test adapters; unsupported connectors return Rust-compatible failed test result) | `connections`, connector adapters | connector adapter tests |

### webhooks

| Method | Rust path | Rust handler | Go path | Go handler | State | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| POST | `/api/v1/webhooks/{id}/invoke` | `handlers::webhooks::invoke_webhook` | `/api/v1/webhooks/{id}/invoke` | `h.InvokeWebhook` | implemented | `connections`, sync/runtime target tables depending webhook definition | webhook handler/domain expectations |

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
| Runtime bridges | `http_runtime`, `catalog_bridge`, `open_table_catalog` | utility packages ported (`internal/adapters/{httpruntime,catalogbridge,opentable}`) with egress policy + agent-proxy + inline-catalog parity; per-adapter wrappers still call stdlib client directly rather than routing through `httpruntime.Client` | discovery/query, Iceberg, runtime dispatch | virtual-table/iceberg tests; `egress.rs` tests ported |

### outbox

| Area | Rust implementation | Go parity state | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- |
| Transactional events | `src/outbox.rs` | route surface mounted where relevant; outbox emission pending | `outbox.events`, `outbox.heartbeat`; `20260503010000_outbox.sql` | `src/outbox.rs` |

### background workers

| Worker | Rust implementation | Trigger/config | Go parity state | Tables/migrations | Rust tests |
| --- | --- | --- | --- | --- | --- |
| Auto registration scheduler | `domain::auto_registration::run/tick` | `OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS` > 0 | Clock/Store `RunOnce`/`RunLoop` worker ported; status reads in-memory last run | `virtual_table_sources_link`, `auto_register_runs`; `20260504000121_auto_registration.sql` | auto-registration tests |
| Sync scheduler/runtime | `domain::scheduler::run_scheduler`, `domain::sync_engine::run_due_jobs` | opt-in `OPENFOUNDRY_SYNC_SCHEDULER_INTERVAL_SECS` > 0 | Clock/Store scheduler shim ported; store remains no-op while runtime is disabled | `batch_sync_defs`, `sync_runs` | sync/dataset versioning tests |
| Update detection | `domain::update_detection` and virtual-table counterpart | per-registration/auto-registration settings | registration signature evaluator ported for auto-registration; virtual-table polling remains pending | `update_detection_polls`, `virtual_tables`; `20260504000122_update_detection.sql` | update-detection tests |
| Agent registry resolution | `domain::agent_registry` | connector agent config | resolver ported with inline URL, registry lookup, online/stale validation | `connector_agents`; `20260425153000_enterprise_connectivity.sql` | enterprise connector tests |

### conformance/tests

| Conformance area | Rust source of truth | Go coverage today | Gap |
| --- | --- | --- | --- |
| Route parity | `src/main.rs` | router tests and `tools/route-audit`; audit reports no Rust `missing` routes | Keep route-audit in CI while replacing 501s. |
| HTTP handler contracts | `src/handlers/*.rs` | router tests cover mounted routes, auth-required behavior, and dev-auth env gating | Credentials, egress, sync run listing, full connector adapter dispatch, and dev-auth behavior remain. |
| Persistence migrations | Rust migrations | Go migrations mirror filenames | Need repo methods for all carried tables and outbox writes. |
| Connector behavior | `src/connectors/*.rs`, `src/virtual_table/connectors/*.rs` | contract fixture test and mounted pending endpoints | Need adapter-level unit/integration parity. |
| Runtime dispatch | `ingestion_bridge`, `dataset_versioning`, media-set runtime | Go port-backed sync run dispatch plus media-set HTTP runtime | `run_sync` now materializes deterministic ingestion payloads, dispatches to ingestion-replication through a testable port, persists terminal run state, and records dataset versions/content hashes; remaining work is broader connector adapter coverage and production auth propagation for remote service calls. |
| Background workers | `domain/*` schedulers | none found | Need worker ports, tests, config gates. |

## Prioritized PR/slices to close migration

1. **Done — catalog/contracts/streaming-source slice**: Rust static catalog/contracts plus streaming source contract response shapes and tests are present.
2. **Partial — connection test/capabilities slice**: capability matrix and a non-dispatching `test_connection` response are present; per-connector runtime dispatch remains.
3. **Credential storage/vending slice**: port encrypted `source_credentials` CRUD and vended credential helpers, including key derivation/encryption compatibility tests.
4. **Done — egress policy slice**: source policy binding handlers and Rust-compatible URL/allowlist/private-network validation are ported; network-boundary enforcement remains external by design.
5. **Done — sync runtime slice**: `run_sync` dispatches to ingestion-replication through a port, materializes payloads/content hashes, records dataset versions when available, persists terminal `sync_runs`, and `GET /syncs/{id}/runs` returns the run ledger.
6. **Media-set parity slice**: reconcile Rust-only create/list vs Go extended run/get/update API, then wire runtime config and filter/classification parity tests.
7. **Done/partial — virtual registrations slice**: list/discover/bulk/preview/delete/query/Arrow endpoints and repo methods over `connection_registrations` are present; full adapter-backed query and audit-table writes remain.
8. **Auto-registration/update-detection workers slice**: replace status/update 501s plus scheduler/update-detection workers and config gates.
9. **Done/partial — Iceberg REST Catalog slice**: `/iceberg/v1/*` endpoints expose config/namespaces/table-loading semantics with foundry-vended vs upstream metadata behavior; full credential vending remains.
10. **Done — Webhooks slice**: webhook lookup/invoke flow and side-effect tests are present.
11. **Dev-auth shim slice**: implement `OPENFOUNDRY_DEV_AUTH=1` gated local web-app compatibility behavior.
12. **Connector adapter breadth slice**: port remaining adapters in batches (object/file, DB/warehouse, streaming, SaaS/BI/API, runtime bridges) with integration tests where Rust has real-service coverage.
13. **Outbox/conformance hardening slice**: add transactional outbox emission, route-audit CI assertions, golden JSON fixtures, and end-to-end conformance tests across Rust-compatible paths.
