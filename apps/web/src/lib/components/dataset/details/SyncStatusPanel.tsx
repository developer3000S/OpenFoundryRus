interface SyncState {
  last_sync_at?: string | null;
  next_sync_at?: string | null;
  status: 'healthy' | 'degraded' | 'failed' | 'paused' | 'never_run' | string;
  rows_synced?: number;
  bytes_synced?: number;
  errors?: Array<{ at: string; message: string }>;
  source?: string;
}

interface SyncStatusPanelProps {
  state: SyncState | null;
}

const TONE: Record<string, { background: string; color: string }> = {
  healthy: { background: '#022c22', color: '#86efac' },
  degraded: { background: '#78350f', color: '#fde68a' },
  paused: { background: '#78350f', color: '#fde68a' },
  failed: { background: '#7f1d1d', color: '#fecaca' },
};

function pillFor(status: string) {
  return TONE[status] ?? { background: '#1e293b', color: '#cbd5e1' };
}

export function SyncStatusPanel({ state }: SyncStatusPanelProps) {
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Sync status</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>External replication</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>
          Health of the connector that keeps this dataset in sync with its source.
        </p>
      </header>
      {!state ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          No external sync is configured for this dataset.
        </div>
      ) : (
        <>
          <dl style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', fontSize: 13 }}>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Status</dt>
              <dd style={{ margin: '2px 0 0' }}>
                <span style={{ ...pillFor(state.status), padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', fontWeight: 500, display: 'inline-block' }}>
                  {state.status}
                </span>
              </dd>
            </div>
            {state.source && (
              <div>
                <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Source</dt>
                <dd style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{state.source}</dd>
              </div>
            )}
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Last sync</dt>
              <dd style={{ margin: '2px 0 0' }}>{state.last_sync_at ? new Date(state.last_sync_at).toLocaleString() : 'Never'}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Next sync</dt>
              <dd style={{ margin: '2px 0 0' }}>{state.next_sync_at ? new Date(state.next_sync_at).toLocaleString() : '—'}</dd>
            </div>
            {state.rows_synced !== undefined && (
              <div>
                <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Rows synced</dt>
                <dd style={{ margin: '2px 0 0' }}>{state.rows_synced.toLocaleString()}</dd>
              </div>
            )}
            {state.bytes_synced !== undefined && (
              <div>
                <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Bytes synced</dt>
                <dd style={{ margin: '2px 0 0' }}>{(state.bytes_synced / (1024 * 1024)).toFixed(1)} MB</dd>
              </div>
            )}
          </dl>
          {state.errors && state.errors.length > 0 && (
            <div className="of-status-danger" style={{ padding: 12, borderRadius: 12 }}>
              <div style={{ fontSize: 10, textTransform: 'uppercase', color: '#fca5a5' }}>Recent errors</div>
              <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
                {state.errors.map((err) => (
                  <li key={err.at} style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#fca5a5' }}>
                    {new Date(err.at).toLocaleString()} — {err.message}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  );
}
