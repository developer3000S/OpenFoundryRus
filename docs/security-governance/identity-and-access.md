# Identity and access

> **Sensitive admin surface.** Changes to identity, SSO, role administration,
> and token policy affect every other security layer. Read the
> [Security overview](./security-overview.md) for how this layer composes
> with markings, restricted views, and scoped sessions, and the
> [Shared responsibility model](./shared-responsibility-model.md) for the
> operator-vs-tenant boundary. Anything modeled on a Foundry concept must
> follow the [Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).

Identity and access are one of the strongest implemented capability areas in the current repo.

## Repository signals

`identity-federation-service` already exposes first-class support for:

- registration and login
- JWT access and refresh flows (with JWKS rotation)
- MFA + WebAuthn
- OIDC sign-in (SAML sign-in flow pending — see [ROADMAP](../../ROADMAP.md))
- SCIM provisioning
- session management
- user, role, group, and permission administration
- control-panel and admin-oriented surfaces

You can see the route surface in `services/identity-federation-service/cmd/identity-federation-service/main.go` and `services/identity-federation-service/internal/server/`.

## Domain building blocks

Relevant internal packages include:

- `internal/domain/jwt` — JWT issuance + validation, JWKS handling
- `internal/domain/rbac` — role-based access primitives
- `internal/domain/mfa` — MFA / WebAuthn enrollment and verification
- `internal/domain/saml` — SAML SP (sign-in flow pending)
- `internal/domain/oauth` — OAuth/OIDC provider integration
- `internal/domain/sessions` — session lifecycle, revocation, scoped claims
- `internal/domain/scim` — SCIM resources

ABAC primitives are owned by `services/authorization-policy-service` (Cedar engine) — see [Policies and authorization](./policies-and-authorization.md).

The shared HTTP layer (`libs/auth-middleware`) extracts claims into `r.Context()` so handlers never parse JWTs themselves; this is enforced by convention in [`CLAUDE.md`](../../CLAUDE.md) §"Conventions".

## SAML and OIDC provider administration (SG.3)

The boot-time OIDC service and SAML registry are seeded from environment
config; the **durable admin source-of-truth** lives in the
`sso_providers` Postgres table introduced in migration
[`0010_slice5c_sso_persistence.sql`](../../services/identity-federation-service/internal/repo/migrations/0010_slice5c_sso_persistence.sql).
A follow-up RFC will hot-load the in-memory registries from this table.

The admin surface is bearer-protected and gated by `authmw.RequireAdmin`:

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/auth/sso/providers` | List every persisted provider with secret fields masked (`client_secret_configured`, `saml_certificate_configured`). |
| `POST /api/v1/auth/sso/providers` | Register a new provider — accepts the full `CreateSsoProviderRequest` shape including `domains[]` and `attribute_mapping`. |
| `PATCH /api/v1/auth/sso/providers/{id}` | Update individual fields; missing keys preserve current values; explicit `null` clears pointer fields. |
| `DELETE /api/v1/auth/sso/providers/{id}` | Remove a row. |
| `POST /api/v1/auth/sso/providers/{id}/refresh-metadata` | For SAML providers, HTTP-GET the metadata URL via [`saml.ResolveMetadataDefaults`](../../services/identity-federation-service/internal/saml/metadata.go), update entity ID / SSO URL / certificate, and stamp `metadata_last_refreshed_at` / `certificate_expires_at`. |
| `GET /api/v1/auth/sso/providers/{id}/health` | Probe the issuer's `.well-known/openid-configuration` (OIDC) or the metadata URL (SAML) and report `overall_status` ∈ {ok, degraded, blocked} with certificate-expiry diagnostics. |

The **claim-mapping shape** is documented in
[`internal/models/sso.go`](../../services/identity-federation-service/internal/models/sso.go)
as `AttributeMapping`:

```jsonc
{
  "subject": "sub",          // claim name → external_id
  "email":   "email",        // claim name → user.email
  "name":    "name",         // claim name → user.name
  "attributes": {            // arbitrary IdP claim → user attribute
    "department": "department"
  },
  "groups": {
    "claim":         "groups",     // claim that carries the group list
    "idp_to_group": { "okta-eng": "eng" },
    "default_role":  "viewer"
  }
}
```

The OIDC and SAML callback flows continue to use the boot-time
defaults (`sub` / `email` / `name`); honouring the persisted mapping at
callback time is the SG.3 follow-up RFC.

## Login troubleshooting (SG.3)

`POST /api/v1/auth/sso/troubleshoot` is **unauthenticated** — the login
page calls it when the user can't get past the email step. The
response classifies the attempt with one of the stable wire constants
from
[`models.LoginTroubleshootState*`](../../services/identity-federation-service/internal/models/sso.go):

- `ok` — at least one healthy provider claims the email's domain.
- `unknown_domain` — no provider claims this email domain.
- `user_disabled` — the user exists but `is_active = false`.
- `provider_disabled` — the matched provider has `enabled = false`.
- `metadata_stale` — SAML metadata hasn't been refreshed in 30+ days.
- `certificate_expired` — SAML signing certificate's NotAfter is in the past.
- `certificate_expiring` — SAML certificate expires within 7 days.
- `configuration_error` — issuer or metadata URL is unreachable.

The `diagnostics[]` array carries one `{code, severity, message}` per
finding so the login UI can render translated, context-specific hints
without re-parsing the state string.

The Control Panel UI at
[`/control-panel/identity-providers`](../../apps/web/src/routes/control-panel/IdentityProvidersPage.tsx)
exposes the full admin surface plus the troubleshoot tool.

## User administration (SG.4)

The user row is the durable identity record. SG.4 (migration
[`0011_slice5d_user_admin.sql`](../../services/identity-federation-service/internal/repo/migrations/0011_slice5d_user_admin.sql))
extends the `users` table with `username`, `realm`, `last_login_at`,
`last_login_ip`, `deleted_at`, `preregistered`, and `invited_by`.
Existing rows backfill `username` from the email localpart and
`realm` from `auth_source`.

The admin surface lives at:

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/users` | Bare-array list of the most recent 200 non-deleted users (legacy SDK shape). |
| `GET /api/v1/users/search` | SG.4 envelope with `{items, total}` and `q` / `organization_id` / `realm` / `status` / `include_deleted` / `limit` / `offset` query params. |
| `GET /api/v1/users/{id}/inspect` | Combined view: user core + roles + groups + token summary + IdP bindings. |
| `POST /api/v1/users/preregister` | Admin-only. Seeds a row with `preregistered = true` and an empty password hash so SSO callback or self-service registration promotes the user. |
| `PATCH /api/v1/users/{id}` | Extended patch surface: name, username, realm, is_active, mfa_enforced, organization_id, attributes. Flipping `is_active` to false automatically revokes every active refresh token (SG.4 token-policy guarantee). |
| `DELETE /api/v1/users/{id}` | Soft-delete by default — sets `deleted_at`, inactivates the user, and revokes refresh tokens in one transaction. Pass `?hard=true` for true row removal (compliance flows). |
| `POST /api/v1/users/{id}/restore` | Clears the soft-delete tombstone. The user stays inactive until an admin re-activates them. |
| `POST /api/v1/users/{id}/revoke-tokens` | Explicit token revocation without changing the user's active state. |

**Login activity** is stamped from the Login handler and both SSO
callbacks (`handlers/auth.go`, `handlers/sso.go`) via
`Repo.StampLogin`. The IP picker honours `X-Forwarded-For` first hop
and falls back to `r.RemoteAddr` so the column reflects the original
client when the gateway terminates TLS.

**Token policy.** Deactivation, soft-delete, and the explicit revoke
endpoint all call `Repo.RevokeAllUserRefreshTokens(userID, time.Now())`.
The existing refresh-token rotation logic in
[`services/identity-federation-service/internal/service`](../../services/identity-federation-service/internal/service/)
keeps the per-rotation invariants; SG.4 only adds bulk revocation
hooks layered on top.

The Control Panel UI lives at
[`/control-panel/users`](../../apps/web/src/routes/control-panel/UsersPage.tsx)
— search bar, preregister form, per-row activate / deactivate /
soft-delete / restore / revoke-tokens, and an inspect side panel.

## Why this matters

This gives OpenFoundry a strong foundation for identity-aware operational workflows, not only for simple API authentication.
