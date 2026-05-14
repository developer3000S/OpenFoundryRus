-- DF.14 — Schedule CRUD + Dataset Preview/Data Lineage sidebar metadata.
--
-- The initial schedule tables carried the durable trigger/target JSON. This
-- migration adds the indexed fields the CRUD/search surfaces need: folder,
-- branch, build strategy, all referenced resource RIDs, run-as identity and
-- last editor.

ALTER TABLE schedules
    ADD COLUMN IF NOT EXISTS folder_rid TEXT NULL,
    ADD COLUMN IF NOT EXISTS target_rids TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS branch TEXT NOT NULL DEFAULT 'master',
    ADD COLUMN IF NOT EXISTS build_strategy TEXT NOT NULL DEFAULT 'STALE_ONLY',
    ADD COLUMN IF NOT EXISTS run_as_identity TEXT NULL,
    ADD COLUMN IF NOT EXISTS last_updated_by TEXT NOT NULL DEFAULT 'system';

UPDATE schedules
SET
    target_rids = ARRAY(
        SELECT DISTINCT rid
        FROM unnest(ARRAY[
            trigger_json #>> '{kind,event,target_rid}',
            target_json #>> '{kind,pipeline_build,pipeline_rid}',
            target_json #>> '{kind,pipelineBuild,pipeline_rid}',
            target_json #>> '{kind,dataset_build,dataset_rid}',
            target_json #>> '{kind,datasetBuild,dataset_rid}',
            target_json #>> '{kind,sync_run,sync_rid}',
            target_json #>> '{kind,syncRun,sync_rid}',
            target_json #>> '{kind,sync_run,source_rid}',
            target_json #>> '{kind,syncRun,source_rid}',
            target_json #>> '{kind,health_check,check_rid}',
            target_json #>> '{kind,healthCheck,check_rid}'
        ]) AS refs(rid)
        WHERE rid IS NOT NULL AND rid <> ''
    ),
    branch = COALESCE(
        NULLIF(target_json #>> '{kind,pipeline_build,build_branch}', ''),
        NULLIF(target_json #>> '{kind,pipelineBuild,build_branch}', ''),
        NULLIF(target_json #>> '{kind,dataset_build,build_branch}', ''),
        NULLIF(target_json #>> '{kind,datasetBuild,build_branch}', ''),
        branch
    ),
    build_strategy = CASE
        WHEN COALESCE((target_json #>> '{kind,pipeline_build,force_build}')::boolean, FALSE)
          OR COALESCE((target_json #>> '{kind,dataset_build,force_build}')::boolean, FALSE)
        THEN 'FORCE'
        ELSE build_strategy
    END
WHERE cardinality(target_rids) = 0
   OR branch = 'master'
   OR build_strategy = 'STALE_ONLY';

CREATE INDEX IF NOT EXISTS idx_schedules_target_rids
    ON schedules USING GIN(target_rids);
CREATE INDEX IF NOT EXISTS idx_schedules_last_updated_by
    ON schedules(last_updated_by);
CREATE INDEX IF NOT EXISTS idx_schedules_branch
    ON schedules(branch);
CREATE INDEX IF NOT EXISTS idx_schedules_build_strategy
    ON schedules(build_strategy);
