CREATE TABLE IF NOT EXISTS lineage_deletion_requests (
    id UUID PRIMARY KEY,
    dataset_id UUID NOT NULL,
    subject_id TEXT NULL,
    hard_delete BOOLEAN NOT NULL DEFAULT FALSE,
    legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
    impact JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    deleted_paths JSONB NOT NULL DEFAULT '[]'::jsonb,
    audit_trace JSONB NOT NULL DEFAULT '[]'::jsonb,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_lineage_deletion_dataset_requested_at
    ON lineage_deletion_requests (dataset_id, requested_at DESC);
