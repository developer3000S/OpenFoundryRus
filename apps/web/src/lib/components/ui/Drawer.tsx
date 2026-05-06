import { useEffect, type ReactNode } from 'react';

interface DrawerProps {
  open: boolean;
  title?: string;
  side?: 'right' | 'left' | 'bottom';
  width?: string;
  onClose?: () => void;
  children: ReactNode;
}

export function Drawer({ open, title = '', side = 'right', width = '480px', onClose, children }: DrawerProps) {
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose?.();
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const positionStyle: React.CSSProperties =
    side === 'right'
      ? { top: 0, right: 0, bottom: 0, width }
      : side === 'left'
        ? { top: 0, left: 0, bottom: 0, width }
        : { left: 0, right: 0, bottom: 0, height: width };

  return (
    <>
      <div
        role="presentation"
        onClick={onClose}
        style={{ position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.55)', zIndex: 80 }}
      />
      <aside
        role="dialog"
        aria-modal="true"
        aria-label={title}
        style={{
          position: 'fixed',
          background: '#0f172a',
          border: '1px solid #1e293b',
          boxShadow: '-4px 0 16px rgba(0,0,0,0.4)',
          zIndex: 90,
          display: 'flex',
          flexDirection: 'column',
          color: '#e2e8f0',
          ...positionStyle,
        }}
      >
        <header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0.75rem 1rem',
            borderBottom: '1px solid #1e293b',
          }}
        >
          <strong>{title}</strong>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            style={{
              background: 'transparent',
              border: 'none',
              color: '#94a3b8',
              fontSize: '1.4rem',
              lineHeight: 1,
              cursor: 'pointer',
              padding: '0 0.25rem',
            }}
          >
            ×
          </button>
        </header>
        <div style={{ flex: 1, overflow: 'auto', padding: '1rem' }}>{children}</div>
      </aside>
    </>
  );
}
