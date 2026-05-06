import type { ObjectInstance, ObjectType, Property } from '@/lib/api/ontology';

interface ObjectCardAction {
  label: string;
  onClick: () => void;
  danger?: boolean;
}

interface ObjectCardProps {
  object: ObjectInstance;
  properties?: Property[];
  objectType?: ObjectType | null;
  actions?: ObjectCardAction[];
  onClick?: () => void;
}

const MARKING_TONE: Record<string, { background: string; color: string }> = {
  public: { background: '#064e3b', color: '#6ee7b7' },
  confidential: { background: '#7c2d12', color: '#fdba74' },
  pii: { background: '#831843', color: '#f9a8d4' },
};

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

export function ObjectCard({ object, properties = [], objectType = null, actions = [], onClick }: ObjectCardProps) {
  const visible = properties.slice(0, 4);
  const marking = object.marking ?? 'public';
  const title = (() => {
    const pkProp = objectType?.primary_key_property;
    if (pkProp && object.properties?.[pkProp] !== undefined) return String(object.properties[pkProp]);
    return object.id.slice(0, 8) + '…';
  })();
  const tone = MARKING_TONE[marking] ?? { background: '#1e293b', color: '#cbd5e1' };

  return (
    <article
      onClick={onClick}
      onKeyDown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && onClick) {
          e.preventDefault();
          onClick();
        }
      }}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      style={{
        background: '#0f172a',
        border: '1px solid #1e293b',
        borderLeft: `4px solid ${objectType?.color ?? '#475569'}`,
        borderRadius: 6,
        padding: '10px 12px',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
        cursor: onClick ? 'pointer' : 'default',
      }}
    >
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
        <strong title={object.id}>{title}</strong>
        <span style={{ ...tone, padding: '2px 8px', borderRadius: 3, fontSize: 11 }}>{marking}</span>
      </header>
      {objectType && <p style={{ color: '#94a3b8', fontSize: 12, margin: 0 }}>{objectType.display_name || objectType.name}</p>}
      <dl style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 4, margin: 0, fontSize: 12 }}>
        {visible.map((p) => (
          <div key={p.id} style={{ display: 'flex', gap: 6 }}>
            <dt style={{ color: '#64748b', minWidth: 90 }}>{p.display_name || p.name}</dt>
            <dd style={{ margin: 0, color: '#e2e8f0', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {formatValue(object.properties?.[p.name])}
            </dd>
          </div>
        ))}
      </dl>
      {actions.length > 0 && (
        <footer style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {actions.map((a, i) => (
            <button
              key={i}
              type="button"
              onClick={(e) => { e.stopPropagation(); a.onClick(); }}
              className="of-button"
              style={{ fontSize: 11, ...(a.danger ? { background: '#b91c1c', color: '#fff', borderColor: '#b91c1c' } : {}) }}
            >
              {a.label}
            </button>
          ))}
        </footer>
      )}
    </article>
  );
}
