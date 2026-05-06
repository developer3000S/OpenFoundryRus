import { useEffect, useState } from 'react';

import {
  createLinkType,
  deleteLinkType,
  listLinkTypes,
  listObjectTypes,
  updateLinkType,
  type LinkType,
  type ObjectType,
} from '@/lib/api/ontology';

type Cardinality = 'one_to_one' | 'one_to_many' | 'many_to_one' | 'many_to_many';

interface LinkEditorProps {
  linkId?: string | null;
  defaultSourceTypeId?: string;
  defaultTargetTypeId?: string;
  onCreated?: (link: LinkType) => void;
  onUpdated?: (link: LinkType) => void;
  onDeleted?: (id: string) => void;
}

export function LinkEditor({ linkId = null, defaultSourceTypeId = '', defaultTargetTypeId = '', onCreated, onUpdated, onDeleted }: LinkEditorProps) {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [editing, setEditing] = useState<LinkType | null>(null);
  const [draft, setDraft] = useState({
    name: '',
    display_name: '',
    description: '',
    source_type_id: defaultSourceTypeId,
    target_type_id: defaultTargetTypeId,
    cardinality: 'many_to_many' as Cardinality,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    listObjectTypes({ per_page: 200 }).then((r) => setObjectTypes(r.data)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!linkId) {
      setEditing(null);
      setDraft({
        name: '',
        display_name: '',
        description: '',
        source_type_id: defaultSourceTypeId,
        target_type_id: defaultTargetTypeId,
        cardinality: 'many_to_many',
      });
      return;
    }
    listLinkTypes({ per_page: 200 }).then((r) => {
      const found = r.data.find((l) => l.id === linkId) ?? null;
      setEditing(found);
      if (found) {
        setDraft({
          name: found.name,
          display_name: found.display_name,
          description: found.description,
          source_type_id: found.source_type_id,
          target_type_id: found.target_type_id,
          cardinality: found.cardinality as Cardinality,
        });
      }
    }).catch(() => {});
  }, [linkId, defaultSourceTypeId, defaultTargetTypeId]);

  async function save() {
    setSaving(true);
    setError('');
    try {
      if (editing) {
        const updated = await updateLinkType(editing.id, {
          display_name: draft.display_name,
          description: draft.description,
          cardinality: draft.cardinality,
        });
        onUpdated?.(updated);
      } else {
        const created = await createLinkType({
          name: draft.name,
          display_name: draft.display_name,
          description: draft.description,
          source_type_id: draft.source_type_id,
          target_type_id: draft.target_type_id,
          cardinality: draft.cardinality,
        });
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
    if (typeof window !== 'undefined' && !window.confirm(`Delete link "${editing.name}"?`)) return;
    setSaving(true);
    try {
      await deleteLinkType(editing.id);
      onDeleted?.(editing.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  return (
    <article style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 12, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, color: '#e2e8f0' }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>{editing ? 'Edit link type' : 'New link type'}</h3>
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
          Source type
          <select value={draft.source_type_id} onChange={(e) => setDraft({ ...draft, source_type_id: e.target.value })} disabled={!!editing} className="of-input" style={{ marginTop: 4 }}>
            <option value="">— pick —</option>
            {objectTypes.map((t) => <option key={t.id} value={t.id}>{t.display_name} ({t.name})</option>)}
          </select>
        </label>
        <label style={{ fontSize: 12, flex: 1 }}>
          Target type
          <select value={draft.target_type_id} onChange={(e) => setDraft({ ...draft, target_type_id: e.target.value })} disabled={!!editing} className="of-input" style={{ marginTop: 4 }}>
            <option value="">— pick —</option>
            {objectTypes.map((t) => <option key={t.id} value={t.id}>{t.display_name} ({t.name})</option>)}
          </select>
        </label>
      </div>
      <label style={{ fontSize: 12 }}>
        Cardinality
        <select value={draft.cardinality} onChange={(e) => setDraft({ ...draft, cardinality: e.target.value as Cardinality })} className="of-input" style={{ marginTop: 4 }}>
          <option value="one_to_one">one_to_one</option>
          <option value="one_to_many">one_to_many</option>
          <option value="many_to_one">many_to_one</option>
          <option value="many_to_many">many_to_many</option>
        </select>
      </label>
      {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
      <div style={{ display: 'flex', gap: 6 }}>
        <button type="button" onClick={() => void save()} disabled={saving} className="of-button of-button--primary" style={{ fontSize: 11 }}>
          {saving ? 'Saving…' : editing ? 'Update' : 'Create'}
        </button>
        {editing && (
          <button type="button" onClick={() => void remove()} disabled={saving} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>Delete</button>
        )}
      </div>
    </article>
  );
}
