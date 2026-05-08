# ai-sink Iceberg writer migration notes

## Rust contract reviewed

The Rust `services/ai-sink` runtime defines the canonical AI event contract:

- Kafka source topic: `ai.events.v1`; consumer group: `ai-sink`.
- Target catalog/namespace: `lakekeeper.of_ai`.
- Target tables by `kind`: `prompt -> prompts`, `response -> responses`, `evaluation -> evaluations`, `trace -> traces`.
- Shared schema field IDs 1-8: `event_id`, `at`, `kind`, `run_id`, `trace_id`, `producer`, `schema_version`, `payload`.
- Shared layout: partition transform `day(at)` and sort order `at ASC`.
- Runtime append shape: group a flushed Kafka batch by target table, append one record batch per non-empty table, then commit offsets only after all table appends succeed.

## Go writer shape

The Go `openfoundry-go/services/ai-sink` writer keeps the same routing and table contract. Production Iceberg mode uses an OpenFoundry HTTP table-writer adapter because Go does not yet have a stable write-side Iceberg API equivalent to Rust `append_record_batches`.

The adapter request contains:

- Lakekeeper catalog identity: `catalog=lakekeeper` and `catalog_url=<ICEBERG_CATALOG_URL>`.
- Optional `warehouse=<ICEBERG_WAREHOUSE>`.
- Table identity: `namespace=of_ai`, `table=<prompts|responses|evaluations|traces>`.
- Layout and schema: `day(at)`, `at ASC`, and field IDs 1-8.
- Rows encoded with the shared envelope columns and payload as JSON string.

The adapter must load the target Lakekeeper table, write Parquet data files, commit the Iceberg snapshot atomically, and return success only after the commit is durable.

## Adapter dependency and in-repo contract

This repository includes the OpenFoundry table-writer adapter in
`services/iceberg-catalog-service`: the append route is
`POST /openfoundry/iceberg/v1/append` in
`services/iceberg-catalog-service/internal/server/server.go`, and the request
model is documented in `services/iceberg-catalog-service/internal/models/models.go`.
The ai-sink writer contract tests pin the exact JSON payload sent to that route,
including table identity, field IDs, layout, and row encoding.

Deployments may point `AI_SINK_TABLE_WRITER_URL` (or the shared
`ICEBERG_TABLE_WRITER_URL` fallback) at that service. If an external adapter is
used instead, it must implement the same route and error contract before
Iceberg mode is considered production-equivalent.

## Configuration

Required for Iceberg mode:

```sh
KAFKA_BOOTSTRAP_SERVERS=...
ICEBERG_CATALOG_URL=http://lakekeeper:8181
AI_SINK_TABLE_WRITER_URL=http://ai-table-writer:8080 # optional when co-located/proxied
ICEBERG_TABLE_WRITER_URL=http://table-writer:8080    # optional shared fallback
ICEBERG_WAREHOUSE=...                                # optional
AI_SINK_JSONL_DIR=                                   # must be unset/empty in production Iceberg mode
```

`AI_SINK_TABLE_WRITER_URL` defaults to `ICEBERG_TABLE_WRITER_URL` and then to `ICEBERG_CATALOG_URL` for deployments that proxy the table-writer endpoint next to the catalog.

Dev/staging JSONL mode remains safe and explicit:

```sh
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
AI_SINK_JSONL_DIR=/tmp/ai-sink-jsonl
```

When `AI_SINK_JSONL_DIR` is set, `ICEBERG_CATALOG_URL` is not required and the process writes one `<table>.jsonl` file per routed table instead of attempting any Iceberg commit.

## Lakekeeper/of_ai compatibility checklist

Before enabling Iceberg mode against Lakekeeper, provision these tables under namespace `of_ai`:

- `prompts`
- `responses`
- `evaluations`
- `traces`

All tables must have the eight shared fields, stable field IDs 1-8, partition transform `day(at)`, and sort order `at ASC`. Schema mismatches should surface as typed writer errors and stop Kafka offset commits so operators can fix the table definition without losing data.

## Adapter error contract

The Go adapter client treats any HTTP `2xx` response as durable append success.
HTTP `404` maps to `ErrTableNotFound`; HTTP `409` and `422` map
to `ErrSchemaMismatch`; network errors and all other non-2xx statuses map to
`ErrCommitFailed`. The runtime commits Kafka offsets only after every non-empty
table group in the flushed batch receives a successful adapter response.
