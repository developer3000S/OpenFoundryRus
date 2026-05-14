-- 0008: SG.2 — enrollment / organization / space model expansion.
--
-- Extends tenancy_organizations with the Foundry-parity metadata,
-- settings, quotas and contact surfaces, and introduces three new
-- tables:
--
--   tenancy_organization_admins   — administrators per organization
--   tenancy_organization_guests   — guest memberships from a different
--                                   primary organization
--   tenancy_spaces                — Foundry-style "space" (store /
--                                   administration boundary inside an
--                                   organization). Distinct from
--                                   nexus_spaces, which is a federation
--                                   peer space.
--
-- All schema is additive. Existing rows get the zero values from the
-- column defaults below.

ALTER TABLE tenancy_organizations
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS contact_email TEXT NULL,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS quotas JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS tenancy_organization_admins (
    organization_id UUID NOT NULL REFERENCES tenancy_organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    scope TEXT NOT NULL DEFAULT 'enrollment_admin',
    granted_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id, scope)
);

CREATE INDEX IF NOT EXISTS tenancy_organization_admins_user_idx
    ON tenancy_organization_admins (user_id);

CREATE TABLE IF NOT EXISTS tenancy_organization_guests (
    organization_id UUID NOT NULL REFERENCES tenancy_organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    primary_organization_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    invited_by UUID NULL,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS tenancy_organization_guests_user_idx
    ON tenancy_organization_guests (user_id);

CREATE INDEX IF NOT EXISTS tenancy_organization_guests_primary_idx
    ON tenancy_organization_guests (primary_organization_id);

CREATE TABLE IF NOT EXISTS tenancy_spaces (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES tenancy_organizations(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    quotas JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, slug)
);

CREATE INDEX IF NOT EXISTS tenancy_spaces_organization_idx
    ON tenancy_spaces (organization_id);
