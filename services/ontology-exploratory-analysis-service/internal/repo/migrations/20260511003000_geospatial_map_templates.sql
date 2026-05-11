-- GEO.13 — saved Map templates that Workshop can embed by rendering them into
-- Map widget props. Templates keep public-doc concepts only: parameters,
-- constant/styling object layers, overlay layers, viewport/interface options.
CREATE TABLE IF NOT EXISTS geospatial_map_templates (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parameters JSONB NOT NULL DEFAULT '[]'::jsonb,
    layers JSONB NOT NULL DEFAULT '[]'::jsonb,
    overlay_layers JSONB NOT NULL DEFAULT '[]'::jsonb,
    viewport JSONB NOT NULL DEFAULT '{}'::jsonb,
    interface_options JSONB NOT NULL DEFAULT '{}'::jsonb,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geospatial_map_templates_updated_at
    ON geospatial_map_templates (updated_at DESC);
