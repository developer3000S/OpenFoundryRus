CREATE TABLE IF NOT EXISTS solution_diagrams (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_solution_diagrams_created_at ON solution_diagrams(created_at);

CREATE TABLE IF NOT EXISTS solution_references (
    id UUID PRIMARY KEY,
    parent_id UUID NOT NULL REFERENCES solution_diagrams(id) ON DELETE CASCADE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_solution_references_parent_id ON solution_references(parent_id);
