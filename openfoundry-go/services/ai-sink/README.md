# ai-sink (Go)

Kafka → Iceberg consumer for `ai.events.v1`. Batches `AiEventEnvelope`
records (default 100k OR 60s) and routes each one to one of four
Iceberg tables under `lakekeeper/of_ai/`:

| `kind` token   | Iceberg table |
| -------------- | ------------- |
| `prompt`       | `prompts`     |
| `response`     | `responses`   |
| `evaluation`   | `evaluations` |
| `trace`        | `traces`      |

Same architecture as `audit-sink`; the only difference is the per-table
routing inside `internal/runtime`. See `audit-sink/README.md` for the
shared design notes (writer backends, at-least-once semantics, Iceberg
stub status).

## Configuration

Identical to `audit-sink` but with AI-specific knobs:

| Variable                          | Required | Purpose                                |
| --------------------------------- | :------: | -------------------------------------- |
| `KAFKA_BOOTSTRAP_SERVERS`         | ✅       | broker list                            |
| `ICEBERG_CATALOG_URL`             | ✅       | Lakekeeper REST URL                    |
| `AI_SINK_BATCH_MAX_RECORDS`       |          | Default 100k                           |
| `AI_SINK_BATCH_MAX_WAIT_SECONDS`  |          | Default 60s                            |
| `AI_SINK_JSONL_DIR`               |          | Directory for one `<table>.jsonl` per Iceberg table; selects JSONLWriter when set |
| `METRICS_ADDR`                    |          | Default `0.0.0.0:9090`                 |

Plus the same `KAFKA_SASL_*` set documented in `audit-sink/README.md`.

## Endpoints

- `GET /healthz` — liveness payload (Rust-compatible).
- `GET /metrics` — Prometheus scrape; metrics labelled by table:
  - `ai_sink_lag_seconds{table=...}`
  - `ai_sink_records_total{table=...}`
  - `ai_sink_batch_size_records{table=...}`
  - `ai_sink_commits_total{table=...,outcome=success|failure|poison}`

## Build / run

```sh
make build-services
mkdir -p /var/log/ai-sink
KAFKA_BOOTSTRAP_SERVERS=localhost:9092 \
ICEBERG_CATALOG_URL=http://localhost:8181 \
AI_SINK_JSONL_DIR=/var/log/ai-sink \
OTEL_TRACES_EXPORTER=none \
./bin/ai-sink
```
