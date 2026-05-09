CREATE SCHEMA IF NOT EXISTS ontology_actions;
SET search_path TO ontology_actions;

CREATE TABLE IF NOT EXISTS object_types (
    id                  UUID PRIMARY KEY,
    name                TEXT NOT NULL,
    display_name        TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    primary_key_property TEXT,
    icon                TEXT,
    color               TEXT,
    owner_id            UUID NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS action_types (
    id                    UUID PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    display_name          TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    object_type_id        UUID NOT NULL,
    operation_kind        TEXT NOT NULL,
    input_schema          JSONB NOT NULL DEFAULT '[]'::jsonb,
    config                JSONB NOT NULL DEFAULT 'null'::jsonb,
    confirmation_required BOOLEAN NOT NULL DEFAULT FALSE,
    permission_key        TEXT,
    authorization_policy  JSONB NOT NULL DEFAULT '{}'::jsonb,
    form_schema           JSONB NOT NULL DEFAULT '{}'::jsonb,
    submission_criteria   JSONB NOT NULL DEFAULT 'null'::jsonb,
    action_log_enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    action_log_summary_template TEXT,
    action_log_extra_property_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    action_log_object_type_id UUID,
    allow_revert_after_action_submission BOOLEAN NOT NULL DEFAULT TRUE,
    owner_id              UUID NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

GRANT USAGE ON SCHEMA ontology_actions TO svc_ontology_actions;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ontology_actions TO svc_ontology_actions;
ALTER DEFAULT PRIVILEGES IN SCHEMA ontology_actions GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO svc_ontology_actions;
