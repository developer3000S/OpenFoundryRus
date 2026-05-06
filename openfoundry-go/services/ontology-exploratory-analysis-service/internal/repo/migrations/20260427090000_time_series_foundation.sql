CREATE TABLE IF NOT EXISTS time_series (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    value_kind TEXT NOT NULL DEFAULT 'numeric',
    unit TEXT NULL,
    granularity_seconds INTEGER NOT NULL DEFAULT 60,
    retention_days INTEGER NULL,
    source_kind TEXT NULL,
    source_ref TEXT NULL,
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS time_series_points (
    series_id UUID NOT NULL REFERENCES time_series(id) ON DELETE CASCADE,
    recorded_at TIMESTAMPTZ NOT NULL,
    value_numeric DOUBLE PRECISION NULL,
    value_text TEXT NULL,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (series_id, recorded_at)
);

CREATE INDEX IF NOT EXISTS idx_time_series_points_recorded
    ON time_series_points (series_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS time_series_storage_partitions (
    id UUID PRIMARY KEY,
    series_id UUID NOT NULL REFERENCES time_series(id) ON DELETE CASCADE,
    tier TEXT NOT NULL DEFAULT 'hot',
    partition_start TIMESTAMPTZ NOT NULL,
    partition_end TIMESTAMPTZ NOT NULL,
    storage_uri TEXT NULL,
    byte_size BIGINT NULL,
    point_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (series_id, partition_start)
);
