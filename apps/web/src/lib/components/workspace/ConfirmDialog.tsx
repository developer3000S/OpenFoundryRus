import { useEffect } from 'react';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({ open, title, message, confirmLabel = 'Confirm', cancelLabel = 'Cancel', danger = false, busy = false, onConfirm, onCancel }: ConfirmDialogProps) {
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape' && !busy) { e.preventDefault(); onCancel(); }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, busy, onCancel]);

  if (!open) return null;
  return (
    <div role="dialog" aria-modal="true" aria-label={title} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16, zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 460, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', borderRadius: 12, boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <div style={{ borderBottom: '1px solid #1e293b', padding: '12px 16px' }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>{title}</div>
        </div>
        <div style={{ padding: 16 }}>
          <p style={{ margin: 0, fontSize: 13 }}>{message}</p>
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, borderTop: '1px solid #1e293b', padding: '12px 16px' }}>
          <button type="button" onClick={onCancel} disabled={busy} className="of-button">{cancelLabel}</button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            className="of-button"
            style={{ ...(danger ? { background: '#b42318', color: '#fff', borderColor: '#b42318' } : { background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' }) }}
          >
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
