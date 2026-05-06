import { useEffect, useState } from 'react';

import {
  createObjectType,
  createProperty,
  deleteObjectType,
  deleteProperty,
  getObjectType,
  listProperties,
  updateObjectType,
  type ObjectType,
  type Property,
} from '@/lib/api/ontology';
import { PropertyPanel } from './PropertyPanel';
import { PropertyTypeEditor, type PropertyType } from './PropertyTypeEditor';

interface TypeEditorProps {
  typeId?: string | null;
  onCreated?: (type: ObjectType) => void;
  onUpdated?: (type: ObjectType) => void;
  onDeleted?: (id: string) => void;
}

export function TypeEditor({ typeId = null, onCreated, onUpdated, onDeleted }: TypeEditorProps) {
  const [draft, setDraft] = useState({
    name: '',
    display_name: '',
    description: '',
    primary_key_property: '',
    icon: '',
    color: '#1d4ed8',
  });
  const [properties, setProperties] = useState<Property[]>([]);
  const [editing, setEditing] = useState<ObjectType | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // new property form
  const [newName, setNewName] = useState('');
  const [newDisplay, setNewDisplay] = useState('');
  const [newType, setNewType] = useState<PropertyType>('string');
  const [newRequired, setNewRequired] = useState(false);

  useEffect(() => {
    if (!typeId) {
      setEditing(null);
      setProperties([]);
      setDraft({ name: '', display_name: '', description: '', primary_key_property: '', icon: '', color: '#1d4ed8' });
      return;
    }
    Promise.all([getObjectType(typeId), listProperties(typeId)])
      .then(([type, props]) => {
        setEditing(type);
        setProperties(props);
        setDraft({
          name: type.name,
          display_name: type.display_name,
          description: type.description,
          primary_key_property: type.primary_key_property ?? '',
          icon: type.icon ?? '',
          color: type.color ?? '#1d4ed8',
        });
      })
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : String(cause)));
  }, [typeId]);

  async function save() {
    setSaving(true);
    setError('');
    try {
      if (editing) {
        const updated = await updateObjectType(editing.id, {
          display_name: draft.display_name,
          description: draft.description,
          primary_key_property: draft.primary_key_property || undefined,
          icon: draft.icon || undefined,
          color: draft.color || undefined,
        });
        setEditing(updated);
        onUpdated?.(updated);
      } else {
        const created = await createObjectType({
          name: draft.name,
          display_name: draft.display_name,
          description: draft.description,
          icon: draft.icon || undefined,
          color: draft.color || undefined,
        });
        setEditing(created);
        onCreated?.(created);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!editing) return;
    if (typeof window !== 'undefined' && !window.confirm(`Delete object type "${editing.name}"?`)) return;
    setSaving(true);
    try {
      await deleteObjectType(editing.id);
      onDeleted?.(editing.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  async function addProperty() {
    if (!editing || !newName.trim()) return;
    try {
      const created = await createProperty(editing.id, {
        name: newName.trim(),
        display_name: newDisplay.trim() || newName.trim(),
        property_type: newType,
        required: newRequired,
      });
      setProperties((prev) => [...prev, created]);
      setNewName('');
      setNewDisplay('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function removeProperty(p: Property) {
    if (!editing) return;
    if (typeof window !== 'undefined' && !window.confirm(`Delete property "${p.name}"?`)) return;
    try {
      await deleteProperty(editing.id, p.id);
      setProperties((prev) => prev.filter((q) => q.id !== p.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  return (
    <article style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 12, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, color: '#e2e8f0' }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>{editing ? 'Edit object type' : 'New object type'}</h3>
      <label style={{ fontSize: 12 }}>
        Name (identifier)
        <input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} disabled={!!editing} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
      </label>
      <label style={{ fontSize: 12 }}>
        Display name
        <input value={draft.display_name} onChange={(e) => setDraft({ ...draft, display_name: e.target.value })} className="of-input" style={{ marginTop: 4 }} />
      </label>
      <label style={{ fontSize: 12 }}>
        Description
        <textarea value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} rows={2} className="of-input" style={{ marginTop: 4 }} />
      </label>
      <div style={{ display: 'flex', gap: 8 }}>
        <label style={{ fontSize: 12, flex: 1 }}>
          Icon
          <input value={draft.icon} onChange={(e) => setDraft({ ...draft, icon: e.target.value })} placeholder="📄" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12, flex: 1 }}>
          Color
          <input type="color" value={draft.color} onChange={(e) => setDraft({ ...draft, color: e.target.value })} className="of-input" style={{ marginTop: 4, padding: 2, height: 36 }} />
        </label>
      </div>
      {editing && (
        <label style={{ fontSize: 12 }}>
          Primary key property
          <select value={draft.primary_key_property} onChange={(e) => setDraft({ ...draft, primary_key_property: e.target.value })} className="of-input" style={{ marginTop: 4 }}>
            <option value="">—</option>
            {properties.map((p) => <option key={p.id} value={p.name}>{p.name}</option>)}
          </select>
        </label>
      )}
      {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
      <div style={{ display: 'flex', gap: 6 }}>
        <button type="button" onClick={() => void save()} disabled={saving} className="of-button of-button--primary" style={{ fontSize: 11 }}>{saving ? 'Saving…' : editing ? 'Update' : 'Create'}</button>
        {editing && <button type="button" onClick={() => void remove()} disabled={saving} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>Delete</button>}
      </div>

      {editing && (
        <section style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 12 }}>
          <h4 style={{ margin: 0, fontSize: 13 }}>Properties ({properties.length})</h4>
          <div style={{ padding: 8, background: '#0b1220', borderRadius: 6, display: 'grid', gap: 6 }}>
            <input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="property name" className="of-input" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }} />
            <input value={newDisplay} onChange={(e) => setNewDisplay(e.target.value)} placeholder="display name" className="of-input" style={{ fontSize: 11 }} />
            <PropertyTypeEditor propertyType={newType} onChange={setNewType} />
            <label style={{ fontSize: 11, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={newRequired} onChange={(e) => setNewRequired(e.target.checked)} />
              required
            </label>
            <button type="button" onClick={() => void addProperty()} disabled={!newName.trim()} className="of-button of-button--primary" style={{ fontSize: 11, alignSelf: 'flex-start' }}>+ Add property</button>
          </div>
          {properties.map((p) => (
            <div key={p.id} style={{ display: 'flex', alignItems: 'flex-start', gap: 6 }}>
              <div style={{ flex: 1 }}>
                <PropertyPanel property={p} typeId={editing.id} isPrimaryKey={draft.primary_key_property === p.name} onUpdated={(next) => setProperties((prev) => prev.map((x) => (x.id === next.id ? next : x)))} />
              </div>
              <button type="button" onClick={() => void removeProperty(p)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>×</button>
            </div>
          ))}
        </section>
      )}
    </article>
  );
}
