import { useState } from 'react';

import { abortTransaction, commitTransaction } from '@/lib/api/datasets';

interface OpenTransactionBannerProps {
  datasetId: string;
  branch: string;
  openTransactionId: string | null;
  canManage: boolean;
  onResolved?: () => void;
}

export function OpenTransactionBanner({
  datasetId,
  branch,
  openTransactionId,
  canManage,
  onResolved,
}: OpenTransactionBannerProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function commit() {
    if (!openTransactionId) return;
    setBusy(true);
    setError('');
    try {
      await commitTransaction(datasetId, branch, openTransactionId);
      onResolved?.();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Commit failed');
    } finally {
      setBusy(false);
    }
  }

  async function abort() {
    if (!openTransactionId) return;
    setBusy(true);
    setError('');
    try {
      await abortTransaction(datasetId, branch, openTransactionId);
      onResolved?.();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Abort failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      role="status"
      style={{
        borderRadius: 16,
        padding: '12px 16px',
        background: '#78350f',
        color: '#fde68a',
        boxShadow: 'inset 0 0 0 1px #b45309',
      }}
    >
      <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <span aria-hidden="true">🔓</span>
          <div>
            <p style={{ margin: 0, fontWeight: 600 }}>An open transaction is in progress on this branch.</p>
            <p style={{ margin: '2px 0 0', fontSize: 11, opacity: 0.8 }}>
              New transactions cannot be started until the current one is committed or aborted.
            </p>
          </div>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {openTransactionId ? (
            <>
              <a
                href={`/datasets/${encodeURIComponent(datasetId)}?tab=transactions&txn=${encodeURIComponent(openTransactionId)}`}
                style={{ padding: '4px 8px', border: '1px solid #b45309', borderRadius: 4, fontSize: 11, color: 'inherit', textDecoration: 'none' }}
              >
                View transaction
              </a>
              {canManage && (
                <>
                  <button
                    type="button"
                    onClick={() => void commit()}
                    disabled={busy}
                    style={{ padding: '4px 8px', borderRadius: 4, fontSize: 11, fontWeight: 500, background: '#059669', color: '#fff', border: 'none', cursor: 'pointer' }}
                  >
                    Commit
                  </button>
                  <button
                    type="button"
                    onClick={() => void abort()}
                    disabled={busy}
                    style={{ padding: '4px 8px', borderRadius: 4, fontSize: 11, fontWeight: 500, background: '#dc2626', color: '#fff', border: 'none', cursor: 'pointer' }}
                  >
                    Abort
                  </button>
                </>
              )}
            </>
          ) : (
            <span style={{ fontSize: 11, fontStyle: 'italic', opacity: 0.7 }}>No transaction id available — refresh to load.</span>
          )}
        </div>
      </div>
      {error && <p style={{ marginTop: 8, fontSize: 11, color: '#fca5a5' }}>{error}</p>}
    </div>
  );
}
