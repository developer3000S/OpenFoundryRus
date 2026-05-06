export interface BulkAction {
  id: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
}

interface BulkActionsToolbarProps {
  count: number;
  actions: BulkAction[];
  onAction: (id: string) => void;
  onClear: () => void;
  busy?: boolean;
}

export function BulkActionsToolbar({ count, actions, onAction, onClear, busy = false }: BulkActionsToolbarProps) {
  if (count === 0) return null;
  return (
    <div
      role="toolbar"
      aria-label="Bulk actions"
      style={{
        position: 'sticky',
        top: 8,
        zIndex: 20,
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        padding: '8px 16px',
        borderRadius: 6,
        border: '1px solid var(--border-default)',
        background: '#1e293b',
        boxShadow: '0 4px 12px rgba(0,0,0,0.2)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 13, fontWeight: 500 }}>
        <span>{count} selected</span>
        <button type="button" onClick={onClear} disabled={busy} style={{ background: 'transparent', border: 'none', color: '#60a5fa', cursor: 'pointer', fontSize: 11 }}>
          Clear
        </button>
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {actions.map((a) => (
          <button
            key={a.id}
            type="button"
            disabled={busy || a.disabled}
            onClick={() => onAction(a.id)}
            className="of-button"
            style={{ fontSize: 12, ...(a.danger ? { color: '#fca5a5', borderColor: '#7f1d1d' } : {}) }}
          >
            {a.label}
          </button>
        ))}
      </div>
    </div>
  );
}
