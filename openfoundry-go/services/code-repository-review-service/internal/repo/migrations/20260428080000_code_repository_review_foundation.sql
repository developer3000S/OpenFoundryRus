CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    default_branch TEXT NOT NULL DEFAULT 'main',
    visibility TEXT NOT NULL DEFAULT 'private',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_repositories_created_at ON repositories(created_at);

CREATE TABLE IF NOT EXISTS commits (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    sha TEXT NOT NULL,
    author TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repository_id, sha)
);

CREATE INDEX IF NOT EXISTS idx_commits_repository_id ON commits(repository_id);

CREATE TABLE IF NOT EXISTS merge_requests (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    source_branch TEXT NOT NULL,
    target_branch TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_merge_requests_repository_id ON merge_requests(repository_id);
CREATE INDEX IF NOT EXISTS idx_merge_requests_status ON merge_requests(status);

CREATE TABLE IF NOT EXISTS review_comments (
    id UUID PRIMARY KEY,
    merge_request_id UUID NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
    author TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_review_comments_mr_id ON review_comments(merge_request_id);

CREATE TABLE IF NOT EXISTS ci_runs (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    commit_sha TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ci_runs_repository_id ON ci_runs(repository_id);
CREATE INDEX IF NOT EXISTS idx_ci_runs_status ON ci_runs(status);
