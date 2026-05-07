-- IRF-5: rotating stream views ("Reset stream" workflow).
--
-- Each stream owns 1..N rotating views. The most-recent generation with
-- `active = true` is the "current view"; resetting a stream retires the
-- current view (active = false, retired_at stamped) and inserts a fresh
-- one with generation + 1. Push consumers must POST against the current
-- `view_rid` — a rotated view rejects subsequent pushes.

CREATE TABLE IF NOT EXISTS streaming_stream_views (
    id UUID PRIMARY KEY,
    stream_rid TEXT NOT NULL,
    view_rid TEXT NOT NULL UNIQUE,
    schema_json JSONB NULL,
    config_json JSONB NULL,
    generation INTEGER NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_streaming_stream_views_stream_active
    ON streaming_stream_views(stream_rid, active, generation DESC);
CREATE INDEX IF NOT EXISTS idx_streaming_stream_views_view_rid
    ON streaming_stream_views(view_rid);

-- One active view per stream — enforced via partial unique index so
-- retired rows can pile up freely.
CREATE UNIQUE INDEX IF NOT EXISTS uq_streaming_stream_views_one_active
    ON streaming_stream_views(stream_rid)
    WHERE active = TRUE;
