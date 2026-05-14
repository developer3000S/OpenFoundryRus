-- DF.7 dataset view calculation and manifest cache.
--
-- `dataset_views` historically carried user-authored/materialized SQL views
-- and later migrations also used it as the dataset-view schema anchor. This
-- migration makes that shared table explicit: named rows may still represent
-- authored views, while rows with `(branch_id, head_transaction_id)` cache the
-- deterministic file manifest reconstructed from committed transaction
-- history.

ALTER TABLE dataset_views
    ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sql_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_branch TEXT,
    ADD COLUMN IF NOT EXISTS source_version INT,
    ADD COLUMN IF NOT EXISTS materialized BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS refresh_on_source_update BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS format TEXT NOT NULL DEFAULT 'manifest',
    ADD COLUMN IF NOT EXISTS current_version INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS storage_path TEXT,
    ADD COLUMN IF NOT EXISTS row_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS schema_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS last_refreshed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS branch_id UUID NULL REFERENCES dataset_branches(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS head_transaction_id UUID NULL REFERENCES dataset_transactions(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS file_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE dataset_views
    ALTER COLUMN name SET DEFAULT '',
    ALTER COLUMN description SET DEFAULT '',
    ALTER COLUMN sql_text SET DEFAULT '',
    ALTER COLUMN materialized SET DEFAULT TRUE,
    ALTER COLUMN refresh_on_source_update SET DEFAULT FALSE,
    ALTER COLUMN format SET DEFAULT 'manifest',
    ALTER COLUMN current_version SET DEFAULT 0,
    ALTER COLUMN row_count SET DEFAULT 0,
    ALTER COLUMN schema_fields SET DEFAULT '[]'::jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS uq_dataset_views_dataset_branch_head
    ON dataset_views(dataset_id, branch_id, head_transaction_id);

CREATE INDEX IF NOT EXISTS idx_dataset_views_manifest_lookup
    ON dataset_views(dataset_id, branch_id, computed_at DESC)
    WHERE branch_id IS NOT NULL AND head_transaction_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS dataset_view_files (
    view_id       UUID NOT NULL REFERENCES dataset_views(id) ON DELETE CASCADE,
    logical_path  TEXT NOT NULL,
    physical_path TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    introduced_by UUID REFERENCES dataset_transactions(id) ON DELETE SET NULL,
    PRIMARY KEY (view_id, logical_path)
);

CREATE INDEX IF NOT EXISTS idx_dataset_view_files_introduced_by
    ON dataset_view_files(introduced_by)
    WHERE introduced_by IS NOT NULL;

-- Keep schema-trigger placeholder rows compatible with the shared
-- `dataset_views` shape. The file-list manifest remains lazily populated by
-- the application so it is always reconstructable from transaction history.
CREATE OR REPLACE FUNCTION fn_dataset_view_schemas_from_txn() RETURNS trigger AS $$
DECLARE
    v_view_id UUID;
    v_schema  JSONB;
    v_format  TEXT;
    v_meta    JSONB;
    v_hash    TEXT;
    v_name    TEXT;
BEGIN
    IF NEW.status <> 'COMMITTED' OR OLD.status = 'COMMITTED' THEN
        RETURN NEW;
    END IF;

    v_schema := NEW.metadata -> 'schema';
    IF v_schema IS NULL OR jsonb_typeof(v_schema) <> 'object' THEN
        RETURN NEW;
    END IF;

    v_format := upper(COALESCE(v_schema ->> 'file_format', 'PARQUET'));
    IF v_format IN ('CSV','TSV','JSON') THEN
        v_format := 'TEXT';
    END IF;
    IF v_format NOT IN ('PARQUET','AVRO','TEXT') THEN
        v_format := 'PARQUET';
    END IF;

    v_meta := v_schema -> 'custom_metadata';
    v_hash := md5(v_schema::text);
    v_name := '__manifest__' || COALESCE(NULLIF(NEW.branch_name, ''), 'branch') || '__' || NEW.id::text;

    INSERT INTO dataset_views
            (id, dataset_id, name, description, sql_text, source_branch,
             materialized, refresh_on_source_update, format, current_version,
             row_count, schema_fields, last_refreshed_at, branch_id,
             head_transaction_id, computed_at, file_count, size_bytes,
             metadata, created_at, updated_at)
    VALUES  (gen_random_uuid(), NEW.dataset_id, v_name, 'Cached dataset view manifest', '',
             NEW.branch_name, TRUE, FALSE, lower(v_format), 0, 0, '[]'::jsonb,
             NOW(), NEW.branch_id, NEW.id, NOW(), 0, 0,
             '{"kind":"transaction_history_manifest","reconstructable":true}'::jsonb,
             NOW(), NOW())
    ON CONFLICT (dataset_id, branch_id, head_transaction_id) DO UPDATE
        SET computed_at = NOW(),
            updated_at = NOW()
    RETURNING id INTO v_view_id;

    INSERT INTO dataset_view_schemas
            (view_id, schema_json, file_format, custom_metadata, content_hash, created_at)
    VALUES  (v_view_id, v_schema, v_format, v_meta, v_hash, NOW())
    ON CONFLICT (view_id) DO UPDATE
        SET schema_json     = EXCLUDED.schema_json,
            file_format     = EXCLUDED.file_format,
            custom_metadata = EXCLUDED.custom_metadata,
            content_hash    = EXCLUDED.content_hash;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
