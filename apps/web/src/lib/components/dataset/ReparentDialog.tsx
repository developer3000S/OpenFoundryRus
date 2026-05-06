import { useEffect, useMemo, useState } from 'react';

import { reparentBranch, type DatasetBranch } from '@/lib/api/datasets';

interface ReparentDialogProps {
  datasetRid: string;
  source: DatasetBranch | null;
  candidateParent: DatasetBranch | null;
  open: boolean;
  onClose: () => void;
  onConfirmed?: (updated: DatasetBranch) => void;
}

export function ReparentDialog({ datasetRid, source, candidateParent, open, onClose, onConfirmed }: ReparentDialogProps) {
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setTyped('');
      setError('');
    }
  }, [open]);

  const expectedFallback = useMemo(() => {
    if (!candidateParent) return [];
    const head = candidateParent.name;
    const rest = (source?.fallback_chain ?? []).filter((b) => b !== head);
    return [head, ...rest];
  }, [candidateParent, source]);

  async function confirm() {
    if (!source || !candidateParent) return;
    if (typed.trim() !== source.name) {
      setError(`Type "${source.name}" exactly to confirm`);
      return;
    }
    setBusy(true);
    setError('');
    try {
      const updated = await reparentBranch(datasetRid, source.name, { new_parent_branch: candidateParent.name });
      onConfirmed?.(updated);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Reparent failed');
    } finally {
      setBusy(false);
    }
  }

  if (!open || !source || !candidateParent) return null;

  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 520, padding: 20, borderRadius: 16, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>Re-parent branch</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: '#94a3b8' }}>
          Re-parenting only changes the ancestry record — no transactions are moved or rewritten.
        </p>
        <dl style={{ marginTop: 12, display: 'grid', gap: 4, fontSize: 13 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <dt style={{ color: '#94a3b8' }}>Branch</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)' }}>{source.name}</dd>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <dt style={{ color: '#94a3b8' }}>Current parent</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)' }}>
              {source.parent_branch_id ? `${source.parent_branch_id.slice(0, 8)}…` : '— (root)'}
            </dd>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <dt style={{ color: '#94a3b8' }}>New parent</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)' }}>{candidateParent.name}</dd>
          </div>
        </dl>
        <div style={{ marginTop: 12, padding: 8, borderRadius: 6, background: '#1e293b', fontSize: 11 }}>
          <p style={{ margin: 0, fontWeight: 600 }}>Children moving with this branch</p>
          <p style={{ margin: '4px 0 0', color: '#94a3b8' }}>
            None — re-parenting only changes the branch ancestry record. Direct descendants of <code>{source.name}</code> still point at it, so its sub-tree is preserved as-is.
          </p>
        </div>
        <div style={{ marginTop: 8, padding: 8, borderRadius: 6, background: '#1e293b', fontSize: 11 }}>
          <p style={{ margin: 0, fontWeight: 600 }}>Resulting fallback chain</p>
          <p style={{ margin: '4px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
            {expectedFallback.length === 0 ? '—' : expectedFallback.join(' → ')}
          </p>
        </div>
        <label style={{ marginTop: 16, display: 'block', fontSize: 13 }}>
          Type <code>{source.name}</code> to confirm
          <input value={typed} onChange={(e) => setTyped(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        {error && <p style={{ marginTop: 8, fontSize: 11, color: '#fca5a5' }}>{error}</p>}
        <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" onClick={onClose} className="of-button">Cancel</button>
          <button type="button" onClick={() => void confirm()} disabled={busy || typed.trim() !== source.name} className="of-button of-button--primary">
            Reparent
          </button>
        </div>
      </div>
    </div>
  );
}
