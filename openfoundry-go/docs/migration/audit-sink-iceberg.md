# audit-sink Iceberg writer cutover

This note documents the Go `audit-sink` writer contract that was ported from
`services/audit-sink` in the Rust workspace.

## Rust contract reviewed

The Rust sink writes decoded `AuditEnvelope` values to Iceberg with this pinned
contract:

- Kafka source topic: `audit.events.v1`; consumer group: `audit-sink`.
- Target table: `lakekeeper.of_audit.events`.
- Schema field ids are stable: `event_id` (uuid, required, id 1), `at`
  (timestamptz microseconds, required, id 2), `correlation_id` (string,
  nullable, id 3), `kind` (string, required, id 4), and `payload` (JSON string,
  required, id 5).
- Partition and sort: `day(at)` and `at ASC`.
- Append behavior: one durable Iceberg append per flushed Kafka batch; Kafka
  offsets are committed only after the append succeeds.
- WORM retention: snapshot expiration remains disabled forever; manifest/data
  file rewrite may compact files but must not drop snapshots or rows.

## Go production writer

Go selects the Iceberg writer by default. `IcebergWriter` converts each decoded
audit event into the same table schema and sends one append request to the
OpenFoundry table-writer adapter:

```text
POST {ICEBERG_CATALOG_URL}/openfoundry/iceberg/v1/append
content-type: application/json
```

The payload contains the pinned table spec plus the rows in the flushed batch.
The table spec includes `catalog=lakekeeper`, `catalog_url=<ICEBERG_CATALOG_URL>`,
optional `warehouse=<ICEBERG_WAREHOUSE>`, `namespace=of_audit`, `table=events`,
field IDs 1-5, partition transform `day(at)`, and sort order `at ASC`.
The table-writer adapter is responsible for writing Parquet data files and
committing the Iceberg snapshot atomically. A successful 2xx response means the
Kafka runtime may commit offsets.

## Adapter dependency and in-repo contract

This repository includes the OpenFoundry table-writer adapter in
`services/iceberg-catalog-service`: the append route is
`POST /openfoundry/iceberg/v1/append` in
`services/iceberg-catalog-service/internal/server/server.go`, and the request
model is documented in `services/iceberg-catalog-service/internal/models/models.go`.
The audit-sink writer contract tests pin the exact JSON payload sent to that
route, including table identity, field IDs, layout, and row encoding.

Deployments must run or proxy that adapter at `ICEBERG_CATALOG_URL` for
`audit-sink`; unlike `ai-sink`, audit-sink currently has no separate
`*_TABLE_WRITER_URL` override. If the adapter is deployed out-of-process, the
external service must implement the same route and error contract before
Iceberg mode is considered production-equivalent.

## Explicit JSONL dev mode

JSONL is an opt-in development/staging fallback only. Set
`AUDIT_SINK_JSONL_PATH` to a path, or `-` for stdout. In that mode the process
uses `JSONLWriter`; `ICEBERG_CATALOG_URL` is not required because no Iceberg
append will be attempted.

Unset `AUDIT_SINK_JSONL_PATH` in production. With JSONL unset,
`ICEBERG_CATALOG_URL` is required and the default writer is the Iceberg HTTP
adapter, not the legacy stub.

Required environment for the real Iceberg writer:

```sh
KAFKA_BOOTSTRAP_SERVERS=...          # required Kafka bootstrap list
ICEBERG_CATALOG_URL=http://lakekeeper-or-adapter:8181
ICEBERG_WAREHOUSE=...               # optional; forwarded in the append spec when set
AUDIT_SINK_JSONL_PATH=              # must be unset/empty in production Iceberg mode
```

## Failure mode

The runtime appends before committing Kafka offsets. If the adapter returns an
error, the process returns the error, the supervisor restarts it, and Kafka
replays the uncommitted batch. Error mapping is intentionally typed:

| Adapter response | Go error | Operational action |
| --- | --- | --- |
| `404` | `ErrTableNotFound` | Create/restore `of_audit.events`; do not commit offsets manually. |
| `409` or `422` | `ErrSchemaMismatch` | Fix table schema or adapter contract; preserve field ids. |
| Network error or other non-2xx | `ErrCommitFailed` | Treat as transient unless repeated; investigate adapter/catalog/storage. |

Malformed audit records are poison-pill skipped and their offsets are committed
so a single bad message cannot wedge a partition. Valid records in the same
batch are still written before their offsets are committed.
