import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { abortBuildV1, listBuildsV1, runBuildV1, type BuildEnvelope } from '@/lib/api/buildsV1';

export function BuildsPage() {
  const [builds, setBuilds] = useState<BuildEnvelope[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [stateFilter, setStateFilter] = useState('');
  const [createBody, setCreateBody] = useState(
    JSON.stringify({ pipeline_rid: 'ri.pipeline.example', branch: 'master', force: false }, null, 2),
  );

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const res = await listBuildsV1({ status: stateFilter || undefined, limit: 100 });
      setBuilds(res.data as BuildEnvelope[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load builds');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function handleAbort(rid: string) {
    if (typeof window !== 'undefined' && !window.confirm('Abort this build?')) return;
    setBusy(true);
    try {
      await abortBuildV1(rid);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Abort failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleCreate() {
    setBusy(true);
    setError('');
    try {
      await runBuildV1(JSON.parse(createBody));
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Builds</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Pipeline builds (V1). Filter by state, abort running builds, or trigger a new build with a JSON request.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        <label style={{ fontSize: 13 }}>
          State:
          <select value={stateFilter} onChange={(e) => setStateFilter(e.target.value)} className="of-input" style={{ marginLeft: 6, width: 'auto' }}>
            <option value="">All</option>
            <option value="WAITING">WAITING</option>
            <option value="RUNNING">RUNNING</option>
            <option value="SUCCEEDED">SUCCEEDED</option>
            <option value="FAILED">FAILED</option>
            <option value="ABORTED">ABORTED</option>
          </select>
        </label>
        <button type="button" onClick={() => void refresh()} className="of-button">Refresh</button>
      </div>

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Builds ({builds.length})</p>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, marginTop: 8 }}>
            <thead style={{ background: 'var(--bg-subtle)' }}>
              <tr>
                {['RID', 'State', 'Branch', 'Pipeline', 'Created', ''].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {builds.map((b) => (
                <tr key={b.rid}>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>
                    <Link to={`/builds/${encodeURIComponent(b.rid)}`}>{b.rid.slice(0, 12)}</Link>
                  </td>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>{b.state}</td>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>{b.build_branch}</td>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>{b.pipeline_rid?.slice(0, 12)}</td>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>
                    {b.created_at ? new Date(b.created_at).toLocaleString() : '—'}
                  </td>
                  <td style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-default)' }}>
                    {(b.state === 'BUILD_QUEUED' || b.state === 'BUILD_RUNNING' || b.state === 'BUILD_RESOLUTION') && (
                      <button
                        type="button"
                        onClick={() => void handleAbort(b.rid)}
                        disabled={busy}
                        className="of-button"
                        style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                      >
                        Abort
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Create build</p>
        <textarea
          value={createBody}
          onChange={(e) => setCreateBody(e.target.value)}
          className="of-input"
          style={{ marginTop: 8, fontFamily: 'var(--font-mono)', fontSize: 12, minHeight: 140 }}
        />
        <button type="button" onClick={() => void handleCreate()} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 8 }}>
          Run build
        </button>
      </section>
    </section>
  );
}
