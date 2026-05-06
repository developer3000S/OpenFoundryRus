import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { dataConnection, type Source, type SourceStatus } from '@/lib/api/data-connection';

const STATUS_COLOR: Record<SourceStatus, string> = {
  healthy: '#10b981',
  degraded: '#f59e0b',
  error: '#ef4444',
  configuring: '#3b82f6',
  draft: '#94a3b8',
};

export function DataConnectionPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const res = await dataConnection.listSources({ page: 1, per_page: 100 });
      setSources(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load sources');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleDelete(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete source?')) return;
    setBusy(true);
    try {
      await dataConnection.deleteSource(id);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Data Connection</h1>
          <p className="of-text-muted" style={{ marginTop: 4 }}>
            Sources, batch syncs, egress policies, agents.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Link to="/data-connection/agents" className="of-button" style={{ fontSize: 12 }}>Agents</Link>
          <Link to="/data-connection/egress-policies" className="of-button" style={{ fontSize: 12 }}>Egress policies</Link>
          <Link to="/data-connection/new" className="of-button of-button--primary">+ New source</Link>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="of-text-muted">Loading sources…</p>
      ) : (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Sources ({sources.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {sources.map((s) => (
              <li
                key={s.id}
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
                  <Link to={`/data-connection/sources/${s.id}`} style={{ fontWeight: 600 }}>
                    {s.name}
                  </Link>
                  <p className="of-text-muted" style={{ fontSize: 11 }}>
                    {s.connector_type} · worker: {s.worker} · last_sync: {s.last_sync_at ?? '—'}
                  </p>
                </div>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <span style={{ fontSize: 11, padding: '2px 10px', borderRadius: 999, background: STATUS_COLOR[s.status], color: '#fff' }}>
                    {s.status}
                  </span>
                  <button type="button" onClick={() => void handleDelete(s.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                    Delete
                  </button>
                </div>
              </li>
            ))}
            {sources.length === 0 && <li className="of-text-muted">No sources.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
