import { useEffect, useState } from 'react';

import {
  deleteDatasetBranch,
  previewDeleteBranch,
  type DatasetBranch,
  type DatasetBranchDeleteResponse,
  type DatasetBranchPreviewDelete,
} from '@/lib/api/datasets';

interface DeleteBranchDialogProps {
  datasetRid: string;
  branch: DatasetBranch | null;
  open: boolean;
  onClose: () => void;
  onDeleted?: (response: DatasetBranchDeleteResponse) => void;
}

export function DeleteBranchDialog({ datasetRid, branch, open, onClose, onDeleted }: DeleteBranchDialogProps) {
  const [preview, setPreview] = useState<DatasetBranchPreviewDelete | null>(null);
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open && branch) {
      setPreview(null);
      setTyped('');
      setError('');
      setBusy(true);
      previewDeleteBranch(datasetRid, branch.name)
        .then((p) => setPreview(p))
        .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Failed to load preview'))
        .finally(() => setBusy(false));
    }
  }, [open, branch, datasetRid]);

  async function confirm() {
    if (!branch) return;
    if (typed.trim() !== branch.name) {
      setError(`Type "${branch.name}" exactly to confirm`);
      return;
    }
    setBusy(true);
    setError('');
    try {
      const result = await deleteDatasetBranch(datasetRid, branch.name);
      onDeleted?.(result);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  if (!open || !branch) return null;

  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 520, padding: 20, borderRadius: 16, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600, color: '#fca5a5' }}>Delete branch &ldquo;{branch.name}&rdquo;</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: '#94a3b8' }}>
          Soft-delete: the branch row is hidden from active listings but the underlying transactions and audit history stay intact.
        </p>

        {busy && !preview && <p style={{ marginTop: 12, fontSize: 13, color: '#94a3b8' }}>Loading preview…</p>}
        {preview && (
          <>
            <div style={{ marginTop: 12, padding: 8, borderRadius: 6, background: '#78350f', color: '#fde68a', fontSize: 11 }}>
              ⚠ Transactions are <b>not</b> deleted. The branch pointer ({preview.head_transaction?.rid ?? 'no head'}) is removed from the active set; re-creating a branch with the same name starts fresh from the parent.
            </div>
            <section style={{ marginTop: 12 }}>
              <p style={{ margin: 0, fontSize: 13, fontWeight: 600 }}>Children to re-parent</p>
              {preview.children_to_reparent.length === 0 ? (
                <p style={{ margin: '4px 0 0', fontSize: 11, color: '#94a3b8' }}>No children — only this branch will be removed.</p>
              ) : (
                <ul style={{ marginTop: 4, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
                  {preview.children_to_reparent.map((child) => (
                    <li key={child.branch_rid} style={{ display: 'flex', justifyContent: 'space-between', gap: 8, fontSize: 11, padding: '4px 8px', background: '#1e293b', borderRadius: 4 }}>
                      <span style={{ fontFamily: 'var(--font-mono)' }}>{child.branch}</span>
                      <span style={{ color: '#94a3b8' }}>→ {child.new_parent ?? '(root)'}</span>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </>
        )}

        <label style={{ marginTop: 16, display: 'block', fontSize: 13 }}>
          Type <code>{branch.name}</code> to confirm
          <input value={typed} onChange={(e) => setTyped(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>

        {error && <p style={{ marginTop: 8, fontSize: 11, color: '#fca5a5' }}>{error}</p>}

        <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" onClick={onClose} className="of-button">Cancel</button>
          <button type="button" onClick={() => void confirm()} disabled={busy || typed.trim() !== branch.name} className="of-button" style={{ background: '#dc2626', color: '#fff', borderColor: '#dc2626' }}>Delete</button>
        </div>
      </div>
    </div>
  );
}
