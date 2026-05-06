-- connector-management-service · media set syncs.
--
-- Foundry contract ("Set up a media set sync.md"): S3 + ABFS sources can
-- back two flavours of media-set sync:
--
--   * MEDIA_SET_SYNC          → bytes are copied into Foundry storage
--                                via media-sets-service `upload-url`.
--   * VIRTUAL_MEDIA_SET_SYNC  → bytes stay in the source; Foundry only
--                                stores the metadata pointer via
--                                media-sets-service `virtual-items`.
--
-- The existing `batch_sync_defs` table tracks generic table-style syncs
-- and stays the source of truth for those. Media-set syncs get their own
-- table because they carry a different config shape (target media set
-- RID + Foundry filter taxonomy) and resolve to a different runtime
-- target (media-sets-service rather than the dataset-versioning sink).
--
-- The discriminator on `batch_sync_defs.sync_type` is added for
-- backward-compat reads ("which kind of sync is this?") even though new
-- media-set syncs never appear in that table.

ALTER TABLE batch_sync_defs
    ADD COLUMN IF NOT EXISTS sync_type TEXT NOT NULL DEFAULT 'TABLE_SYNC'
        CHECK (sync_type IN ('TABLE_SYNC', 'MEDIA_SET_SYNC', 'VIRTUAL_MEDIA_SET_SYNC'));

CREATE TABLE IF NOT EXISTS media_set_syncs (
    id                   UUID        PRIMARY KEY,
    source_id            UUID        NOT NULL,
    sync_type            TEXT        NOT NULL
                             CHECK (sync_type IN ('MEDIA_SET_SYNC', 'VIRTUAL_MEDIA_SET_SYNC')),
    target_media_set_rid TEXT        NOT NULL,
    subfolder            TEXT        NOT NULL DEFAULT '',
    -- Foundry sync filters (Set up a media set sync.md → step 7):
    --   { "exclude_already_synced": bool,
    --     "path_glob":              string,
    --     "file_size_limit":        i64,    -- bytes
    --     "ignore_unmatched_schema": bool }
    filters              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    schedule_cron        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_media_set_syncs_source
    ON media_set_syncs(source_id);
CREATE INDEX IF NOT EXISTS idx_media_set_syncs_target
    ON media_set_syncs(target_media_set_rid);
