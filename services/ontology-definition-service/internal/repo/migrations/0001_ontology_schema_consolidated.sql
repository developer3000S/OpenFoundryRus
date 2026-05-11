-- Consolidated schema for the ontology schema-of-types domain.
--
-- Per S1.6 of the Cassandra-Foundry parity plan, the historical
-- migrations under services/ontology-definition-service/migrations/
-- (now archived under docs/architecture/legacy-migrations/) are
-- collapsed into a single idempotent script applied by
-- pre-upgrade Helm jobs against the pg-schemas cluster.
--
-- This consolidated file intentionally carries only the declarative
-- ontology schema boundary that still belongs in `pg-schemas`.
-- Hot/runtime tables such as `object_instances`, `link_instances`
-- and execution/run ledgers remain archived under
-- `docs/architecture/legacy-migrations/` until their owning services
-- finish the Cassandra/runtime cut-over.
--
-- This file MUST run inside the ontology_schema schema; the
-- service's sqlx pool sets search_path=ontology_schema at the
-- connection level (see services/ontology-definition-service/src/db.rs).

CREATE SCHEMA IF NOT EXISTS ontology_schema;
SET search_path TO ontology_schema, public;

-- =====================================================================
-- Source: migrations/20260419100004_initial_ontology.sql
--         (archived; declarative subset retained below)
-- =====================================================================
-- Ontology: object types, properties and link types

CREATE TABLE IF NOT EXISTS object_types (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    plural_display_name TEXT,
    description TEXT NOT NULL DEFAULT '',
    primary_key_property TEXT,
    icon        TEXT,
    color       TEXT,
    editable    BOOLEAN NOT NULL DEFAULT FALSE,
    backing_dataset_id UUID,
    backing_dataset_rid TEXT,
    pipeline_rid TEXT,
    managed_by TEXT,
    owner_id    UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE object_types
    ADD COLUMN IF NOT EXISTS plural_display_name TEXT,
    ADD COLUMN IF NOT EXISTS editable BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS backing_dataset_id UUID,
    ADD COLUMN IF NOT EXISTS backing_dataset_rid TEXT,
    ADD COLUMN IF NOT EXISTS pipeline_rid TEXT,
    ADD COLUMN IF NOT EXISTS managed_by TEXT;

CREATE TABLE IF NOT EXISTS properties (
    id               UUID PRIMARY KEY,
    object_type_id   UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    display_name     TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    property_type    TEXT NOT NULL,
    required         BOOLEAN NOT NULL DEFAULT FALSE,
    unique_constraint BOOLEAN NOT NULL DEFAULT FALSE,
    default_value    JSONB,
    validation_rules JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (object_type_id, name)
);

CREATE TABLE IF NOT EXISTS link_types (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    source_type_id  UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    target_type_id  UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    cardinality     TEXT NOT NULL DEFAULT 'many_to_many',
    owner_id        UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name, source_type_id, target_type_id)
);

CREATE INDEX IF NOT EXISTS idx_properties_object_type ON properties(object_type_id);
CREATE INDEX IF NOT EXISTS idx_link_types_source ON link_types(source_type_id);
CREATE INDEX IF NOT EXISTS idx_link_types_target ON link_types(target_type_id);

-- `object_instances` and `link_instances` are runtime state owned by the
-- object database / query / actions path. They are intentionally excluded
-- from `pg-schemas`; see the archived legacy DDL plus the Cassandra tables
-- under `object-database-service` for their current owner path.

-- =====================================================================
-- Source: docs/architecture/legacy-migrations/ontology-actions-service/
--         20260423113000_action_types.sql
--         20260426001500_action_type_policies_and_what_if.sql
--         20260426113000_action_type_form_schema.sql
--         20260502000000_add_submission_criteria.sql
--         20260503000000_action_log.sql
--         20260504000000_action_executions_revert.sql
--         (archived; declarative subset retained below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS action_types (
    id                    UUID PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    display_name          TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    object_type_id        UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    operation_kind        TEXT NOT NULL,
    input_schema          JSONB NOT NULL DEFAULT '[]'::jsonb,
    config                JSONB NOT NULL DEFAULT 'null'::jsonb,
    confirmation_required BOOLEAN NOT NULL DEFAULT FALSE,
    permission_key        TEXT,
    authorization_policy  JSONB NOT NULL DEFAULT '{}'::jsonb,
    form_schema           JSONB NOT NULL DEFAULT '{}'::jsonb,
    submission_criteria   JSONB NOT NULL DEFAULT 'null'::jsonb,
    action_log_enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    action_log_summary_template TEXT,
    action_log_extra_property_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    action_log_object_type_id UUID,
    allow_revert_after_action_submission BOOLEAN NOT NULL DEFAULT TRUE,
    owner_id              UUID NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_action_types_object_type
    ON action_types(object_type_id);

CREATE INDEX IF NOT EXISTS idx_action_types_operation_kind
    ON action_types(operation_kind);

CREATE INDEX IF NOT EXISTS idx_action_types_authorization_policy
    ON action_types USING GIN (authorization_policy);

COMMENT ON COLUMN action_types.submission_criteria IS
    'Typed SubmissionNode AST evaluated server-side after auth checks and before plan_action commits. Null disables evaluation.';

-- `action_executions`, `action_execution_side_effects`, `action_log_records`
-- and `action_what_if_branches` are operational runtime tables. Only the
-- action type declaration stays in `pg-schemas`.

-- =====================================================================
-- Source: migrations/20260425003000_p3_semantic_runtime.sql
--         (archived; declarative subset retained below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_interfaces (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interface_properties (
    id UUID PRIMARY KEY,
    interface_id UUID NOT NULL REFERENCES ontology_interfaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    property_type TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    unique_constraint BOOLEAN NOT NULL DEFAULT FALSE,
    time_dependent BOOLEAN NOT NULL DEFAULT FALSE,
    default_value JSONB,
    validation_rules JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (interface_id, name)
);

CREATE TABLE IF NOT EXISTS object_type_interfaces (
    object_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    interface_id UUID NOT NULL REFERENCES ontology_interfaces(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (object_type_id, interface_id)
);

ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS time_dependent BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_interface_properties_interface
    ON interface_properties(interface_id);

CREATE INDEX IF NOT EXISTS idx_object_type_interfaces_object_type
    ON object_type_interfaces(object_type_id);

-- =====================================================================
-- Source: migrations/20260425233000_functions_rules_runtime.sql
--         (archived; declarative subset retained below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_function_packages (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    runtime TEXT NOT NULL,
    source TEXT NOT NULL,
    entrypoint TEXT NOT NULL DEFAULT 'handler',
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_function_packages_runtime
    ON ontology_function_packages(runtime);

CREATE TABLE IF NOT EXISTS ontology_rules (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    object_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    evaluation_mode TEXT NOT NULL DEFAULT 'advisory',
    trigger_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    effect_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_rules_object_type
    ON ontology_rules(object_type_id);

CREATE INDEX IF NOT EXISTS idx_ontology_rules_evaluation_mode
    ON ontology_rules(evaluation_mode);

-- `ontology_rule_runs` is runtime telemetry, not declarative schema.

-- =====================================================================
-- Source: migrations/20260425235900_shared_property_types.sql
--         (archived; preserved verbatim below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS shared_property_types (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    property_type TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    unique_constraint BOOLEAN NOT NULL DEFAULT FALSE,
    time_dependent BOOLEAN NOT NULL DEFAULT FALSE,
    default_value JSONB,
    validation_rules JSONB,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS object_type_shared_property_types (
    object_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    shared_property_type_id UUID NOT NULL REFERENCES shared_property_types(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (object_type_id, shared_property_type_id)
);

CREATE INDEX IF NOT EXISTS idx_shared_property_types_property_type
    ON shared_property_types(property_type);

CREATE INDEX IF NOT EXISTS idx_object_type_shared_property_types_object_type
    ON object_type_shared_property_types(object_type_id);

CREATE INDEX IF NOT EXISTS idx_object_type_shared_property_types_shared_property
    ON object_type_shared_property_types(shared_property_type_id);

-- =====================================================================
-- Source: migrations/20260426013000_machinery_queue.sql
--         (archived; runtime section intentionally excluded)
-- =====================================================================
-- `ontology_rule_schedules` belongs to runtime orchestration, not to the
-- declarative ontology schema boundary.

-- =====================================================================
-- Source: migrations/20260426023000_quiver_visual_functions.sql (archived; preserved verbatim below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_quiver_visual_functions (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    primary_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    secondary_type_id UUID REFERENCES object_types(id) ON DELETE SET NULL,
    join_field TEXT NOT NULL,
    secondary_join_field TEXT NOT NULL DEFAULT '',
    date_field TEXT NOT NULL,
    metric_field TEXT NOT NULL,
    group_field TEXT NOT NULL,
    selected_group TEXT,
    chart_kind TEXT NOT NULL DEFAULT 'line',
    shared BOOLEAN NOT NULL DEFAULT FALSE,
    vega_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT quiver_visual_chart_kind_valid CHECK (chart_kind IN ('line', 'area', 'bar', 'point'))
);

CREATE INDEX IF NOT EXISTS idx_quiver_visual_functions_owner
    ON ontology_quiver_visual_functions(owner_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_quiver_visual_functions_shared
    ON ontology_quiver_visual_functions(shared, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_quiver_visual_functions_primary_type
    ON ontology_quiver_visual_functions(primary_type_id);

-- =====================================================================
-- Source: migrations/20260426050000_object_sets_runtime.sql (archived; preserved verbatim below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_object_sets (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    base_object_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    filters JSONB NOT NULL DEFAULT '[]'::jsonb,
    traversals JSONB NOT NULL DEFAULT '[]'::jsonb,
    join_config JSONB,
    projections JSONB NOT NULL DEFAULT '[]'::jsonb,
    what_if_label TEXT,
    policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    materialized_snapshot JSONB,
    materialized_at TIMESTAMPTZ,
    materialized_row_count INTEGER NOT NULL DEFAULT 0,
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_object_sets_owner
    ON ontology_object_sets(owner_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_object_sets_base_type
    ON ontology_object_sets(base_object_type_id, updated_at DESC);

-- =====================================================================
-- Source: migrations/20260426124500_property_inline_edit_config.sql (archived; preserved verbatim below)
-- =====================================================================
ALTER TABLE properties
ADD COLUMN IF NOT EXISTS inline_edit_config JSONB;

-- =====================================================================
-- Source: migrations/20260426141500_function_package_versions.sql (archived; preserved verbatim below)
-- =====================================================================
ALTER TABLE ontology_function_packages
    ADD COLUMN IF NOT EXISTS version TEXT NOT NULL DEFAULT '0.1.0';

UPDATE ontology_function_packages
SET version = '0.1.0'
WHERE version IS NULL OR BTRIM(version) = '';

ALTER TABLE ontology_function_packages
    DROP CONSTRAINT IF EXISTS ontology_function_packages_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ontology_function_packages_name_version
    ON ontology_function_packages(name, version);

CREATE INDEX IF NOT EXISTS idx_ontology_function_packages_name
    ON ontology_function_packages(name);

-- =====================================================================
-- Source: migrations/20260426170000_function_package_run_metrics.sql
--         (archived; runtime section intentionally excluded)
-- =====================================================================
-- `ontology_function_package_runs` is runtime telemetry, not declarative
-- control-plane schema.

-- =====================================================================
-- Source: migrations/20260426200500_ontology_projects.sql (archived; preserved verbatim below)
-- =====================================================================
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

CREATE INDEX IF NOT EXISTS idx_ontology_projects_owner
    ON ontology_projects(owner_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_projects_workspace
    ON ontology_projects(workspace_slug);

CREATE INDEX IF NOT EXISTS idx_ontology_project_memberships_user
    ON ontology_project_memberships(user_id, role);

CREATE INDEX IF NOT EXISTS idx_ontology_project_resources_project
    ON ontology_project_resources(project_id, resource_kind, created_at DESC);

-- =====================================================================
-- Source: migrations/20260426213000_ontology_funnel.sql
--         (archived; declarative subset retained below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_funnel_sources (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    object_type_id UUID NOT NULL REFERENCES object_types(id) ON DELETE CASCADE,
    dataset_id UUID NOT NULL,
    pipeline_id UUID,
    dataset_branch TEXT,
    dataset_version INTEGER,
    preview_limit INTEGER NOT NULL DEFAULT 500,
    default_marking TEXT NOT NULL DEFAULT 'public',
    status TEXT NOT NULL DEFAULT 'active',
    property_mappings JSONB NOT NULL DEFAULT '[]'::jsonb,
    trigger_context JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_id UUID NOT NULL,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_funnel_sources_object_type
    ON ontology_funnel_sources(object_type_id);
CREATE INDEX IF NOT EXISTS idx_ontology_funnel_sources_dataset
    ON ontology_funnel_sources(dataset_id);
CREATE INDEX IF NOT EXISTS idx_ontology_funnel_sources_pipeline
    ON ontology_funnel_sources(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_ontology_funnel_sources_owner
    ON ontology_funnel_sources(owner_id);
CREATE INDEX IF NOT EXISTS idx_ontology_funnel_sources_status
    ON ontology_funnel_sources(status);

-- `ontology_funnel_runs` is execution history and stays outside
-- `pg-schemas`.

-- =====================================================================
-- Source: migrations/20260429104000_ontology_project_governance.sql (archived; preserved verbatim below)
-- =====================================================================
CREATE TABLE IF NOT EXISTS ontology_project_working_states (
    project_id UUID PRIMARY KEY REFERENCES ontology_projects(id) ON DELETE CASCADE,
    changes JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ontology_project_branches (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    proposal_id UUID NULL,
    changes JSONB NOT NULL DEFAULT '[]'::jsonb,
    conflict_resolutions JSONB NOT NULL DEFAULT '{}'::jsonb,
    enable_indexing BOOLEAN NOT NULL DEFAULT FALSE,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    latest_rebased_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE TABLE IF NOT EXISTS ontology_project_proposals (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES ontology_project_branches(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    reviewer_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    tasks JSONB NOT NULL DEFAULT '[]'::jsonb,
    comments JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'ontology_project_branches_proposal_fk'
  ) THEN
    ALTER TABLE ontology_project_branches
      ADD CONSTRAINT ontology_project_branches_proposal_fk
      FOREIGN KEY (proposal_id) REFERENCES ontology_project_proposals(id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ontology_project_migrations (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    source_project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    target_project_id UUID NOT NULL REFERENCES ontology_projects(id) ON DELETE CASCADE,
    resources JSONB NOT NULL DEFAULT '[]'::jsonb,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status TEXT NOT NULL DEFAULT 'planned',
    note TEXT NOT NULL DEFAULT '',
    submitted_by UUID NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ontology_project_branches_project
    ON ontology_project_branches(project_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_project_proposals_project
    ON ontology_project_proposals(project_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_ontology_project_migrations_project
    ON ontology_project_migrations(project_id, submitted_at DESC);

-- =====================================================================
-- Source: migrations/20260429104500_ontology_project_folders.sql (archived; preserved verbatim below)
-- =====================================================================
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

CREATE INDEX IF NOT EXISTS idx_ontology_project_folders_project
    ON ontology_project_folders(project_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_ontology_project_folders_parent
    ON ontology_project_folders(parent_folder_id, created_at ASC);

-- =====================================================================
-- Source: migrations/20260429123000_schema_bundles.sql (archived; preserved verbatim below)
-- =====================================================================
-- Schema bundle publication table for ontology-definition-service.
-- ontology-definition-service is the single source of truth for all
-- schema definitions (object types, properties, interfaces, link types,
-- action types, function packages, object-set definitions, funnel
-- definitions).  This table enables publishing versioned, compiled schema
-- snapshots to cells so that those cells can operate autonomously without
-- synchronous calls to the control plane.

-- ---------------------------------------------------------------------------
-- schema_bundle
-- ---------------------------------------------------------------------------
-- A versioned snapshot of the compiled schema for a given (workspace, cell).
--
-- version          – monotonically increasing integer within (workspace_id,
--                    cell_id).  Incremented on every schema-changing commit.
-- content          – full schema snapshot as JSONB, containing:
--                      object_types, properties, interfaces, link_types,
--                      action_types, function_packages, object_set_defs,
--                      funnel_source_defs
-- checksum         – SHA-256 hex of the serialized content for integrity
--                    verification at cell ingestion time
-- status           – 'draft' | 'published' | 'superseded'
--                    * draft: compiled but not yet distributed
--                    * published: distributed to the target cell
--                    * superseded: replaced by a newer version
-- published_at     – timestamp when status moved to 'published'
-- cell_id          – identifier of the target deployment cell; use 'core'
--                    for the primary cell in single-cell deployments
-- commit_message   – optional human-readable description of what changed
CREATE TABLE IF NOT EXISTS schema_bundle (
    id              UUID        PRIMARY KEY,
    workspace_id    UUID,
    cell_id         TEXT        NOT NULL DEFAULT 'core',
    version         BIGINT      NOT NULL CHECK (version >= 1),
    content         JSONB       NOT NULL,
    checksum        TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft', 'published', 'superseded')),
    commit_message  TEXT        NOT NULL DEFAULT '',
    created_by      UUID        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ,
    UNIQUE (workspace_id, cell_id, version)
);

CREATE INDEX IF NOT EXISTS idx_schema_bundle_workspace_cell
    ON schema_bundle (workspace_id, cell_id, status, version DESC);

CREATE INDEX IF NOT EXISTS idx_schema_bundle_status
    ON schema_bundle (status, cell_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_schema_bundle_published
    ON schema_bundle (published_at DESC)
    WHERE published_at IS NOT NULL;

-- =====================================================================
-- Source: migrations/20260501000100_workspace_soft_delete.sql (archived; preserved verbatim below)
-- =====================================================================
-- Phase 1 (B3 Workspace): soft-delete columns for ontology projects, folders
-- and resource bindings. Trash UX (`/trash`, restore, purge) reads these
-- columns; list endpoints must filter `WHERE is_deleted = false` going
-- forward. Hard delete remains available via the purge handler.

ALTER TABLE ontology_projects
    ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS deleted_by UUID NULL;

ALTER TABLE ontology_project_folders
    ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS deleted_by UUID NULL;

ALTER TABLE ontology_project_resources
    ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS deleted_by UUID NULL;

-- Partial indexes keep the trash query cheap without bloating the hot path.
CREATE INDEX IF NOT EXISTS idx_ontology_projects_trash
    ON ontology_projects (deleted_at DESC) WHERE is_deleted = TRUE;

CREATE INDEX IF NOT EXISTS idx_ontology_project_folders_trash
    ON ontology_project_folders (deleted_at DESC) WHERE is_deleted = TRUE;

CREATE INDEX IF NOT EXISTS idx_ontology_project_resources_trash
    ON ontology_project_resources (deleted_at DESC) WHERE is_deleted = TRUE;

-- =====================================================================
-- Source: migrations/20260501000200_folder_permissions.sql (archived; preserved verbatim below)
-- =====================================================================
-- Phase 1 (B3 Workspace): folder-level RBAC with explicit inheritance.
--
-- Default behaviour is implicit inheritance from the parent folder (and
-- ultimately from the project membership table). A row in this table marks
-- an *explicit* override at a specific folder; the application layer is
-- responsible for resolving the effective access level by walking the
-- folder hierarchy.

CREATE TABLE IF NOT EXISTS ontology_folder_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    folder_id UUID NOT NULL REFERENCES ontology_project_folders(id) ON DELETE CASCADE,
    -- Exactly one of (user_id, group_id) is populated. Enforced via CHECK.
    user_id UUID NULL,
    group_id UUID NULL,
    access_level TEXT NOT NULL CHECK (access_level IN ('viewer', 'editor', 'owner')),
    -- When TRUE, the override is propagated to every descendant folder
    -- unless they declare their own override. When FALSE, the override
    -- applies only to the named folder.
    inherited BOOLEAN NOT NULL DEFAULT TRUE,
    granted_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ontology_folder_permissions_principal
        CHECK ((user_id IS NOT NULL) <> (group_id IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ontology_folder_permissions_user
    ON ontology_folder_permissions (folder_id, user_id)
    WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ontology_folder_permissions_group
    ON ontology_folder_permissions (folder_id, group_id)
    WHERE group_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ontology_folder_permissions_folder
    ON ontology_folder_permissions (folder_id);
