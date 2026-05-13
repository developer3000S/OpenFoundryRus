-- AIPLE.1: project/folder-managed AIP Logic file metadata.
-- Logic files deliberately require both project_id and folder_id so this
-- service does not model personal-home-only Logic placement.
CREATE TABLE IF NOT EXISTS logic_files (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    project_id UUID NOT NULL,
    folder_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    current_draft_version_id UUID NOT NULL,
    published_version_id UUID,
    execution_mode TEXT NOT NULL DEFAULT 'user_scoped'
        CHECK (execution_mode IN ('user_scoped', 'project_scoped')),
    permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (btrim(name) <> '')
);

CREATE INDEX IF NOT EXISTS idx_logic_files_project_folder
    ON logic_files(project_id, folder_id) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_logic_files_owner
    ON logic_files(owner_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_logic_files_updated
    ON logic_files(updated_at DESC);
