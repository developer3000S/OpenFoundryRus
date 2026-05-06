import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  createObjectSet,
  deleteObjectSet,
  evaluateObjectSet,
  listObjectSets,
  listObjectTypes,
  materializeObjectSet,
  type ObjectSetDefinition,
  type ObjectSetEvaluationResponse,
  type ObjectSetFilter,
  type ObjectType,
} from '@/lib/api/ontology';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { ObjectSetFilterBuilder } from '@/lib/components/ontology/ObjectSetFilterBuilder';

function parseJson<T>(text: string, fallback: T): T {
  const trimmed = text.trim();
  if (!trimmed) return fallback;
  return JSON.parse(trimmed) as T;
}

export function ObjectSetsPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [objectSets, setObjectSets] = useState<ObjectSetDefinition[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // create draft
  const [baseTypeId, setBaseTypeId] = useState('');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [filtersText, setFiltersText] = useState(
    JSON.stringify([{ field: 'status', operator: 'equals', value: 'active' }], null, 2),
  );
  const [traversalsText, setTraversalsText] = useState('[]');
  const [joinText, setJoinText] = useState('null');
  const [projectionsText, setProjectionsText] = useState('base.id,base.properties.status');
  const [whatIfLabel, setWhatIfLabel] = useState('');
  const [policyText, setPolicyText] = useState(
    JSON.stringify({ allowed_markings: ['public'], deny_guest_sessions: false }, null, 2),
  );

  const [evaluation, setEvaluation] = useState<ObjectSetEvaluationResponse | null>(null);

  async function refresh() {
    setError('');
    try {
      const [t, s] = await Promise.all([listObjectTypes({ per_page: 200 }), listObjectSets()]);
      setObjectTypes(t.data);
      setObjectSets(s.data);
      if (!baseTypeId && t.data[0]) setBaseTypeId(t.data[0].id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!baseTypeId || !name.trim()) {
      setError('Name and base type are required.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await createObjectSet({
        name,
        description,
        base_object_type_id: baseTypeId,
        filters: parseJson(filtersText, []),
        traversals: parseJson(traversalsText, []),
        join: parseJson(joinText, null),
        projections: projectionsText.split(',').map((p) => p.trim()).filter(Boolean),
        what_if_label: whatIfLabel.trim() || null,
        policy: parseJson(policyText, {}),
      });
      setName('');
      setDescription('');
      setWhatIfLabel('');
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  async function evaluateSet(s: ObjectSetDefinition, mode: 'preview' | 'materialize') {
    setBusy(true);
    setError('');
    try {
      const res =
        mode === 'preview'
          ? await evaluateObjectSet(s.id, { limit: 100 })
          : await materializeObjectSet(s.id, { limit: 500 });
      setEvaluation(res);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `${mode} failed`);
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete object set?')) return;
    setBusy(true);
    try {
      await deleteObjectSet(id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/ontology" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology</Link>
      <header>
        <h1 className="of-heading-xl">Object sets</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
          Reusable saved object queries with filters + traversals + join + projections + what-if/policy.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <form onSubmit={submit} className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
        <p className="of-eyebrow">Create object set</p>
        <label style={{ fontSize: 13 }}>
          Base object type
          <select value={baseTypeId} onChange={(e) => setBaseTypeId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
            <option value="">— pick —</option>
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>{t.display_name} ({t.name})</option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          Description
          <input value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <ObjectSetFilterBuilder
          filters={(() => {
            try { return JSON.parse(filtersText) as ObjectSetFilter[]; }
            catch { return []; }
          })()}
          onChange={(next) => setFiltersText(JSON.stringify(next, null, 2))}
          disabled={busy}
        />
        <details>
          <summary style={{ cursor: 'pointer', fontSize: 12, color: 'var(--text-muted)' }}>Filters as raw JSON</summary>
          <div style={{ marginTop: 6 }}>
            <JsonEditor value={filtersText} onChange={setFiltersText} minHeight={100} />
          </div>
        </details>
        <JsonEditor label="Traversals JSON" value={traversalsText} onChange={setTraversalsText} minHeight={80} />
        <JsonEditor label="Join JSON (or null)" value={joinText} onChange={setJoinText} minHeight={80} />
        <label style={{ fontSize: 13 }}>
          Projections (comma-separated paths)
          <input value={projectionsText} onChange={(e) => setProjectionsText(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          What-if label
          <input value={whatIfLabel} onChange={(e) => setWhatIfLabel(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <JsonEditor label="Policy JSON" value={policyText} onChange={setPolicyText} minHeight={80} />
        <button type="submit" disabled={busy} className="of-button of-button--primary">Create object set</button>
      </form>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Object sets ({objectSets.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
          {objectSets.map((s) => (
            <li key={s.id} style={{ padding: 10, borderBottom: '1px solid var(--border-subtle)' }}>
              <strong>{s.name}</strong> · base type {s.base_object_type_id}
              {s.description && <p className="of-text-muted" style={{ fontSize: 11, margin: '2px 0' }}>{s.description}</p>}
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button type="button" onClick={() => void evaluateSet(s, 'preview')} disabled={busy} className="of-button" style={{ fontSize: 11 }}>Preview</button>
                <button type="button" onClick={() => void evaluateSet(s, 'materialize')} disabled={busy} className="of-button" style={{ fontSize: 11 }}>Materialize</button>
                <button type="button" onClick={() => void remove(s.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>Delete</button>
              </div>
            </li>
          ))}
          {objectSets.length === 0 && <li className="of-text-muted">No object sets.</li>}
        </ul>
      </section>

      {evaluation && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Last evaluation</p>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 360 }}>
            {JSON.stringify(evaluation, null, 2)}
          </pre>
        </section>
      )}
    </section>
  );
}
