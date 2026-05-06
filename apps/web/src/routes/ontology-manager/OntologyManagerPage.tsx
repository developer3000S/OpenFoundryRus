import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  listInterfaces,
  listLinkTypes,
  listObjectTypes,
  listProjects,
  listSharedPropertyTypes,
  type LinkType,
  type OntologyInterface,
  type ObjectType,
  type OntologyProject,
  type SharedPropertyType,
} from '@/lib/api/ontology';

type Section = 'overview' | 'types' | 'interfaces' | 'shared' | 'links' | 'projects';

export function OntologyManagerPage() {
  const [section, setSection] = useState<Section>('overview');
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [interfaces, setInterfaces] = useState<OntologyInterface[]>([]);
  const [shared, setShared] = useState<SharedPropertyType[]>([]);
  const [linkTypes, setLinkTypes] = useState<LinkType[]>([]);
  const [projects, setProjects] = useState<OntologyProject[]>([]);
  const [search, setSearch] = useState('');
  const [error, setError] = useState('');

  async function refresh() {
    setError('');
    try {
      const [types, ifs, sh, links, prs] = await Promise.all([
        listObjectTypes({ per_page: 200, search: search || undefined }),
        listInterfaces({ per_page: 200, search: search || undefined }),
        listSharedPropertyTypes({ per_page: 200, search: search || undefined }),
        listLinkTypes({ per_page: 200 }),
        listProjects({ per_page: 200 }),
      ]);
      setObjectTypes(types.data);
      setInterfaces(ifs.data);
      setShared(sh.data);
      setLinkTypes(links.data);
      setProjects(prs.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Ontology Manager</h1>
          <p className="of-text-muted" style={{ marginTop: 4 }}>
            Hub: object types, interfaces, shared properties, links, projects, dataset bindings.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Link to="/ontology-manager/bindings" className="of-button">+ Bind dataset to object type</Link>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <div style={{ display: 'flex', gap: 6 }}>
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search…" className="of-input" />
          <button type="button" onClick={() => void refresh()} className="of-button">Apply</button>
        </div>
      </section>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
        {(['overview', 'types', 'interfaces', 'shared', 'links', 'projects'] as Section[]).map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => setSection(s)}
            style={{
              fontSize: 12,
              borderBottom: section === s ? '2px solid #1d4ed8' : '2px solid transparent',
              background: 'transparent',
              border: 'none',
              padding: '8px 16px',
              cursor: 'pointer',
              color: section === s ? 'var(--text-default)' : 'var(--text-muted)',
              textTransform: 'capitalize',
            }}
          >
            {s}
          </button>
        ))}
      </div>

      {section === 'overview' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Stats</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            <li>{objectTypes.length} object types</li>
            <li>{interfaces.length} interfaces</li>
            <li>{shared.length} shared property types</li>
            <li>{linkTypes.length} link types</li>
            <li>{projects.length} projects</li>
          </ul>
          <p className="of-eyebrow" style={{ marginTop: 12 }}>Related routes</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            <li><Link to="/object-link-types">Object & link types →</Link></li>
            <li><Link to="/interfaces">Interfaces →</Link></li>
            <li><Link to="/ontologies">Ontology projects →</Link></li>
            <li><Link to="/ontology-design">Ontology design →</Link></li>
            <li><Link to="/ontology-indexing">Ontology indexing (Funnel) →</Link></li>
            <li><Link to="/projects">Workspace projects →</Link></li>
            <li><Link to="/ontology-manager/bindings">Dataset → ObjectType bindings →</Link></li>
          </ul>
        </section>
      )}

      {section === 'types' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Object types ({objectTypes.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {objectTypes.map((t) => (
              <li key={t.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{t.display_name}</strong> · {t.name} · pk: {t.primary_key_property ?? '—'}
              </li>
            ))}
          </ul>
        </section>
      )}

      {section === 'interfaces' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Interfaces ({interfaces.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {interfaces.map((i) => (
              <li key={i.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{i.display_name}</strong> · {i.name}
                {i.description && <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>{i.description}</p>}
              </li>
            ))}
          </ul>
        </section>
      )}

      {section === 'shared' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Shared property types ({shared.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {shared.map((s) => (
              <li key={s.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{s.display_name}</strong> · {s.name} · {s.property_type}
              </li>
            ))}
          </ul>
        </section>
      )}

      {section === 'links' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Link types ({linkTypes.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {linkTypes.map((l) => (
              <li key={l.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <strong>{l.display_name}</strong> · {l.name} · {l.source_type_id} → {l.target_type_id}
              </li>
            ))}
          </ul>
        </section>
      )}

      {section === 'projects' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Projects ({projects.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {projects.map((p) => (
              <li key={p.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <Link to={`/projects/${p.id}`}><strong>{p.display_name || p.slug}</strong></Link> · {p.id}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
