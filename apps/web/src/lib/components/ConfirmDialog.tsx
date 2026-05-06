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

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  danger = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  useEffect(() => {
    if (!open) return;
    function onKeydown(e: KeyboardEvent) {
      if (e.key === 'Escape' && !busy) {
        e.preventDefault();
        onCancel();
      }
    }
    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  }, [open, busy, onCancel]);

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 50,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'rgba(0,0,0,0.4)',
        padding: 16,
      }}
    >
      <div
        className="of-panel"
        style={{ width: '100%', maxWidth: 420, background: '#fff' }}
      >
        <div style={{ borderBottom: '1px solid var(--border-default)', padding: '12px 16px' }}>
          <div id="confirm-dialog-title" className="of-heading-sm">
            {title}
          </div>
        </div>
        <div style={{ padding: '16px' }}>
          <p style={{ margin: 0, fontSize: 13, color: 'var(--text-default)' }}>{message}</p>
        </div>
        <div
          style={{
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 8,
            borderTop: '1px solid var(--border-default)',
            padding: '12px 16px',
          }}
        >
          <button type="button" className="of-btn of-btn-ghost" onClick={onCancel} disabled={busy}>
            {cancelLabel}
          </button>
          <button
            type="button"
            className={`of-btn ${danger ? 'of-btn-danger' : 'of-btn-primary'}`}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
