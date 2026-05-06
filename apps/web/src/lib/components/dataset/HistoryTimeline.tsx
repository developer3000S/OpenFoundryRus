import type { DatasetTransaction } from '@/lib/api/datasets';

export type RetentionPurgeMarker = {
  policyName: string;
  daysUntilPurge: number;
};

interface HistoryTimelineProps {
  transactions: DatasetTransaction[];
  rollingBack?: string | null;
  onView?: (tx: DatasetTransaction) => void;
  onRollback?: (tx: DatasetTransaction) => void | Promise<void>;
  retentionPurges?: Record<string, RetentionPurgeMarker>;
  onOpenRetention?: () => void;
}

const TONE: Record<string, { border: string; bg: string }> = {
  SNAPSHOT: { border: '#1e40af', bg: '#172554' },
  APPEND: { border: '#047857', bg: '#022c22' },
  UPDATE: { border: '#b45309', bg: '#78350f' },
  DELETE: { border: '#b91c1c', bg: '#7f1d1d' },
};

function icon(op: string) {
  const u = op.toUpperCase();
  if (u === 'SNAPSHOT') return '📸';
  if (u === 'APPEND') return '➕';
  if (u === 'UPDATE') return '✏️';
  if (u === 'DELETE') return '🗑️';
  return '•';
}

function delta(tx: DatasetTransaction) {
  const added = Number(tx.metadata?.['files_added'] ?? 0);
  const removed = Number(tx.metadata?.['files_removed'] ?? 0);
  const parts: string[] = [];
  if (added) parts.push(`+${added}`);
  if (removed) parts.push(`−${removed}`);
  return parts.join(' / ') || '—';
}

function author(tx: DatasetTransaction) {
  const a = tx.metadata?.['author'];
  return typeof a === 'string' ? a : 'system';
}

export function HistoryTimeline({
  transactions,
  rollingBack = null,
  onView,
  onRollback,
  retentionPurges = {},
  onOpenRetention,
}: HistoryTimelineProps) {
  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>History</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Transaction timeline</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: 'var(--text-muted)' }}>
          {transactions.length} transaction{transactions.length === 1 ? '' : 's'} on the active branch.
        </p>
      </header>

      {transactions.length === 0 ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          No transactions yet.
        </div>
      ) : (
        <ol style={{ paddingLeft: 20, margin: 0, borderLeft: '1px solid var(--border-default)', display: 'grid', gap: 12, listStyle: 'none' }}>
          {transactions.map((tx) => {
            const tone = TONE[tx.operation.toUpperCase()] ?? { border: '#475569', bg: '#1e293b' };
            const marker = retentionPurges[tx.id];
            return (
              <li
                key={tx.id}
                style={{
                  padding: 12,
                  borderRadius: 12,
                  background: tone.bg,
                  boxShadow: `inset 0 0 0 1px ${tone.border}`,
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <span style={{ fontSize: 18, lineHeight: 1 }} aria-hidden="true">{icon(tx.operation)}</span>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, fontWeight: 500 }}>
                        <span style={{ textTransform: 'uppercase' }}>{tx.operation}</span>
                        <span style={{ background: 'rgba(255,255,255,0.1)', padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                          {tx.status}
                        </span>
                      </div>
                      <div style={{ marginTop: 2, fontSize: 11, opacity: 0.8 }}>
                        {author(tx)} · {new Date(tx.created_at).toLocaleString()} · files {delta(tx)}
                      </div>
                      <div style={{ marginTop: 2, fontFamily: 'var(--font-mono)', fontSize: 10, opacity: 0.6 }}>{tx.id}</div>
                      {tx.summary && <p style={{ margin: '4px 0 0', fontSize: 13 }}>{tx.summary}</p>}
                      {marker && (
                        <button
                          type="button"
                          onClick={() => onOpenRetention?.()}
                          style={{
                            marginTop: 4,
                            padding: '2px 8px',
                            borderRadius: 999,
                            background: '#7f1d1d',
                            color: '#fecaca',
                            border: 'none',
                            cursor: 'pointer',
                            fontSize: 10,
                            fontWeight: 600,
                            textTransform: 'uppercase',
                          }}
                        >
                          Will be purged{' '}
                          {marker.daysUntilPurge <= 0 ? 'now' : `in ${marker.daysUntilPurge}d`} by {marker.policyName}
                        </button>
                      )}
                    </div>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <button type="button" onClick={() => onView?.(tx)} className="of-button" style={{ fontSize: 11 }}>
                      View at this point in time
                    </button>
                    <button
                      type="button"
                      onClick={() => void onRollback?.(tx)}
                      disabled={rollingBack === tx.id}
                      className="of-button"
                      style={{ fontSize: 11 }}
                    >
                      {rollingBack === tx.id ? 'Rolling back…' : 'Roll back to this transaction'}
                    </button>
                  </div>
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
