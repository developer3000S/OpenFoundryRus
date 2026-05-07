# connector-management-service (Go)

Go runtime for the Foundry Data Connection app migration. The Go service now uses the same runtime configuration names and defaults as the Rust service where those defaults exist.

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
