import { NavLink } from 'react-router-dom';

import { Glyph, type GlyphName } from './ui/Glyph';

interface NavItem {
  to: string;
  label: string;
  icon: GlyphName;
}

interface NavGroup {
  title: string;
  items: NavItem[];
}

const NAV_GROUPS: NavGroup[] = [
  {
    title: 'Core',
    items: [
      { to: '/', label: 'Home', icon: 'home' },
      { to: '/search', label: 'Search', icon: 'search' },
      { to: '/projects', label: 'Projects & files', icon: 'folder' },
      { to: '/datasets', label: 'Datasets', icon: 'database' },
    ],
  },
  {
    title: 'Apps',
    items: [
      { to: '/object-explorer', label: 'Object explorer', icon: 'search' },
      { to: '/dashboards', label: 'Dashboards', icon: 'graph' },
      { to: '/contour', label: 'Contour', icon: 'graph' },
      { to: '/quiver', label: 'Quiver', icon: 'graph' },
      { to: '/notepad', label: 'Notepad', icon: 'document' },
      { to: '/reports', label: 'Reports', icon: 'list' },
      { to: '/apps', label: 'Workshop', icon: 'object' },
      { to: '/pipelines', label: 'Pipeline builder', icon: 'run' },
      { to: '/code-repos', label: 'Code repositories', icon: 'code' },
    ],
  },
  {
    title: 'Ontology',
    items: [
      { to: '/ontology-manager', label: 'Ontology manager', icon: 'ontology' },
      { to: '/ontology', label: 'Ontology', icon: 'cube' },
      { to: '/object-link-types', label: 'Object & link types', icon: 'link' },
      { to: '/interfaces', label: 'Interfaces', icon: 'artifact' },
      { to: '/functions', label: 'Functions', icon: 'code' },
      { to: '/foundry-rules', label: 'Foundry Rules', icon: 'settings' },
    ],
  },
  {
    title: 'Platform',
    items: [
      { to: '/data-connection', label: 'Data Connection', icon: 'database' },
      { to: '/streaming', label: 'Streaming', icon: 'run' },
      { to: '/builds', label: 'Builds', icon: 'history' },
      { to: '/build-schedules', label: 'Build schedules', icon: 'history' },
      { to: '/lineage', label: 'Lineage', icon: 'graph' },
      { to: '/settings', label: 'Control & settings', icon: 'settings' },
    ],
  },
];

function navLinkClass({ isActive }: { isActive: boolean }) {
  return `of-sidebar__link${isActive ? ' of-sidebar__link--active' : ''}`;
}

export function Sidebar() {
  return (
    <aside className="of-sidebar">
      <div className="of-sidebar__brand">
        <NavLink to="/" className="of-sidebar__product" aria-label="OpenFoundry home">
          <span className="of-sidebar__mark" aria-hidden="true">
            OF
          </span>
          <span className="of-sidebar__brand-copy">
            <strong>OpenFoundry</strong>
            <span>Documentation workspace</span>
          </span>
        </NavLink>
        <button type="button" className="of-sidebar__collapse" aria-label="Collapse navigation">
          <Glyph name="menu" size={17} />
        </button>
      </div>

      <nav className="of-sidebar__nav" aria-label="Primary navigation">
        {NAV_GROUPS.map((group) => (
          <section key={group.title} className="of-sidebar__section">
            <div className="of-sidebar__heading">
              <span>{group.title}</span>
              <a href={`#${group.title.toLowerCase()}`} aria-label={`View all ${group.title}`}>
                View all
              </a>
            </div>
            {group.items.map((item) => (
              <NavLink key={item.to} to={item.to} end={item.to === '/'} className={navLinkClass}>
                <span className="of-sidebar__icon">
                  <Glyph name={item.icon} size={18} />
                </span>
                <span className="of-sidebar__label">{item.label}</span>
              </NavLink>
            ))}
          </section>
        ))}
      </nav>

      <div className="of-sidebar__footer">
        <label className="of-sidebar__select">
          <Glyph name="help" size={17} />
          <span>English</span>
          <Glyph name="chevron-down" size={14} />
        </label>
        <label className="of-sidebar__select">
          <Glyph name="settings" size={17} />
          <span>Default</span>
          <Glyph name="chevron-down" size={14} />
        </label>
        <NavLink to="/developers" className={navLinkClass}>
          <span className="of-sidebar__icon">
            <Glyph name="logout" size={18} />
          </span>
          <span className="of-sidebar__label">Open other workspaces</span>
        </NavLink>
      </div>
    </aside>
  );
}
