# identity-federation-service (Go)

Identity & federation: register, login, JWT issuance, refresh-token rotation,
MFA, WebAuthn, OIDC, SAML helpers, RBAC, restricted views, Cedar admin-policy
helpers, SCIM handler package, JWKS rotation logic, and Vault Transit signer
logic. This README records the **current Go implementation status**; the
historical slice plan remains in
[`openfoundry-go/INVENTORY-identity-federation-service.md`](../../INVENTORY-identity-federation-service.md).

## Current implementation audit (2026-05-08)

### Wired into the binary / router

- Public auth endpoints: bootstrap status, register, login, refresh-token
  exchange, MFA login completion, WebAuthn login challenge/finish.
- Bearer-protected MFA/WebAuthn management endpoints.
- OIDC SSO provider listing, start, and callback routes.
- SAML ACS/start code is present in the SSO handler, and the ACS route is
  mounted, but SAML providers are not constructed in `cmd/.../main.go`; the
  default runtime is therefore OIDC-only unless a future wiring slice passes a
  `*saml.Registry` into `handlers.SSO`.
- RBAC CRUD for users, roles, permissions, groups, API keys.
- Restricted-view CRUD.
- Health and Prometheus metrics.

### Implemented as packages, not wired as active runtime surfaces

- `internal/saml`: AuthnRequest construction, metadata parsing, signed-response
  validation, signature validation, registry, and fixtures/tests. Runtime SAML
  provider config loading remains pending.
- `internal/scim`: SCIM 2.0 discovery, User CRUD, Group CRUD helpers/handlers,
  in-memory store, Postgres-shaped store contracts, and conformance-style wire
  tests. `/scim/v2/*` is not mounted in `internal/server` and is not yet guarded
  by Cedar in the active router.
- `internal/cedarauthz`: bundled identity-admin policies, principal/resource
  hydration, and `AdminGuard` middleware for JWKS/SCIM admin actions. The guard
  is tested but not applied to mounted routes because JWKS admin and SCIM routes
  are not mounted yet.
- `internal/jwksrotation`: 90/14 day rotation orchestration, Postgres key-store
  adapter, JWKS publication builders, in-memory fakes, and tests. No
  `/.well-known/jwks.json` or admin rotate/rollback route is mounted yet.
- `internal/jwksrotation/vault_signer.go`: production Vault Transit signer,
  env parsing, token/Kubernetes auth, retry handling, signing, public-key lookup,
  and rotation calls are implemented and tested. The binary does not construct
  this signer yet.
- `internal/sessionscassandra`: Cassandra user-session and refresh-token DDL plus
  adapter methods exist, with pinned migration tests. The active `Issuer` still
  stores refresh tokens in Postgres.

### Still pending / not implemented end-to-end

- Cassandra-backed sessions as the active refresh-token/session backend.
- Runtime SAML provider configuration and end-to-end SAML login wiring.
- Mounting `/scim/v2/*` in the service router and applying the Cedar SCIM guard.
- JWKS publication/admin endpoints and wiring the JWKS rotation service into the
  binary.
- Constructing the Vault signer from env in the binary and using it for JWT/JWKS
  signing.
- Control-panel scoped-session governance and ABAC administration.
- Redis rate limiting and Kafka audit publishing to `audit.identity.v1`.

## Active endpoint surface

Endpoints under `/api/v1/auth` that do not require bearer credentials:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/bootstrap-status` | Returns whether first-admin bootstrap is still open. |
| POST | `/register` | Argon2id-hashed user creation; first user becomes admin. |
| POST | `/login` | Password verification → access JWT + refresh token, or MFA-required response. |
| POST | `/token/refresh` | One-time refresh-token exchange + family-replay detection. |
| POST | `/mfa/totp/complete-login` | Completes a TOTP-gated login. |
| POST | `/mfa/webauthn/login/challenge` | Starts WebAuthn login. |
| POST | `/mfa/webauthn/login/finish` | Finishes WebAuthn login. |
| GET | `/sso/providers` | Lists configured OIDC providers plus SAML providers when a registry is wired. |
| GET | `/sso/{provider}/start` | Starts OIDC, or SAML only if a registry is wired. |
| GET | `/sso/{provider}/callback` | OIDC callback. |
| POST | `/sso/{provider}/acs` | SAML ACS; returns 404 when SAML is not configured. |

Other mounted endpoints:

- `GET /healthz`, `GET /metrics`.
- Bearer-protected `/api/v1/auth/mfa/*` management routes.
- Bearer-protected `/api/v1/{users,roles,permissions,groups,api-keys}` RBAC
  CRUD routes.
- Bearer-protected `/api/v1/restricted-views` CRUD routes.

## Storage status

- Postgres is the active runtime store for users, roles/groups/permissions,
  refresh tokens, MFA, WebAuthn, OIDC state, RBAC, restricted views, SAML config
  tables, SCIM tables, and JWKS key rows.
- Cassandra/Scylla session support exists only as the `internal/sessionscassandra`
  adapter. Its migrations create `auth_runtime.user_session` and
  `auth_runtime.refresh_token`; tables from the historical Rust inventory such as
  OAuth pending auth, session revocations, and WebAuthn Cassandra credentials are
  not part of this active Go service slice.

## Cedar conformance status

`libs/authz-cedar-go` is the adopted Cedar engine wrapper. It now has a local
conformance suite covering policy/request/entity fixtures, default deny,
permit-reason diagnostics, missing-attribute diagnostic errors, strict validation
rejection, parent-graph membership, and forbid-overrides-permit semantics. The
identity service also tests the bundled `identity_admin.cedar` policy records and
`AdminGuard` allow/deny behavior for JWKS rotation and SCIM provisioning.

## Wire-format invariants preserved

- `User` JSON uses snake_case fields and never emits `password_hash`.
- `LoginResponse` is a discriminated union with `status: "authenticated"` or
  `status: "mfa_required"`.
- `TokenResponse` uses `access_token`, `refresh_token`, `token_type`,
  `expires_in`.
- Argon2id PHC encoding remains
  `$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>` so hashes issued by either
  implementation validate during cutover.
- JWT claims are issued via `libs/auth-middleware`.
- SCIM ListResponse/JWKS/SAML wire shapes are pinned by package tests but are not
  all mounted as active HTTP surfaces yet.

## Configuration

| Variable | Required | Purpose |
| --- | :---: | --- |
| `DATABASE_URL` | ✅ | Postgres connection string. |
| `JWT_SECRET` (or `OPENFOUNDRY_JWT_SECRET`) | ✅ | HS256 secret for the active JWT issuer. |
| `HOST` / `PORT` | | Defaults to `0.0.0.0:50112`. |
| `ACCESS_TOKEN_TTL` | | Defaults to `1h`. |
| `REFRESH_TOKEN_TTL` | | Defaults to `168h` (7 days). |
| `METRICS_ADDR` | | Defaults to `0.0.0.0:9090`. |
| `OTEL_TRACES_EXPORTER=none` | | Disable tracing. |

Vault, Cassandra, Redis, Kafka, and NATS variables are parsed by package-level
helpers where applicable, but they are not part of the active binary wiring yet.

## Build / run

```sh
make build-services
DATABASE_URL=postgres://localhost/identity JWT_SECRET=$(openssl rand -hex 32) \
OTEL_TRACES_EXPORTER=none ./bin/identity-federation-service
```
