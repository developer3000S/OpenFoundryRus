# `notebook-runtime-service` (Go)

Notebook + notepad runtime: notebooks, cells, sessions, kernel execution,
workspace files, notepad documents + presence + export.

## Port status

| Component | Status |
|---|---|
| Health (`/health`, `/healthz`) + Prometheus (`/metrics`) | ✅ wired |
| URL grid (every Rust route mounted under `/api/v1`) | ✅ wired |
| Notebook CRUD | ✅ pgx-backed in production; explicit `NOTEBOOK_RUNTIME_SMOKE_MODE=true` enables in-memory smoke CRUD without `DATABASE_URL` |
| Cell CRUD | ✅ pgx-backed in production; explicit smoke mode persists cells in memory for smoke tests |
| Session CRUD | ✅ pgx-backed in production; explicit smoke mode persists session lifecycle in memory |
| Workspace files | ✅ ported through `internal/domain/environment` with path-normalisation/traversal tests |
| Notepad document CRUD | ✅ repository-backed via Postgres with in-memory no-DB fallback |
| Notepad presence | ✅ repository-backed via Postgres with in-memory no-DB fallback |
| Notepad export | ✅ wired through `internal/domain/notepad` HTML rendering |
| Cell execute (Python) | ✅ wired through `libs/python-sidecar` when `PYTHON_SIDECAR_BINARY` is set; unset config returns an explicit `python kernel sidecar is not configured` execution error rather than a placeholder envelope |
| Cell execute (SQL / R / LLM) | ✅ adapters are wired: SQL posts to query-service, R shells out to `Rscript`, LLM posts to ai-service chat completions and tracks session conversation IDs |

## Smoke mode contract

`NOTEBOOK_RUNTIME_SMOKE_MODE=true` is the only documented mode where notebook,
cell, and session routes may operate without a database. In smoke mode, the Go
service uses an in-memory repository so CRUD routes return concrete resources
instead of empty envelopes. With `DATABASE_URL` unset and smoke mode disabled,
notebook/cell/session CRUD returns `503` with a clear database-required error.

Notepad document and presence routes keep their existing in-memory fallback when
no Postgres pool is available because the repository abstraction owns that
fallback and the route contract remains concrete.

## Python sidecar contract

- `PYTHON_SIDECAR_BINARY` unset: the service starts, but Python cell execution
  returns an explicit sidecar-not-configured error payload.
- `PYTHON_SIDECAR_BINARY` set: startup creates and health-checks a managed
  `openfoundry-pyruntime` subprocess via `libs/python-sidecar`.
- Tests cover both unset configuration and a fake sidecar binary that speaks the
  same gRPC/health contract used by the manager.

## Build & run

```sh
go build -o bin/notebook-runtime-service ./services/notebook-runtime-service/cmd/notebook-runtime-service
go test ./services/notebook-runtime-service/... ./libs/python-sidecar/...
```

## Configuration

| Variable | Default |
|---|---|
| `HOST` | `0.0.0.0` |
| `PORT` | `50134` |
| `JWT_SECRET` | (required for protected routes) |
| `DATABASE_URL` | unset; production CRUD returns `503` unless smoke mode is enabled |
| `NOTEBOOK_RUNTIME_SMOKE_MODE` | `false` |
| `DATA_DIR` | `/tmp/notebook-data` (workspace files live under `<data_dir>/workspaces/<notebook_id>/`) |
| `QUERY_SERVICE_URL` | `http://127.0.0.1:50133` |
| `AI_SERVICE_URL` | `http://127.0.0.1:50127` |
| `PYTHON_SIDECAR_BINARY` | unset |
| `PYTHON_SIDECAR_TIMEOUT_SECONDS` | `60` |
