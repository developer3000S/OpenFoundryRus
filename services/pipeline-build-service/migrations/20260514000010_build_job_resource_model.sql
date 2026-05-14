-- DF.11 — Build and job resource model.
--
-- Builds are one-time computations over an explicit target dataset set.
-- Jobs are immutable-JobSpec executions that can update one or more output
-- datasets as a single unit. The executor already rolls back every output
-- transaction if any output commit fails; these columns make that resource
-- contract visible and queryable from the API.

ALTER TABLE builds
    ADD COLUMN IF NOT EXISTS target_dataset_rids TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_builds_target_dataset_rids_gin
    ON builds USING GIN (target_dataset_rids);

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS input_dataset_rids TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS output_dataset_rids TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS logic_kind TEXT NULL,
    ADD COLUMN IF NOT EXISTS job_spec_content_hash TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_jobs_output_dataset_rids_gin
    ON jobs USING GIN (output_dataset_rids);
CREATE INDEX IF NOT EXISTS idx_jobs_input_dataset_rids_gin
    ON jobs USING GIN (input_dataset_rids);
CREATE INDEX IF NOT EXISTS idx_jobs_logic_kind
    ON jobs (logic_kind);

-- Future lookup rows carry the immutable JobSpec RID inside job_spec_json so
-- every output of a multi-output spec resolves to the same shared logic unit.
UPDATE pipeline_job_specs
SET job_spec_json = jsonb_set(job_spec_json, '{rid}', to_jsonb(rid), true)
WHERE NOT (job_spec_json ? 'rid');

CREATE OR REPLACE VIEW job_output_atomicity AS
SELECT
    job_id,
    COUNT(*)::INT AS total_outputs,
    COUNT(*) FILTER (WHERE committed)::INT AS committed_outputs,
    COUNT(*) FILTER (WHERE aborted)::INT AS aborted_outputs,
    CASE
        WHEN COUNT(*) = 0 THEN 'NO_OUTPUTS'
        WHEN COUNT(*) FILTER (WHERE committed) = COUNT(*) THEN 'COMMITTED'
        WHEN COUNT(*) FILTER (WHERE aborted) = COUNT(*) THEN 'ABORTED'
        WHEN COUNT(*) FILTER (WHERE committed) = 0
             AND COUNT(*) FILTER (WHERE aborted) = 0 THEN 'OPEN'
        ELSE 'PARTIAL'
    END AS atomic_commit_status
FROM job_outputs
GROUP BY job_id;

COMMENT ON VIEW job_output_atomicity IS
    'DF.11 invariant monitor: multi-output jobs should be OPEN, COMMITTED, ABORTED, or NO_OUTPUTS; PARTIAL indicates a failed atomic update/rollback path.';
