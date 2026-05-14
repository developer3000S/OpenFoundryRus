-- DF.12 Build staleness resolution.
--
-- The core freshness columns were introduced with multi-output jobs. This
-- migration adds the lookup shape used by build resolution and documents the
-- "ignored because fresh" terminal path exposed in build/schedule histories.

CREATE INDEX IF NOT EXISTS idx_jobs_freshness_lookup
    ON jobs (job_spec_rid, state, stale_skipped, canonical_logic_hash, input_signature);

CREATE INDEX IF NOT EXISTS idx_job_outputs_freshness_commits
    ON job_outputs (job_id, output_dataset_rid, committed);

COMMENT ON COLUMN jobs.input_signature IS
    'DF.12: hash of resolved external input heads/schemas and internal producer logic used to determine whether an output is fresh.';

COMMENT ON COLUMN jobs.canonical_logic_hash IS
    'DF.12: stable hash of the JobSpec logic used with input_signature for staleness resolution.';

COMMENT ON COLUMN jobs.stale_skipped IS
    'DF.12: true when the job was ignored because its output was fresh and no recomputation was required.';
