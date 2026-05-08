# Inventory: identity-federation-service

> Phase 3 prep — sized 2026-05-06 against
> `services/identity-federation-service/` in the Rust workspace.

## Size

- **22,738 LOC** of Rust across **92 source files**.
- 9 Postgres migrations.
- Cassandra-backed runtime state (sessions, refresh tokens, OAuth pending auth, WebAuthn).
- This is the **largest service in the workspace**. A faithful port lands across **8 vertical slices** (see plan below) — at least 6–8 autonomous iterations.

## Module map

```
services/identity-federation-service/src/
├── main.rs                                  316  bin entry; tokio::main + AppState wiring
├── lib.rs                                    65  AppState struct
├── config.rs                                 63  AppConfig (env)
├── cedar_authz.rs                           459  ⚠️ CEDAR — bootstrap_engine + spawn_policy_reload
├── sessions_cassandra.rs                    732  Cassandra session adapter (auth_runtime.sessions)
│
├── domain/                                       pure-logic primitives
│   ├── abac.rs                              383  attribute-based access control
│   ├── access.rs                             55
│   ├── api_keys.rs                          124
│   ├── idp_mapping.rs                       546  IdP claim → role/group mapping
│   ├── jwt.rs                               152
│   ├── mfa.rs                               127  TOTP
│   ├── oauth.rs                             197  OAuth grants + flows
│   ├── rbac.rs                               97
│   ├── saml.rs                              789  SAML assertions, signing, validation
│   ├── security.rs                           68
│   └── sessions.rs                          304
│
├── handlers/                                     HTTP endpoints
│   ├── login.rs                             314  /auth/login
│   ├── register.rs                          244  /auth/register
│   ├── token.rs                             159  /auth/token/refresh
│   ├── sessions.rs                          544  /auth/sessions/*
│   ├── mfa.rs                               426  /auth/mfa/*
│   ├── sso.rs                               850  /auth/sso/* (SAML + OIDC)
│   ├── user_mgmt.rs                         223  /users/*
│   ├── role_mgmt.rs                         276  /roles/*
│   ├── group_mgmt.rs                        318  /groups/*
│   ├── permission_mgmt.rs                    69
│   ├── policy_mgmt.rs                       183  /policies/*
│   ├── api_key_mgmt.rs                      120  /api-keys/*
│   ├── restricted_views.rs                  226  /restricted-views/*
│   ├── control_panel.rs                    1108  /control-panel/* (admin surface)
│   ├── scim.rs                             1951  /scim/v2/* (SCIM 2.0 RFC)
│   ├── security_ops.rs                      239  /jwks/rotate, /audit/metrics
│   ├── common.rs                             98  shared extractors
│   └── mod.rs                                14
│
├── hardening/                                    S3.1 ASVS L2 controls
│   ├── audit_topic.rs                       421  Kafka audit.identity.v1 publisher
│   ├── jwks_rotation.rs                     818  90/14 day JWKS rotation, Postgres-backed key store
│   ├── rate_limit.rs                        233  Redis-backed per-(user,IP) limiter
│   ├── refresh_family.rs                    127  refresh-token replay detection
│   ├── scim.rs                              515  SCIM external-id mapping
│   ├── vault_signer.rs                      759  ⚠️ VAULT — Vault Transit signing for JWT
│   └── webauthn.rs                         1462  WebAuthn second factor (Cassandra-backed)
│
├── models/                                       wire types (FromRow + Serialize)
│   ├── user.rs                              58
│   ├── session.rs                          149
│   ├── role.rs                              12
│   ├── group.rs                             12
│   ├── permission.rs                        13
│   ├── policy.rs                            22
│   ├── api_key.rs                           29
│   ├── mfa.rs                               16
│   ├── sso.rs                               75
│   ├── restricted_view.rs                   95
│   └── control_panel.rs                    248
│
├── oauth_integration/                            Subservice consolidated per ADR-0030 (B16)
│   │                                              — duplicates many of the parent files;
│   │                                              the data-side will be extracted to
│   │                                              connector-management-service.
│   ├── pending_auth_cassandra.rs            452
│   ├── handlers/sso.rs                      849
│   ├── handlers/oauth_clients.rs            171
│   ├── handlers/applications.rs             249
│   ├── handlers/external_integrations.rs    143
│   ├── handlers/api_key_mgmt.rs             120
│   ├── domain/{oauth,saml,idp_mapping,…}    (mostly mirrors parent)
│   └── models/{oauth_client,application,…}  (mostly mirrors parent)
│
└── session_governance/                           Subservice consolidated per ADR-0030 (B16)
    ├── handlers/revocation.rs               316  /control-panel/sessions/revoke
    ├── revocation_cassandra.rs              285  auth_runtime.session_revocations
    └── policy_postgres.rs                    46  postgres revocation policies
```

## External dependencies (runtime infra)

| Dep            | Role                                                                 |
| -------------- | -------------------------------------------------------------------- |
| **Postgres**   | Users, RBAC (roles/groups/permissions/policies), JWKS keys, control panel, restricted views. |
| **Cassandra/Scylla** | `auth_runtime.sessions`, `auth_runtime.refresh_tokens`, `auth_runtime.oauth_pending_auth`, `auth_runtime.session_revocations`, `auth_runtime.webauthn_credentials`. |
| **NATS**       | `authz.policy.changed` subject for hot-reloading Cedar policies.     |
| **Kafka**      | `audit.identity.v1` topic — every auth event emitted here.           |
| **Redis**      | Per-(user,IP) rate limiting at `/login`, `/oauth/token`, `/oauth/authorize`. |
| **Vault**      | Transit secrets engine signs JWT tokens (S3.1.b). Optional — `RotationPolicy::env_only` falls back to local ed25519. |
| **Cedar**      | Embedded policy engine (`authz_cedar` workspace lib). Bundles `policies/identity_admin.cedar`. |

## Wire-format invariants (must preserve)

- `User`, `Session`, `Role`, `Group`, `Permission`, `Policy` JSON shapes (snake_case fields).
- JWT claim set already locked by `libs/auth-middleware/claims.go`.
- SCIM 2.0 RFC 7643/7644 envelope (these are external standards — non-negotiable).
- SAML assertion XML (RFC 7522) — also external.
- OIDC discovery + JWKS (RFC 8414, RFC 7517).
- WebAuthn (RFC 8809) registration / assertion options + responses.
- Audit event JSON published on `audit.identity.v1`.

## Postgres schema (9 migrations)

- `20260419000001_initial_auth.sql` — users, sessions, roles, groups, permissions, policies, api_keys.
- `20260420000002_enterprise_auth.sql` — SSO, MFA, SAML config.
- `20260425150000_control_panel_foundation.sql` — control panel admin surface.
- `20260425193000_scoped_sessions_security.sql` — session scope (zero-trust restrictions).
- `20260425222000_control_panel_enterprise_admin.sql` — enterprise admin pages.
- `20260426030000_markings_cbac_restricted_views.sql` — markings + CBAC + restricted views.
- `20260427010100_oauth_applications_and_integrations.sql` — OAuth clients, external integrations.
- `20260503000000_jwks_keys.sql` — JWKS key store.
- `20260503002000_scim_external_ids.sql` — SCIM external-id mapping.

## Go implementation audit (2026-05-08)

The original inventory below is historical planning material. Current code review
shows the Go service is partly ahead of the slice plan at package level but not
fully wired at runtime:

### Completed and wired

- Auth/register/login/refresh-token flow, MFA, WebAuthn, OIDC SSO, RBAC CRUD,
  restricted-view CRUD, health, and metrics are mounted in `internal/server`.
- Refresh tokens remain Postgres-backed in the active `service.Issuer`.

### Completed as packages but not wired into the active router/binary

- SAML helpers and tests exist (`internal/saml`), and SSO has SAML start/ACS
  methods, but `cmd/.../main.go` does not build a `saml.Registry`; default
  runtime behavior is OIDC-only.
- SCIM 2.0 handlers/stores/tests exist under `internal/scim`, but `/scim/v2/*`
  is not mounted and Cedar is not applied to a live SCIM route.
- Cedar identity-admin policy bootstrap and `AdminGuard` exist under
  `internal/cedarauthz`, with tests for JWKS and SCIM allow/deny decisions, but
  no mounted routes currently use it.
- JWKS rotation, Postgres key-store, JWKS document builders, and Vault Transit
  signer are implemented and tested under `internal/jwksrotation`, but the
  binary does not construct them and no JWKS endpoint/admin route is mounted.
- Cassandra session support exists as `internal/sessionscassandra` with only
  `auth_runtime.user_session` and `auth_runtime.refresh_token` DDL plus adapter
  methods. It is not the active backend.

### Pending end-to-end work

- Flip sessions/refresh tokens from Postgres to Cassandra only after Scylla/CQL
  dev infrastructure exists.
- Add runtime SAML provider loading and wire the registry into `handlers.SSO`.
- Mount `/scim/v2/*` and guard mutations with `internal/cedarauthz.AdminGuard`.
- Mount JWKS publication/rotation endpoints and construct the Vault/JWKS services
  in `cmd/.../main.go`.
- Finish control-panel scoped-session governance, Redis rate limiting, and Kafka
  audit publishing.

## Cassandra schema (`auth_runtime` keyspace)

Current Go adapter status (2026-05-08): `internal/sessionscassandra` ports only
the session/refresh-token subset and is not wired into the active binary. Its
embedded migrations create:

- `user_session` (30 minute sliding TTL).
- `refresh_token` (30 day TTL, family-id'd for replay detection).

Historical Rust inventory also listed `oauth_pending_auth`,
`session_revocations`, and `webauthn_credentials`; those Cassandra tables are not
part of the current Go identity-service runtime. Postgres remains the active
backend for refresh tokens, OIDC state, WebAuthn, SCIM, SAML config, and JWKS
rows until the Cassandra/Scylla wiring follow-up lands.

## Tier-2 lib dependencies (must be ported in parallel)

- **`cassandra-kernel`** — now present in `openfoundry-go/libs/cassandra-kernel`; the identity service has an `internal/sessionscassandra` adapter, but it is not wired as the active backend.
- **`authz-cedar-go`** — adopted per the 2026-05-06 Cedar decision. `libs/authz-cedar-go` is implemented with local conformance tests; identity `internal/cedarauthz` is implemented/tested but not mounted on live routes yet.
- **`event-bus-data`** + **`event-bus-control`** — already ported.

## Recommended porting plan — 8 vertical slices

Each slice lands in its own iteration with its own commit. After slice 8 the service is at full parity.

### Slice 1 — Foundation + auth/login (target ~1500 LOC Go)

- `cmd/identity-federation-service/main.go` + `internal/server/server.go`.
- Minimal AppState (Postgres pool, JWT config, `/healthz`, `/metrics`).
- Postgres migration runner (embedded SQL).
- `handlers/login.rs` + `handlers/register.rs` + `handlers/token.rs`.
- `domain/jwt.rs` (JWT issuance using `libs/auth-middleware`).
- `domain/security.rs` (Argon2 password hashing — `golang.org/x/crypto/argon2`).
- Skip Cassandra: sessions go to Postgres for slice 1 (a `sessions_postgres` adapter parallel to the Rust `sessions_cassandra`).
- **Ship**: a working register → login → refresh flow.

### Slice 2 — Sessions + revocation (target ~1200 LOC Go)

- Port `cassandra-kernel` lib (minimal: keyspace + session + prepared-statement helpers via `gocql`).
- `sessions_cassandra.rs` → Go `sessionscassandra` package.
- `session_governance/revocation_cassandra.rs` → Go.
- `handlers/sessions.rs` (list/get/revoke own sessions).
- `session_governance/handlers/revocation.rs` (admin revoke).
- **Ship**: distributed sessions with revocation + replay detection.

### Slice 3 — MFA TOTP + recovery codes (target ~700 LOC Go)

- `domain/mfa.rs` → Go (TOTP via `pquerna/otp` or stdlib `crypto/hmac`).
- `handlers/mfa.rs` (enrol / verify / list factors).
- `models/mfa.rs`.
- **Defer**: WebAuthn (slice 4 — its own iteration).

### Slice 4 — WebAuthn (target ~1500 LOC Go)

- Port `hardening/webauthn.rs` (1462 LOC — borderline) using `go-webauthn/webauthn`.
- `CassandraWebAuthnStore` → Go.
- WebAuthn registration + assertion endpoints.
- **Ship**: WebAuthn second factor available end-to-end.

### Slice 5 — OAuth + SAML + SSO (target ~2000 LOC Go, may split into 5a/5b)

- `domain/oauth.rs` + `domain/saml.rs` + `domain/idp_mapping.rs` → Go.
- `handlers/sso.rs` (~850 LOC, OIDC + SAML).
- `oauth_integration/handlers/sso.rs` + the consolidated subservice surface.
- For SAML: use `crewjam/saml` (XML signing, AuthnRequest, response validation).
- For OIDC: use `coreos/go-oidc` for client-side, hand-roll the server-side OIDC discovery (small).
- **Ship**: SAML + OIDC SSO end-to-end.

### Slice 6 — User / Role / Group / Permission / Policy CRUD (target ~1200 LOC Go)

- `handlers/user_mgmt.rs`, `role_mgmt.rs`, `group_mgmt.rs`, `permission_mgmt.rs`, `policy_mgmt.rs`.
- `models/{user,role,group,permission,policy,api_key}.rs`.
- `handlers/api_key_mgmt.rs`.
- `domain/{rbac,api_keys}.rs`.
- **Ship**: management surface for the whole RBAC graph.

### Slice 7 — Control panel + scoped sessions + restricted views (target ~1400 LOC Go)

- `handlers/control_panel.rs` (1108 LOC — large; may need its own iteration).
- `handlers/restricted_views.rs`.
- `models/{control_panel,restricted_view}.rs`.
- `domain/abac.rs` (383 LOC — attribute-based access control evaluation).
- **Ship**: enterprise admin surface.

### Slice 8 — Hardening + Cedar + JWKS + Vault + SCIM (target ~3000 LOC Go, will split)

- **Cedar decision resolved**: use `github.com/cedar-policy/cedar-go`. `libs/authz-cedar-go` and identity `internal/cedarauthz` are now implemented/tested; remaining work is route wiring. The Rust crate gates `/jwks/rotate` + SCIM mutations on Cedar.
- `cedar_authz.rs` → Go (using `cedar-go`).
- `hardening/jwks_rotation.rs` (818 LOC) — 90/14-day JWKS rotation, Postgres-backed key store.
- `hardening/vault_signer.rs` (759 LOC) — Vault Transit signing. Use `hashicorp/vault/api` Go SDK.
- `hardening/audit_topic.rs` (421 LOC) — Kafka publisher; reuse `libs/event-bus-data`.
- `hardening/rate_limit.rs` (233 LOC) — Redis limiter; can lift the gateway's `ratelimit.RedisStore` Lua.
- `hardening/scim.rs` + `handlers/scim.rs` (1951 LOC SCIM 2.0) — **largest single file in the codebase**. Likely needs its own iteration. Use `elimity-com/scim` Go lib if its API matches; otherwise hand-roll RFC 7643/7644 envelopes (the RFC is small).
- `hardening/refresh_family.rs` — refresh-token replay detection.
- `oauth_integration/pending_auth_cassandra.rs` — OAuth pending-auth state.

### Total estimated effort

| Slice | LOC Rust | LOC Go (1.4×) | Hard parts                                        |
| ----- | -------- | -------------- | ------------------------------------------------- |
| 1     | ~1100    | ~1500          | password hashing, JWT, embedded migrations        |
| 2     | ~1300    | ~1800          | Cassandra adapter via gocql                       |
| 3     | ~570     | ~800           | TOTP                                              |
| 4     | ~1500    | ~2000          | WebAuthn protocol                                 |
| 5     | ~3000    | ~4000          | SAML XML signing, OIDC server-side                |
| 6     | ~1100    | ~1500          | nothing exotic                                    |
| 7     | ~1700    | ~2300          | ABAC evaluator, scoped session policies           |
| 8     | ~5500    | ~7500          | cedar-go wiring, Vault, SCIM 2.0, JWKS            |
| **Total** | **~15800** | **~21000+**  | (with the 22,738 LOC Rust → ~30k Go ceiling)      |

## Decisions deferred for human review

1. **`cedar_authz.rs` & cedar-go**: resolved 2026-05-06 — use `github.com/cedar-policy/cedar-go`. The Go wrapper and identity-admin policy tests now pass local conformance-style fixtures; remaining work is route/binary wiring, not SDK selection.
2. **Vault Transit signer** (`hardening/vault_signer.rs`, 759 LOC): Go signer/config/retry logic is implemented under `internal/jwksrotation`, but the binary does not construct it yet. Production still depends on Vault PKI / Transit secrets-engine configuration.
3. **SCIM 2.0**: Go has a hand-rolled RFC 7643/7644 package with handlers/stores/tests under `internal/scim`. Pending work is mounting `/scim/v2/*`, attaching persistence/runtime config, and applying Cedar mutation guards.
4. **OAuth integration data-side** (`oauth_integration/`): per ADR-0030 the data-side connectors will be extracted to `connector-management-service` as a follow-up. The Go port should keep the auth-side here and skip the data-side until that extraction lands.

## Wire-compat tests to add (after slice 1)

- Login round-trip: Rust /auth/login → JWT → Go /auth/login → JWT, both decode to same Claims under `libs/auth-middleware`.
- /healthz payload byte-identical (already locked).
- JWKS endpoint shape (RFC 7517) byte-identical.
- SCIM ListResponse envelope shape (RFC 7644 §3.4.2) byte-identical.

---

**Status**: inventory done. Next iteration starts **Slice 1** (foundation + auth/login).
