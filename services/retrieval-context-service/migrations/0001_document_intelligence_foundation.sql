CREATE TABLE IF NOT EXISTS document_intelligence_jobs (
    id UUID PRIMARY KEY,
    source_uri TEXT NOT NULL,
    mime_type TEXT,
    pipeline TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_doc_intel_jobs_status ON document_intelligence_jobs(status);
CREATE INDEX IF NOT EXISTS idx_doc_intel_jobs_pipeline ON document_intelligence_jobs(pipeline);

CREATE TABLE IF NOT EXISTS document_intelligence_status_events (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES document_intelligence_jobs(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_doc_intel_status_events_job_id ON document_intelligence_status_events(job_id);

CREATE TABLE IF NOT EXISTS document_intelligence_extractions (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES document_intelligence_jobs(id) ON DELETE CASCADE,
    extraction_kind TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence REAL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_doc_intel_extractions_job_id ON document_intelligence_extractions(job_id);
CREATE INDEX IF NOT EXISTS idx_doc_intel_extractions_kind ON document_intelligence_extractions(extraction_kind);
