import { useEffect, useState } from 'react';

import {
  createFunctionPackage,
  deleteFunctionPackage,
  executeFunctionPackage,
  listFunctionPackageMetrics,
  listFunctionPackageRuns,
  listFunctionPackages,
  updateFunctionPackage,
  type FunctionPackage,
  type FunctionPackageMetrics,
  type FunctionPackageRun,
} from '@/lib/api/ontology';
import { JsonEditor } from '@/lib/components/JsonEditor';

interface PackageDraft {
  id?: string;
  name: string;
  version: string;
  display_name: string;
  description: string;
  runtime: string;
  source: string;
  entrypoint: string;
  capabilities_json: string;
}

const RUNTIMES = ['python', 'wasm', 'typescript', 'rust'];

function emptyDraft(): PackageDraft {
  return {
    name: 'package_name',
    version: '0.1.0',
    display_name: 'Package name',
    description: '',
    runtime: 'python',
    source: '',
    entrypoint: 'main.handler',
    capabilities_json: JSON.stringify(
      {
        allow_ontology_read: true,
        allow_ontology_write: false,
        allow_ai: false,
        allow_network: false,
        timeout_seconds: 30,
        max_source_bytes: 1048576,
      },
      null,
      2,
    ),
  };
}

export function FunctionsPage() {
  const [packages, setPackages] = useState<FunctionPackage[]>([]);
  const [runs, setRuns] = useState<FunctionPackageRun[]>([]);
  const [metrics, setMetrics] = useState<FunctionPackageMetrics[]>([]);
  const [draft, setDraft] = useState<PackageDraft>(emptyDraft());
  const [executeInputJson, setExecuteInputJson] = useState('{}');
  const [executeResult, setExecuteResult] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function refresh() {
    setError('');
    try {
      const [pRes, rRes, mRes] = await Promise.all([
        listFunctionPackages({ per_page: 200 }),
        listFunctionPackageRuns({ per_page: 50 }),
        listFunctionPackageMetrics(),
      ]);
      setPackages(pRes.data);
      setRuns(rRes.data);
      setMetrics(mRes.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load functions');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  function loadPackage(pkg: FunctionPackage) {
    setDraft({
      id: pkg.id,
      name: pkg.name,
      version: pkg.version,
      display_name: pkg.display_name,
      description: pkg.description,
      runtime: pkg.runtime,
      source: pkg.source,
      entrypoint: pkg.entrypoint,
      capabilities_json: JSON.stringify(pkg.capabilities, null, 2),
    });
  }

  async function save() {
    setBusy(true);
    setError('');
    try {
      const capabilities = JSON.parse(draft.capabilities_json);
      if (draft.id) {
        await updateFunctionPackage(draft.id, {
          display_name: draft.display_name,
          description: draft.description,
          source: draft.source,
          entrypoint: draft.entrypoint,
          capabilities,
        });
      } else {
        await createFunctionPackage({
          name: draft.name,
          version: draft.version,
          display_name: draft.display_name,
          description: draft.description,
          runtime: draft.runtime,
          source: draft.source,
          entrypoint: draft.entrypoint,
          capabilities,
        });
      }
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!draft.id) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete package?')) return;
    setBusy(true);
    try {
      await deleteFunctionPackage(draft.id);
      setDraft(emptyDraft());
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function execute() {
    if (!draft.id) return;
    setBusy(true);
    try {
      const input = JSON.parse(executeInputJson);
      setExecuteResult(await executeFunctionPackage(draft.id, { input }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Execute failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Functions</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Manage ontology function packages, view metrics, recent runs, and execute against arbitrary input.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Packages ({packages.length})</p>
          <button type="button" onClick={() => setDraft(emptyDraft())} className="of-button" style={{ marginTop: 8, fontSize: 12 }}>
            New package
          </button>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {packages.map((p) => (
              <li key={p.id}>
                <button
                  type="button"
                  onClick={() => loadPackage(p)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 10,
                    borderRadius: 8,
                    border: `1px solid ${draft.id === p.id ? '#1d4ed8' : 'var(--border-default)'}`,
                    background: draft.id === p.id ? '#eff6ff' : 'transparent',
                    cursor: 'pointer',
                    marginBottom: 4,
                  }}
                >
                  <strong>{p.display_name}</strong> · {p.version} · {p.runtime}
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Package draft</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
            <label style={{ fontSize: 13 }}>
              Name
              <input value={draft.name} disabled={Boolean(draft.id)} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Version
              <input value={draft.version} disabled={Boolean(draft.id)} onChange={(e) => setDraft((d) => ({ ...d, version: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Display name
              <input value={draft.display_name} onChange={(e) => setDraft((d) => ({ ...d, display_name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Runtime
              <select value={draft.runtime} disabled={Boolean(draft.id)} onChange={(e) => setDraft((d) => ({ ...d, runtime: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                {RUNTIMES.map((r) => (
                  <option key={r} value={r}>{r}</option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              Entrypoint
              <input value={draft.entrypoint} onChange={(e) => setDraft((d) => ({ ...d, entrypoint: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Source
              <textarea value={draft.source} onChange={(e) => setDraft((d) => ({ ...d, source: e.target.value }))} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 200 }} />
            </label>
            <JsonEditor
              label="Capabilities JSON"
              value={draft.capabilities_json}
              onChange={(v) => setDraft((d) => ({ ...d, capabilities_json: v }))}
              minHeight={140}
            />
            <div style={{ display: 'flex', gap: 6 }}>
              <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">
                {draft.id ? 'Update' : 'Create'}
              </button>
              {draft.id && (
                <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
                  Delete
                </button>
              )}
            </div>
          </div>

          {draft.id && (
            <>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Execute</p>
              <JsonEditor value={executeInputJson} onChange={setExecuteInputJson} minHeight={80} placeholder='{"input": ...}' />
              <button type="button" onClick={() => void execute()} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 6 }}>
                Run
              </button>
              {!!executeResult && (
                <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 240 }}>
                  {JSON.stringify(executeResult, null, 2)}
                </pre>
              )}
            </>
          )}
        </section>
      </div>

      {metrics.length > 0 && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Metrics</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {metrics.map((m) => (
              <li key={m.package.id}>
                <strong>{m.package.display_name}</strong> · {m.total_runs} runs · {(m.success_rate * 100).toFixed(0)}% · avg {m.avg_duration_ms ?? '—'}ms
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Recent runs ({runs.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {runs.map((r) => (
            <li key={r.id}>
              <strong>{r.function_package_name}</strong> v{r.function_package_version} · {r.status} · {r.duration_ms}ms · {new Date(r.started_at).toLocaleString()}
            </li>
          ))}
        </ul>
      </section>
    </section>
  );
}
