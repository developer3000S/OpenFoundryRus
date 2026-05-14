-- DF.8 versioned dataset schemas.
--
-- Schemas are metadata on dataset views. This migration keeps the legacy
-- `dataset_schemas` table as a fallback, but makes `dataset_view_schemas`
-- capable of answering Foundry v2 getSchema/putSchema requests with stable
-- schema version identifiers and end-transaction anchors.

ALTER TABLE dataset_view_schemas
    ADD COLUMN IF NOT EXISTS dataset_id UUID NULL REFERENCES datasets(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS branch TEXT,
    ADD COLUMN IF NOT EXISTS end_transaction_id UUID NULL REFERENCES dataset_transactions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS schema_version_id UUID,
    ADD COLUMN IF NOT EXISTS dataframe_reader TEXT NOT NULL DEFAULT 'PARQUET',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE dataset_view_schemas AS s
   SET dataset_id = v.dataset_id,
       branch = COALESCE(s.branch, v.source_branch),
       end_transaction_id = COALESCE(s.end_transaction_id, v.head_transaction_id),
       schema_version_id = COALESCE(s.schema_version_id, gen_random_uuid()),
       dataframe_reader = COALESCE(NULLIF(s.dataframe_reader, ''), upper(COALESCE(s.file_format, 'PARQUET'))),
       updated_at = COALESCE(s.updated_at, s.created_at)
  FROM dataset_views v
 WHERE v.id = s.view_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_dataset_view_schemas_version
    ON dataset_view_schemas(dataset_id, schema_version_id)
    WHERE dataset_id IS NOT NULL AND schema_version_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dataset_view_schemas_dataset_branch_txn
    ON dataset_view_schemas(dataset_id, branch, end_transaction_id)
    WHERE dataset_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dataset_view_schemas_updated
    ON dataset_view_schemas(dataset_id, updated_at DESC)
    WHERE dataset_id IS NOT NULL;

-- Recreate the commit trigger so schema rows created from transaction metadata
-- carry the same view-scoped versioning fields as API writes.
CREATE OR REPLACE FUNCTION fn_dataset_view_schemas_from_txn() RETURNS trigger AS $$
DECLARE
    v_view_id UUID;
    v_schema  JSONB;
    v_format  TEXT;
    v_meta    JSONB;
    v_hash    TEXT;
    v_name    TEXT;
    v_version UUID;
BEGIN
    IF NEW.status <> 'COMMITTED' OR OLD.status = 'COMMITTED' THEN
        RETURN NEW;
    END IF;

    v_schema := NEW.metadata -> 'schema';
    IF v_schema IS NULL OR jsonb_typeof(v_schema) <> 'object' THEN
        RETURN NEW;
    END IF;

    v_format := upper(COALESCE(v_schema ->> 'file_format', v_schema ->> 'dataframeReader', 'PARQUET'));
    IF v_format IN ('CSV','TSV','JSON','TEXT','DATASOURCE') THEN
        v_format := 'TEXT';
    END IF;
    IF v_format NOT IN ('PARQUET','AVRO','TEXT') THEN
        v_format := 'PARQUET';
    END IF;

    v_meta := v_schema -> 'custom_metadata';
    v_hash := md5(v_schema::text);
    v_name := '__manifest__' || COALESCE(NULLIF(NEW.branch_name, ''), 'branch') || '__' || NEW.id::text;
    v_version := gen_random_uuid();

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
            (view_id, dataset_id, branch, end_transaction_id, schema_json,
             file_format, custom_metadata, content_hash, schema_version_id,
             dataframe_reader, created_at, updated_at)
    VALUES  (v_view_id, NEW.dataset_id, NEW.branch_name, NEW.id, v_schema,
             v_format, v_meta, v_hash, v_version, v_format, NOW(), NOW())
    ON CONFLICT (view_id) DO UPDATE
        SET dataset_id          = EXCLUDED.dataset_id,
            branch              = EXCLUDED.branch,
            end_transaction_id  = EXCLUDED.end_transaction_id,
            schema_json         = EXCLUDED.schema_json,
            file_format         = EXCLUDED.file_format,
            custom_metadata     = EXCLUDED.custom_metadata,
            content_hash        = EXCLUDED.content_hash,
            schema_version_id   = EXCLUDED.schema_version_id,
            dataframe_reader    = EXCLUDED.dataframe_reader,
            updated_at          = NOW();

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
