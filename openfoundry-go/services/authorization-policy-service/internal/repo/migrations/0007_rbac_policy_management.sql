-- RBAC + tenant-scoped policy-management foundation.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- These tables mirror the Rust authorization-policy-service slice for
-- roles, groups, permissions, user role assignment, group membership, and
-- group role grants. tenant_id is nullable for bootstrap/global rows; when
-- claims carry org_id, handlers only read/write rows for that tenant.

ALTER TABLE cedar_policies
    ADD COLUMN IF NOT EXISTS tenant_id UUID NULL;

CREATE INDEX IF NOT EXISTS idx_cedar_policies_tenant_active
    ON cedar_policies (tenant_id, active)
    WHERE active = TRUE;

CREATE TABLE IF NOT EXISTS permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NULL,
    resource    TEXT NOT NULL,
    action      TEXT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT permissions_tenant_resource_action_unique UNIQUE (tenant_id, resource, action)
);

CREATE UNIQUE INDEX IF NOT EXISTS permissions_global_resource_action_unique
    ON permissions (resource, action)
    WHERE tenant_id IS NULL;

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NULL,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roles_tenant_name_unique UNIQUE (tenant_id, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS roles_global_name_unique
    ON roles (name)
    WHERE tenant_id IS NULL;

CREATE TABLE IF NOT EXISTS groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NULL,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT groups_tenant_name_unique UNIQUE (tenant_id, name)
);

CREATE UNIQUE INDEX IF NOT EXISTS groups_global_name_unique
    ON groups (name)
    WHERE tenant_id IS NULL;

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS group_roles (
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    role_id  UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, role_id)
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

INSERT INTO permissions (id, resource, action, description) VALUES
    ('0196c3f0-7f7f-7000-8000-000000000101', 'roles', 'read', 'List and read roles'),
    ('0196c3f0-7f7f-7000-8000-000000000102', 'roles', 'write', 'Create, update, and assign roles'),
    ('0196c3f0-7f7f-7000-8000-000000000103', 'groups', 'read', 'List and read groups'),
    ('0196c3f0-7f7f-7000-8000-000000000104', 'groups', 'write', 'Create, update, and manage group membership'),
    ('0196c3f0-7f7f-7000-8000-000000000105', 'permissions', 'read', 'List permission catalog'),
    ('0196c3f0-7f7f-7000-8000-000000000106', 'permissions', 'write', 'Manage permission catalog'),
    ('0196c3f0-7f7f-7000-8000-000000000107', 'policies', 'read', 'List and read policies'),
    ('0196c3f0-7f7f-7000-8000-000000000108', 'policies', 'write', 'Create, update, and delete policies')
ON CONFLICT (id) DO NOTHING;
