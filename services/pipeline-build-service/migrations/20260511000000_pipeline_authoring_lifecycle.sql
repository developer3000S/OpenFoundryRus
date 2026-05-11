-- Pipeline Builder authoring lifecycle:
-- keep draft graph edits separate from the published graph used by builds,
-- and persist immutable history snapshots that can be restored.

ALTER TABLE pipelines
    ADD COLUMN IF NOT EXISTS branch_name TEXT NOT NULL DEFAULT 'main',
    ADD COLUMN IF NOT EXISTS draft_dag JSONB,
    ADD COLUMN IF NOT EXISTS published_dag JSONB,
    ADD COLUMN IF NOT EXISTS draft_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS active_version_id UUID,
    ADD COLUMN IF NOT EXISTS proposal_state TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS proposal_title TEXT,
    ADD COLUMN IF NOT EXISTS proposal_description TEXT;

UPDATE pipelines
SET draft_dag = COALESCE(draft_dag, dag),
    published_dag = CASE
        WHEN published_dag IS NOT NULL THEN published_dag
        WHEN status = 'active' THEN dag
        ELSE NULL
    END,
    draft_updated_at = COALESCE(draft_updated_at, updated_at),
    published_at = CASE
        WHEN published_at IS NOT NULL THEN published_at
        WHEN status = 'active' THEN updated_at
        ELSE NULL
    END
WHERE draft_dag IS NULL
   OR draft_updated_at IS NULL
   OR (status = 'active' AND (published_dag IS NULL OR published_at IS NULL));

CREATE TABLE IF NOT EXISTS pipeline_versions (
    id                       UUID PRIMARY KEY,
    pipeline_id              UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    version_number           BIGINT NOT NULL,
    branch_name              TEXT NOT NULL DEFAULT 'main',
    version_kind             TEXT NOT NULL DEFAULT 'draft',
    dag                      JSONB NOT NULL,
    name                     TEXT NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    schedule_config          JSONB NOT NULL DEFAULT '{}',
    retry_policy             JSONB NOT NULL DEFAULT '{"max_attempts": 1, "retry_on_failure": false, "allow_partial_reexecution": true}',
    created_by               UUID,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    message                  TEXT NOT NULL DEFAULT '',
    restored_from_version_id UUID REFERENCES pipeline_versions(id) ON DELETE SET NULL,
    UNIQUE (pipeline_id, version_number)
);

CREATE INDEX IF NOT EXISTS idx_pipeline_versions_pipeline
    ON pipeline_versions(pipeline_id, version_number DESC);

CREATE INDEX IF NOT EXISTS idx_pipeline_versions_branch
    ON pipeline_versions(pipeline_id, branch_name, version_number DESC);
