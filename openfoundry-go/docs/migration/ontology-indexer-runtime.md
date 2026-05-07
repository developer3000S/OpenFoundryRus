# Ontology indexer runtime

The Go `ontology-indexer` is an operational Kafka worker: it consumes ontology mutation events, projects them into the configured search backend, and commits Kafka offsets only after the projection path has completed.

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `KAFKA_BOOTSTRAP_SERVERS` | unset | Comma-separated Kafka bootstrap brokers. Required by the runtime. |
| `KAFKA_CONSUMER_GROUP` | `ontology-indexer` | Consumer group used by all indexer replicas. Override only for isolated replays. |
| `SEARCH_BACKEND` | `vespa` | Search backend selector. Accepted values are `vespa` and `opensearch`. |
| `SEARCH_ENDPOINT` | unset | Base URL for the selected search backend. Required by the runtime. |
| `SEARCH_USERNAME` / `SEARCH_PASSWORD` | unset | Optional basic-auth credentials. |
| `SEARCH_API_KEY` | unset | Optional API-key auth. Takes priority over basic auth. |
| `SEARCH_BEARER_TOKEN` | unset | Optional bearer token. Takes priority over API-key and basic auth. |
| `INDEXER_RETRY_MAX_ATTEMPTS` | `3` | Number of attempts for a backend write before the record is considered failed. |
| `INDEXER_RETRY_INITIAL_BACKOFF` | `100ms` | Initial retry backoff. Parsed with Go duration syntax. |
| `INDEXER_RETRY_MAX_BACKOFF` | `2s` | Maximum exponential backoff delay between attempts. |
| `INDEXER_DLQ_TOPIC` | `ontology-indexer.dlq.v1` | DLQ topic for records that still fail after retries. Set to `off`, `none`, or `disabled` to disable DLQ publishing and leave the offset uncommitted. |
| `METRICS_ADDR` | `0.0.0.0:9090` | Metrics listener address. |

## Topics

| Topic | Direction | Purpose |
| --- | --- | --- |
| `ontology.objects.changed.v1` | consume | Live object upserts and tombstones. |
| `ontology.links.changed.v1` | consume | Live link upserts and tombstones, materialised as link documents. |
| `ontology-indexer.dlq.v1` | produce | Failed records after retry exhaustion when DLQ publishing is enabled. |

## Event shape

Object events must carry `tenant`, `id`, `type_id`, `version`, and `payload`. A `deleted: true` record deletes the document. Link events must carry `tenant`, `link_type`, `from`, `to`, and `version`; migration aliases `type_id`, `source_id`, and `target_id` are accepted.

## Idempotency and ordering guarantees

The worker is at-least-once. It commits a Kafka offset only after either the search backend accepted the projection, the projection was identified as a duplicate or stale version, malformed input was intentionally skipped, or the record was successfully published to the DLQ.

Projection idempotency uses the stable `(tenant, id, version)` tuple:

1. During a process lifetime, the runtime keeps a per-document high-water mark and skips records whose version is less than or equal to the newest successfully applied version for that document.
2. Across restarts, the search backend remains the durable authority. Vespa and OpenSearch index writes are version-guarded by the `IndexDoc.Version` field and discard stale or duplicate upserts.
3. Delete calls are idempotent when the target document is already absent. Kafka partitioning should keep events for the same object key ordered, so a stale tombstone should not arrive after a newer upsert for the same key in normal operation.

## Retry, backoff, and DLQ

Backend errors are retried with exponential backoff using `INDEXER_RETRY_*` settings. If all attempts fail and `INDEXER_DLQ_TOPIC` is configured, the original Kafka key and value are published to the DLQ and the source offset is committed. If DLQ publishing is disabled or fails, the source offset remains uncommitted so Kafka can redeliver the record after restart or rebalance.
