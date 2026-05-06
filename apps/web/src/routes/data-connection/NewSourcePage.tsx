import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import {
  CONNECTOR_FAMILY_ORDER,
  FALLBACK_CONNECTOR_CATALOG,
  capabilityLabel,
  dataConnection,
  filterCatalog,
  type ConnectorCatalogEntry,
  type ConnectorFamily,
  type DiscoveredSource,
  type SourceWorker,
  type TestConnectionResult,
} from '@/lib/api/data-connection';

type Step = 1 | 2 | 3;

export function NewSourcePage() {
  const [catalog, setCatalog] = useState<ConnectorCatalogEntry[]>(FALLBACK_CONNECTOR_CATALOG);
  const [query, setQuery] = useState('');
  const [familyFilter, setFamilyFilter] = useState<ConnectorFamily | 'All'>('All');
  const [step, setStep] = useState<Step>(1);
  const [selected, setSelected] = useState<ConnectorCatalogEntry | null>(null);
  const [name, setName] = useState('');
  const [worker, setWorker] = useState<SourceWorker>('foundry');
  const [configJson, setConfigJson] = useState('{}');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [createdSourceId, setCreatedSourceId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null);
  const [discovered, setDiscovered] = useState<DiscoveredSource[]>([]);
  const [selectedSelectors, setSelectedSelectors] = useState<Record<string, boolean>>({});
  const navigate = useNavigate();

  useEffect(() => {
    dataConnection
      .getCatalog()
      .then((res) => setCatalog(res.connectors))
      .catch(() => setCatalog(FALLBACK_CONNECTOR_CATALOG));
  }, []);

  const filtered = useMemo(
    () => filterCatalog(catalog, query).filter((e) => familyFilter === 'All' || e.family === familyFilter),
    [catalog, query, familyFilter],
  );

  const grouped = useMemo(() => {
    const map = new Map<ConnectorFamily | 'Other', ConnectorCatalogEntry[]>();
    for (const entry of filtered) {
      const key = (entry.family ?? 'Other') as ConnectorFamily | 'Other';
      const list = map.get(key) ?? [];
      list.push(entry);
      map.set(key, list);
    }
    const ordered: { family: ConnectorFamily | 'Other'; entries: ConnectorCatalogEntry[] }[] = [];
    for (const fam of CONNECTOR_FAMILY_ORDER) {
      const entries = map.get(fam);
      if (entries) ordered.push({ family: fam, entries });
    }
    const other = map.get('Other');
    if (other) ordered.push({ family: 'Other', entries: other });
    return ordered;
  }, [filtered]);

  function pick(entry: ConnectorCatalogEntry) {
    setSelected(entry);
    setName(`${entry.name} source`);
    setStep(2);
  }

  async function createSource() {
    if (!selected) return;
    setBusy(true);
    setError('');
    try {
      const cfg = JSON.parse(configJson || '{}');
      const created = await dataConnection.createSource({
        name: name.trim(),
        connector_type: selected.type,
        worker,
        config: cfg,
      });
      setCreatedSourceId(created.id);
      setStep(3);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  async function testConnection() {
    if (!createdSourceId) return;
    setBusy(true);
    try {
      setTestResult(await dataConnection.testConnection(createdSourceId));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Test failed');
    } finally {
      setBusy(false);
    }
  }

  async function discover() {
    if (!createdSourceId) return;
    setBusy(true);
    try {
      const res = await dataConnection.discoverSources(createdSourceId);
      setDiscovered(res.sources);
      const next: Record<string, boolean> = {};
      for (const s of res.sources) next[s.selector] = false;
      setSelectedSelectors(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Discover failed');
    } finally {
      setBusy(false);
    }
  }

  async function bulkRegister() {
    if (!createdSourceId) return;
    const items = discovered
      .filter((d) => selectedSelectors[d.selector])
      .map((d) => ({ selector: d.selector, source_kind: d.source_kind ?? undefined }));
    if (items.length === 0) {
      setError('Select at least one selector to register.');
      return;
    }
    setBusy(true);
    try {
      await dataConnection.bulkRegister(createdSourceId, items);
      navigate(`/data-connection/sources/${createdSourceId}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Register failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Back to sources</Link>
      <header>
        <h1 className="of-heading-xl">New source</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>Step {step} of 3 · {step === 1 ? 'Pick connector' : step === 2 ? 'Configure' : 'Test + register'}</p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {step === 1 && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search connectors…" className="of-input" style={{ flex: 1, minWidth: 200 }} />
              <select value={familyFilter} onChange={(e) => setFamilyFilter(e.target.value as ConnectorFamily | 'All')} className="of-input">
                <option value="All">All families</option>
                {CONNECTOR_FAMILY_ORDER.map((f) => (
                  <option key={f} value={f}>{f}</option>
                ))}
              </select>
            </div>
          </section>

          {grouped.map(({ family, entries }) => (
            <section key={family} className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">{family}</p>
              <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }}>
                {entries.map((entry) => (
                  <li key={entry.type}>
                    <button
                      type="button"
                      onClick={() => pick(entry)}
                      style={{
                        width: '100%',
                        textAlign: 'left',
                        padding: 12,
                        borderRadius: 12,
                        border: '1px solid var(--border-default)',
                        background: 'transparent',
                        cursor: 'pointer',
                      }}
                    >
                      <strong>{entry.name}</strong>
                      <p style={{ fontSize: 11, color: 'var(--text-muted)', margin: '4px 0' }}>{entry.description}</p>
                      <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                        {entry.capabilities.map((c) => (
                          <span key={c} style={{ fontSize: 10, padding: '2px 6px', background: 'var(--bg-subtle)', borderRadius: 999 }}>{capabilityLabel(c)}</span>
                        ))}
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </>
      )}

      {step === 2 && selected && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <p className="of-eyebrow">Configure {selected.name}</p>
          <p className="of-text-muted" style={{ fontSize: 12 }}>{selected.description}</p>
          <label style={{ fontSize: 13 }}>
            Source name
            <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Worker
            <select value={worker} onChange={(e) => setWorker(e.target.value as SourceWorker)} className="of-input" style={{ marginTop: 4 }}>
              <option value="foundry">foundry</option>
              <option value="agent">agent (legacy)</option>
            </select>
          </label>
          <label style={{ fontSize: 13 }}>
            Config JSON
            <textarea value={configJson} onChange={(e) => setConfigJson(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 240 }} />
          </label>
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => setStep(1)} className="of-button">← Back</button>
            <button type="button" onClick={() => void createSource()} disabled={busy || !name.trim()} className="of-button of-button--primary">
              Create source
            </button>
          </div>
        </section>
      )}

      {step === 3 && createdSourceId && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
          <p className="of-eyebrow">Test & register</p>
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => void testConnection()} disabled={busy} className="of-button">Test connection</button>
            <button type="button" onClick={() => void discover()} disabled={busy} className="of-button">Discover sources</button>
            <Link to={`/data-connection/sources/${createdSourceId}`} className="of-button">Skip → source detail</Link>
          </div>
          {testResult && (
            <div style={{ padding: 10, background: testResult.success ? '#d1fae5' : '#fee2e2', borderRadius: 8, fontSize: 12 }}>
              <strong>{testResult.success ? '✓' : '✗'}</strong> {testResult.message}
              {testResult.latency_ms !== null && ` · ${testResult.latency_ms}ms`}
            </div>
          )}
          {discovered.length > 0 && (
            <>
              <p className="of-eyebrow">Discovered ({discovered.length})</p>
              <ul style={{ paddingLeft: 0, listStyle: 'none', maxHeight: 320, overflow: 'auto' }}>
                {discovered.map((d) => (
                  <li key={d.selector} style={{ padding: 6, borderBottom: '1px solid var(--border-subtle)' }}>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
                      <input
                        type="checkbox"
                        checked={Boolean(selectedSelectors[d.selector])}
                        onChange={(e) => setSelectedSelectors((prev) => ({ ...prev, [d.selector]: e.target.checked }))}
                      />
                      <code>{d.selector}</code>
                      {d.source_kind && <span className="of-text-muted" style={{ marginLeft: 6 }}>· {d.source_kind}</span>}
                    </label>
                  </li>
                ))}
              </ul>
              <button type="button" onClick={() => void bulkRegister()} disabled={busy} className="of-button of-button--primary">
                Register selected
              </button>
            </>
          )}
        </section>
      )}
    </section>
  );
}
