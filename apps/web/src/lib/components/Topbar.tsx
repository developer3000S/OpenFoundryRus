import { Link, useLocation } from 'react-router-dom';

import { Glyph } from './ui/Glyph';

const ROUTE_LABELS: Record<string, string> = {
  '': 'Home',
  ai: 'AI Platform',
  apps: 'Workshop',
  audit: 'Audit',
  builds: 'Builds',
  'build-schedules': 'Build schedules',
  'code-repos': 'Code repositories',
  'control-panel': 'Control Panel',
  contour: 'Contour',
  dashboards: 'Dashboards',
  datasets: 'Datasets',
  developers: 'Developers',
  'data-connection': 'Data Connection',
  functions: 'Functions',
  lineage: 'Lineage',
  marketplace: 'Marketplace',
  notebooks: 'Notebooks',
  notepad: 'Notepad',
  ontology: 'Ontology',
  'ontology-manager': 'Ontology Manager',
  pipelines: 'Pipeline Builder',
  projects: 'Projects & files',
  quiver: 'Quiver',
  reports: 'Reports',
  search: 'Search',
  settings: 'Settings',
  streaming: 'Streaming',
  vertex: 'Vertex',
};

function titleCase(segment: string) {
  return segment
    .split('-')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function crumbsFor(pathname: string) {
  const segments = pathname.split('/').filter(Boolean);
  if (segments.length === 0) return [{ label: 'Home', href: '/' }];
  return segments.map((segment, index) => {
    const href = `/${segments.slice(0, index + 1).join('/')}`;
    const label = ROUTE_LABELS[segment] ?? titleCase(segment);
    return { label, href };
  });
}

export function Topbar() {
  const { pathname } = useLocation();
  const crumbs = crumbsFor(pathname);
  const current = crumbs[crumbs.length - 1]?.label ?? 'Home';

  return (
    <header className="of-topbar">
      <div className="of-topbar__identity">
        <div className="of-topbar__file-menu">
          <button type="button" className="of-topbar__menu-item">File</button>
          <button type="button" className="of-topbar__menu-item">Help</button>
        </div>
        <div className="of-topbar__crumbs" aria-label="Breadcrumb">
          <Link to="/" className="of-topbar__home">
            <Glyph name="folder" size={16} />
          </Link>
          {crumbs.map((crumb, index) => {
            const isLast = index === crumbs.length - 1;
            return (
              <span key={crumb.href} className="of-topbar__crumb-wrap">
                {index > 0 && <span className="of-topbar__separator">/</span>}
                {isLast ? (
                  <span className="of-topbar__crumb of-topbar__current">{crumb.label}</span>
                ) : (
                  <Link to={crumb.href} className="of-topbar__crumb of-topbar__crumb-link">
                    {crumb.label}
                  </Link>
                )}
              </span>
            );
          })}
          <button type="button" className="of-topbar__star" aria-label={`Favorite ${current}`}>
            ☆
          </button>
        </div>
      </div>

      <div className="of-topbar__center">
        <button type="button" className="of-topbar__icon-button" aria-label="Undo">
          ↶
        </button>
        <button type="button" className="of-topbar__icon-button" aria-label="Redo" disabled>
          ↷
        </button>
        <button type="button" className="of-topbar__branch">
          <Glyph name="graph" size={16} />
          <span>Main</span>
          <Glyph name="chevron-down" size={13} />
        </button>
      </div>

      <div className="of-topbar__actions">
        <span className="of-topbar__build-status">
          <span>↻ 0</span>
          <span>✓ 1</span>
          <span>× 0</span>
        </span>
        <button type="button" className="of-topbar__action">
          <Glyph name="users" size={15} />
          Share
        </button>
        <button type="button" className="of-topbar__action of-topbar__action--primary">
          Save
        </button>
        <span className="of-topbar__avatar">OF</span>
      </div>
    </header>
  );
}
