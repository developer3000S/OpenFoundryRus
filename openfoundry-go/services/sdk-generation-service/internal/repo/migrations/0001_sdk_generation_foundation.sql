CREATE TABLE IF NOT EXISTS sdk_generation_jobs (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sdk_generation_jobs_created_at ON sdk_generation_jobs(created_at);

CREATE TABLE IF NOT EXISTS sdk_generation_publications (
    id UUID PRIMARY KEY,
    parent_id UUID NOT NULL REFERENCES sdk_generation_jobs(id) ON DELETE CASCADE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sdk_generation_publications_parent_id ON sdk_generation_publications(parent_id);
