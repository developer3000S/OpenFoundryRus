-- Global branching schema (S8 / ADR-0030 — absorbed from
-- `global-branch-service`).
--
-- Mirrors Foundry "Global Branching application" + "Branch taskbar":
-- a *global branch* names a logical workstream that spans multiple
-- planes (datasets, ontology, pipelines, code repos). Each plane
-- still owns its own `*_branches` table; the global record carries
-- the cross-plane label and the link table associates a global
-- branch with the local branches of each resource.

CREATE TABLE IF NOT EXISTS global_branches (
    id                   UUID PRIMARY KEY,
    rid                  TEXT UNIQUE GENERATED ALWAYS AS
                            ('ri.foundry.main.globalbranch.' || id::text) STORED,
    name                 TEXT NOT NULL UNIQUE,
    parent_global_branch UUID NULL REFERENCES global_branches(id) ON DELETE SET NULL,
    description          TEXT NOT NULL DEFAULT '',
    created_by           TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at          TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_global_branches_parent
    ON global_branches (parent_global_branch);

-- One link per (global, resource_type, resource_rid). A global branch
-- can therefore reference at most one local branch *per resource*,
-- which matches the Foundry "Branching lifecycle" doc — a global
-- branch is a stable name for the workstream, not a 1:1 mapping to
-- multiple local branches of the same resource.
CREATE TABLE IF NOT EXISTS global_branch_resource_links (
    global_branch_id  UUID NOT NULL
                          REFERENCES global_branches(id) ON DELETE CASCADE,
    resource_type     TEXT NOT NULL,
    resource_rid      TEXT NOT NULL,
    branch_rid        TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'in_sync'
                          CHECK (status IN ('in_sync', 'drifted', 'archived')),
    last_synced_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (global_branch_id, resource_type, resource_rid)
);

CREATE INDEX IF NOT EXISTS idx_global_branch_links_branch_rid
    ON global_branch_resource_links (branch_rid);

-- Shared transactional outbox (ADR-0022). Idempotent — pg-schemas
-- bootstrap-sql is the canonical owner; this migration only ensures
-- the table exists for local dev / single-DB deployments.
CREATE SCHEMA IF NOT EXISTS outbox;
CREATE TABLE IF NOT EXISTS outbox.events (
    event_id     UUID PRIMARY KEY,
    aggregate    TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload      JSONB NOT NULL,
    headers      JSONB NOT NULL DEFAULT '{}'::jsonb,
    topic        TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
