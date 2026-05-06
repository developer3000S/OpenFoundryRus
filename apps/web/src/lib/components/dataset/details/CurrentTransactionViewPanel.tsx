import type { DatasetTransaction } from '@/lib/api/datasets';

interface CurrentTransactionViewPanelProps {
  head: DatasetTransaction | null;
  composedOf: DatasetTransaction[];
  fileCount: number;
  totalBytes: number;
}

const TONE: Record<string, { background: string; color: string }> = {
  SNAPSHOT: { background: '#1e3a8a', color: '#bfdbfe' },
  APPEND: { background: '#022c22', color: '#86efac' },
  UPDATE: { background: '#78350f', color: '#fde68a' },
  DELETE: { background: '#7f1d1d', color: '#fecaca' },
};

function fmtBytes(b: number) {
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / (1024 * 1024)).toFixed(1)} MB`;
}

function tone(op: string) {
  return TONE[op.toUpperCase()] ?? { background: '#1e293b', color: '#cbd5e1' };
}

export function CurrentTransactionViewPanel({ head, composedOf, fileCount, totalBytes }: CurrentTransactionViewPanelProps) {
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Current transaction view</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>HEAD of branch</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>The transaction whose contents make up the live view.</p>
      </header>

      {!head ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          No transactions have been committed yet.
        </div>
      ) : (
        <div className="of-panel" style={{ padding: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
            <code style={{ fontSize: 11 }}>{head.id}</code>
            <span style={{ ...tone(head.operation), padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', fontWeight: 500 }}>
              {head.operation}
            </span>
          </div>
          <dl style={{ marginTop: 12, display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12, fontSize: 13 }}>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Status</dt>
              <dd style={{ margin: '2px 0 0' }}>{head.status}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Branch</dt>
              <dd style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{(head as DatasetTransaction & { branch_name?: string | null }).branch_name ?? '—'}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Created</dt>
              <dd style={{ margin: '2px 0 0' }}>{new Date(head.created_at).toLocaleString()}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Committed</dt>
              <dd style={{ margin: '2px 0 0' }}>{head.committed_at ? new Date(head.committed_at).toLocaleString() : '—'}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Files</dt>
              <dd style={{ margin: '2px 0 0' }}>{fileCount.toLocaleString()}</dd>
            </div>
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Total size</dt>
              <dd style={{ margin: '2px 0 0' }}>{fmtBytes(totalBytes)}</dd>
            </div>
          </dl>
          {head.summary && <p style={{ marginTop: 12, fontSize: 13 }}>{head.summary}</p>}
        </div>
      )}

      {composedOf.length > 0 && (
        <div>
          <div className="of-text-muted" style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em' }}>Composed of</div>
          <ul className="of-panel" style={{ marginTop: 8, padding: 0, listStyle: 'none' }}>
            {composedOf.map((tx) => (
              <li
                key={tx.id}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  gap: 12,
                  padding: '8px 12px',
                  borderBottom: '1px solid var(--border-subtle)',
                  fontSize: 13,
                }}
              >
                <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ ...tone(tx.operation), padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', fontWeight: 500 }}>{tx.operation}</span>
                  <code style={{ fontSize: 11 }}>{tx.id.slice(0, 12)}…</code>
                </span>
                <span className="of-text-muted" style={{ fontSize: 11 }}>{new Date(tx.created_at).toLocaleString()}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
