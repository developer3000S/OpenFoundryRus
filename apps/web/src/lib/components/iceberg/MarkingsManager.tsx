import { useState } from 'react';

import type { IcebergTableMarkings } from '@/lib/api/icebergTables';

interface Props {
  markings: IcebergTableMarkings;
  canManage: boolean;
  onUpdate?: (next: string[]) => Promise<void> | void;
}

const KNOWN_MARKINGS = ['public', 'confidential', 'pii', 'restricted', 'secret'];

export function MarkingsManager({ markings, canManage, onUpdate }: Props) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  function startEdit() {
    setDraft(markings.explicit.map((p) => p.name));
    setEditing(true);
  }

  async function save() {
    if (!onUpdate) return;
    setSaving(true);
    setError('');
    try {
      await onUpdate(draft);
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save markings');
    } finally {
      setSaving(false);
    }
  }

  function toggleMarking(name: string) {
    setDraft((prev) => prev.includes(name) ? prev.filter((m) => m !== name) : [...prev, name]);
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Markings</h2>
        {canManage && !editing && (
          <button type="button" onClick={startEdit} style={btnStyle}>Edit</button>
        )}
      </header>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 16 }}>
        <Bucket title="Effective" empty="— no markings —">
          {markings.effective.map((item) => (
            <li key={item.marking_id}>
              <span style={badgeStyle}>{item.name}</span>
              <span style={{ ...mutedStyle, fontSize: 11, marginLeft: 6 }}>{item.description}</span>
            </li>
          ))}
        </Bucket>
        <Bucket title="Explicit" empty="— none set —">
          {markings.explicit.map((item) => (
            <li key={item.marking_id}><span style={explicitBadgeStyle}>{item.name}</span></li>
          ))}
        </Bucket>
        <Bucket title="Inherited from namespace" empty="— none inherited —">
          {markings.inherited_from_namespace.map((item) => (
            <li key={item.marking_id}><span style={inheritedBadgeStyle}>{item.name}</span></li>
          ))}
        </Bucket>
      </div>

      {editing && (
        <>
          <div style={{ borderTop: '1px solid #e5e7eb', paddingTop: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <p style={{ ...mutedStyle, fontSize: 13, margin: 0 }}>
              Toggle the explicit overrides applied <strong>in addition to</strong> the inherited set. Inherited markings cannot be removed here; manage them on the namespace.
            </p>
            <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
              {KNOWN_MARKINGS.map((name) => (
                <label key={name}>
                  <input type="checkbox" checked={draft.includes(name)} onChange={() => toggleMarking(name)} /> {name}
                </label>
              ))}
            </div>
            {error && <p role="alert" style={{ color: '#b91c1c', background: '#fef2f2', padding: '8px 12px', borderRadius: 4 }}>{error}</p>}
            <div style={{ display: 'flex', gap: 8 }}>
              <button type="button" onClick={() => void save()} disabled={saving} style={btnStyle}>{saving ? 'Saving…' : 'Save'}</button>
              <button type="button" onClick={() => { setEditing(false); setError(''); }} style={ghostBtnStyle}>Cancel</button>
            </div>
          </div>

          {draft.length < markings.explicit.length && (
            <aside style={whatIfStyle}>
              <strong>What-if simulator:</strong> Removing a marking widens table access. Verify that users currently relying on this clearance retain access through other roles before saving. The Permissions tab lists the impacted principals.
            </aside>
          )}
        </>
      )}
    </section>
  );
}

function Bucket({ title, empty, children }: { title: string; empty: string; children: React.ReactNode[] }) {
  return (
    <article style={bucketStyle}>
      <h3 style={{ margin: '0 0 8px', fontSize: 14, color: '#4b5563' }}>{title}</h3>
      {children.length === 0 ? (
        <p style={mutedStyle}>{empty}</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>{children}</ul>
      )}
    </article>
  );
}

const btnStyle: React.CSSProperties = { padding: '6px 12px', borderRadius: 4, border: '1px solid #2563eb', background: '#2563eb', color: '#fff', fontSize: 14, cursor: 'pointer' };
const ghostBtnStyle: React.CSSProperties = { ...btnStyle, background: 'transparent', color: 'inherit', borderColor: '#d1d5db' };
const bucketStyle: React.CSSProperties = { border: '1px solid #e5e7eb', borderRadius: 8, padding: 12, background: '#fff' };
const badgeStyle: React.CSSProperties = { display: 'inline-block', padding: '2px 8px', borderRadius: 4, fontSize: 12, background: '#f3f4f6', border: '1px solid #e5e7eb' };
const explicitBadgeStyle: React.CSSProperties = { ...badgeStyle, background: '#fef3c7', borderColor: '#fde68a', color: '#92400e' };
const inheritedBadgeStyle: React.CSSProperties = { ...badgeStyle, background: '#dbeafe', borderColor: '#bfdbfe', color: '#1e3a8a' };
const mutedStyle: React.CSSProperties = { color: '#6b7280' };
const whatIfStyle: React.CSSProperties = { border: '1px dashed #fde68a', background: '#fef3c7', color: '#78350f', padding: '8px 12px', borderRadius: 6, fontSize: 13 };
