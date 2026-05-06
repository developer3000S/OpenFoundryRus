import { useState } from 'react';

export interface SchemaField {
  name: string;
  type: string;
  nullable?: boolean;
  description?: string;
  subSchemas?: SchemaField[];
  arraySubType?: SchemaField;
  mapKeyType?: SchemaField;
  mapValueType?: SchemaField;
  precision?: number;
  scale?: number;
}

interface SchemaPanelProps {
  fields: SchemaField[];
  options?: Record<string, string | number | boolean>;
  format?: string;
}

function isComplex(t: string) {
  const u = t.toUpperCase();
  return u.startsWith('STRUCT') || u.startsWith('ARRAY') || u.startsWith('MAP');
}

function renderType(field: SchemaField): string {
  const t = field.type.toUpperCase();
  if (t === 'DECIMAL' && field.precision !== undefined) return `DECIMAL(${field.precision},${field.scale ?? 0})`;
  return t;
}

function typeBadge(t: string) {
  const u = t.toUpperCase();
  if (u === 'STRING' || u === 'BINARY') return { background: '#3b1e6e', color: '#d8b4fe' };
  if (u === 'DATE' || u === 'TIMESTAMP') return { background: '#78350f', color: '#fde68a' };
  if (u === 'BOOLEAN') return { background: '#831843', color: '#fbcfe8' };
  if (u.startsWith('STRUCT') || u.startsWith('ARRAY') || u.startsWith('MAP')) return { background: '#022c22', color: '#86efac' };
  return { background: '#1e3a8a', color: '#bfdbfe' };
}

function Row({ field, depth, path, expanded, toggle }: { field: SchemaField; depth: number; path: string; expanded: Set<string>; toggle: (key: string) => void }) {
  const expandable = isComplex(field.type);
  const isOpen = expanded.has(path);
  const tone = typeBadge(field.type);
  return (
    <li style={{ borderBottom: '1px solid var(--border-subtle)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: 8, paddingLeft: 12 + depth * 16, fontSize: 12 }}>
        {expandable ? (
          <button type="button" onClick={() => toggle(path)} aria-label={isOpen ? 'Collapse' : 'Expand'} style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', padding: 0 }}>
            {isOpen ? '▾' : '▸'}
          </button>
        ) : (
          <span style={{ display: 'inline-block', width: 12 }} />
        )}
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{field.name}</span>
        <span style={{ ...tone, padding: '2px 8px', borderRadius: 999, fontSize: 10, fontWeight: 500 }}>{renderType(field)}</span>
        {field.nullable === false && (
          <span style={{ background: 'var(--bg-subtle)', padding: '1px 6px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>NOT NULL</span>
        )}
        {field.description && <span className="of-text-muted" style={{ marginLeft: 8, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis' }}>{field.description}</span>}
      </div>
      {expandable && isOpen && (
        <ul style={{ borderTop: '1px solid var(--border-subtle)', background: 'rgba(15,23,42,0.4)', listStyle: 'none', margin: 0, padding: 0 }}>
          {field.subSchemas?.map((c, i) => <Row key={`${c.name}.${i}`} field={c} depth={depth + 1} path={`${path}.${i}`} expanded={expanded} toggle={toggle} />)}
          {field.arraySubType && <Row field={{ ...field.arraySubType, name: '[item]' }} depth={depth + 1} path={`${path}.item`} expanded={expanded} toggle={toggle} />}
          {field.mapKeyType && <Row field={{ ...field.mapKeyType, name: '[key]' }} depth={depth + 1} path={`${path}.key`} expanded={expanded} toggle={toggle} />}
          {field.mapValueType && <Row field={{ ...field.mapValueType, name: '[value]' }} depth={depth + 1} path={`${path}.value`} expanded={expanded} toggle={toggle} />}
        </ul>
      )}
    </li>
  );
}

export function SchemaPanel({ fields, options = {}, format = '' }: SchemaPanelProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  function toggle(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }
  const optionEntries = Object.entries(options);
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Schema</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Columns</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>
          {fields.length} top-level field{fields.length === 1 ? '' : 's'}. Click any STRUCT / ARRAY / MAP to drill into nested types.
        </p>
      </header>
      {fields.length === 0 ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          Schema is inferred from the next quality profile or upload.
        </div>
      ) : (
        <ul className="of-panel" style={{ padding: 0, margin: 0, listStyle: 'none', overflow: 'hidden' }}>
          {fields.map((f, i) => <Row key={`${f.name}.${i}`} field={f} depth={0} path={String(i)} expanded={expanded} toggle={toggle} />)}
        </ul>
      )}
      {optionEntries.length > 0 && (
        <div className="of-panel" style={{ padding: 16 }}>
          <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>
            Format options{format ? ` · ${format}` : ''}
          </div>
          <dl style={{ marginTop: 8, display: 'grid', gap: 4, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', fontSize: 12 }}>
            {optionEntries.map(([key, value]) => (
              <div key={key} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 8px', background: 'var(--bg-subtle)', borderRadius: 4 }}>
                <dt style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{key}</dt>
                <dd style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 11 }}>{String(value)}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </section>
  );
}
