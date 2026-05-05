# edge-gateway-service (Go)

HTTP edge gateway: reverse-proxies every public route to the right
bounded-context service. Stateless. **First Phase 2 service migrated**
from the Rust workspace at `services/edge-gateway-service/`.

## What it does

- **Listens** on `:8080` (configurable). No TLS termination — assumed
  done by an upstream LB.
- **Validates** JWTs (HS256 / RS256) optionally. Anonymous traffic is
  fine — downstream services enforce auth where they need it.
- **Enforces zero-trust scope**: 403 with `scoped_session_method_denied`
  / `scoped_session_path_denied` when the token's `session_scope`
  forbids the method/path.
- **Resolves a TenantContext** from claims (tier → quota policy +
  per-claim overrides) and forwards it as
  `x-openfoundry-tenant-*` / `x-openfoundry-quota-*` headers.
- **Forwards** the request to the right upstream (~80 path-prefix
  rules, see [`internal/proxy/router_table.go`](internal/proxy/router_table.go)).
- **Rewrites** dataset paths (`/api/v1/datasets/...` → `/v1/datasets/...`,
  `.../filesystem` → `.../files`, `/api/v1/datasets/catalog/facets`
  → `/v1/catalog/facets`).
- **Strips** the `Host` header and **injects** auth context
  (`x-openfoundry-auth-*`, `x-openfoundry-org-id`,
  `x-openfoundry-zero-trust`, scope details).
- **Rate-limits** with a token-bucket (Redis when `redis_url` set;
  in-memory fallback). Tenant-scoped for authenticated calls,
  IP-scoped for anonymous.
- **Audits** every request to the `OF_AUDIT.gateway` NATS subject
  (fire-and-forget, only when `nats_url` is set).
- **Body limit** clamped to the tenant's quota with a 10 MiB fallback.

## Direct endpoints (NOT proxied)

| Method | Path       | Purpose                             |
| ------ | ---------- | ----------------------------------- |
| GET    | `/healthz` | Liveness payload (Rust-compatible). |
| GET    | `/metrics` | Prometheus scrape (default Go runtime + process collectors). |

Everything else is forwarded — see the router table for the full map.

## Error envelope

Every gateway-emitted error uses the canonical envelope:

```json
{ "error": { "code": "<stable_code>", "message": "<human msg>" } }
```

Stable codes (do **not** rename — frontend branches on them):
- `unknown_service_route` → 404
- `invalid_upstream_uri` → 502
- `body_too_large` → 413
- `rate_limit_exceeded` → 429
- `scoped_session_method_denied` → 403
- `scoped_session_path_denied` → 403
- `upstream_unavailable` → 502
- `proxy_response_build_failed` → 500

## Configuration

YAML (defaults shipped in image at `/etc/openfoundry/config.yaml`) +
`OF_*` env overrides (separator `__`). Examples:

```sh
OF_SERVER__PORT=9090
OF_JWT__SECRET=...
OF_REDIS_URL=redis://redis-cluster:6379
OF_NATS_URL=nats://jetstream:4222
OF_RATE_LIMIT__ANONYMOUS_REQUESTS_PER_MINUTE=300
OF_UPSTREAM__DATASET_VERSIONING_SERVICE_URL=http://dvs.openfoundry.svc:50078
```

The full upstream URL set + every default port mirrors the Rust
gateway's `config.rs` so a single Helm `values.yaml` drives both
implementations during cutover.

## Build / run

```sh
make build-services           # produces ./bin/edge-gateway-service
OTEL_TRACES_EXPORTER=none \
OF_JWT__SECRET=$(openssl rand -hex 32) \
./bin/edge-gateway-service -config services/edge-gateway-service/config.yaml
```

Image:

```sh
docker build -t openfoundry/edge-gateway-service:dev \
  -f services/edge-gateway-service/Dockerfile .
```

## Cutover protocol

This is the strangler-fig pattern's first real run:

1. Deploy the Go pod alongside the Rust pod, **same Helm release**.
2. Add a header-based router rule on the upstream LB:
   `X-Of-Migration: go-canary` → Go pod, default → Rust pod.
3. Run the **contract diff suite** (TODO under
   `tests/contract-diff/`) on every CI build comparing byte-identical
   responses between the two implementations.
4. Traffic ramp: 1 % → 10 % → 50 % → 100 %, ≥ 24 h per step with
   error-budget gates.
5. Soak: 100 % Go for ≥ 14 days before removing the Rust pod.
6. Decommission: remove the Rust crate from the Cargo workspace.

## Wire-compat invariants (do not break)

- Error envelope shape + every code listed above.
- Header names: `x-openfoundry-tenant-*`, `x-openfoundry-quota-*`,
  `x-openfoundry-auth-*`, `x-openfoundry-zero-trust`,
  `x-openfoundry-scope-*`, `x-openfoundry-org-id`,
  `x-openfoundry-classification-clearance`, `x-openfoundry-session-kind`.
- Rate-limit response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`,
  `X-RateLimit-Reset`, `Retry-After`.
- Tenant tier names: `standard` / `team` / `enterprise`.
- `/healthz` payload shape (`status`, `service`, `version`, `timestamp`).
- Dataset path rewriting rules.
