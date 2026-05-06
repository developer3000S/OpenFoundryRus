import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import {
  createObject,
  deleteObject,
  executeAction,
  getObjectType,
  listActionTypes,
  listLinkTypes,
  listObjects,
  listProperties,
  listRules,
  listTypeSharedPropertyTypes,
  type ActionType,
  type ExecuteActionResponse,
  type LinkType,
  type ObjectInstance,
  type ObjectType,
  type OntologyRule,
  type Property,
  type SharedPropertyType,
} from '@/lib/api/ontology';

type Tab = 'overview' | 'properties' | 'objects' | 'actions' | 'links' | 'rules' | 'shared';

export function ObjectTypeDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const [tab, setTab] = useState<Tab>('overview');
  const [type, setType] = useState<ObjectType | null>(null);
  const [properties, setProperties] = useState<Property[]>([]);
  const [objects, setObjects] = useState<ObjectInstance[]>([]);
  const [actions, setActions] = useState<ActionType[]>([]);
  const [links, setLinks] = useState<LinkType[]>([]);
  const [rules, setRules] = useState<OntologyRule[]>([]);
  const [shared, setShared] = useState<SharedPropertyType[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const [createPropsJson, setCreatePropsJson] = useState('{}');
  const [executeActionId, setExecuteActionId] = useState('');
  const [executeTargetId, setExecuteTargetId] = useState('');
  const [executeParamsJson, setExecuteParamsJson] = useState('{}');
  const [executeResult, setExecuteResult] = useState<ExecuteActionResponse | null>(null);

  async function loadOverview() {
    if (!id) return;
    setLoading(true);
    try {
      setType(await getObjectType(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load type');
    } finally {
      setLoading(false);
    }
  }

  async function loadTab(next: Tab) {
    setTab(next);
    if (!id) return;
    try {
      if (next === 'properties' && properties.length === 0) setProperties(await listProperties(id));
      if (next === 'objects' && objects.length === 0) setObjects((await listObjects(id, { per_page: 100 })).data);
      if (next === 'actions' && actions.length === 0) setActions((await listActionTypes({ object_type_id: id, per_page: 100 })).data);
      if (next === 'links' && links.length === 0) setLinks((await listLinkTypes({ object_type_id: id, per_page: 100 })).data);
      if (next === 'rules' && rules.length === 0) setRules((await listRules({ object_type_id: id })).data);
      if (next === 'shared' && shared.length === 0) setShared(await listTypeSharedPropertyTypes(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load tab');
    }
  }

  useEffect(() => {
    void loadOverview();
  }, [id]);

  async function createObj() {
    if (!type) return;
    setBusy(true);
    setError('');
    try {
      const props = JSON.parse(createPropsJson || '{}');
      await createObject(type.id, { properties: props });
      setObjects((await listObjects(type.id, { per_page: 100 })).data);
      setCreatePropsJson('{}');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create object failed');
    } finally {
      setBusy(false);
    }
  }

  async function removeObj(objId: string) {
    if (!type) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete object?')) return;
    setBusy(true);
    try {
      await deleteObject(type.id, objId);
      setObjects((await listObjects(type.id, { per_page: 100 })).data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete object failed');
    } finally {
      setBusy(false);
    }
  }

  async function runAction() {
    if (!executeActionId) return;
    setBusy(true);
    try {
      const res = await executeAction(executeActionId, {
        target_object_id: executeTargetId || undefined,
        parameters: JSON.parse(executeParamsJson || '{}'),
      });
      setExecuteResult(res);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Execute failed');
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading…</p>
      </section>
    );
  }

  if (!type) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/ontology" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Object type not found'}</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/ontology" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology</Link>
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
        <div
          style={{
            width: 56,
            height: 56,
            background: type.color || '#4d8cf0',
            borderRadius: 8,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'white',
            fontSize: 24,
          }}
        >
          {type.icon || '◧'}
        </div>
        <div>
          <h1 className="of-heading-xl">{type.display_name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {type.id} · name: {type.name} · pk: {type.primary_key_property ?? '—'}
          </p>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs
        tabs={['overview', 'properties', 'objects', 'actions', 'links', 'rules', 'shared'] as const}
        active={tab}
        onChange={(t) => void loadTab(t)}
      />

      {tab === 'overview' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {JSON.stringify(type, null, 2)}
          </pre>
        </section>
      )}

      {tab === 'properties' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Properties ({properties.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {properties.map((p) => (
              <li key={p.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{p.name}</strong> · {p.property_type} · {p.required ? 'required' : 'optional'}
                {p.description && <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>{p.description}</p>}
              </li>
            ))}
            {properties.length === 0 && <li className="of-text-muted">No properties.</li>}
          </ul>
        </section>
      )}

      {tab === 'objects' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create object</p>
            <textarea
              value={createPropsJson}
              onChange={(e) => setCreatePropsJson(e.target.value)}
              className="of-input"
              style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 120 }}
            />
            <button type="button" onClick={() => void createObj()} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 6 }}>
              Create
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Objects ({objects.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {objects.map((o) => (
                <li key={o.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                    {o.id}
                  </span>
                  <button type="button" onClick={() => void removeObj(o.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                    Delete
                  </button>
                </li>
              ))}
              {objects.length === 0 && <li className="of-text-muted">No objects.</li>}
            </ul>
          </section>
        </>
      )}

      {tab === 'actions' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Action types ({actions.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {actions.map((a) => (
                <li key={a.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                  <strong>{a.display_name}</strong> · <code>{a.name}</code> · {a.operation_kind}
                  <button type="button" onClick={() => setExecuteActionId(a.id)} className="of-button" style={{ fontSize: 11, marginLeft: 8 }}>
                    Execute
                  </button>
                </li>
              ))}
              {actions.length === 0 && <li className="of-text-muted">No actions for this type.</li>}
            </ul>
            <p className="of-text-muted" style={{ fontSize: 11, marginTop: 8 }}>
              Full authoring at <Link to="/action-types">/action-types</Link>.
            </p>
          </section>

          {executeActionId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Execute action {executeActionId}</p>
              <label style={{ fontSize: 13 }}>
                Target object id
                <input value={executeTargetId} onChange={(e) => setExecuteTargetId(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13, display: 'block', marginTop: 8 }}>
                Parameters JSON
                <textarea value={executeParamsJson} onChange={(e) => setExecuteParamsJson(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 100 }} />
              </label>
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button type="button" onClick={() => void runAction()} disabled={busy} className="of-button of-button--primary">Execute</button>
                <button type="button" onClick={() => setExecuteActionId('')} className="of-button">Close</button>
              </div>
              {executeResult && (
                <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 240 }}>
                  {JSON.stringify(executeResult, null, 2)}
                </pre>
              )}
            </section>
          )}
        </>
      )}

      {tab === 'links' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Link types ({links.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {links.map((l) => (
              <li key={l.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{l.display_name}</strong> · {l.name} · {l.source_type_id} → {l.target_type_id}
              </li>
            ))}
            {links.length === 0 && <li className="of-text-muted">No links.</li>}
          </ul>
        </section>
      )}

      {tab === 'rules' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Rules ({rules.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {rules.map((r) => (
              <li key={r.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{r.display_name || r.name}</strong> · {r.evaluation_mode}
              </li>
            ))}
            {rules.length === 0 && <li className="of-text-muted">No rules.</li>}
          </ul>
          <p className="of-text-muted" style={{ fontSize: 11, marginTop: 8 }}>
            CRUD via <Link to="/foundry-rules">/foundry-rules</Link>.
          </p>
        </section>
      )}

      {tab === 'shared' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Shared property types ({shared.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {shared.map((s) => (
              <li key={s.id}>
                <strong>{s.display_name}</strong> · {s.name} · {s.property_type}
              </li>
            ))}
            {shared.length === 0 && <li className="of-text-muted">None attached.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
