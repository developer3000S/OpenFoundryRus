CREATE TABLE IF NOT EXISTS scenario_simulations (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_scenario_simulations_created_at ON scenario_simulations(created_at);

CREATE TABLE IF NOT EXISTS scenario_runs (
    id UUID PRIMARY KEY,
    parent_id UUID NOT NULL REFERENCES scenario_simulations(id) ON DELETE CASCADE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_scenario_runs_parent_id ON scenario_runs(parent_id);
