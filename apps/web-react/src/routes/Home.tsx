import { Link } from 'react-router-dom';

const MIGRATED_ROUTES: { path: string; title: string; description: string }[] = [
  { path: '/settings', title: 'Settings', description: 'Identity, RBAC, ABAC, MFA, API keys, SSO.' },
  { path: '/dashboards', title: 'Dashboards', description: 'Charts, tables, KPI cards on a responsive grid.' },
  { path: '/lineage', title: 'Lineage', description: 'Dataset / pipeline / workflow graph with impact analysis and build dispatch.' },
  { path: '/notebooks', title: 'Notebooks', description: 'Multi-kernel notebooks with Monaco-backed cells and a workspace file tree.' },
  { path: '/notepad', title: 'Notepad', description: 'Markdown documents with widget embeds, presence, and AIP knowledge-base indexing.' },
  { path: '/contour', title: 'Contour', description: 'Top-down dataset analysis: join, drill, chart-to-chart filter, materialize.' },
  { path: '/quiver', title: 'Quiver', description: 'Time-series + grouped object analytics with reusable Vega-Lite visual functions.' },
  { path: '/vertex', title: 'Vertex', description: 'Cytoscape graph + templates + scenarios + media annotations + EChartView sidecars.' },
  { path: '/geospatial', title: 'Geospatial', description: 'MapLibre canvas with layers, queries, clustering, geocoding, routing, templates.' },
  { path: '/queries', title: 'Queries', description: 'SQL editor with saved queries, explain plan, and ontology catalog inspection.' },
  { path: '/search', title: 'Search', description: 'Discovery hub linking to object explorer, queries, and ontology.' },
  { path: '/auth/login', title: 'Sign in', description: 'Login + register + MFA + SSO callback.' },
  { path: '/charts-demo', title: 'Charts demo', description: 'ECharts wrapper validator.' },
  { path: '/monaco-demo', title: 'Monaco demo', description: 'Monaco editor wrapper validator.' },
  { path: '/maplibre-demo', title: 'MapLibre demo', description: 'MapLibre map wrapper validator.' },
  { path: '/cytoscape-demo', title: 'Cytoscape demo', description: 'Cytoscape graph wrapper validator.' },
];

export function Home() {
  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header className="of-hero-strip">
        <p className="of-eyebrow">OpenFoundry</p>
        <h1 className="of-heading-xl">React shell</h1>
        <p className="of-text-muted">
          Routes ported so far. The Svelte app at <code>apps/web</code> still owns everything
          else; each migrated folder gets registered in <code>src/router.tsx</code>.
        </p>
      </header>

      <div
        className="of-card-grid"
        style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))' }}
      >
        {MIGRATED_ROUTES.map((route) => (
          <Link key={route.path} to={route.path} className="of-card" style={{ textDecoration: 'none' }}>
            <p className="of-eyebrow">{route.path}</p>
            <h2 className="of-heading-md">{route.title}</h2>
            <p className="of-text-muted" style={{ fontSize: 13 }}>
              {route.description}
            </p>
          </Link>
        ))}
      </div>
    </section>
  );
}
