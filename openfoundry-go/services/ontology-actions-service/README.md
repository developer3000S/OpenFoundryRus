# `ontology-actions-service` (Go)

Sole runtime owner of the consolidated ontology bounded contexts per
[ADR-0030](../../../docs/architecture/adr/ADR-0030-service-consolidation-30-targets.md):

- **actions** — `/api/v1/ontology/actions/*` and inline-edit
- **funnel** — `/api/v1/ontology/funnel/*` and `/storage/insights`
- **functions** — `/api/v1/ontology/functions/*` plus the Foundry
  "Functions on objects → Media" runtime ([`internal/mediafunctions`](internal/mediafunctions/media.go))
- **rules** — `/api/v1/ontology/rules/*`, `/types/{id}/rules`,
  `/objects/{id}/rule-runs`

## Port status

| Component | Status |
|---|---|
| Health (`/health`, `/healthz`) + Prometheus (`/metrics`) | ✅ wired |
| JWT-protected `/api/v1/ontology/*` mount | ✅ wired |
| URL grid (every Rust route mounted with the same path / verb) | ✅ wired |
| `mediafunctions` package (`read_raw`, `ocr`, `extract_text`, `transcribe_audio`, `read_metadata` + `MockRuntime`) | ✅ ported 1:1 with Rust H6 tests |
| Action / funnel / function / rule handlers | ✅ mounted from `libs/ontology-kernel/handlers/*` whenever the service has an `AppState` |
| Missing `DATABASE_URL` or Cassandra runtime stores | ✅ fails startup by default with a clear error |
| Object/link/action hot-path stores | ✅ wired to Cassandra/Scylla via `libs/cassandra-kernel` |
| Definition store | ✅ wired to PostgreSQL via `domain.PostgresDefinitionStore` |
| Read-model store | ✅ wired to Cassandra/Scylla generic read models |
| Search backend | ✅ Vespa/OpenSearch when `SEARCH_ENDPOINT` is configured; otherwise nil fallback path |
| Explicit local/test in-memory mode | ✅ opt-in with `OF_DEV_STUB_MODE=true` |

The same in-process integration tests the Rust crate ships with
(`tests/health.rs::list_action_types_requires_bearer_token` and
`absorbed_routes_require_bearer_token`) are mirrored under
[`internal/server/server_test.go`](internal/server/server_test.go), using an
explicit in-memory `AppState`.

## Build & run

```sh
go build -o bin/ontology-actions-service ./services/ontology-actions-service/cmd/ontology-actions-service
go test ./services/ontology-actions-service/...
```

## Configuration

Env vars (defaults match the Rust port):

| Variable | Default |
|---|---|
| `HOST` | `0.0.0.0` |
| `PORT` | `50106` |
| `JWT_SECRET` | (required) |
| `DATABASE_URL` | required unless `OF_DEV_STUB_MODE=true` |
| `OF_DEV_STUB_MODE` | `false` |
| `CASSANDRA_CONTACT_POINTS` | required unless `OF_DEV_STUB_MODE=true` |
| `CASSANDRA_KEYSPACE` | required unless `OF_DEV_STUB_MODE=true` |
| `CASSANDRA_USERNAME` | empty |
| `CASSANDRA_PASSWORD` | empty |
| `AUDIT_SERVICE_URL` | `http://localhost:50115` |
| `DATASET_SERVICE_URL` | `http://localhost:50079` |
| `ONTOLOGY_SERVICE_URL` | `http://localhost:50103` |
| `PIPELINE_SERVICE_URL` | `http://localhost:50081` |
| `AI_SERVICE_URL` | `http://localhost:50127` |
| `NOTIFICATION_SERVICE_URL` | `http://localhost:50114` |
| `CONNECTOR_MANAGEMENT_SERVICE_URL` | `http://localhost:50130` |
| `SEARCH_EMBEDDING_PROVIDER` | `deterministic-hash` |
| `SEARCH_BACKEND` | unset (`vespa` is assumed when `SEARCH_ENDPOINT` is set without a backend) |
| `SEARCH_ENDPOINT` | unset (search store remains nil and object-set loaders use store fallback paths) |
| `SEARCH_AUTH_HEADER` | unset |
| `SEARCH_API_KEY` | unset (`Bearer <token>` helper for `SEARCH_AUTH_HEADER`) |
| `NODE_RUNTIME_COMMAND` | `node` |
| `PYTHON_PACKAGES_ENABLED` | `false` (set `true` in production when Python function packages are enabled) |
| `PYTHON_SIDECAR_BINARY` | required when `PYTHON_PACKAGES_ENABLED=true` outside `OF_DEV_STUB_MODE` |
| `PYTHON_SIDECAR_ARGS` | unset (shell-style extra arguments appended after the managed `--bind <socket>` flags) |
| `PYTHON_SIDECAR_ENV` | unset (comma/newline-separated `KEY=VALUE` entries appended to sidecar env) |
| `PYTHON_SIDECAR_TIMEOUT` | `15s` (Go duration or integer seconds for startup and hard-call timeout) |

### Python function runtime

When deployments allow inline Python function packages, set `PYTHON_PACKAGES_ENABLED=true` and provide `PYTHON_SIDECAR_BINARY` pointing at the deployed `openfoundry-pyruntime` binary. Startup fails in production if Python packages are enabled without a sidecar, turning `python_runtime_not_wired` into a deployment configuration error instead of a runtime 503. Local/test runs using `OF_DEV_STUB_MODE=true` may intentionally omit the sidecar to exercise the Rust-compatible `python_runtime_not_wired` response.

Use `PYTHON_SIDECAR_ARGS` for additional sidecar flags, `PYTHON_SIDECAR_ENV` for comma/newline-separated `KEY=VALUE` environment entries, and `PYTHON_SIDECAR_TIMEOUT` for both sidecar startup and hard-call timeout.

## Cassandra/Scylla bootstrap

Production startup expects the `CASSANDRA_KEYSPACE` keyspace to already exist
with the operator-selected replication policy. On startup the Go service applies
idempotent table migrations from `libs/cassandra-kernel.OntologyRuntimeMigrations`
for:

- `objects_by_id`, `objects_by_type`, `objects_by_owner`, `objects_by_marking`
- `links_outgoing`, `links_incoming`
- `actions_log`, `actions_by_object`, `actions_by_action`, `actions_by_event`
- `schemas_by_type`, `schemas_latest`
- `read_models`, `read_models_by_parent`

The action-log table shapes are adapted from
`services/ontology-actions-service/cql/actions_log/*.cql`; the keyspace name is
configurable through `CASSANDRA_KEYSPACE` so deployments can colocate the runtime
tables under their provisioned ontology keyspace.
