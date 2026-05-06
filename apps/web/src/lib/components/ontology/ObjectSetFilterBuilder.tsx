import type { ObjectSetFilter } from '@/lib/api/ontology';

interface ObjectSetFilterBuilderProps {
  filters: ObjectSetFilter[];
  onChange: (next: ObjectSetFilter[]) => void;
  disabled?: boolean;
}

const OPERATORS = [
  'equals',
  'not_equals',
  'contains',
  'starts_with',
  'ends_with',
  'gt',
  'gte',
  'lt',
  'lte',
  'in',
  'not_in',
  'is_null',
  'is_not_null',
];

function valueToText(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  return JSON.stringify(value);
}

function textToValue(text: string): unknown {
  if (text === '') return '';
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function ObjectSetFilterBuilder({ filters, onChange, disabled }: ObjectSetFilterBuilderProps) {
  function patch(index: number, patch: Partial<ObjectSetFilter>) {
    onChange(filters.map((f, i) => (i === index ? { ...f, ...patch } : f)));
  }

  function add() {
    onChange([...filters, { field: 'status', operator: 'equals', value: 'active' }]);
  }

  function remove(index: number) {
    onChange(filters.filter((_, i) => i !== index));
  }

  return (
    <div style={{ display: 'grid', gap: 6 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <p className="of-eyebrow">Filters ({filters.length})</p>
        <button type="button" onClick={add} disabled={disabled} className="of-button" style={{ fontSize: 11 }}>
          + Add filter
        </button>
      </div>
      {filters.length === 0 ? (
        <p className="of-text-muted" style={{ fontSize: 12, fontStyle: 'italic' }}>No filters.</p>
      ) : (
        <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
          {filters.map((f, i) => {
            const isUnary = f.operator === 'is_null' || f.operator === 'is_not_null';
            return (
              <li
                key={i}
                style={{
                  display: 'grid',
                  gap: 6,
                  gridTemplateColumns: 'minmax(0, 1fr) 140px minmax(0, 1.2fr) auto',
                  alignItems: 'center',
                  padding: 6,
                  background: 'var(--bg-subtle)',
                  borderRadius: 6,
                }}
              >
                <input
                  value={f.field}
                  onChange={(e) => patch(i, { field: e.target.value })}
                  placeholder="field path"
                  disabled={disabled}
                  className="of-input"
                  style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}
                />
                <select
                  value={f.operator}
                  onChange={(e) => patch(i, { operator: e.target.value })}
                  disabled={disabled}
                  className="of-input"
                  style={{ fontSize: 11 }}
                >
                  {OPERATORS.map((op) => (
                    <option key={op} value={op}>{op}</option>
                  ))}
                  {!OPERATORS.includes(f.operator) && <option value={f.operator}>{f.operator}</option>}
                </select>
                <input
                  value={isUnary ? '' : valueToText(f.value)}
                  onChange={(e) => patch(i, { value: textToValue(e.target.value) })}
                  placeholder={isUnary ? '— no value —' : 'string or JSON'}
                  disabled={disabled || isUnary}
                  className="of-input"
                  style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}
                />
                <button
                  type="button"
                  onClick={() => remove(i)}
                  disabled={disabled}
                  className="of-button"
                  style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                >
                  ✕
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
