-- Data Connection webhook invocation history.

CREATE TABLE IF NOT EXISTS webhook_invocation_history (
    id                    UUID PRIMARY KEY,
    source_id             UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    user_id               UUID NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('succeeded', 'failed')),
    http_status           INTEGER,
    input_policy          JSONB NOT NULL DEFAULT '{}',
    inputs                JSONB,
    output_parameters     JSONB,
    error                 TEXT,
    call_count            INTEGER NOT NULL DEFAULT 0,
    started_at            TIMESTAMPTZ NOT NULL,
    finished_at           TIMESTAMPTZ NOT NULL,
    duration_ms           BIGINT NOT NULL DEFAULT 0,
    retention_expires_at  TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_history_source_created
    ON webhook_invocation_history(source_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_history_expires
    ON webhook_invocation_history(retention_expires_at);
