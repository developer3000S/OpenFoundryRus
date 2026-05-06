CREATE TABLE IF NOT EXISTS tenancy_organizations (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    organization_type TEXT NOT NULL,
    default_workspace TEXT NULL,
    tenant_tier TEXT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenancy_enrollments (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES tenancy_organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    workspace_slug TEXT NULL,
    role_slug TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, user_id, workspace_slug, role_slug)
);
