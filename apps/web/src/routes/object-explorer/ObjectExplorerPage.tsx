import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  createObjectSet,
  evaluateObjectSet,
  getObjectView,
  listObjectSets,
  listObjectTypes,
  searchOntology,
  type ObjectSetDefinition,
  type ObjectType,
  type ObjectViewResponse,
  type SearchResult,
} from '@/lib/api/ontology';

type SearchMode = 'lexical' | 'semantic';

interface RecentItem {
  kind: string;
  id: string;
  title: string;
  route: string;
  createdAt: string;
}

const RECENTS_KEY = 'of.objectExplorer.recents';

function readRecents(): RecentItem[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(RECENTS_KEY);
    return raw ? (JSON.parse(raw) as RecentItem[]) : [];
  } catch {
    return [];
  }
}

function writeRecents(items: RecentItem[]) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(RECENTS_KEY, JSON.stringify(items.slice(0, 30)));
}

export function ObjectExplorerPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [objectSets, setObjectSets] = useState<ObjectSetDefinition[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('lexical');
  const [searchKindFilter, setSearchKindFilter] = useState('');
  const [searchTypeFilter, setSearchTypeFilter] = useState('');
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchError, setSearchError] = useState('');
  const [recents, setRecents] = useState<RecentItem[]>([]);
  const [selectedObject, setSelectedObject] = useState<ObjectViewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  // Object set creation
  const [newSetName, setNewSetName] = useState('Saved set');
  const [newSetType, setNewSetType] = useState('');
  const [newSetWhatIf, setNewSetWhatIf] = useState('');
  const [evaluation, setEvaluation] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [typeRes, setRes] = await Promise.all([listObjectTypes({ per_page: 200 }), listObjectSets()]);
        if (cancelled) return;
        setObjectTypes(typeRes.data);
        setObjectSets(setRes.data);
        if (typeRes.data[0]) setNewSetType(typeRes.data[0].id);
        setRecents(readRecents());
      } catch (cause) {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load object explorer');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  async function runSearch() {
    if (!searchQuery.trim()) return;
    setSearchLoading(true);
    setSearchError('');
    try {
      const res = await searchOntology({
        query: searchQuery,
        kind: searchKindFilter || undefined,
        object_type_id: searchTypeFilter || undefined,
        limit: 50,
        semantic: searchMode === 'semantic',
      });
      setSearchResults(res.data);
    } catch (cause) {
      setSearchError(cause instanceof Error ? cause.message : 'Search failed');
    } finally {
      setSearchLoading(false);
    }
  }

  async function selectResult(result: SearchResult) {
    if (!result.object_type_id) return;
    setPreviewLoading(true);
    try {
      const view = await getObjectView(result.object_type_id, result.id);
      setSelectedObject(view);
      const recent: RecentItem = {
        kind: result.kind,
        id: result.id,
        title: result.title,
        route: result.route,
        createdAt: new Date().toISOString(),
      };
      const next = [recent, ...recents.filter((r) => r.id !== recent.id)];
      setRecents(next);
      writeRecents(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load object view');
    } finally {
      setPreviewLoading(false);
    }
  }

  async function createSet() {
    setBusy(true);
    try {
      const created = await createObjectSet({
        name: newSetName,
        base_object_type_id: newSetType,
        what_if_label: newSetWhatIf || undefined,
      });
      const res = await listObjectSets();
      setObjectSets(res.data);
      setEvaluation(created);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create object set');
    } finally {
      setBusy(false);
    }
  }

  async function evaluateSet(id: string) {
    setBusy(true);
    try {
      setEvaluation(await evaluateObjectSet(id, { limit: 50 }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to evaluate object set');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16, padding: 24 }}>
      <header>
        <h1 className="of-heading-xl">Object explorer</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Lexical + semantic search across the ontology, recent objects (localStorage), saved object sets with
          evaluation.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Search</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
          <input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search ontology…"
            className="of-input"
            style={{ flex: 1, minWidth: 240 }}
            onKeyDown={(e) => e.key === 'Enter' && runSearch()}
          />
          <select
            value={searchMode}
            onChange={(e) => setSearchMode(e.target.value as SearchMode)}
            className="of-input"
            style={{ width: 'auto' }}
          >
            <option value="lexical">Lexical</option>
            <option value="semantic">Semantic</option>
          </select>
          <input
            value={searchKindFilter}
            onChange={(e) => setSearchKindFilter(e.target.value)}
            placeholder="Kind filter"
            className="of-input"
            style={{ width: 160 }}
          />
          <select
            value={searchTypeFilter}
            onChange={(e) => setSearchTypeFilter(e.target.value)}
            className="of-input"
            style={{ width: 'auto' }}
          >
            <option value="">All types</option>
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name}
              </option>
            ))}
          </select>
          <button type="button" onClick={runSearch} disabled={searchLoading} className="of-button of-button--primary">
            {searchLoading ? 'Searching…' : 'Search'}
          </button>
        </div>

        {searchError && (
          <p className="of-status-danger" style={{ padding: '8px 12px', borderRadius: 'var(--radius-md)', fontSize: 13, marginTop: 8 }}>
            {searchError}
          </p>
        )}

        {searchResults.length > 0 && (
          <div style={{ display: 'grid', gap: 6, marginTop: 12 }}>
            {searchResults.map((result, index) => (
              <button
                key={`${result.kind}-${result.id}-${index}`}
                type="button"
                onClick={() => void selectResult(result)}
                className="of-panel-muted"
                style={{ padding: 10, textAlign: 'left', cursor: 'pointer' }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                  <strong>{result.title}</strong>
                  <span className="of-chip">
                    {result.kind} · {result.score.toFixed(2)}
                  </span>
                </div>
                {result.subtitle && <p className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>{result.subtitle}</p>}
                <p className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>
                  {result.snippet}
                </p>
              </button>
            ))}
          </div>
        )}
      </section>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Selected object</p>
          {previewLoading ? (
            <p className="of-text-muted" style={{ marginTop: 8 }}>Loading preview…</p>
          ) : selectedObject ? (
            <>
              <h3 className="of-heading-md" style={{ marginTop: 8 }}>{selectedObject.object.id.slice(0, 8)}</h3>
              <p className="of-text-muted" style={{ fontSize: 13 }}>Type: {selectedObject.object.object_type_id}</p>
              <pre style={{ marginTop: 10, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 240 }}>
                {JSON.stringify(selectedObject.summary, null, 2)}
              </pre>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Neighbors ({selectedObject.neighbors.length})</p>
              <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
                {selectedObject.neighbors.slice(0, 10).map((n, i) => (
                  <li key={i}>{n.direction} · {n.link_name} → {n.object.id.slice(0, 8)}</li>
                ))}
              </ul>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Applicable actions</p>
              <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
                {selectedObject.applicable_actions.map((a) => (
                  <li key={a.id}>{a.display_name}</li>
                ))}
              </ul>
            </>
          ) : (
            <p className="of-text-muted">Select a search result to preview.</p>
          )}
        </section>

        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Recent ({recents.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {recents.map((r) => (
              <li key={r.id}>
                <Link to={r.route}>{r.title}</Link>{' '}
                <span className="of-text-muted" style={{ fontSize: 11 }}>
                  · {r.kind} · {new Date(r.createdAt).toLocaleString()}
                </span>
              </li>
            ))}
          </ul>
        </section>
      </div>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Saved object sets</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
          {objectSets.map((s) => (
            <li key={s.id} style={{ marginBottom: 4 }}>
              <strong>{s.name}</strong> — base {s.base_object_type_id}
              {' '}
              <button
                type="button"
                onClick={() => void evaluateSet(s.id)}
                disabled={busy}
                className="of-button"
                style={{ fontSize: 11, marginLeft: 6 }}
              >
                Evaluate
              </button>
            </li>
          ))}
          {objectSets.length === 0 && <li className="of-text-muted">No saved object sets yet.</li>}
        </ul>

        <p className="of-eyebrow" style={{ marginTop: 14 }}>Create new object set</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
          <input
            value={newSetName}
            onChange={(e) => setNewSetName(e.target.value)}
            placeholder="Set name"
            className="of-input"
            style={{ width: 220 }}
          />
          <select
            value={newSetType}
            onChange={(e) => setNewSetType(e.target.value)}
            className="of-input"
            style={{ width: 'auto' }}
          >
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name}
              </option>
            ))}
          </select>
          <input
            value={newSetWhatIf}
            onChange={(e) => setNewSetWhatIf(e.target.value)}
            placeholder="What-if label (optional)"
            className="of-input"
            style={{ width: 220 }}
          />
          <button type="button" onClick={() => void createSet()} disabled={busy} className="of-button of-button--primary">
            Create
          </button>
        </div>

        {!!evaluation && (
          <pre style={{ marginTop: 12, padding: 12, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 280 }}>
            {JSON.stringify(evaluation, null, 2)}
          </pre>
        )}
      </section>
    </section>
  );
}
