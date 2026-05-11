CREATE TABLE IF NOT EXISTS inbound_listener_events (
    id UUID PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    listener_id TEXT NOT NULL,
    event_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('accepted', 'rejected')),
    signature_verified BOOLEAN NOT NULL DEFAULT FALSE,
    payload JSONB,
    headers JSONB,
    destination JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inbound_listener_events_source_created
    ON inbound_listener_events (source_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_inbound_listener_events_source_listener_created
    ON inbound_listener_events (source_id, listener_id, created_at DESC);
