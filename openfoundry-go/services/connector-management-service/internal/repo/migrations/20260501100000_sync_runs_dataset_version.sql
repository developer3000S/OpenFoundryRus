-- Tarea 8 — Versionado de datasets integrado al pipeline de ingesta.
--
-- After a successful sync_run, connector-management-service registers a
-- dataset version with dataset-versioning-service (POST
-- /api/v1/datasets/{id}/append) using a deterministic content hash. The
-- resulting DatasetVersion id and the hash that produced it are persisted
-- here so that a re-run with the same content can reuse the previous
-- version (idempotent commit, mirroring Foundry's "no-op transaction"
-- semantics).
ALTER TABLE sync_runs
    ADD COLUMN IF NOT EXISTS dataset_version_id UUID,
    ADD COLUMN IF NOT EXISTS content_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_sync_runs_def_hash
    ON sync_runs (sync_def_id, content_hash);
