import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { deletePipeline, listPipelines, runDuePipelines, type Pipeline } from '@/lib/api/pipelines';

export function PipelinesPage() {
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const res = await listPipelines({ search: search || undefined, status: status || undefined, per_page: 100 });
      setPipelines(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load pipelines');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function handleDelete(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete pipeline?')) return;
    setBusy(true);
    try {
      await deletePipeline(id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleRunDue() {
    setBusy(true);
    try {
      await runDuePipelines();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to dispatch due runs');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Pipelines</h1>
          <p className="of-text-muted" style={{ marginTop: 4 }}>
            Hybrid batch + streaming pipelines. Author DAG, schedule, and inspect runs.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={() => void handleRunDue()} disabled={busy} className="of-button">
            Run due
          </button>
          <button type="button" onClick={() => navigate('/pipelines/new')} className="of-button of-button--primary">
            New pipeline
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search…"
            className="of-input"
            style={{ width: 240 }}
          />
          <select value={status} onChange={(e) => setStatus(e.target.value)} className="of-input">
            <option value="">All statuses</option>
            <option value="draft">draft</option>
            <option value="active">active</option>
            <option value="paused">paused</option>
            <option value="archived">archived</option>
          </select>
          <button type="button" onClick={() => void refresh()} disabled={loading} className="of-button">
            Apply
          </button>
        </div>
      </section>

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Pipelines ({pipelines.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {pipelines.map((p) => (
              <li
                key={p.id}
                style={{
                  padding: 12,
                  borderBottom: '1px solid var(--border-default)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 8,
                }}
              >
                <div>
                  <Link to={`/pipelines/${p.id}/edit`} style={{ fontWeight: 600 }}>
                    {p.name}
                  </Link>
                  <p className="of-text-muted" style={{ fontSize: 11 }}>
                    {p.id} · {p.status} · {p.pipeline_type} · {p.dag.length} node{p.dag.length === 1 ? '' : 's'}
                    {p.schedule_config.enabled && p.schedule_config.cron ? ` · cron: ${p.schedule_config.cron}` : ''}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => void handleDelete(p.id)}
                  disabled={busy}
                  className="of-button"
                  style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                >
                  Delete
                </button>
              </li>
            ))}
            {pipelines.length === 0 && <li className="of-text-muted">No pipelines.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
