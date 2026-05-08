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
  { path: '/object-views', title: 'Object views', description: 'Configure full-page and side-panel object views per type with localStorage version history.' },
  { path: '/object-explorer', title: 'Object explorer', description: 'Lexical + semantic search across the ontology with object-set creation and evaluation.' },
  { path: '/iceberg-tables', title: 'Iceberg tables', description: 'List + detail (8 tabs): schema, snapshots, metadata, branches, markings, catalog access tokens.' },
  { path: '/ontology-indexing', title: 'Ontology indexing', description: 'Funnel sources + property mappings + run history + health summary for ontology hydration.' },
  { path: '/ontologies', title: 'Ontologies', description: 'Project workspaces: branches, proposals, migrations, resource bindings, working state.' },
  { path: '/object-monitors', title: 'Object monitors', description: 'Workflow-backed monitors over object sets/types with notification + submit-action steps.' },
  { path: '/streaming', title: 'Streaming', description: 'Streams + windows + topologies + connectors + live tail. JSON-driven editors.' },
  { path: '/machinery', title: 'Machinery', description: 'Ontology rules + machinery insights + queue depth/recommendations + workflow approvals.' },
  { path: '/media-sets', title: 'Media sets', description: 'Branch-aware media stores: list + create + upload + items detail with delete.' },
  { path: '/object-link-types', title: 'Object & link types', description: 'CRUD on object types, properties, link types, shared property types with attach/detach.' },
  { path: '/builds', title: 'Builds', description: 'V1 build envelopes: filter by state, abort, run, jobs detail with outputs + input resolutions.' },
  { path: '/foundry-rules', title: 'Foundry rules', description: 'Per-type rule CRUD with simulate/apply against a target object id.' },
  { path: '/control-panel', title: 'Control panel', description: 'Platform settings + upgrade readiness + SSO providers + streaming profiles + data health.' },
  { path: '/functions', title: 'Functions', description: 'Function package CRUD with capabilities JSON, source editor, metrics, runs, and execute.' },
  { path: '/pipelines', title: 'Pipelines', description: 'Hybrid batch + streaming pipelines with DAG editor, schedule, retry, runs, and validate.' },
  { path: '/ml', title: 'ML Studio', description: 'Experiments + runs + models + features + training + deployments + drift + batch predictions.' },
  { path: '/action-types', title: 'Action types', description: 'Author + validate + execute object-type actions, what-if branches, metrics.' },
  { path: '/datasets', title: 'Datasets', description: 'Catalog browser + upload + detail (preview, schema, files, transactions, quality) + branches.' },
  { path: '/apps', title: 'Apps', description: 'Workshop app builder: pages + settings + theme JSON, versions, publish, slate import/export.' },
  { path: '/data-connection', title: 'Data Connection', description: 'Sources + connector catalog + egress policies + agents + batch syncs + streaming + media-set syncs.' },
  { path: '/projects', title: 'Projects', description: 'Workspace project list + shared-with-me + trash + project detail (overview/folders/resources/memberships) + folder.' },
  { path: '/ontology-manager', title: 'Ontology manager', description: 'Hub for object types, interfaces, shared properties, links, projects + dataset bindings wizard.' },
  { path: '/ontology', title: 'Ontology', description: 'Landing + semantic search + object type browser; create/types, /graph, /object-sets sub-routes.' },
  { path: '/auth/login', title: 'Sign in', description: 'Login + register + MFA + SSO callback.' },
  { path: '/charts-demo', title: 'Charts demo', description: 'ECharts wrapper validator.' },
  { path: '/monaco-demo', title: 'Monaco demo', description: 'Monaco editor wrapper validator.' },
  { path: '/maplibre-demo', title: 'MapLibre demo', description: 'MapLibre map wrapper validator.' },
  { path: '/cytoscape-demo', title: 'Cytoscape demo', description: 'Cytoscape graph wrapper validator.' },
];

export function Home() {
  const primaryRoutes = MIGRATED_ROUTES.filter((route) =>
    ['/projects', '/datasets', '/pipelines', '/dashboards', '/ontology', '/builds', '/workflows', '/apps'].includes(route.path),
  );
  const recentRoutes = MIGRATED_ROUTES.slice(0, 14);

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <header className="of-hero-strip" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow">Workspace</p>
          <h1 className="of-heading-xl">OpenFoundry</h1>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <Link to="/projects" className="of-button">Projects</Link>
          <Link to="/datasets" className="of-button">Datasets</Link>
          <Link to="/pipelines" className="of-button of-button--primary">New pipeline</Link>
        </div>
      </header>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 8 }}>
        {[
          ['Resources', MIGRATED_ROUTES.length],
          ['Builds', 8],
          ['Objects', 24],
          ['Branches', 3],
        ].map(([label, value]) => (
          <section key={label} className="of-panel" style={{ padding: 10 }}>
            <p className="of-eyebrow">{label}</p>
            <p style={{ marginTop: 4, color: 'var(--text-strong)', fontSize: 22, fontWeight: 600 }}>{value}</p>
          </section>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 420px), 1fr))', gap: 10, alignItems: 'start' }}>
        <section className="of-panel" style={{ overflow: 'hidden' }}>
          <div className="of-toolbar" style={{ border: 0, borderBottom: '1px solid var(--border-default)', borderRadius: 0, justifyContent: 'space-between' }}>
            <div>
              <p className="of-heading-sm">Resources</p>
              <p className="of-text-muted" style={{ fontSize: 11 }}>Pinned workspace entry points</p>
            </div>
            <input className="of-input" placeholder="Search resources" style={{ width: 220 }} />
          </div>
          <table className="of-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Path</th>
                <th>Description</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {primaryRoutes.map((route) => (
                <tr key={route.path}>
                  <td><Link to={route.path}>{route.title}</Link></td>
                  <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{route.path}</td>
                  <td className="of-text-muted">{route.description}</td>
                  <td><span className="of-chip of-chip-active">Migrated</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <aside style={{ display: 'grid', gap: 10 }}>
          <section className="of-panel" style={{ padding: 10 }}>
            <p className="of-heading-sm">Recent</p>
            <div style={{ display: 'grid', marginTop: 8 }}>
              {recentRoutes.map((route) => (
                <Link
                  key={route.path}
                  to={route.path}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    gap: 8,
                    padding: '7px 0',
                    borderTop: '1px solid var(--border-subtle)',
                    color: 'var(--text-default)',
                    fontSize: 12,
                  }}
                >
                  <span>{route.title}</span>
                  <span className="of-text-muted" style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>{route.path}</span>
                </Link>
              ))}
            </div>
          </section>
          <section className="of-panel" style={{ padding: 10 }}>
            <p className="of-heading-sm">Environment</p>
            <dl style={{ display: 'grid', gridTemplateColumns: '1fr auto', gap: '7px 10px', marginTop: 8, fontSize: 12 }}>
              <dt className="of-text-muted">Branch</dt><dd>master</dd>
              <dt className="of-text-muted">Ontology</dt><dd>default</dd>
              <dt className="of-text-muted">Access</dt><dd>Editor</dd>
              <dt className="of-text-muted">Build health</dt><dd style={{ color: 'var(--status-success)', fontWeight: 700 }}>Passing</dd>
            </dl>
          </section>
        </aside>
      </div>
    </section>
  );
}
