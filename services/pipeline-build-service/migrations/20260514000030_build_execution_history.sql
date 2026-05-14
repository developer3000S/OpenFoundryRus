-- DF.13 Build execution status, logs, and history.
--
-- These columns denormalize the execution report fields shown in build/job
-- history while keeping job_state_transitions and job_logs as the append-only
-- sources for timeline and live log reconstruction.

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS runtime TEXT NULL,
    ADD COLUMN IF NOT EXISTS worker_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS row_count BIGINT NULL,
    ADD COLUMN IF NOT EXISTS file_count BIGINT NULL,
    ADD COLUMN IF NOT EXISTS output_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE jobs j
SET started_at = COALESCE(j.started_at, t.started_at),
    finished_at = COALESCE(j.finished_at, t.finished_at)
FROM (
    SELECT
        job_id,
        MIN(occurred_at) FILTER (WHERE to_state = 'RUNNING') AS started_at,
        MIN(occurred_at) FILTER (WHERE to_state IN ('COMPLETED','FAILED','ABORTED')) AS finished_at
    FROM job_state_transitions
    GROUP BY job_id
) t
WHERE j.id = t.job_id;

CREATE INDEX IF NOT EXISTS idx_jobs_execution_status_started
    ON jobs (state, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_jobs_runtime
    ON jobs (runtime);

COMMENT ON COLUMN jobs.started_at IS
    'DF.13: first observed RUNNING timestamp for build execution history.';

COMMENT ON COLUMN jobs.finished_at IS
    'DF.13: first terminal timestamp for build execution history.';

COMMENT ON COLUMN jobs.runtime IS
    'DF.13: runtime family reported by the worker, for example lightweight_table, python, distributed, or llm.';

COMMENT ON COLUMN jobs.worker_id IS
    'DF.13: worker/runtime executor identifier reported by the job result.';

COMMENT ON COLUMN jobs.row_count IS
    'DF.13: rows produced or affected by the job when the runner reports them.';

COMMENT ON COLUMN jobs.file_count IS
    'DF.13: output file/transaction count reported or inferred during commit.';

COMMENT ON COLUMN jobs.output_metadata IS
    'DF.13: compact persisted job result metadata used by build detail and history views.';
