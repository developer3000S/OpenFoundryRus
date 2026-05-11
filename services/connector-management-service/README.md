# connector-management-service (Go)

Go runtime for the Foundry Data Connection app migration. The Go service now uses the same runtime configuration names and defaults as the Rust service where those defaults exist.

## Compatibility naming

Data Connection public payloads should follow the frozen terminology in
[`docs/reference/foundry-compatibility-glossary.md`](../../docs/reference/foundry-compatibility-glossary.md):
use `source` for Data Connection sources, `source_rid` for stable source
identity, `webhook` for outbound callable definitions, and `output_parameters`
for parsed webhook/action outputs. Legacy `/connections` aliases may remain at
the HTTP edge, but internal and new product docs should prefer `source`.

## REST API sources

`connector_type: "rest_api"` sources use a normalized config shape for outbound
webhooks and external HTTP reads. The service accepts either `domain` or
`base_url` and stores both, plus auth metadata, secret references, worker/runtime
policy, and source permissions. This lets an Open-Meteo source be configured
without code changes:

```json
{
  "name": "Open-Meteo",
  "connector_type": "rest_api",
  "config": {
    "domain": "api.open-meteo.com",
    "auth": { "type": "none" },
    "runtime": {
      "worker": "foundry",
      "timeout_ms": 10000,
      "retry_count": 0,
      "allowed_methods": ["GET", "POST"]
    },
    "permissions": {
      "discoverable": true,
      "syncable": false,
      "invokable": true,
      "usable_in_code": true,
      "allowed_egress_hosts": ["api.open-meteo.com"]
    },
    "webhook": {
      "path": "/v1/forecast",
      "method": "GET",
      "inputs": [
        { "id": "latitude", "type": "number", "required": true },
        { "id": "longitude", "type": "number", "required": true }
      ],
      "calls": [
        {
          "id": "weather",
          "method": "GET",
          "path": "/v1/forecast",
          "query_params": {
            "latitude": "{{latitude}}",
            "longitude": "{{longitude}}",
            "current": "temperature_2m,wind_speed_10m,relative_humidity_2m"
          }
        }
      ],
      "outputs": [
        { "id": "temperature", "type": "number", "extractor": { "from_call": "weather", "path": "/current/temperature_2m" } },
        { "id": "wind_speed", "type": "number", "extractor": { "from_call": "weather", "path": "/current/wind_speed_10m" } },
        { "id": "humidity", "type": "number", "extractor": { "from_call": "weather", "path": "/current/relative_humidity_2m" } }
      ],
      "timeout_ms": 10000,
      "concurrency_limit": 10,
      "rate_limit": { "max_requests": 60, "per_seconds": 60 },
      "history": { "enabled": true, "retention_days": 30, "store_outputs": true }
    }
  }
}
```

Supported `auth.type` values are `none`, `basic`, `bearer`, `api_key`, and
`custom_header`. Local/dev tests may pass `auth.value` or legacy
`bearer_token`; production configs should prefer `auth.secret_ref` plus the
credential endpoints.

`POST /api/v1/webhooks/{source_id}/invoke` executes either this `rest_api`
source-backed webhook model or the legacy absolute-URL webhook config. The
handler enforces source ownership/invoke permissions, source `invokable`,
allowed HTTP methods, egress host allowlists, timeout, max call/input/response
limits, rate limits, and concurrency limits before returning sanitized
diagnostics and typed `output_parameters`.

Webhook history is persisted when the definition has `history.enabled: true`
or uses the default history settings. `store_inputs` controls whether request
inputs are retained; `store_outputs` controls whether parsed
`output_parameters` are retained. Source owners, admins, or callers with
webhook read permission can inspect retained entries with:

- `GET /api/v1/webhooks/{source_id}/history`
- `GET /api/v1/data-connection/sources/{source_id}/webhook-history`

## HTTPS inbound listeners

Sources can also expose an HTTPS listener for external systems that push events
into OpenFoundry. Listener requests are authenticated by the listener config, so
the receive routes are intentionally mounted outside JWT auth:

- `POST /api/v1/listeners/{source_id}/events`
- `POST /api/v1/data-connection/sources/{source_id}/listeners/{listener_id}/events`

Example source config:

```json
{
  "listener": {
    "id": "trail-events",
    "type": "https",
    "enabled": true,
    "auth": {
      "type": "hmac_sha256",
      "header": "X-OpenFoundry-Signature",
      "secret": "dev-only-local-secret"
    },
    "destination": {
      "mode": "object",
      "object_type_id": "00000000-0000-0000-0000-000000000000"
    },
    "limits": {
      "max_payload_bytes": 1048576
    }
  }
}
```

The receiver accepts JSON payloads, validates either `hmac_sha256`,
`shared_secret`, or `none` auth, redacts auth headers before storage, and writes
an `inbound_listener_events` record with the payload plus destination metadata
(`event_log`, `dataset`, or `object`). Source owners, admins, or callers with
listener read permission can inspect received records with:

- `GET /api/v1/listeners/{source_id}/events`
- `GET /api/v1/data-connection/sources/{source_id}/listener-events`

## Required in production

| Variable | Description |
| --- | --- |
| `DATABASE_URL` | PostgreSQL connection string. The service fails to start when absent. |
| `JWT_SECRET` or `OPENFOUNDRY_JWT_SECRET` | JWT signing/validation secret. `OPENFOUNDRY_JWT_SECRET` wins when both are set. The service fails to start when neither is set. |
| `CREDENTIAL_ENCRYPTION_KEY` | Base64 AES-256-GCM key for credential ciphertext at rest. Dev environments may derive from JWT secret while credential storage is being ported; production should set a dedicated key. |

## Rust-compatible runtime defaults

| Variable | Default | Description |
| --- | --- | --- |
| `HOST` | `0.0.0.0` | HTTP bind host. |
| `PORT` | `50088` | HTTP bind port, matching Rust. |
| `DATASET_SERVICE_URL` | `http://localhost:50079` | Dataset-versioning service base URL. |
| `PIPELINE_SERVICE_URL` | `http://localhost:50080` | Pipeline service base URL. |
| `ONTOLOGY_SERVICE_URL` | `http://localhost:50103` | Ontology service base URL. |
| `INGESTION_REPLICATION_GRPC_URL` | empty | Optional ingestion-replication gRPC endpoint; empty means runtime dispatch remains unavailable/pending. |
| `NETWORK_BOUNDARY_SERVICE_URL` | `http://localhost:50119` | Network boundary service base URL. |
| `SYNC_POLL_INTERVAL_SECS` | `2` | Sync scheduler poll interval. |
| `ALLOW_PRIVATE_NETWORK_EGRESS` | `true` | Allows private-network egress during local/dev bring-up. |
| `ALLOWED_EGRESS_HOSTS` | empty list | Comma-separated host allowlist. Wildcards may be interpreted by later egress logic. |
| `AGENT_STALE_AFTER_SECS` | `120` | Connector agent staleness threshold. |
| `MEDIA_SETS_SERVICE_URL` | `http://localhost:50156` | Media sets service base URL used by media-set sync runtime. |
| `OPENFOUNDRY_VENDED_CREDENTIALS_TTL_SECS` | `900` | Vended credential TTL for Iceberg/catalog integrations. |

## Go-only operational knobs

| Variable | Default | Description |
| --- | --- | --- |
| `SERVICE_VERSION` | `dev` | Version reported by health/observability. Build-time version replaces `dev` in `main`. |
| `METRICS_ADDR` | `0.0.0.0:9090` | Reserved metrics bind address for consistency with other Go services; this service exposes `/metrics` on the main router. |
| `SECRET_MANAGER_URL` | empty | Placeholder for future secret-manager integration. |
| `OUTBOX_ENABLED` | `true` | Placeholder switch for future outbox emission. |

## Dev-only

| Variable | Default | Description |
| --- | --- | --- |
| `OPENFOUNDRY_DEV_AUTH` | `false` | Mounts `/api/v1/auth/*` and `/api/v1/users/me` shim routes when true/`1`. Do not enable in production. |
| `OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS` | `0` | Enables future recurring virtual-table auto-registration worker when greater than zero. |
