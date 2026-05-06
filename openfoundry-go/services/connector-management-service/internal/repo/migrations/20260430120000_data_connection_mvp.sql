-- Data Connection MVP: credentials, egress policy bindings, batch sync defs and runs.
--
-- Schema is shaped by apps/web/src/lib/api/data-connection.ts. Egress policies
-- themselves are owned by network-boundary-service (table
-- network_boundary_policies); bindings here only carry the policy_id.

CREATE TABLE IF NOT EXISTS source_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id       UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('password','api_key','oauth_token','aws_keys','service_account_json')),
    -- Encrypted at rest (envelope encryption handled at the application layer
    -- before insert; the raw plaintext is never stored).
    secret_ciphertext BYTEA NOT NULL,
    -- Non-reversible fingerprint surfaced in the UI (sha256 of the plaintext).
    fingerprint     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_source_credentials_source ON source_credentials(source_id);

CREATE TABLE IF NOT EXISTS source_policy_bindings (
    source_id       UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    policy_id       UUID NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('direct','agent_proxy')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_id, policy_id)
);

CREATE INDEX IF NOT EXISTS idx_source_policy_bindings_policy ON source_policy_bindings(policy_id);

CREATE TABLE IF NOT EXISTS batch_sync_defs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id         UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    output_dataset_id UUID NOT NULL,
    file_glob         TEXT,
    schedule_cron     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_batch_sync_defs_source ON batch_sync_defs(source_id);

CREATE TABLE IF NOT EXISTS sync_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sync_def_id     UUID NOT NULL REFERENCES batch_sync_defs(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','running','succeeded','failed','aborted')),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    bytes_written   BIGINT NOT NULL DEFAULT 0,
    files_written   BIGINT NOT NULL DEFAULT 0,
    error           TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_runs_def ON sync_runs(sync_def_id);
CREATE INDEX IF NOT EXISTS idx_sync_runs_status ON sync_runs(status);
