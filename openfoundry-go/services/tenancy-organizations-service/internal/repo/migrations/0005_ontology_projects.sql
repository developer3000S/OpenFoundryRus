-- TO-3: ontology project schema (projects + memberships + resource bindings + folders).
-- Mirrors the Rust source in services/ontology-definition-service/migrations-pg/
-- 0001_ontology_schema_consolidated.sql so JSON wire shapes and relational
-- invariants stay byte-exact across the Go and Rust ports.

CREATE TABLE IF NOT EXISTS ontology_projects (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    workspace_slug TEXT NULL,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ontology_project_memberships (
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('viewer', 'editor', 'owner')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS ontology_project_resources (
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    resource_kind TEXT NOT NULL,
    resource_id UUID NOT NULL,
    bound_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (resource_kind, resource_id)
);

CREATE TABLE IF NOT EXISTS ontology_project_folders (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    parent_folder_id UUID NULL REFERENCES ontology_project_folders(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_projects_owner
    ON ontology_projects(owner_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_projects_workspace
    ON ontology_projects(workspace_slug);

CREATE INDEX IF NOT EXISTS idx_ontology_project_memberships_user
    ON ontology_project_memberships(user_id, role);

CREATE INDEX IF NOT EXISTS idx_ontology_project_resources_project
    ON ontology_project_resources(project_id, resource_kind, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_project_folders_project
    ON ontology_project_folders(project_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_ontology_project_folders_parent
    ON ontology_project_folders(parent_folder_id, created_at ASC);
