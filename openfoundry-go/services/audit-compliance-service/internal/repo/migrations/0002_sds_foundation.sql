CREATE TABLE IF NOT EXISTS sds_scan_jobs (
    id UUID PRIMARY KEY,
    target_name TEXT NOT NULL,
    scope TEXT NOT NULL,
    status TEXT NOT NULL,
    risk_score INTEGER NOT NULL,
    findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    issue_count INTEGER NOT NULL DEFAULT 0,
    redacted_content TEXT NOT NULL DEFAULT '',
    remediations JSONB NOT NULL DEFAULT '[]'::jsonb,
    requested_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sds_issues (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES sds_scan_jobs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL,
    matched_value TEXT NOT NULL,
    redacted_value TEXT NOT NULL,
    match_count INTEGER NOT NULL DEFAULT 1,
    markings JSONB NOT NULL DEFAULT '[]'::jsonb,
    remediation_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sds_issues_job_id_created_at
    ON sds_issues (job_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sds_remediation_rules (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    match_conditions JSONB NOT NULL DEFAULT '[]'::jsonb,
    remediation_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO sds_remediation_rules (id, name, scope, match_conditions, remediation_actions, updated_by)
VALUES
    (
        '01968522-2f75-7f3a-bb5d-000000000401',
        'Mask PII Findings',
        'record',
        '[{"field":"kind","operator":"in","value":"email,ssn,credit_card"}]'::jsonb,
        '["mask_pii","notify_data_steward"]'::jsonb,
        NULL
    ),
    (
        '01968522-2f75-7f3a-bb5d-000000000402',
        'Revoke Exposed Credentials',
        'prompt',
        '[{"field":"kind","operator":"in","value":"api_key,bearer_token"}]'::jsonb,
        '["revoke_credential","rotate_secret","open_security_issue"]'::jsonb,
        NULL
    )
ON CONFLICT (id) DO NOTHING;
