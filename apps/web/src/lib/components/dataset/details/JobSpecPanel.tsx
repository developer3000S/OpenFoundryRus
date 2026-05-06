interface JobSpec {
  pipeline_id: string;
  pipeline_name: string;
  repo_url?: string;
  repo_path?: string;
  branch?: string;
  last_run_at?: string | null;
  last_run_status?: 'success' | 'failed' | 'running' | string;
}

interface JobSpecPanelProps {
  jobSpec: JobSpec | null;
}

const TONE: Record<string, { background: string; color: string }> = {
  success: { background: '#022c22', color: '#86efac' },
  failed: { background: '#7f1d1d', color: '#fecaca' },
  running: { background: '#1e3a8a', color: '#bfdbfe' },
};

function tone(s?: string) {
  return TONE[s ?? ''] ?? { background: '#1e293b', color: '#cbd5e1' };
}

export function JobSpecPanel({ jobSpec }: JobSpecPanelProps) {
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Job spec</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Producing pipeline</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>The pipeline definition responsible for materialising this dataset.</p>
      </header>
      {!jobSpec ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          No pipeline is bound to this dataset. Bind one from <a href="/pipelines" style={{ color: '#93c5fd', textDecoration: 'underline' }}>Pipelines</a> or push a job-spec from a Code Repo.
        </div>
      ) : (
        <dl style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', fontSize: 13 }}>
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Pipeline</dt>
            <dd style={{ margin: '2px 0 0' }}>
              <a href={`/pipelines/${jobSpec.pipeline_id}/edit`} style={{ color: '#93c5fd', textDecoration: 'underline' }}>{jobSpec.pipeline_name}</a>
            </dd>
          </div>
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Branch</dt>
            <dd style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{jobSpec.branch ?? '—'}</dd>
          </div>
          <div style={{ gridColumn: '1 / -1' }}>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Source</dt>
            <dd style={{ margin: '2px 0 0' }}>
              {jobSpec.repo_url ? (
                <a href={jobSpec.repo_url} target="_blank" rel="noopener noreferrer" style={{ color: '#93c5fd', textDecoration: 'underline' }}>
                  {jobSpec.repo_url}{jobSpec.repo_path ? ` :: ${jobSpec.repo_path}` : ''}
                </a>
              ) : (
                <span className="of-text-muted">No source repository linked</span>
              )}
            </dd>
          </div>
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Last run</dt>
            <dd style={{ margin: '2px 0 0', display: 'flex', alignItems: 'center', gap: 8 }}>
              {jobSpec.last_run_at ? <span>{new Date(jobSpec.last_run_at).toLocaleString()}</span> : <span className="of-text-muted">Never</span>}
              {jobSpec.last_run_status && (
                <span style={{ ...tone(jobSpec.last_run_status), padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', fontWeight: 500 }}>
                  {jobSpec.last_run_status}
                </span>
              )}
            </dd>
          </div>
        </dl>
      )}
    </section>
  );
}
