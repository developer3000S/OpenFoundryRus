-- T4.1 — system policies & policy criteria
--
-- Adds the `is_system` flag (so the UI can render the "System policy"
-- badge from img_001.png) and optional `selector` / `criteria` JSONB
-- columns that capture the structured contract from the spec:
--
--   * selectors: by dataset_rid, by project_id, by marking_id
--   * criteria : transaction_age | transaction_state=ABORTED |
--                view_age        | last_accessed
--
-- The existing `rules: TEXT[]` (stored as JSONB) is kept for legacy
-- callers — the new fields are purely additive and default to `null`
-- so the migration is a no-op for already-seeded rows.

ALTER TABLE retention_policies
    ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS selector  JSONB   NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS criteria  JSONB   NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS grace_period_minutes INTEGER NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS last_applied_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS next_run_at     TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_retention_policies_active_system
    ON retention_policies (active, is_system);

-- The DELETE_ABORTED_TRANSACTIONS system policy (visible in
-- Datasets_assets/img_001.png). Acts on every dataset with no
-- additional selector and triggers when a transaction has been in the
-- ABORTED state — purges the orphaned files immediately after the
-- grace period.
INSERT INTO retention_policies (
    id, name, scope, target_kind, retention_days, legal_hold, purge_mode,
    rules, updated_by, active, is_system, selector, criteria,
    grace_period_minutes
)
VALUES (
    '01968522-2f75-7f3a-bb5d-000000000610',
    'DELETE_ABORTED_TRANSACTIONS',
    'system',
    'transaction',
    0,
    FALSE,
    'hard-delete-after-ttl',
    '["delete_aborted_transactions"]'::jsonb,
    'system',
    TRUE,
    TRUE,
    '{"all_datasets": true}'::jsonb,
    '{"transaction_state": "ABORTED"}'::jsonb,
    15
)
ON CONFLICT (id) DO UPDATE SET
    is_system  = EXCLUDED.is_system,
    selector   = EXCLUDED.selector,
    criteria   = EXCLUDED.criteria,
    grace_period_minutes = EXCLUDED.grace_period_minutes;
