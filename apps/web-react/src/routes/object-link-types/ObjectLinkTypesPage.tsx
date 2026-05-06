import { useEffect, useState } from 'react';

import {
  attachSharedPropertyType,
  createLinkType,
  createObjectType,
  createProperty,
  createSharedPropertyType,
  deleteLinkType,
  deleteObjectType,
  deleteProperty,
  deleteSharedPropertyType,
  detachSharedPropertyType,
  listLinkTypes,
  listObjectTypes,
  listProperties,
  listSharedPropertyTypes,
  listTypeSharedPropertyTypes,
  updateProperty,
  type LinkType,
  type ObjectType,
  type Property,
  type SharedPropertyType,
} from '@/lib/api/ontology';

const PROPERTY_TYPES = [
  'string',
  'integer',
  'float',
  'boolean',
  'date',
  'json',
  'array',
  'reference',
  'geo_point',
  'media_reference',
  'vector',
];

export function ObjectLinkTypesPage() {
  const [tab, setTab] = useState<'types' | 'links' | 'shared'>('types');
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [linkTypes, setLinkTypes] = useState<LinkType[]>([]);
  const [sharedProperties, setSharedProperties] = useState<SharedPropertyType[]>([]);
  const [selectedTypeId, setSelectedTypeId] = useState('');
  const [selectedTypeProps, setSelectedTypeProps] = useState<Property[]>([]);
  const [selectedTypeShared, setSelectedTypeShared] = useState<SharedPropertyType[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // Object type draft
  const [otName, setOtName] = useState('CaseRecord');
  const [otDisplayName, setOtDisplayName] = useState('Case Record');
  const [otDescription, setOtDescription] = useState('');

  // Property draft
  const [pName, setPName] = useState('status');
  const [pDisplayName, setPDisplayName] = useState('Status');
  const [pType, setPType] = useState('string');
  const [pRequired, setPRequired] = useState(false);

  // Link type draft
  const [ltName, setLtName] = useState('case_relates_to');
  const [ltDisplay, setLtDisplay] = useState('Case relates to');
  const [ltSource, setLtSource] = useState('');
  const [ltTarget, setLtTarget] = useState('');
  const [ltCardinality, setLtCardinality] = useState('many_to_many');

  // Shared property draft
  const [sptName, setSptName] = useState('shared_status');
  const [sptDisplay, setSptDisplay] = useState('Shared Status');
  const [sptType, setSptType] = useState('string');

  async function refresh() {
    setError('');
    try {
      const [otRes, ltRes, sptRes] = await Promise.all([
        listObjectTypes({ per_page: 200 }),
        listLinkTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0 })),
        listSharedPropertyTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
      ]);
      setObjectTypes(otRes.data);
      setLinkTypes(ltRes.data);
      setSharedProperties(sptRes.data);
      if (!selectedTypeId && otRes.data[0]) setSelectedTypeId(otRes.data[0].id);
      if (!ltSource && otRes.data[0]) setLtSource(otRes.data[0].id);
      if (!ltTarget && otRes.data[1]) setLtTarget(otRes.data[1].id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load ontology');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedTypeId) return;
    let cancelled = false;
    async function load() {
      try {
        const [props, shared] = await Promise.all([
          listProperties(selectedTypeId),
          listTypeSharedPropertyTypes(selectedTypeId).catch(() => []),
        ]);
        if (cancelled) return;
        setSelectedTypeProps(props);
        setSelectedTypeShared(shared);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load properties');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [selectedTypeId]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Operation failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Object &amp; link types</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Manage object types, properties, link types, and shared property types from a single console.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <nav style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
        {(['types', 'links', 'shared'] as const).map((t) => {
          const active = tab === t;
          return (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              style={{
                padding: '8px 14px',
                background: 'transparent',
                border: 'none',
                borderBottom: `2px solid ${active ? '#1d4ed8' : 'transparent'}`,
                color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
                textTransform: 'capitalize',
              }}
            >
              {t === 'shared' ? 'Shared properties' : `Object ${t}`}
            </button>
          );
        })}
      </nav>

      {tab === 'types' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Object types ({objectTypes.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {objectTypes.map((t) => (
                <li key={t.id} style={{ padding: 8, borderBottom: '1px solid var(--border-default)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                    <button
                      type="button"
                      onClick={() => setSelectedTypeId(t.id)}
                      style={{ background: 'none', border: 'none', textAlign: 'left', cursor: 'pointer' }}
                    >
                      <strong>{t.display_name}</strong> · <span className="of-text-muted">{t.name}</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => void run(() => deleteObjectType(t.id))}
                      disabled={busy}
                      className="of-button"
                      style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                    >
                      Delete
                    </button>
                  </div>
                </li>
              ))}
            </ul>
            <p className="of-eyebrow" style={{ marginTop: 14 }}>Create object type</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
              <input value={otName} onChange={(e) => setOtName(e.target.value)} placeholder="snake_case_name" className="of-input" style={{ width: 220 }} />
              <input value={otDisplayName} onChange={(e) => setOtDisplayName(e.target.value)} placeholder="Display name" className="of-input" style={{ width: 220 }} />
              <input value={otDescription} onChange={(e) => setOtDescription(e.target.value)} placeholder="Description" className="of-input" style={{ width: 280 }} />
              <button
                type="button"
                onClick={() =>
                  void run(async () => {
                    await createObjectType({ name: otName, display_name: otDisplayName, description: otDescription });
                  })
                }
                disabled={busy}
                className="of-button of-button--primary"
              >
                Create
              </button>
            </div>
          </section>

          {selectedTypeId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Properties for {objectTypes.find((t) => t.id === selectedTypeId)?.display_name}</p>
              <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
                {selectedTypeProps.map((p) => (
                  <li key={p.id} style={{ padding: 6, borderBottom: '1px solid var(--border-default)', display: 'flex', justifyContent: 'space-between', gap: 6, fontSize: 13 }}>
                    <div>
                      <strong>{p.display_name}</strong> · {p.name} · {p.property_type}
                      {p.required && <span className="of-chip" style={{ marginLeft: 6, fontSize: 10 }}>required</span>}
                    </div>
                    <div style={{ display: 'flex', gap: 4 }}>
                      <button
                        type="button"
                        onClick={() => void run(() => updateProperty(selectedTypeId, p.id, { required: !p.required }))}
                        disabled={busy}
                        className="of-button"
                        style={{ fontSize: 11 }}
                      >
                        Toggle required
                      </button>
                      <button
                        type="button"
                        onClick={() => void run(() => deleteProperty(selectedTypeId, p.id))}
                        disabled={busy}
                        className="of-button"
                        style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                      >
                        Delete
                      </button>
                    </div>
                  </li>
                ))}
              </ul>

              <p className="of-eyebrow" style={{ marginTop: 14 }}>Add property</p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
                <input value={pName} onChange={(e) => setPName(e.target.value)} placeholder="property_name" className="of-input" style={{ width: 200 }} />
                <input value={pDisplayName} onChange={(e) => setPDisplayName(e.target.value)} placeholder="Display" className="of-input" style={{ width: 220 }} />
                <select value={pType} onChange={(e) => setPType(e.target.value)} className="of-input" style={{ width: 'auto' }}>
                  {PROPERTY_TYPES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
                <label style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 13 }}>
                  <input type="checkbox" checked={pRequired} onChange={(e) => setPRequired(e.target.checked)} /> Required
                </label>
                <button
                  type="button"
                  onClick={() =>
                    void run(async () => {
                      await createProperty(selectedTypeId, { name: pName, display_name: pDisplayName, property_type: pType, required: pRequired });
                    })
                  }
                  disabled={busy}
                  className="of-button of-button--primary"
                >
                  Add property
                </button>
              </div>

              <p className="of-eyebrow" style={{ marginTop: 14 }}>Attached shared properties ({selectedTypeShared.length})</p>
              <ul style={{ marginTop: 6, paddingLeft: 0, listStyle: 'none' }}>
                {selectedTypeShared.map((s) => (
                  <li key={s.id} style={{ padding: 6, borderBottom: '1px solid var(--border-default)', display: 'flex', justifyContent: 'space-between', gap: 6, fontSize: 13 }}>
                    <div>
                      <strong>{s.display_name}</strong> · {s.name} · {s.property_type}
                    </div>
                    <button
                      type="button"
                      onClick={() => void run(() => detachSharedPropertyType(selectedTypeId, s.id))}
                      disabled={busy}
                      className="of-button"
                      style={{ fontSize: 11 }}
                    >
                      Detach
                    </button>
                  </li>
                ))}
              </ul>
              <select
                onChange={async (e) => {
                  const v = e.target.value;
                  if (!v) return;
                  await run(() => attachSharedPropertyType(selectedTypeId, v));
                  e.currentTarget.value = '';
                }}
                className="of-input"
                style={{ width: 280, marginTop: 8 }}
              >
                <option value="">Attach shared property…</option>
                {sharedProperties.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.display_name}
                  </option>
                ))}
              </select>
            </section>
          )}
        </>
      )}

      {tab === 'links' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Link types ({linkTypes.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {linkTypes.map((l) => (
              <li key={l.id} style={{ padding: 8, borderBottom: '1px solid var(--border-default)', display: 'flex', justifyContent: 'space-between', gap: 6, fontSize: 13 }}>
                <div>
                  <strong>{l.display_name}</strong> · {l.name} · {l.cardinality}
                  <p className="of-text-muted" style={{ fontSize: 11 }}>{l.source_type_id.slice(0, 8)} → {l.target_type_id.slice(0, 8)}</p>
                </div>
                <button
                  type="button"
                  onClick={() => void run(() => deleteLinkType(l.id))}
                  disabled={busy}
                  className="of-button"
                  style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                >
                  Delete
                </button>
              </li>
            ))}
          </ul>

          <p className="of-eyebrow" style={{ marginTop: 14 }}>Create link type</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
            <input value={ltName} onChange={(e) => setLtName(e.target.value)} placeholder="snake_case" className="of-input" style={{ width: 200 }} />
            <input value={ltDisplay} onChange={(e) => setLtDisplay(e.target.value)} placeholder="Display name" className="of-input" style={{ width: 200 }} />
            <select value={ltSource} onChange={(e) => setLtSource(e.target.value)} className="of-input" style={{ width: 'auto' }}>
              {objectTypes.map((t) => (
                <option key={t.id} value={t.id}>{t.display_name}</option>
              ))}
            </select>
            <select value={ltTarget} onChange={(e) => setLtTarget(e.target.value)} className="of-input" style={{ width: 'auto' }}>
              {objectTypes.map((t) => (
                <option key={t.id} value={t.id}>{t.display_name}</option>
              ))}
            </select>
            <select value={ltCardinality} onChange={(e) => setLtCardinality(e.target.value)} className="of-input" style={{ width: 'auto' }}>
              <option value="one_to_one">one-to-one</option>
              <option value="one_to_many">one-to-many</option>
              <option value="many_to_many">many-to-many</option>
            </select>
            <button
              type="button"
              onClick={() =>
                void run(async () => {
                  await createLinkType({
                    name: ltName,
                    display_name: ltDisplay,
                    source_type_id: ltSource,
                    target_type_id: ltTarget,
                    cardinality: ltCardinality,
                  });
                })
              }
              disabled={busy}
              className="of-button of-button--primary"
            >
              Create
            </button>
          </div>
        </section>
      )}

      {tab === 'shared' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Shared property types ({sharedProperties.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {sharedProperties.map((s) => (
              <li key={s.id} style={{ padding: 8, borderBottom: '1px solid var(--border-default)', display: 'flex', justifyContent: 'space-between', gap: 6, fontSize: 13 }}>
                <div>
                  <strong>{s.display_name}</strong> · {s.name} · {s.property_type}
                </div>
                <button
                  type="button"
                  onClick={() => void run(() => deleteSharedPropertyType(s.id))}
                  disabled={busy}
                  className="of-button"
                  style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                >
                  Delete
                </button>
              </li>
            ))}
          </ul>

          <p className="of-eyebrow" style={{ marginTop: 14 }}>Create shared property</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
            <input value={sptName} onChange={(e) => setSptName(e.target.value)} placeholder="shared_property_name" className="of-input" style={{ width: 220 }} />
            <input value={sptDisplay} onChange={(e) => setSptDisplay(e.target.value)} placeholder="Display name" className="of-input" style={{ width: 220 }} />
            <select value={sptType} onChange={(e) => setSptType(e.target.value)} className="of-input" style={{ width: 'auto' }}>
              {PROPERTY_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
            <button
              type="button"
              onClick={() =>
                void run(async () => {
                  await createSharedPropertyType({ name: sptName, display_name: sptDisplay, property_type: sptType });
                })
              }
              disabled={busy}
              className="of-button of-button--primary"
            >
              Create
            </button>
          </div>
        </section>
      )}
    </section>
  );
}
