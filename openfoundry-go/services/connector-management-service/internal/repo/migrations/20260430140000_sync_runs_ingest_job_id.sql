-- Track the ingest_job_id returned by ingestion-replication-service when
-- the sync run was materialised over gRPC. NULL means the bridge was not
-- invoked (URL unset or the call failed before the remote job id was issued).
ALTER TABLE sync_runs
    ADD COLUMN IF NOT EXISTS ingest_job_id TEXT;
