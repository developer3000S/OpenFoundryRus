-- DF.2 dataset file browser and logical path model.
--
-- `dataset_files` already separates the Foundry-visible logical path from the
-- backing physical URI. This migration adds the remaining browser/API metadata
-- needed to inspect and mutate files through open transactions.

ALTER TABLE dataset_transaction_files
    ADD COLUMN IF NOT EXISTS physical_uri TEXT,
    ADD COLUMN IF NOT EXISTS media_type TEXT,
    ADD COLUMN IF NOT EXISTS sha256 TEXT,
    ADD COLUMN IF NOT EXISTS row_count_hint BIGINT,
    ADD COLUMN IF NOT EXISTS storage_location JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE dataset_files
    ADD COLUMN IF NOT EXISTS media_type TEXT,
    ADD COLUMN IF NOT EXISTS row_count_hint BIGINT,
    ADD COLUMN IF NOT EXISTS storage_location JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE dataset_files
   SET storage_location = storage_location || jsonb_build_object('uri', physical_uri)
 WHERE storage_location = '{}'::jsonb;

UPDATE dataset_transaction_files
   SET physical_uri = CASE
        WHEN COALESCE(physical_uri, '') <> '' THEN physical_uri
        WHEN physical_path LIKE 'local:///%' OR physical_path LIKE 's3://%' OR physical_path LIKE 'hdfs://%' THEN physical_path
        WHEN COALESCE(physical_path, '') <> '' THEN 'local:///' || trim(both '/' from physical_path)
        ELSE physical_uri
       END
 WHERE physical_uri IS NULL OR BTRIM(physical_uri) = '';

CREATE INDEX IF NOT EXISTS idx_dataset_files_media_type
    ON dataset_files(dataset_id, media_type)
    WHERE deleted_at IS NULL;

CREATE OR REPLACE FUNCTION fn_dataset_files_from_committed_txn() RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'COMMITTED' OR OLD.status = 'COMMITTED' THEN
        RETURN NEW;
    END IF;

    INSERT INTO dataset_files (
            id, dataset_id, transaction_id, logical_path,
            physical_uri, size_bytes, sha256, media_type, row_count_hint,
            storage_location, deleted_at, created_at)
    SELECT
        gen_random_uuid(),
        NEW.dataset_id,
        NEW.id,
        f.logical_path,
        CASE
            WHEN COALESCE(f.physical_uri, '') <> '' THEN f.physical_uri
            WHEN f.physical_path LIKE 'local:///%' OR f.physical_path LIKE 's3://%' OR f.physical_path LIKE 'hdfs://%' THEN f.physical_path
            WHEN COALESCE(f.physical_path, '') <> '' THEN 'local:///' || trim(both '/' from f.physical_path)
            ELSE 'local:///' || NEW.id::text || '/' || trim(both '/' from f.logical_path)
        END,
        f.size_bytes,
        f.sha256,
        f.media_type,
        f.row_count_hint,
        COALESCE(f.storage_location, '{}'::jsonb) || jsonb_build_object(
            'uri',
            CASE
                WHEN COALESCE(f.physical_uri, '') <> '' THEN f.physical_uri
                WHEN f.physical_path LIKE 'local:///%' OR f.physical_path LIKE 's3://%' OR f.physical_path LIKE 'hdfs://%' THEN f.physical_path
                WHEN COALESCE(f.physical_path, '') <> '' THEN 'local:///' || trim(both '/' from f.physical_path)
                ELSE 'local:///' || NEW.id::text || '/' || trim(both '/' from f.logical_path)
            END,
            'logical_path', f.logical_path
        ),
        CASE WHEN f.op = 'REMOVE' THEN NOW() ELSE NULL END,
        NOW()
      FROM dataset_transaction_files f
     WHERE f.transaction_id = NEW.id
    ON CONFLICT (dataset_id, transaction_id, logical_path) DO UPDATE
      SET physical_uri = EXCLUDED.physical_uri,
          size_bytes = EXCLUDED.size_bytes,
          sha256 = EXCLUDED.sha256,
          media_type = EXCLUDED.media_type,
          row_count_hint = EXCLUDED.row_count_hint,
          storage_location = EXCLUDED.storage_location,
          deleted_at = EXCLUDED.deleted_at;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
