CREATE TABLE IF NOT EXISTS retention_policies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '',
    target_kind TEXT NOT NULL,
    retention_days INTEGER NOT NULL,
    legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
    purge_mode TEXT NOT NULL,
    rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_by TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS retention_jobs (
    id UUID PRIMARY KEY,
    policy_id UUID NOT NULL REFERENCES retention_policies(id) ON DELETE CASCADE,
    target_dataset_id UUID NULL,
    target_transaction_id UUID NULL,
    status TEXT NOT NULL,
    action_summary TEXT NOT NULL,
    affected_record_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_retention_jobs_policy_created_at
    ON retention_jobs(policy_id, created_at DESC);

INSERT INTO retention_policies (
    id, name, scope, target_kind, retention_days, legal_hold, purge_mode, rules, updated_by, active
)
VALUES
    (
        '01968522-2f75-7f3a-bb5d-000000000601',
        'Dataset hot-window retention',
        'datasets.hot',
        'dataset',
        90,
        FALSE,
        'archive',
        '["prune_cold_versions","archive_historical_snapshots"]'::jsonb,
        'retention-policy-service',
        TRUE
    ),
    (
        '01968522-2f75-7f3a-bb5d-000000000602',
        'Transaction operational retention',
        'transactions.operational',
        'transaction',
        30,
        FALSE,
        'hard-delete-after-ttl',
        '["drop_committed_transaction_metadata","expire_non_lineage_transactions"]'::jsonb,
        'retention-policy-service',
        TRUE
    )
ON CONFLICT (id) DO NOTHING;
