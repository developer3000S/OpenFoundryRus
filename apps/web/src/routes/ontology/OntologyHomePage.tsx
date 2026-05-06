import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  listActionTypes,
  listLinkTypes,
  listObjectTypes,
  listSharedPropertyTypes,
  searchOntology,
  type ObjectType,
  type SearchResult,
} from '@/lib/api/ontology';

type SearchKind = 'all' | 'object_type' | 'object_instance' | 'action_type' | 'link_type' | 'shared_property_type';

const SURFACES: { name: string; route: string; summary: string }[] = [
  { name: 'Action types', route: '/action-types', summary: 'Author actions on object types, validate + execute, what-if branches, metrics.' },
  { name: 'Functions', route: '/functions', summary: 'Function packages with capabilities, source, metrics, runs, and execute.' },
  { name: 'Indexing (Funnel)', route: '/ontology-indexing', summary: 'Funnel sources + property mappings + run history + health summary.' },
  { name: 'Object databases', route: '/object-databases', summary: 'Storage topology: object rows, link edges, search projections, indexes.' },
  { name: 'Object & link types', route: '/object-link-types', summary: 'CRUD on object types, properties, link types, shared property types.' },
  { name: 'Interfaces', route: '/interfaces', summary: 'Interface library + property definitions + object-type implementation bindings.' },
  { name: 'Object explorer', route: '/object-explorer', summary: 'Lexical + semantic search across the ontology with object-set creation.' },
  { name: 'Object monitors', route: '/object-monitors', summary: 'Workflow-backed monitors over object sets/types.' },
  { name: 'Object views', route: '/object-views', summary: 'Configure full-page and side-panel object views per type.' },
  { name: 'Ontology design', route: '/ontology-design', summary: 'Scorecard + anti-patterns + playbook + review notes.' },
  { name: 'Ontology projects', route: '/ontologies', summary: 'Project workspaces: branches, proposals, migrations, resource bindings.' },
  { name: 'Ontology manager (hub)', route: '/ontology-manager', summary: 'Aggregate hub for ontology authoring + dataset bindings wizard.' },
];

export function OntologyHomePage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [actionCount, setActionCount] = useState(0);
  const [linkCount, setLinkCount] = useState(0);
  const [sharedCount, setSharedCount] = useState(0);
  const [error, setError] = useState('');

  const [query, setQuery] = useState('');
  const [kind, setKind] = useState<SearchKind>('all');
  const [busy, setBusy] = useState(false);
  const [results, setResults] = useState<SearchResult[]>([]);

  async function refresh() {
    setError('');
    try {
      const [types, actions, links, shared] = await Promise.all([
        listObjectTypes({ per_page: 200 }),
        listActionTypes({ per_page: 1 }),
        listLinkTypes({ per_page: 1 }),
        listSharedPropertyTypes({ per_page: 1 }),
      ]);
      setObjectTypes(types.data);
      setActionCount(actions.total);
      setLinkCount(links.total);
      setSharedCount(shared.total);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load ontology');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function runSearch() {
    if (!query.trim()) return;
    setBusy(true);
    setError('');
    try {
      const res = await searchOntology({
        query: query.trim(),
        kind: kind === 'all' ? undefined : kind,
        limit: 50,
      });
      setResults(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Search failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Ontology</h1>
        <p className="of-text-muted" style={{ marginTop: 4, maxWidth: 720 }}>
          Models, types, and applications. Browse object types, search semantically, and jump into
          authoring surfaces (actions, functions, interfaces, indexing, projects).
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Stats</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          <li>{objectTypes.length} object types</li>
          <li>{linkCount} link types</li>
          <li>{actionCount} action types</li>
          <li>{sharedCount} shared property types</li>
        </ul>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Semantic search</p>
        <div style={{ display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search the ontology…"
            className="of-input"
            style={{ flex: 1, minWidth: 240 }}
            onKeyDown={(e) => { if (e.key === 'Enter') void runSearch(); }}
          />
          <select value={kind} onChange={(e) => setKind(e.target.value as SearchKind)} className="of-input">
            <option value="all">All kinds</option>
            <option value="object_type">Object types</option>
            <option value="object_instance">Object instances</option>
            <option value="action_type">Action types</option>
            <option value="link_type">Link types</option>
            <option value="shared_property_type">Shared property types</option>
          </select>
          <button type="button" onClick={() => void runSearch()} disabled={busy || !query.trim()} className="of-button of-button--primary">
            Search
          </button>
        </div>
        {results.length > 0 && (
          <ul style={{ marginTop: 12, paddingLeft: 0, listStyle: 'none' }}>
            {results.map((r, i) => (
              <li key={`${r.kind}-${r.id ?? i}`} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{r.title || r.id}</strong> <span className="of-text-muted">· {r.kind}</span>
                {r.subtitle && <span className="of-text-muted" style={{ fontSize: 11, marginLeft: 6 }}>· {r.subtitle}</span>}
                {r.snippet && <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>{r.snippet}</p>}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Object types ({objectTypes.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))' }}>
          {objectTypes.map((t) => (
            <li key={t.id}>
              <Link to={`/ontology/${t.id}`} className="of-card" style={{ display: 'block', textDecoration: 'none', padding: 12 }}>
                <strong>{t.display_name}</strong>
                <p className="of-text-muted" style={{ fontSize: 11, margin: '4px 0' }}>
                  {t.name} · pk: {t.primary_key_property ?? '—'}
                </p>
                {t.description && <p style={{ fontSize: 12, margin: 0 }}>{t.description}</p>}
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Application surfaces</p>
        <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))' }}>
          {SURFACES.map((s) => (
            <li key={s.route}>
              <Link to={s.route} className="of-card" style={{ display: 'block', textDecoration: 'none', padding: 12 }}>
                <strong>{s.name}</strong>
                <p style={{ fontSize: 12, margin: '4px 0' }}>{s.summary}</p>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Tools</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          <li><Link to="/ontology/types">+ Create object type</Link></li>
          <li><Link to="/ontology/object-sets">Object sets editor</Link></li>
          <li><Link to="/ontology/graph">Graph explorer</Link></li>
          <li><Link to="/ontology-manager/bindings">Dataset → object type binding wizard</Link></li>
        </ul>
      </section>
    </section>
  );
}
