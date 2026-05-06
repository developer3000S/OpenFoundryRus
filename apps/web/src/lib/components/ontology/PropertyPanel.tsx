import { useState } from 'react';

import { updateProperty, type Property } from '@/lib/api/ontology';
import { JsonEditor } from '@/lib/components/JsonEditor';

interface PropertyPanelProps {
  property: Property;
  typeId: string;
  isPrimaryKey?: boolean;
  onUpdated?: (property: Property) => void;
}

interface Draft {
  display_name: string;
  description: string;
  required: boolean;
  unique_constraint: boolean;
  time_dependent: boolean;
  default_value_json: string;
}

function makeDraft(source: Property): Draft {
  return {
    display_name: source.display_name,
    description: source.description,
    required: source.required,
    unique_constraint: source.unique_constraint,
    time_dependent: source.time_dependent,
    default_value_json:
      source.default_value === null || source.default_value === undefined
        ? ''
        : JSON.stringify(source.default_value, null, 2),
  };
}

export function PropertyPanel({ property, typeId, isPrimaryKey = false, onUpdated }: PropertyPanelProps) {
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [draft, setDraft] = useState<Draft>(makeDraft(property));

  function start() {
    setDraft(makeDraft(property));
    setError('');
    setEditing(true);
  }

  async function save() {
    setSaving(true);
    setError('');
    try {
      let defaultValue: unknown = null;
      if (draft.default_value_json.trim()) {
        try { defaultValue = JSON.parse(draft.default_value_json); }
        catch { setError('Default value must be valid JSON'); setSaving(false); return; }
      }
      const updated = await updateProperty(typeId, property.id, {
        display_name: draft.display_name,
        description: draft.description,
        required: draft.required,
        unique_constraint: draft.unique_constraint,
        time_dependent: draft.time_dependent,
        default_value: defaultValue,
      });
      onUpdated?.(updated);
      setEditing(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  return (
    <article style={{ padding: 12, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, color: '#e2e8f0', display: 'flex', flexDirection: 'column', gap: 8 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 6 }}>
        <div>
          <strong style={{ fontFamily: 'var(--font-mono)' }}>{property.name}</strong>
          {isPrimaryKey && <span style={{ marginLeft: 6, padding: '2px 8px', borderRadius: 999, background: '#1e3a8a', color: '#bfdbfe', fontSize: 10 }}>PK</span>}
          <p style={{ margin: '2px 0 0', fontSize: 11, color: '#94a3b8' }}>
            {property.display_name} · {property.property_type}{property.required ? ' · required' : ''}{property.unique_constraint ? ' · unique' : ''}{property.time_dependent ? ' · temporal' : ''}
          </p>
          {property.description && <p style={{ margin: '4px 0 0', fontSize: 12 }}>{property.description}</p>}
        </div>
        {!editing && (
          <button type="button" onClick={start} className="of-button" style={{ fontSize: 11 }}>Edit</button>
        )}
      </header>
      {editing && (
        <div style={{ display: 'grid', gap: 6 }}>
          <label style={{ fontSize: 12 }}>
            Display name
            <input value={draft.display_name} onChange={(e) => setDraft({ ...draft, display_name: e.target.value })} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Description
            <textarea value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} rows={3} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div style={{ display: 'flex', gap: 12, fontSize: 12, flexWrap: 'wrap' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <input type="checkbox" checked={draft.required} onChange={(e) => setDraft({ ...draft, required: e.target.checked })} /> required
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <input type="checkbox" checked={draft.unique_constraint} onChange={(e) => setDraft({ ...draft, unique_constraint: e.target.checked })} /> unique
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <input type="checkbox" checked={draft.time_dependent} onChange={(e) => setDraft({ ...draft, time_dependent: e.target.checked })} /> time-dependent
            </label>
          </div>
          <JsonEditor label="Default value (JSON, leave empty for null)" value={draft.default_value_json} onChange={(v) => setDraft({ ...draft, default_value_json: v })} minHeight={80} />
          {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => void save()} disabled={saving} className="of-button of-button--primary" style={{ fontSize: 11 }}>{saving ? 'Saving…' : 'Save'}</button>
            <button type="button" onClick={() => setEditing(false)} className="of-button" style={{ fontSize: 11 }}>Cancel</button>
          </div>
        </div>
      )}
    </article>
  );
}
