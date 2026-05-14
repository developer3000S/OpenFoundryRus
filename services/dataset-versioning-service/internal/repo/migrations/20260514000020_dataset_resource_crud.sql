-- DF.1 dataset resource CRUD metadata.
--
-- The original dataset catalog stored the runtime storage pointer and owner,
-- but did not persist the resource location/visibility attributes that make a
-- dataset browseable as a first-class Foundry-style resource. This migration
-- keeps the existing storage_path untouched and adds a logical resource path.

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS parent_folder_rid TEXT,
    ADD COLUMN IF NOT EXISTS folder_path TEXT,
    ADD COLUMN IF NOT EXISTS project_id TEXT,
    ADD COLUMN IF NOT EXISTS project_rid TEXT,
    ADD COLUMN IF NOT EXISTS path TEXT,
    ADD COLUMN IF NOT EXISTS resource_visibility TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE datasets
   SET parent_folder_rid = 'ri.openfoundry.main.folder.root'
 WHERE parent_folder_rid IS NULL OR BTRIM(parent_folder_rid) = '';

UPDATE datasets
   SET folder_path = '/datasets'
 WHERE folder_path IS NULL OR BTRIM(folder_path) = '';

UPDATE datasets
   SET project_id = 'default'
 WHERE project_id IS NULL OR BTRIM(project_id) = '';

UPDATE datasets
   SET project_rid = 'ri.openfoundry.main.project.default'
 WHERE project_rid IS NULL OR BTRIM(project_rid) = '';

UPDATE datasets
   SET resource_visibility = 'private'
 WHERE resource_visibility IS NULL OR BTRIM(resource_visibility) = '';

UPDATE datasets
   SET path = CASE
        WHEN folder_path = '/' THEN '/' || name
        ELSE REGEXP_REPLACE(folder_path, '/+$', '') || '/' || name
       END
 WHERE path IS NULL OR BTRIM(path) = '';

ALTER TABLE datasets
    ALTER COLUMN parent_folder_rid SET DEFAULT 'ri.openfoundry.main.folder.root',
    ALTER COLUMN parent_folder_rid SET NOT NULL,
    ALTER COLUMN folder_path SET DEFAULT '/datasets',
    ALTER COLUMN folder_path SET NOT NULL,
    ALTER COLUMN project_id SET DEFAULT 'default',
    ALTER COLUMN project_id SET NOT NULL,
    ALTER COLUMN project_rid SET DEFAULT 'ri.openfoundry.main.project.default',
    ALTER COLUMN project_rid SET NOT NULL,
    ALTER COLUMN path SET NOT NULL,
    ALTER COLUMN resource_visibility SET DEFAULT 'private',
    ALTER COLUMN resource_visibility SET NOT NULL;

ALTER TABLE datasets
    DROP CONSTRAINT IF EXISTS chk_datasets_resource_visibility;

ALTER TABLE datasets
    ADD CONSTRAINT chk_datasets_resource_visibility
        CHECK (resource_visibility IN ('private', 'shared', 'organization', 'public'));

CREATE INDEX IF NOT EXISTS idx_datasets_parent_folder
    ON datasets(parent_folder_rid)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_datasets_project
    ON datasets(project_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_datasets_folder_path
    ON datasets(folder_path)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_datasets_deleted_at
    ON datasets(deleted_at)
    WHERE deleted_at IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_datasets_parent_folder_name_active
    ON datasets(parent_folder_rid, LOWER(name))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_datasets_path_active
    ON datasets(path)
    WHERE deleted_at IS NULL;
