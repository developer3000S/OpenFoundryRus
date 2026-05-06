import { useEffect, useState } from 'react';

import { renameResource, type ResourceKind } from '@/lib/api/workspace';

interface RenameDialogProps {
  open: boolean;
  resourceKind: ResourceKind | null;
  resourceId: string | null;
  currentName: string;
  onClose?: () => void;
  onRenamed?: (newName: string) => void;
}

export function RenameDialog({ open, resourceKind, resourceId, currentName, onClose, onRenamed }: RenameDialogProps) {
  const [value, setValue] = useState(currentName);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setValue(currentName);
      setSubmitting(false);
      setError('');
    }
  }, [open, currentName]);

  async function submit() {
    if (!resourceKind || !resourceId) return;
    const trimmed = value.trim();
    if (!trimmed) { setError('Name cannot be empty.'); return; }
    if (trimmed === currentName) { onClose?.(); return; }
    setSubmitting(true);
    setError('');
    try {
      await renameResource(resourceKind, resourceId, { name: trimmed });
      onRenamed?.(trimmed);
      onClose?.();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to rename resource');
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) return null;
  return (
    <div role="dialog" aria-modal="true" aria-label="Rename resource" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16, zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 460, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', borderRadius: 12, boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #1e293b', padding: '12px 16px' }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>Rename</div>
          <button type="button" onClick={() => !submitting && onClose?.()} style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: 13 }}>✕</button>
        </div>
        <div style={{ display: 'grid', gap: 8, padding: 16 }}>
          <label style={{ fontSize: 11, textTransform: 'uppercase', color: '#94a3b8', letterSpacing: '0.05em' }}>New name</label>
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void submit(); } }}
            className="of-input"
            autoFocus
          />
          {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, borderTop: '1px solid #1e293b', padding: '12px 16px' }}>
          <button type="button" onClick={() => onClose?.()} disabled={submitting} className="of-button">Cancel</button>
          <button type="button" onClick={() => void submit()} disabled={submitting} className="of-button of-button--primary">
            {submitting ? 'Renaming…' : 'Rename'}
          </button>
        </div>
      </div>
    </div>
  );
}
