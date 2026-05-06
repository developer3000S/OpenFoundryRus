import { Link } from 'react-router-dom';

const MIGRATED_ROUTES: { path: string; title: string; description: string }[] = [
  { path: '/settings', title: 'Settings', description: 'Identity, RBAC, ABAC, MFA, API keys, SSO.' },
  { path: '/dashboards', title: 'Dashboards', description: 'Charts, tables, KPI cards on a responsive grid.' },
  { path: '/lineage', title: 'Lineage', description: 'Dataset / pipeline / workflow graph with impact analysis and build dispatch.' },
  { path: '/notebooks', title: 'Notebooks', description: 'Multi-kernel notebooks with Monaco-backed cells and a workspace file tree.' },
  { path: '/notepad', title: 'Notepad', description: 'Markdown documents with widget embeds, presence, and AIP knowledge-base indexing.' },
  { path: '/reports', title: 'Reports', description: 'Report definitions + schedules + distributions + PDF/PPTX generation.' },
  { path: '/contour', title: 'Contour', description: 'Top-down dataset analysis: join, drill, chart-to-chart filter, materialize.' },
  { path: '/quiver', title: 'Quiver', description: 'Time-series + grouped object analytics with reusable Vega-Lite visual functions.' },
  { path: '/vertex', title: 'Vertex', description: 'Cytoscape graph + templates + scenarios + media annotations + EChartView sidecars.' },
  { path: '/geospatial', title: 'Geospatial', description: 'MapLibre canvas with layers, queries, clustering, geocoding, routing, templates.' },
  { path: '/queries', title: 'Queries', description: 'SQL editor with saved queries, explain plan, and ontology catalog inspection.' },
  { path: '/search', title: 'Search', description: 'Discovery hub linking to object explorer, queries, and ontology.' },
  { path: '/global-branching', title: 'Global branching', description: 'Workspace branches + scoped resources + merge / promote workflow.' },
  { path: '/developers', title: 'Developers', description: 'Plugin SDK + CLI cookbook + OpenAPI explorer + Terraform provider + Git integrations.' },
  { path: '/object-databases', title: 'Object databases', description: 'Storage topology: object rows, link edges, search projections, Funnel runs, indexes.' },
  { path: '/workflows', title: 'Workflows', description: 'Builder + run history + HITL approvals for event/cron/manual/webhook automations.' },
  { path: '/ontology-design', title: 'Ontology design', description: 'Scorecard + anti-patterns + playbook + review notes for ontology quality.' },
  { path: '/dynamic-scheduling', title: 'Dynamic scheduling', description: 'Machinery queue board with capability rows, drag-staged moves, conflict detection.' },
  { path: '/interfaces', title: 'Interfaces', description: 'Interface library + property definitions + object-type implementation bindings.' },
  { path: '/build-schedules', title: 'Build schedules', description: 'Find/manage schedules with file/user/project filters, name search, pause+sort.' },
  { path: '/fusion', title: 'Fusion', description: 'Identity resolution: match rules + merge strategies + jobs + clusters + reviews + golden records.' },
  { path: '/nexus', title: 'Nexus', description: 'Cross-org sharing: peers + contracts + spaces + shares + federated query + audit bridge.' },
  { path: '/audit', title: 'Audit', description: 'Immutable audit chain + retention policies + GDPR workflows + governance templates.' },
  { path: '/code-repos', title: 'Code repos', description: 'Object-backed repos: branches + commits + CI + merge requests + reviewers + comments.' },
  { path: '/marketplace', title: 'Marketplace', description: 'Listings + versions + reviews + installs + product fleets + enrollment branches.' },
  { path: '/virtual-tables', title: 'Virtual tables', description: 'Source-pointer tables (BigQuery, Snowflake, Iceberg…) with capability + update detection management.' },
  { path: '/ai', title: 'AI Platform', description: 'Providers + prompts + knowledge bases + tools + agents + chat + guardrails. JSON-driven editors.' },
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
