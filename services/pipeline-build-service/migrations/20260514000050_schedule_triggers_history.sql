-- DF.15 — Schedule triggers and run history.
--
-- Extends the DF.14 CRUD/sidebar schema with dispatcher state needed to
-- evaluate time/data/logic/compound triggers, coalesce triggers while a prior
-- run is active, and preserve diagnostics on every schedule run.

ALTER TABLE schedules
    ADD COLUMN IF NOT EXISTS pending_trigger_snapshot JSONB NULL,
    ADD COLUMN IF NOT EXISTS last_triggered_at TIMESTAMPTZ NULL;

ALTER TABLE schedule_runs
    ADD COLUMN IF NOT EXISTS trigger_type TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS diagnostics JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_schedules_last_triggered
    ON schedules(last_triggered_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS idx_schedule_runs_build_rid
    ON schedule_runs(build_rid)
    WHERE build_rid IS NOT NULL;
