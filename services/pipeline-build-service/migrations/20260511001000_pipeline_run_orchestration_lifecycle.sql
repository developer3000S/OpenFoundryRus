-- PB.11: normalize legacy pipeline_run statuses to the run lifecycle used by
-- the Pipeline Builder run UI. The typed BuildState projection still accepts
-- canonical BUILD_* values, but new pipeline_runs now persist this smaller
-- queue/runtime vocabulary directly.

UPDATE pipeline_runs
SET status = 'queued'
WHERE status IN ('pending', 'BUILD_QUEUED', 'BUILD_RESOLUTION');

UPDATE pipeline_runs
SET status = 'running'
WHERE status IN ('BUILD_RUNNING', 'BUILD_ABORTING');

UPDATE pipeline_runs
SET status = 'succeeded'
WHERE status IN ('completed', 'success', 'BUILD_COMPLETED');

UPDATE pipeline_runs
SET status = 'failed'
WHERE status IN ('BUILD_FAILED');

UPDATE pipeline_runs
SET status = 'cancelled'
WHERE status IN ('aborted', 'canceled', 'BUILD_ABORTED');
