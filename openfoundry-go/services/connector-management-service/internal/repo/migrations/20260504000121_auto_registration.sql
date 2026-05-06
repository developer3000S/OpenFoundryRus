-- Tarea D1.1.9 P4 — auto-registration scanner state.
--
-- Foundry baseline:
--   docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/
--   Core concepts/Virtual tables.md § "Auto-registration",
--   "Tag filtering for Databricks sources".
--
-- The scanner mirrors the source's catalog hierarchy into a Foundry-managed
-- project that is read-only for end-users. Two layout strategies are
-- supported:
--   * NESTED — `<project>/<database>/<schema>/<table>` folders.
--   * FLAT   — `<project>/<database>__<schema>__<table>` flat names.
--
-- For Databricks sources the scanner also intersects the discovered table
-- list with `auto_register_table_tag_filters` against the
-- `INFORMATION_SCHEMA.TABLE_TAGS` system table so a tenant can ship gold-
-- only / pii-only / classified-only mirrors without registering the entire
-- catalog.

ALTER TABLE virtual_table_sources_link
    ADD COLUMN IF NOT EXISTS auto_register_folder_mirror_kind TEXT NOT NULL
        DEFAULT 'NESTED'
        CHECK (auto_register_folder_mirror_kind IN ('FLAT', 'NESTED')),
    ADD COLUMN IF NOT EXISTS auto_register_table_tag_filters TEXT[] NOT NULL
        DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS auto_register_last_run_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS auto_register_last_run_added INT NULL,
    ADD COLUMN IF NOT EXISTS auto_register_last_run_updated INT NULL,
    ADD COLUMN IF NOT EXISTS auto_register_last_run_orphaned INT NULL;

CREATE TABLE IF NOT EXISTS auto_register_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_rid TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ NULL,
    -- One of: 'running', 'succeeded', 'failed'.
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'succeeded', 'failed')),
    added INT NOT NULL DEFAULT 0,
    updated INT NOT NULL DEFAULT 0,
    orphaned INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_auto_register_runs_source_started
    ON auto_register_runs (source_rid, started_at DESC);
