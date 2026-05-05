# audit-sink (Go)

Kafka → Iceberg consumer. Subscribes to `audit.events.v1`, batches the
`AuditEnvelope` records (default: 100k OR 60s, whichever first) and
appends them to `lakekeeper/of_audit/events`. Closes the loop on
everything `edge-gateway-service` and `libs/audit-trail` publish.

## At-least-once semantics

`Writer.Append` happens BEFORE the Kafka offset commit. A crash between
the Iceberg append and the commit replays the batch on restart;
downstream dedup (Iceberg primary key by `event_id`, or an
`idempotency.Store` if one is wired in) handles duplicates. Same
trade-off the Rust crate makes — see ADR-0022 § Consumer-side contract.

## Writer backends

`internal/writer.Writer` interface, two implementations:

| Implementation   | Trigger                              | Status                      |
| ---------------- | ------------------------------------ | --------------------------- |
| `JSONLWriter`    | `AUDIT_SINK_JSONL_PATH` set          | Production-suitable for staging / observability |
| `IcebergWriter`  | (default — when JSONL path is unset) | **Stub** — fails the first batch with `ErrNotImplemented` until iceberg-go's write API stabilises |

The stub is deliberately loud so an operator who forgets to set the
JSONL path notices on the first batch instead of silently dropping audit
events. Wire the real Iceberg writer once `apache/iceberg-go` ships
write-side stability docs.

## Configuration

Operator-facing env contract — names match the Rust crate exactly so a
single Helm `values.yaml` drives both implementations during cutover.

| Variable                            | Required | Purpose                                   |
| ----------------------------------- | :------: | ----------------------------------------- |
| `KAFKA_BOOTSTRAP_SERVERS`           | ✅       | CSV `host:port` list of brokers          |
| `KAFKA_SASL_USERNAME`               |          | SASL principal (default: `KAFKA_CLIENT_ID` or `audit-sink`) |
| `KAFKA_SASL_PASSWORD`               |          | When unset → PLAINTEXT (dev / docker-compose)            |
| `KAFKA_SASL_MECHANISM`              |          | Default `SCRAM-SHA-512`                   |
| `KAFKA_SECURITY_PROTOCOL`           |          | Default `SASL_SSL`                        |
| `ICEBERG_CATALOG_URL`               | ✅       | Lakekeeper REST URL                       |
| `ICEBERG_WAREHOUSE`                 |          | Warehouse location override               |
| `AUDIT_SINK_BATCH_MAX_RECORDS`      |          | Default 100k                              |
| `AUDIT_SINK_BATCH_MAX_WAIT_SECONDS` |          | Default 60s                               |
| `AUDIT_SINK_JSONL_PATH`             |          | Path to JSONL output file (`-` = stdout). When set, JSONLWriter is used instead of the Iceberg stub |
| `METRICS_ADDR`                      |          | Default `0.0.0.0:9090`                    |
| `OTEL_TRACES_EXPORTER`              |          | `none` to disable tracing                 |
| `LOG_FORMAT`                        |          | `json` for production                     |

## Endpoints

- `GET /healthz` — liveness payload (Rust-compatible).
- `GET /metrics` — Prometheus scrape (`audit_sink_lag_seconds`,
  `audit_sink_records_total`, `audit_sink_batch_size_records`,
  `audit_sink_commits_total{outcome=success|failure|poison}`).

## Build / run

```sh
make build-services
KAFKA_BOOTSTRAP_SERVERS=localhost:9092 \
ICEBERG_CATALOG_URL=http://localhost:8181 \
AUDIT_SINK_JSONL_PATH=/var/log/audit.jsonl \
OTEL_TRACES_EXPORTER=none \
./bin/audit-sink
```
