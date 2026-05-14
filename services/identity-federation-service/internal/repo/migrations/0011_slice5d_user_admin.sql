-- identity-federation-service slice 5d — SG.4 user-administration
-- columns.
--
-- Slice 1 stored the minimal user shape (email + password_hash +
-- is_active). SG.4 requires an admin surface that can list, search,
-- preregister, activate/inactivate, soft-delete/undelete, and
-- inspect users. To support that we add:
--
--   username       — login handle distinct from email (defaults to
--                    the email-localpart of the user, stays unique
--                    case-insensitively).
--   realm          — IdP/realm the user is sourced from (e.g.
--                    'local', 'okta', 'acme-saml'). Mirrors the
--                    Foundry "Realm" concept used in group-management.
--   last_login_at  — most recent successful login. Stamped from the
--                    Login handler in a follow-up patch alongside
--                    the existing IssueTokens call.
--   last_login_ip  — companion IP for the most recent login.
--   deleted_at     — soft-delete tombstone. Live queries filter on
--                    deleted_at IS NULL; the admin "Show inactive"
--                    surface drops the filter.
--   preregistered  — true when a row was created by an admin invite
--                    that has not yet completed self-service signup
--                    or SSO bind. Password hash is the empty string.
--   invited_by     — UUID of the admin that preregistered the user.
--
-- Schema is additive; existing rows backfill username from the
-- email-localpart and realm from auth_source.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS username      TEXT NULL,
    ADD COLUMN IF NOT EXISTS realm         TEXT NOT NULL DEFAULT 'local',
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS last_login_ip TEXT NULL,
    ADD COLUMN IF NOT EXISTS deleted_at    TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS preregistered BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS invited_by    UUID NULL;

-- Backfill: username = email-localpart, realm = auth_source.
UPDATE users
   SET username = COALESCE(username, split_part(email, '@', 1))
 WHERE username IS NULL;

UPDATE users
   SET realm = auth_source
 WHERE realm = 'local' AND auth_source NOT IN ('local', '');

-- Unique by lower(username); allow NULL during the brief migration
-- window (a row inserted between the ADD COLUMN and the backfill).
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx
    ON users (LOWER(username))
    WHERE username IS NOT NULL;

CREATE INDEX IF NOT EXISTS users_deleted_at_idx ON users (deleted_at);
CREATE INDEX IF NOT EXISTS users_realm_idx ON users (realm);
