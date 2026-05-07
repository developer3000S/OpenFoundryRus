# `pipeline-build-service` (Go)

Build / execution side of Pipeline Builder. The Rust crate is the
largest in the workspace (≈36 KLOC of source + 86 integration tests
covering DAG resolution, branch lock acquisition, multi-output
atomicity, log streaming, Spark / Iceberg orchestration). This Go
port lands the **substrate + the testable domain primitives** while
the heavy domain modules are migrated incrementally.

## Port status

| Component | Status |
|---|---|
| Health (`/health`, `/healthz`) + Prometheus (`/metrics`) | ✅ |
| HTTP surface | 🟡 partial — Go exposes the current `/api/v1` compatibility surface plus health/metrics; `docs/migration/route-parity-audit.md` lists remaining Rust `/v1`, `/api/v1/data-integration`, and `/api/v1/pipeline` gaps route-by-route |
| `internal/models` (`Build`, `BuildState`, `AbortPolicy`, `Job`, `JobState`, `Pipeline*`, `PipelineRun`) | ✅ ported 1:1 with the Rust enums + JSON tags |
| `internal/domain/joblifecycle` (Job state machine + `IsValidTransition` + `TransitionJob` + audit-log insert) | ✅ ported 1:1 with the four Rust unit tests |
| `internal/domain/markings` (T3.4 marking propagation SQL + transaction wrapper) | ✅ ported 1:1; idempotency via `ON CONFLICT DO NOTHING` |
| Build resolution (`resolve_build`, branch lock acquisition, fan-out) | ⏳ pending — handler paths that need this still return 501 |
| Build executor (DAG runner, multi-output transactions) | ⏳ pending — execution/rerun/cancel paths still return 501 |
| Iceberg output client (ADR-0041) | ⏳ pending — config warning remains accurate; multi-table atomicity is not enforced without the catalog URL |
| SparkApplication / kube-rs CR submission (FASE 3 / Tarea 3.4) | 🟡 partially wired — Spark run submit/status helpers exist; remaining Rust pipeline Spark routes are tracked as missing in the route audit |
| SSE log streaming | 🟡 config-gated placeholder — `/jobs/{id}/logs/stream` emits an initial event and then depends on log runtime wiring |

The endpoint shape is no longer documented from memory: use
`openfoundry-go/docs/migration/route-parity-audit.md` as the source of
truth for route parity. Current productive gaps are a mix of missing
legacy Rust paths, explicit `501 Not Implemented` handlers, empty list
envelopes, and config-gated log/runtime behavior.

## Build & run

```sh
go build -o bin/pipeline-build-service ./services/pipeline-build-service/cmd/pipeline-build-service
go test ./services/pipeline-build-service/...
```

## Configuration

| Variable | Default |
|---|---|
| `HOST` | `0.0.0.0` |
| `PORT` | `50081` |
| `JWT_SECRET` | (required) |
| `DATABASE_URL` | unset (handlers gated on this when the resolver lands) |
| `DATA_DIR` | `/var/lib/openfoundry/pipeline-build` |
| `DATASET_SERVICE_URL` | `http://localhost:50079` |
| `WORKFLOW_SERVICE_URL` | `http://localhost:50080` |
| `AI_SERVICE_URL` | `http://localhost:50127` |
| `STORAGE_BACKEND` | `local` |
| `STORAGE_BUCKET` | unset |
| `S3_*` | unset |
| `LOCAL_STORAGE_ROOT` | unset |
| `DISTRIBUTED_PIPELINE_WORKERS` | `4` |
| `DISTRIBUTED_COMPUTE_POLL_INTERVAL_MS` | `1000` |
| `DISTRIBUTED_COMPUTE_TIMEOUT_SECS` | `1800` |
| `SPARK_NAMESPACE` | `openfoundry-spark` |
| `PIPELINE_RUNNER_IMAGE` | `openfoundry/pipeline-runner:dev` |
| `FOUNDRY_ICEBERG_CATALOG_URL` | unset (boot-time warn matches Rust) |
| `FOUNDRY_ICEBERG_CATALOG_BEARER` | unset |
