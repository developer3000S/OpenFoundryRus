import { useEffect, useState } from 'react';

import { getDatasetHealth, type DatasetHealthResponse } from '@/lib/api/datasets';

interface QualityDashboardProps {
  datasetRid: string;
  freshnessSlaSeconds?: number;
  onAddHealthCheck?: () => void;
  onViewLogs?: () => void;
  onOpenSchemaDiff?: () => void;
}

function fmtSeconds(seconds: number) {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86_400) return `${(seconds / 3600).toFixed(1)}h`;
  return `${(seconds / 86_400).toFixed(1)}d`;
}

function fmtPct(v: number) {
  return `${(v * 100).toFixed(1)}%`;
}

function freshnessColor(seconds: number, sla: number) {
  if (seconds <= sla) return { background: '#022c22', color: '#86efac' };
  if (seconds <= 2 * sla) return { background: '#78350f', color: '#fde68a' };
  return { background: '#7f1d1d', color: '#fecaca' };
}

function buildStatusIcon(status: string) {
  if (status === 'success') return '✅';
  if (status === 'failed') return '❌';
  if (status === 'stale') return '⏳';
  return '❓';
}

export function QualityDashboard({
  datasetRid,
  freshnessSlaSeconds = 24 * 3600,
  onAddHealthCheck,
  onViewLogs,
  onOpenSchemaDiff,
}: QualityDashboardProps) {
  const [health, setHealth] = useState<DatasetHealthResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!datasetRid) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    getDatasetHealth(datasetRid)
      .then((h) => { if (!cancelled) setHealth(h); })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Quality unavailable.');
          setHealth(null);
        }
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetRid]);

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Quality dashboard</h3>
        {loading && <span className="of-text-muted" style={{ fontSize: 11 }}>refreshing…</span>}
      </header>
      {error && <p className="of-status-danger" style={{ padding: 8, fontSize: 12 }}>{error}</p>}

      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
        <Card title="Freshness">
          {health ? (
            <div style={{ ...freshnessColor(health.freshness_seconds, freshnessSlaSeconds), padding: '6px 8px', borderRadius: 6, fontSize: 13, fontWeight: 600, display: 'inline-block' }}>
              {fmtSeconds(health.freshness_seconds)} since last commit
            </div>
          ) : '—'}
          {health?.last_commit_at && (
            <p className="of-text-muted" style={{ fontSize: 11, margin: '6px 0 0' }}>{new Date(health.last_commit_at).toLocaleString()}</p>
          )}
        </Card>

        <Card title="Last build">
          <p style={{ margin: 0, fontSize: 14 }}>{buildStatusIcon(health?.last_build_status ?? 'unknown')} {health?.last_build_status ?? '—'}</p>
          {onViewLogs && <button type="button" onClick={onViewLogs} className="of-button" style={{ marginTop: 6, fontSize: 11 }}>View logs</button>}
        </Card>

        <Card title="Schema drift">
          <p style={{ margin: 0, fontSize: 13 }}>{health ? (health.schema_drift_flag ? 'Drift detected' : 'No drift') : '—'}</p>
          {onOpenSchemaDiff && health?.schema_drift_flag && <button type="button" onClick={onOpenSchemaDiff} className="of-button" style={{ marginTop: 6, fontSize: 11 }}>View diff</button>}
        </Card>

        <Card title="Row / Col">
          <p style={{ margin: 0, fontSize: 13 }}>
            {health ? `${health.row_count.toLocaleString()} rows · ${health.col_count} cols` : '—'}
          </p>
        </Card>

        <Card title="Txn failures (24h)">
          <p style={{ margin: 0, fontSize: 13 }}>{health ? fmtPct(health.txn_failure_rate_24h) : '—'}</p>
        </Card>

        <Card title="Health-check policies">
          {onAddHealthCheck && <button type="button" onClick={onAddHealthCheck} className="of-button" style={{ fontSize: 11 }}>+ Add health check</button>}
        </Card>
      </div>
      {health && health.last_computed_at && (
        <p className="of-text-muted" style={{ fontSize: 10 }}>Last computed: {new Date(health.last_computed_at).toLocaleString()}</p>
      )}
    </section>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="of-panel" style={{ padding: 12 }}>
      <p className="of-eyebrow" style={{ fontSize: 10 }}>{title}</p>
      <div style={{ marginTop: 6 }}>{children}</div>
    </div>
  );
}
