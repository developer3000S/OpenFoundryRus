import { useMemo, useState } from 'react';

import type { DatasetField, DatasetFieldType, DatasetSchemaPayload } from '@/lib/api/datasets';

export type Mode = 'view' | 'edit';

export interface SchemaDiff {
  added: string[];
  removed: string[];
  changed: string[];
}

export const FIELD_TYPES: DatasetFieldType['type'][] = [
  'BOOLEAN', 'BYTE', 'SHORT', 'INTEGER', 'LONG', 'FLOAT', 'DOUBLE',
  'STRING', 'BINARY', 'DATE', 'TIMESTAMP', 'DECIMAL', 'ARRAY', 'MAP', 'STRUCT',
];

export function emptyField(name = 'new_field', type: DatasetFieldType['type'] = 'STRING'): DatasetField {
  const base: DatasetField = { name, type, nullable: true };
  if (type === 'DECIMAL') { base.precision = 38; base.scale = 18; }
  else if (type === 'ARRAY') { base.arraySubType = { name: 'item', type: 'STRING', nullable: true }; }
  else if (type === 'MAP') {
    base.mapKeyType = { name: 'key', type: 'STRING', nullable: false };
    base.mapValueType = { name: 'value', type: 'STRING', nullable: true };
  } else if (type === 'STRUCT') {
    base.subSchemas = [{ name: 'field_1', type: 'STRING', nullable: true }];
  }
  return base;
}

export function renderType(f: DatasetField): string {
  if (f.type === 'DECIMAL') return `DECIMAL(${f.precision ?? '?'},${f.scale ?? '?'})`;
  if (f.type === 'ARRAY') return f.arraySubType ? `ARRAY<${renderType(f.arraySubType)}>` : 'ARRAY';
  if (f.type === 'MAP') {
    const k = f.mapKeyType ? renderType(f.mapKeyType) : '?';
    const v = f.mapValueType ? renderType(f.mapValueType) : '?';
    return `MAP<${k}, ${v}>`;
  }
  if (f.type === 'STRUCT') return `STRUCT<${(f.subSchemas ?? []).length}>`;
  return f.type;
}

export function diffSchemas(prev: DatasetField[], next: DatasetField[]): SchemaDiff {
  const added: string[] = [];
  const removed: string[] = [];
  const changed: string[] = [];
  const prevByName = new Map(prev.map((f) => [f.name, f]));
  const nextByName = new Map(next.map((f) => [f.name, f]));
  for (const [name, field] of nextByName) {
    const before = prevByName.get(name);
    if (!before) added.push(name);
    else if (renderType(before) !== renderType(field) || (before.nullable ?? true) !== (field.nullable ?? true)) changed.push(name);
  }
  for (const name of prevByName.keys()) if (!nextByName.has(name)) removed.push(name);
  return { added, removed, changed };
}

interface SchemaViewerProps {
  schema: DatasetSchemaPayload;
  mode?: Mode;
  onSave?: (next: DatasetSchemaPayload) => Promise<void> | void;
  saving?: boolean;
  errorMessage?: string;
}

export function SchemaViewer({ schema, mode = 'view', onSave, saving = false, errorMessage }: SchemaViewerProps) {
  const [draft, setDraft] = useState<DatasetField[]>(() => structuredClone(schema.fields));

  const diff = useMemo(() => diffSchemas(schema.fields, draft), [schema.fields, draft]);

  function patchField(idx: number, patch: Partial<DatasetField>) {
    setDraft((prev) => prev.map((f, i) => (i === idx ? { ...f, ...patch } : f)));
  }
  function addField() {
    setDraft((prev) => [...prev, emptyField(`field_${prev.length + 1}`)]);
  }
  function removeField(idx: number) {
    setDraft((prev) => prev.filter((_, i) => i !== idx));
  }
  function moveField(idx: number, dir: -1 | 1) {
    setDraft((prev) => {
      const next = [...prev];
      const target = idx + dir;
      if (target < 0 || target >= next.length) return prev;
      [next[idx], next[target]] = [next[target], next[idx]];
      return next;
    });
  }

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', gap: 8 }}>
        <div>
          <h3 style={{ margin: 0, fontSize: 14 }}>Schema</h3>
          <p className="of-text-muted" style={{ margin: '2px 0 0', fontSize: 11 }}>
            {schema.file_format} · {draft.length} field{draft.length === 1 ? '' : 's'}
          </p>
        </div>
        {mode === 'edit' && (
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={addField} className="of-button" style={{ fontSize: 11 }}>+ Field</button>
            {onSave && (
              <button
                type="button"
                onClick={() => void onSave({ ...schema, fields: draft })}
                disabled={saving}
                className="of-button of-button--primary"
                style={{ fontSize: 11 }}
              >
                {saving ? 'Saving…' : 'Save schema'}
              </button>
            )}
          </div>
        )}
      </header>

      {errorMessage && <p style={{ fontSize: 11, color: '#fca5a5' }}>{errorMessage}</p>}

      {mode === 'edit' && (diff.added.length || diff.removed.length || diff.changed.length) > 0 && (
        <p className="of-text-muted" style={{ fontSize: 11 }}>
          Diff vs saved: +{diff.added.length} / −{diff.removed.length} / ~{diff.changed.length}
        </p>
      )}

      <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 6 }}>
        {draft.map((f, idx) => (
          <li key={idx}>
            {mode === 'view' ? (
              <FieldView field={f} />
            ) : (
              <FieldEdit
                field={f}
                idx={idx}
                onPatch={(patch) => patchField(idx, patch)}
                onRemove={() => removeField(idx)}
                onMoveUp={() => moveField(idx, -1)}
                onMoveDown={() => moveField(idx, 1)}
              />
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function FieldView({ field }: { field: DatasetField }) {
  return (
    <div style={{ padding: '6px 10px', background: 'var(--bg-subtle)', borderRadius: 6, fontSize: 12, fontFamily: 'var(--font-mono)' }}>
      <strong>{field.name}</strong> <span style={{ color: 'var(--text-muted)' }}>{renderType(field)}{field.nullable === false ? ' NOT NULL' : ''}</span>
      {field.description && <p style={{ margin: '4px 0 0', fontSize: 11, color: 'var(--text-muted)', fontFamily: 'inherit' }}>{field.description}</p>}
      {field.type === 'STRUCT' && (field.subSchemas ?? []).length > 0 && (
        <ul style={{ margin: '4px 0 0', paddingLeft: 16, listStyle: 'none', display: 'grid', gap: 2 }}>
          {(field.subSchemas ?? []).map((c) => (
            <li key={c.name} style={{ fontSize: 11 }}>↳ <FieldView field={c} /></li>
          ))}
        </ul>
      )}
    </div>
  );
}

function FieldEdit({
  field,
  idx,
  onPatch,
  onRemove,
  onMoveUp,
  onMoveDown,
}: {
  field: DatasetField;
  idx: number;
  onPatch: (patch: Partial<DatasetField>) => void;
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
}) {
  return (
    <div style={{ display: 'grid', gap: 6, padding: 8, background: 'var(--bg-subtle)', borderRadius: 6 }}>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
        <input
          value={field.name}
          onChange={(e) => onPatch({ name: e.target.value })}
          className="of-input"
          style={{ fontSize: 12, fontFamily: 'var(--font-mono)', width: 200 }}
        />
        <select
          value={field.type}
          onChange={(e) => onPatch({ type: e.target.value as DatasetFieldType['type'] })}
          className="of-input"
          style={{ fontSize: 12 }}
        >
          {FIELD_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        <label style={{ fontSize: 11, display: 'flex', gap: 4, alignItems: 'center' }}>
          <input type="checkbox" checked={field.nullable !== false} onChange={(e) => onPatch({ nullable: e.target.checked })} />
          nullable
        </label>
        <button type="button" onClick={onMoveUp} className="of-button" style={{ fontSize: 10, padding: '2px 6px' }}>↑</button>
        <button type="button" onClick={onMoveDown} className="of-button" style={{ fontSize: 10, padding: '2px 6px' }}>↓</button>
        <button type="button" onClick={onRemove} className="of-button" style={{ fontSize: 10, color: '#b91c1c', borderColor: '#fecaca' }}>×</button>
      </div>
      {field.type === 'DECIMAL' && (
        <div style={{ display: 'flex', gap: 6 }}>
          <label style={{ fontSize: 11 }}>
            precision
            <input type="number" value={field.precision ?? 38} onChange={(e) => onPatch({ precision: Number(e.target.value) || 0 })} className="of-input" style={{ marginTop: 2, width: 80 }} />
          </label>
          <label style={{ fontSize: 11 }}>
            scale
            <input type="number" value={field.scale ?? 18} onChange={(e) => onPatch({ scale: Number(e.target.value) || 0 })} className="of-input" style={{ marginTop: 2, width: 80 }} />
          </label>
        </div>
      )}
      <input
        value={field.description ?? ''}
        onChange={(e) => onPatch({ description: e.target.value })}
        placeholder="Description (optional)"
        className="of-input"
        style={{ fontSize: 12 }}
      />
      <span className="of-text-muted" style={{ fontSize: 10 }}>field {idx} · {renderType(field)}</span>
    </div>
  );
}
