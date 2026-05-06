import { useState } from 'react';

import { executeInlineEdit, type Property } from '@/lib/api/ontology';

interface InlineEditCellProps {
  typeId: string;
  objectId: string;
  property: Property;
  value: unknown;
  onUpdated?: (next: unknown) => void;
}

function formatDisplay(raw: unknown): string {
  if (raw === null || raw === undefined) return '';
  if (typeof raw === 'object') return JSON.stringify(raw);
  return String(raw);
}

function parseSubmissionValue(property: Property, raw: string): unknown {
  const trimmed = raw.trim();
  switch (property.property_type) {
    case 'integer':
      return trimmed === '' ? null : Number.parseInt(trimmed, 10);
    case 'float':
      return trimmed === '' ? null : Number.parseFloat(trimmed);
    case 'boolean':
      return trimmed.toLowerCase() === 'true';
    case 'json':
    case 'array':
    case 'struct':
      return trimmed === '' ? null : JSON.parse(trimmed);
    default:
      return trimmed === '' ? null : trimmed;
  }
}

export function InlineEditCell({ typeId, objectId, property, value, onUpdated }: InlineEditCellProps) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const editable = Boolean(property.inline_edit_config);

  function startEditing() {
    if (!editable) return;
    setDraft(formatDisplay(value));
    setError('');
    setEditing(true);
  }

  async function commit() {
    if (!editing || saving) return;
    setSaving(true);
    try {
      const next = parseSubmissionValue(property, draft);
      if (formatDisplay(next) === formatDisplay(value)) {
        setEditing(false);
        return;
      }
      await executeInlineEdit(typeId, objectId, property.id, { value: next });
      onUpdated?.(next);
      setEditing(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  if (editing) {
    const inputType =
      property.property_type === 'date' ? 'date' :
      property.property_type === 'integer' || property.property_type === 'float' ? 'number' : 'text';
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {property.property_type === 'boolean' ? (
          <select
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={() => void commit()}
            onKeyDown={(e) => { if (e.key === 'Enter') void commit(); else if (e.key === 'Escape') { setEditing(false); setError(''); } }}
            disabled={saving}
            className="of-input"
            style={{ fontSize: 12, borderColor: '#10b981' }}
          >
            <option value="true">true</option>
            <option value="false">false</option>
          </select>
        ) : (
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={() => void commit()}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void commit(); } else if (e.key === 'Escape') { e.preventDefault(); setEditing(false); setError(''); } }}
            disabled={saving}
            autoFocus
            type={inputType}
            step={property.property_type === 'float' ? 'any' : undefined}
            className="of-input"
            style={{ fontSize: 12, borderColor: '#10b981' }}
          />
        )}
        {saving && <span style={{ fontSize: 11, color: '#94a3b8' }}>Saving…</span>}
        {error && <span style={{ fontSize: 11, color: '#fca5a5' }}>{error}</span>}
      </div>
    );
  }

  return (
    <button
      type="button"
      onDoubleClick={startEditing}
      title={editable ? 'Double-click to edit' : 'No inline edit configured'}
      style={{
        display: 'block',
        width: '100%',
        textAlign: 'left',
        padding: '2px 6px',
        background: 'transparent',
        border: 'none',
        cursor: editable ? 'text' : 'default',
        fontSize: 12,
        color: editable ? 'inherit' : '#cbd5e1',
        borderRadius: 4,
      }}
    >
      {formatDisplay(value) || (editable ? '—' : '')}
    </button>
  );
}
