-- identity-federation-service slice 5c — SG.3 SSO provider persistence
-- and login-routing metadata.
--
-- Slice 5a wired OIDC at boot from env; slice 5b added SAML alongside.
-- Slice 5c adds the durable admin surface required by the security &
-- governance parity checklist item SG.3:
--
--   * Persistent storage for the in-memory SsoProvider row so an admin
--     can add / disable / update providers without a service restart.
--   * Per-provider email-domain allow-list so the login UI can route
--     a "Sign in with…" flow by the user's email domain.
--   * SAML metadata refresh metadata: last refresh timestamp and the
--     certificate's NotAfter so the troubleshooting endpoint can
--     surface "metadata stale" / "certificate expiring".
--
-- The OIDC service + SAML registry continue to be seeded from env at
-- boot; the DB rows are the admin source-of-truth that a follow-up
-- slice will hot-load. Schema is additive.

CREATE TABLE IF NOT EXISTS sso_providers (
    id                          UUID PRIMARY KEY,
    slug                        TEXT NOT NULL UNIQUE,
    name                        TEXT NOT NULL,
    provider_type               TEXT NOT NULL,
    enabled                     BOOLEAN NOT NULL DEFAULT TRUE,

    -- OIDC
    client_id                   TEXT NULL,
    client_secret               TEXT NULL,
    issuer_url                  TEXT NULL,
    authorization_url           TEXT NULL,
    token_url                   TEXT NULL,
    userinfo_url                TEXT NULL,
    scopes                      JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- SAML
    saml_metadata_url           TEXT NULL,
    saml_entity_id              TEXT NULL,
    saml_sso_url                TEXT NULL,
    saml_certificate            TEXT NULL,

    -- Claim mapping. The structured shape is documented in
    -- internal/models/sso.go (AttributeMapping). The column accepts
    -- any JSON object so legacy free-form mappings keep working.
    attribute_mapping           JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Email domains this provider claims (lower-case). Used by the
    -- login troubleshooting endpoint to recommend a provider given
    -- an email address.
    domains                     JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- SAML metadata-refresh diagnostics. NULL when no refresh has
    -- ever succeeded (or when the provider is OIDC-only).
    metadata_last_refreshed_at  TIMESTAMPTZ NULL,
    metadata_last_error         TEXT NULL,
    certificate_expires_at      TIMESTAMPTZ NULL,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT sso_providers_provider_type_check
        CHECK (provider_type IN ('oidc', 'saml'))
);

CREATE INDEX IF NOT EXISTS sso_providers_enabled_idx
    ON sso_providers (enabled);
