-- DF.17 logical views over backing datasets.
--
-- Logical views are schema-backed resources that reference one or more backing
-- datasets. They do not own files; reads are reconstructed from the backing
-- dataset views, optionally deduplicated by a configured primary key.

ALTER TABLE dataset_views
    ADD COLUMN IF NOT EXISTS view_kind TEXT NOT NULL DEFAULT 'materialized',
    ADD COLUMN IF NOT EXISTS primary_key JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS auto_rebuild BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS transform_input_only BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE dataset_views
   SET view_kind = 'materialized'
 WHERE view_kind IS NULL OR view_kind = '';

CREATE TABLE IF NOT EXISTS dataset_view_backing_datasets (
    view_id           UUID NOT NULL REFERENCES dataset_views(id) ON DELETE CASCADE,
    dataset_id        UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    dataset_rid       TEXT NOT NULL DEFAULT '',
    branch            TEXT NOT NULL DEFAULT '',
    alias             TEXT NOT NULL DEFAULT '',
    position          INT NOT NULL DEFAULT 0,
    schema_version_id TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (view_id, dataset_id, branch)
);

CREATE INDEX IF NOT EXISTS idx_dataset_view_backing_datasets_dataset
    ON dataset_view_backing_datasets(dataset_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_dataset_view_backing_datasets_view_position
    ON dataset_view_backing_datasets(view_id, position ASC);
