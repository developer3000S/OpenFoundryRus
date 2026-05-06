-- S8 / ADR-0030 — absorbed from `code-security-scanning-service`.
-- Schema relocated `pg-policy` → `pg-schemas` (single DSN with the
-- merged code-repository-review-service).

CREATE TABLE IF NOT EXISTS code_security_scans (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_code_security_scans_created_at ON code_security_scans(created_at);

CREATE TABLE IF NOT EXISTS code_security_findings (
    id UUID PRIMARY KEY,
    parent_id UUID NOT NULL REFERENCES code_security_scans(id) ON DELETE CASCADE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_code_security_findings_parent_id ON code_security_findings(parent_id);
