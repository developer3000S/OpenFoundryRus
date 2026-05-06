-- Tarea D1.1.9 P5 — Update detection for virtual table inputs.
--
-- Foundry baseline:
--   docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/
--   Core concepts/Virtual tables.md § "Update detection for virtual table
--   inputs", § "Viewing virtual table details".
--
-- The poller iterates `virtual_tables` rows with
-- `update_detection_enabled = true` and probes each source for the
-- current snapshot id / last commit / ETag. When the version changes
-- the row is bumped (`last_observed_version`, `last_polled_at`) and an
-- outbox event lands on `foundry.dataset.events.v1` so D1.1.6 P1's
-- trigger engine wakes any downstream schedule that registered an
-- `EventTrigger { type: DATA_UPDATED, target_rid: virtual_table.rid }`.

CREATE TABLE IF NOT EXISTS update_detection_polls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    virtual_table_id UUID NOT NULL
        REFERENCES virtual_tables(id) ON DELETE CASCADE,
    polled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Version observed on this poll. NULL when the source does not
    -- support versioning (CSV / Parquet / Avro plain object stores) —
    -- the poll is then treated as a *potential* update by downstream
    -- triggers.
    observed_version TEXT NULL,
    -- TRUE when `observed_version` differs from the previous poll.
    change_detected BOOLEAN NOT NULL DEFAULT FALSE,
    latency_ms INT NOT NULL DEFAULT 0,
    -- Free-form upstream error. Keeps the poll row even when the call
    -- failed so the operator can graph error rates by source.
    error_message TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_update_detection_polls_table_time
    ON update_detection_polls (virtual_table_id, polled_at DESC);

CREATE INDEX IF NOT EXISTS idx_update_detection_polls_change_detected
    ON update_detection_polls (virtual_table_id)
    WHERE change_detected;

-- P5 — backoff bookkeeping for the poller. Stored on the row instead
-- of an in-memory map so a service restart doesn't lose the backoff
-- state and start hammering a degraded source.
ALTER TABLE virtual_tables
    ADD COLUMN IF NOT EXISTS update_detection_consecutive_failures INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS update_detection_next_poll_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_virtual_tables_update_detection_due
    ON virtual_tables (update_detection_next_poll_at)
    WHERE update_detection_enabled;
