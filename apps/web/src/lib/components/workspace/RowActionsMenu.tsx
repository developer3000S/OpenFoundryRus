import { useEffect, useRef, useState } from 'react';

import { Glyph } from '@/lib/components/ui/Glyph';

export interface RowAction {
  id: string;
  label: string;
  icon?: 'pencil' | 'duplicate' | 'move' | 'delete' | 'share';
  danger?: boolean;
  disabled?: boolean;
}

interface RowActionsMenuProps {
  actions: RowAction[];
  onSelect: (id: string) => void;
  label?: string;
}

export function RowActionsMenu({ actions, onSelect, label = 'Row actions' }: RowActionsMenuProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    function onMouse(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('mousedown', onMouse);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onMouse);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  return (
    <div ref={containerRef} style={{ position: 'relative', display: 'inline-flex' }}>
      <button
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={(e) => { e.stopPropagation(); setOpen((o) => !o); }}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          width: 28,
          height: 28,
          borderRadius: 6,
          border: 'none',
          background: 'transparent',
          color: '#94a3b8',
          cursor: 'pointer',
        }}
      >
        ⋯
      </button>
      {open && (
        <div
          role="menu"
          style={{
            position: 'absolute',
            right: 0,
            top: '100%',
            marginTop: 4,
            minWidth: 180,
            zIndex: 30,
            background: '#0f172a',
            color: '#e2e8f0',
            border: '1px solid #1e293b',
            borderRadius: 6,
            padding: 4,
            boxShadow: '0 8px 24px rgba(0,0,0,0.3)',
          }}
        >
          {actions.map((a) => (
            <button
              key={a.id}
              type="button"
              role="menuitem"
              disabled={a.disabled}
              onClick={(e) => {
                e.stopPropagation();
                if (a.disabled) return;
                setOpen(false);
                onSelect(a.id);
              }}
              style={{
                display: 'flex',
                width: '100%',
                alignItems: 'center',
                gap: 8,
                padding: '6px 12px',
                textAlign: 'left',
                fontSize: 13,
                background: 'transparent',
                border: 'none',
                color: a.danger ? '#fca5a5' : 'inherit',
                cursor: a.disabled ? 'not-allowed' : 'pointer',
                opacity: a.disabled ? 0.5 : 1,
              }}
            >
              {a.icon === 'share' && <Glyph name="users" size={13} />}
              {a.icon === 'duplicate' && <Glyph name="object" size={13} />}
              {a.icon === 'move' && <Glyph name="folder" size={13} />}
              {a.icon === 'delete' && <span aria-hidden="true">🗑</span>}
              {a.icon === 'pencil' && <span aria-hidden="true">✎</span>}
              <span>{a.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
