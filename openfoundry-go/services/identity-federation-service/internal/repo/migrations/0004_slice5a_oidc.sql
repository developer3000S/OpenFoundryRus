-- identity-federation-service slice 5a — OIDC SSO state table.
--
-- Slice 5a does basic OIDC SSO (Google / Microsoft / GitHub / GitLab
-- via `coreos/go-oidc`). The Rust crate keeps OAuth pending state in
-- Cassandra `auth_runtime.oauth_state` (10 min TTL); the Go port keeps
-- it in Postgres until slice 2b lands the Cassandra wiring.
--
-- Slice 5b will add SAML (XML signing + assertion validation).

CREATE TABLE IF NOT EXISTS oauth_state (
    state          TEXT PRIMARY KEY,
    code_verifier  TEXT NOT NULL,
    provider       TEXT NOT NULL,
    redirect_after TEXT NOT NULL DEFAULT '/',
    nonce          TEXT NOT NULL,
    issued_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_oauth_state_expires ON oauth_state (expires_at);

-- External identity bindings: which IdP "subject" (the OIDC `sub`
-- claim, IdP-scoped) maps to which OpenFoundry user. One row per
-- (provider, external_id) pair so a user can chain multiple SSO
-- providers under one OpenFoundry account.
CREATE TABLE IF NOT EXISTS user_external_identities (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    email           TEXT,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, external_id)
);

CREATE INDEX IF NOT EXISTS idx_user_external_identities_user
    ON user_external_identities (user_id);
