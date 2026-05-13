import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';

import { useTranslator } from '@/lib/i18n/store';
import { Glyph, type GlyphName } from './ui/Glyph';

interface NavItem {
  to: string;
  label: string;
  icon: GlyphName;
  shortcut?: string;
  dot?: boolean;
  end?: boolean;
  iconTone?: string;
}

interface CategoryDef {
  id: string;
  label: string;
  isHeading?: boolean;
}

interface LauncherApp {
  id: string;
  href: string;
  name: string;
  description: string;
  icon: GlyphName;
  iconTone: string;
  category: string;
  promoted?: boolean;
}

const COLLAPSED_KEY = 'of_sidebar_collapsed';
const FAVORITES_KEY = 'of_favorite_apps';

function readFavorites(): string[] {
  if (typeof localStorage === 'undefined') return DEFAULT_FAVORITES;
  try {
    const raw = localStorage.getItem(FAVORITES_KEY);
    if (raw === null) return DEFAULT_FAVORITES;
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === 'string') : [];
  } catch {
    return [];
  }
}

const PRIMARY_NAV: NavItem[] = [
  { to: '/', label: 'Home', icon: 'home', end: true },
  { to: '/search', label: 'Search', icon: 'search', shortcut: 'ctrl + J' },
  { to: '/notifications', label: 'Notifications', icon: 'bell', dot: true },
  { to: '/whats-new', label: "What's New", icon: 'sparkles', dot: true },
];

const SECONDARY_NAV: NavItem[] = [
  { to: '/recent', label: 'Recent', icon: 'history' },
  { to: '/projects', label: 'Files', icon: 'folder' },
];

// Favorites are now user-driven via the launcher star toggle (see
// FAVORITES_KEY below). Workshop ships as a default favorite on first
// load so the section is not empty on a fresh install.
const DEFAULT_FAVORITES: string[] = ['workshop'];

const FOOTER_NAV: NavItem[] = [
  { to: '/ai', label: 'AIP Assist', icon: 'asterisk', shortcut: 'ctrl + shift + U', iconTone: '#67e8f9' },
  { to: '/developers', label: 'Support', icon: 'help' },
  { to: '/settings', label: 'Account', icon: 'users' },
];

const CATEGORIES: CategoryDef[] = [
  { id: 'all', label: 'All apps' },
  { id: '__platform', label: 'PLATFORM APPS', isHeading: true },
  { id: 'administration', label: 'Administration' },
  { id: 'analytics-operations', label: 'Analytics & Operations' },
  { id: 'application-development', label: 'Application development' },
  { id: 'data-integration', label: 'Data integration' },
  { id: 'developer-toolchain', label: 'Developer toolchain' },
  { id: 'models', label: 'Models' },
  { id: 'ontology', label: 'Ontology' },
  { id: 'security-governance', label: 'Security & governance' },
  { id: 'support', label: 'Support' },
];

const LAUNCHER_APPS: LauncherApp[] = [
  // Administration
  { id: 'control-panel', href: '/control-panel', icon: 'settings', iconTone: '#60a5fa', category: 'administration',
    name: 'Control Panel',
    description: 'Manage critical platform operations for an enrollment or organization.' },
  { id: 'resource-management', href: '/control-panel/data-health', icon: 'pie-chart', iconTone: '#34d399', category: 'administration',
    name: 'Resource Management',
    description: 'Track and manage costs, budgets, resource queues and usage limits across Foundry.' },
  { id: 'upgrade-assistant', href: '/control-panel/streaming-profiles', icon: 'badge-check', iconTone: '#fbbf24', category: 'administration',
    name: 'Upgrade Assistant',
    description: 'Track important platform updates and changes affecting the platform.' },

  // Analytics & Operations
  { id: 'contour', href: '/contour', icon: 'graph', iconTone: '#fb923c', category: 'analytics-operations',
    name: 'Contour',
    description: 'Analyze large datasets with filters, joins, and visualizations. Export the result to contribute back to the Foundry pipeline.' },
  { id: 'fusion', href: '/fusion', icon: 'spreadsheet', iconTone: '#22c55e', category: 'analytics-operations',
    name: 'Fusion',
    description: 'Interact with live Foundry data in a familiar spreadsheet interface.' },
  { id: 'map', href: '/geospatial', icon: 'view-grid', iconTone: '#4ade80', category: 'analytics-operations',
    name: 'Map',
    description: 'Analyze geospatial and geotemporal data.' },
  { id: 'notepad', href: '/notepad', icon: 'document', iconTone: '#f472b6', category: 'analytics-operations',
    name: 'Notepad',
    description: 'Create, share and export object-aware documents and reports.' },
  { id: 'quiver', href: '/quiver', icon: 'graph', iconTone: '#a78bfa', category: 'analytics-operations',
    name: 'Quiver',
    description: 'Visualize, analyze, and build interactive dashboards from object and time series data using a point-and-click environment.' },
  { id: 'vertex', href: '/vertex', icon: 'project', iconTone: '#22d3ee', category: 'analytics-operations',
    name: 'Vertex',
    description: 'Explore object graphs and system diagrams.' },

  // Application development
  { id: 'workshop', href: '/apps', icon: 'app', iconTone: '#a78bfa', category: 'application-development', promoted: true,
    name: 'Workshop',
    description: 'Build operational apps with widgets, templates, runtime previews, and publishing.' },
  { id: 'apps-marketplace', href: '/marketplace', icon: 'cube', iconTone: '#60a5fa', category: 'application-development',
    name: 'Marketplace',
    description: 'Discover internal packages, release channels, and one-click rollout bundles.' },
  { id: 'object-views', href: '/object-views', icon: 'object', iconTone: '#fb923c', category: 'application-development',
    name: 'Object Views',
    description: 'Configure operational record views, related lists, and quick actions.' },
  { id: 'workflows', href: '/workflows', icon: 'graph', iconTone: '#4ade80', category: 'application-development',
    name: 'Workflows',
    description: 'Author multi-step automations and orchestration logic.' },
  { id: 'reports', href: '/reports', icon: 'document', iconTone: '#f472b6', category: 'application-development',
    name: 'Reports',
    description: 'Compose narrative reports rendered from live ontology data.' },
  { id: 'dashboards', href: '/dashboards', icon: 'view-grid', iconTone: '#22d3ee', category: 'application-development',
    name: 'Dashboards',
    description: 'Operational KPIs, cohorts, and drill-down dashboards.' },
  { id: 'global-branching', href: '/global-branching', icon: 'asterisk', iconTone: '#facc15', category: 'application-development',
    name: 'Global Branching',
    description: 'Promote, review, and merge cross-app branches with policy.' },
  { id: 'logic', href: '/logic', icon: 'graph', iconTone: '#38bdf8', category: 'application-development', promoted: true,
    name: 'AIP Logic',
    description: 'Author no-code Logic functions with inputs, blocks, outputs, debugging, and run previews.' },

  // Data integration
  { id: 'datasets', href: '/datasets', icon: 'database', iconTone: '#60a5fa', category: 'data-integration',
    name: 'Data Catalog',
    description: 'Search datasets by owner, tags, and quality status from one registry.' },
  { id: 'data-connection', href: '/data-connection', icon: 'link', iconTone: '#34d399', category: 'data-integration',
    name: 'Data Connection',
    description: 'Sync data from external systems via sources, egress policies, and batch syncs.' },
  { id: 'pipelines', href: '/pipelines', icon: 'graph', iconTone: '#a78bfa', category: 'data-integration',
    name: 'Pipeline Builder',
    description: 'Design and monitor operational pipelines, runs, and automation jobs.' },
  { id: 'builds', href: '/builds', icon: 'cube', iconTone: '#fb923c', category: 'data-integration',
    name: 'Builds',
    description: 'Inspect every build, drill into the job graph, follow live logs.' },
  { id: 'streaming', href: '/streaming', icon: 'graph', iconTone: '#22d3ee', category: 'data-integration',
    name: 'Streaming',
    description: 'Operate real-time topologies, windows, joins, and live event tails.' },
  { id: 'lineage', href: '/lineage', icon: 'graph', iconTone: '#f472b6', category: 'data-integration',
    name: 'Data Lineage',
    description: 'Inspect upstream and downstream dependencies across the data estate.' },
  { id: 'media-sets', href: '/media-sets', icon: 'image', iconTone: '#facc15', category: 'data-integration',
    name: 'Media Sets',
    description: 'Manage image, audio, and video collections with metadata.' },
  { id: 'iceberg-tables', href: '/iceberg-tables', icon: 'database', iconTone: '#4ade80', category: 'data-integration',
    name: 'Iceberg Tables',
    description: 'Provision and govern Iceberg-managed table assets.' },
  { id: 'virtual-tables', href: '/virtual-tables', icon: 'database', iconTone: '#60a5fa', category: 'data-integration',
    name: 'Virtual Tables',
    description: 'Federate external sources without copying data.' },
  { id: 'schedules', href: '/build-schedules', icon: 'history', iconTone: '#fbbf24', category: 'data-integration',
    name: 'Schedules',
    description: 'Cron and event-based schedules for builds and pipelines.' },
  { id: 'machinery', href: '/machinery', icon: 'cube', iconTone: '#a78bfa', category: 'data-integration',
    name: 'Machinery',
    description: 'Manage long-running infrastructure jobs and workers.' },
  { id: 'object-databases', href: '/object-databases', icon: 'database', iconTone: '#f472b6', category: 'data-integration',
    name: 'Object Databases',
    description: 'Configure object-store-backed databases.' },
  { id: 'ontology-indexing', href: '/ontology-indexing', icon: 'database', iconTone: '#22c55e', category: 'data-integration',
    name: 'Ontology Indexing',
    description: 'Index ontology objects for fast retrieval.' },
  { id: 'foundry-rules', href: '/foundry-rules', icon: 'shield', iconTone: '#fb923c', category: 'data-integration',
    name: 'Foundry Rules',
    description: 'Author rule-based automations and triggers.' },

  // Developer toolchain
  { id: 'code-repos', href: '/code-repos', icon: 'code', iconTone: '#22d3ee', category: 'developer-toolchain',
    name: 'Code Repositories',
    description: 'Browse repositories, reviews, CI gates, commits, and protected merge flows.' },
  { id: 'notebooks', href: '/notebooks', icon: 'code', iconTone: '#a78bfa', category: 'developer-toolchain',
    name: 'Workshop Notebooks',
    description: 'Develop notebook-based workflows, analyses, and collaborative experiments.' },
  { id: 'functions', href: '/functions', icon: 'code', iconTone: '#facc15', category: 'developer-toolchain',
    name: 'Functions',
    description: 'Author serverless functions invoked by workflows and apps.' },

  // Models
  { id: 'ai', href: '/ai', icon: 'sparkles', iconTone: '#a78bfa', category: 'models',
    name: 'AI Platform',
    description: 'Manage providers, prompts, guardrails, agents, and copilots from one control plane.' },
  { id: 'ml', href: '/ml', icon: 'sparkles', iconTone: '#60a5fa', category: 'models',
    name: 'ML Studio',
    description: 'Track experiments, training jobs, model versions, and online feature flows.' },
  { id: 'nexus', href: '/nexus', icon: 'sparkles', iconTone: '#22d3ee', category: 'models',
    name: 'Nexus',
    description: 'Connect knowledge sources to grounded LLM agents.' },
  { id: 'agents', href: '/ai', icon: 'sparkles', iconTone: '#34d399', category: 'models',
    name: 'Agents',
    description: 'Build, evaluate, and deploy autonomous reasoning agents.' },

  // Ontology
  { id: 'ontology-manager', href: '/ontology-manager', icon: 'ontology', iconTone: '#a78bfa', category: 'ontology',
    name: 'Ontology Manager',
    description: 'Shape core object models, semantics, and linked operational concepts.' },
  { id: 'object-explorer', href: '/object-explorer', icon: 'object', iconTone: '#fb923c', category: 'ontology',
    name: 'Object Explorer',
    description: 'Explore linked operational entities, activity, and related records.' },
  { id: 'action-types', href: '/action-types', icon: 'run', iconTone: '#22d3ee', category: 'ontology',
    name: 'Action Types',
    description: 'Author actions that mutate ontology objects with policy and audit.' },
  { id: 'object-link-types', href: '/object-link-types', icon: 'link', iconTone: '#34d399', category: 'ontology',
    name: 'Object Link Types',
    description: 'Define typed relationships between ontology objects.' },
  { id: 'interfaces', href: '/interfaces', icon: 'object', iconTone: '#facc15', category: 'ontology',
    name: 'Interfaces',
    description: 'Polymorphic interfaces over ontology object types.' },

  // Security & governance
  { id: 'audit', href: '/audit', icon: 'shield', iconTone: '#f472b6', category: 'security-governance',
    name: 'Governance Center',
    description: 'Review policies, approvals, applications, and governance templates.' },
  { id: 'object-monitors', href: '/object-monitors', icon: 'eye', iconTone: '#22d3ee', category: 'security-governance',
    name: 'Object Monitors',
    description: 'Watch for ontology drift and rule violations.' },
  { id: 'object-views-gov', href: '/object-views', icon: 'lock', iconTone: '#fb923c', category: 'security-governance',
    name: 'Access Reviews',
    description: 'Review user access and entitlement assignments.' },
  { id: 'shield-plus', href: '/audit', icon: 'shield-plus', iconTone: '#facc15', category: 'security-governance',
    name: 'Policy Engine',
    description: 'Author access policies enforced across the platform.' },
  { id: 'settings-gov', href: '/settings', icon: 'settings', iconTone: '#a78bfa', category: 'security-governance',
    name: 'Settings',
    description: 'Configure identity, language, security posture, and account defaults.' },

  // Support
  { id: 'help', href: '/developers', icon: 'help', iconTone: '#60a5fa', category: 'support',
    name: 'Help Center',
    description: 'Browse docs, guides, and platform tutorials.' },
  { id: 'tour', href: '/developers', icon: 'tour', iconTone: '#fbbf24', category: 'support',
    name: 'Product Tours',
    description: 'Guided walkthroughs for the most common workflows.' },
  { id: 'developers', href: '/developers', icon: 'code', iconTone: '#22d3ee', category: 'support',
    name: 'Developers',
    description: 'API references, SDK downloads and webhooks.' },
  { id: 'contact', href: '/developers', icon: 'email', iconTone: '#f472b6', category: 'support',
    name: 'Contact Support',
    description: 'Open a support ticket with the platform team.' },
];

function isActive(href: string, pathname: string, end?: boolean) {
  if (end) return pathname === href;
  return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(`${href}/`);
}

function readCollapsedPref(): boolean {
  if (typeof localStorage === 'undefined') return false;
  return localStorage.getItem(COLLAPSED_KEY) === '1';
}

interface SidebarLinkProps {
  item: NavItem;
  pathname: string;
  collapsed: boolean;
}

function SidebarLink({ item, pathname, collapsed }: SidebarLinkProps) {
  const active = isActive(item.to, pathname, item.end);
  return (
    <NavLink
      to={item.to}
      className={`of-sidebar__link${active ? ' of-sidebar__link--active' : ''}`}
      title={collapsed ? item.label : undefined}
      aria-label={item.label}
    >
      <span className="of-sidebar__icon" style={item.iconTone ? { color: item.iconTone } : undefined}>
        <Glyph name={item.icon} size={17} tone={item.iconTone ?? null} />
        {item.dot && <span className="of-sidebar__dot" aria-hidden="true" />}
      </span>
      <span className="of-sidebar__label">{item.label}</span>
      {item.shortcut && <span className="of-sidebar__hint">{item.shortcut}</span>}
    </NavLink>
  );
}

export function Sidebar() {
  const t = useTranslator();
  const { pathname } = useLocation();
  const [collapsed, setCollapsed] = useState<boolean>(() => readCollapsedPref());
  const [launcherOpen, setLauncherOpen] = useState(false);
  const [category, setCategory] = useState<string>('all');
  const [search, setSearch] = useState('');
  const [hoveredAppId, setHoveredAppId] = useState<string | null>(null);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [favorites, setFavorites] = useState<string[]>(() => readFavorites());
  const searchRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(COLLAPSED_KEY, collapsed ? '1' : '0');
  }, [collapsed]);

  useEffect(() => {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(FAVORITES_KEY, JSON.stringify(favorites));
  }, [favorites]);

  const toggleFavorite = (id: string) => {
    setFavorites((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  const favoriteApps = useMemo(
    () =>
      favorites
        .map((id) => LAUNCHER_APPS.find((a) => a.id === id))
        .filter((a): a is LauncherApp => Boolean(a)),
    [favorites],
  );

  useEffect(() => {
    if (launcherOpen) searchRef.current?.focus();
  }, [launcherOpen]);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && launcherOpen) setLauncherOpen(false);
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [launcherOpen]);

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: LAUNCHER_APPS.length };
    for (const c of CATEGORIES) {
      if (c.isHeading || c.id === 'all') continue;
      counts[c.id] = LAUNCHER_APPS.filter((a) => a.category === c.id).length;
    }
    return counts;
  }, []);

  const visibleApps = useMemo(() => {
    const term = search.trim().toLowerCase();
    return LAUNCHER_APPS.filter((app) => {
      if (category !== 'all' && app.category !== category) return false;
      if (!term) return true;
      return app.name.toLowerCase().includes(term) || app.description.toLowerCase().includes(term);
    });
  }, [search, category]);

  const groupedApps = useMemo(() => {
    const groups = new Map<string, LauncherApp[]>();
    for (const app of visibleApps) {
      if (!groups.has(app.category)) groups.set(app.category, []);
      groups.get(app.category)!.push(app);
    }
    const order = CATEGORIES.filter((c) => !c.isHeading && c.id !== 'all').map((c) => c.id);
    return order
      .filter((id) => groups.has(id))
      .map((id) => ({
        id,
        label: CATEGORIES.find((c) => c.id === id)?.label ?? id,
        apps: groups.get(id)!,
      }));
  }, [visibleApps]);

  const hoveredApp = useMemo(
    () => LAUNCHER_APPS.find((a) => a.id === hoveredAppId) ?? null,
    [hoveredAppId],
  );

  const promotedApps = useMemo(() => LAUNCHER_APPS.filter((a) => a.promoted), []);

  function openLauncher() {
    setSearch('');
    setCategory('all');
    setHoveredAppId(null);
    setFiltersOpen(false);
    setLauncherOpen(true);
  }

  return (
    <>
      <aside className="of-sidebar of-scrollbar" data-collapsed={collapsed || undefined}>
        <div className="of-sidebar__brand">
          <Link to="/" className="of-sidebar__logo" aria-label="OpenFoundry home" title="OpenFoundry">
            <img src="/empty-logo.png" alt="" width={20} height={20} style={{ display: 'block', objectFit: 'contain' }} />
          </Link>
          <button
            type="button"
            className="of-sidebar__collapse"
            aria-label={collapsed ? t('nav.expand') : t('nav.collapse')}
            aria-pressed={collapsed}
            onClick={() => setCollapsed((c) => !c)}
            title={collapsed ? t('nav.expand') : t('nav.collapse')}
          >
            <Glyph name={collapsed ? 'chevron-right' : 'chevron-left'} size={15} />
          </button>
        </div>

        <nav className="of-sidebar__nav" aria-label="Primary navigation">
          <section className="of-sidebar__section">
            {PRIMARY_NAV.map((item) => (
              <SidebarLink key={item.to} item={item} pathname={pathname} collapsed={collapsed} />
            ))}
          </section>

          <section className="of-sidebar__section">
            {SECONDARY_NAV.map((item) => (
              <SidebarLink key={item.to} item={item} pathname={pathname} collapsed={collapsed} />
            ))}
            <button
              type="button"
              className={`of-sidebar__link of-sidebar__link--button${launcherOpen ? ' of-sidebar__link--active' : ''}`}
              data-expanded={launcherOpen || undefined}
              aria-haspopup="dialog"
              aria-expanded={launcherOpen}
              onClick={() => (launcherOpen ? setLauncherOpen(false) : openLauncher())}
              title={collapsed ? 'Applications' : undefined}
            >
              <span className="of-sidebar__icon"><Glyph name="view-grid" size={17} /></span>
              <span className="of-sidebar__label">Applications</span>
            </button>
          </section>

          {favoriteApps.length > 0 && (
            <section className="of-sidebar__section">
              <div className="of-sidebar__heading">{t('nav.workshop.section')}</div>
              {favoriteApps.map((app) => (
                <SidebarLink
                  key={`fav-${app.id}`}
                  item={{ to: app.href, label: app.name, icon: app.icon, iconTone: app.iconTone }}
                  pathname={pathname}
                  collapsed={collapsed}
                />
              ))}
            </section>
          )}
        </nav>

        <section className="of-sidebar__section of-sidebar__section--footer">
          {FOOTER_NAV.map((item) => (
            <SidebarLink key={item.to} item={item} pathname={pathname} collapsed={collapsed} />
          ))}
        </section>
      </aside>

      {launcherOpen && (
        <div className="of-app-launcher" data-sidebar-collapsed={collapsed || undefined}>
          <button
            type="button"
            className="of-app-launcher__backdrop"
            aria-label="Close applications launcher"
            onClick={() => setLauncherOpen(false)}
          />
          <div className="of-app-launcher__surface" role="dialog" aria-modal="true" aria-label="Applications">
            <div className="of-app-launcher__header">
              <label className="of-app-launcher__search">
                <span className="of-app-launcher__search-icon"><Glyph name="search" size={14} /></span>
                <input
                  ref={searchRef}
                  type="search"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search for applications…"
                  aria-label="Search applications"
                />
              </label>
              <button
                type="button"
                className={`of-app-launcher__filters${filtersOpen ? ' is-open' : ''}`}
                aria-pressed={filtersOpen}
                onClick={() => setFiltersOpen((v) => !v)}
              >
                <Glyph name="tag" size={13} />
                <span>{t('launcher.filters')}</span>
              </button>
              <button
                type="button"
                className="of-app-launcher__close"
                aria-label="Close applications launcher"
                onClick={() => setLauncherOpen(false)}
              >
                <Glyph name="x" size={14} />
              </button>
            </div>

            <div className="of-app-launcher__body">
              <div className="of-app-launcher__categories">
                {CATEGORIES.map((c) =>
                  c.isHeading ? (
                    <div key={c.id} className="of-app-launcher__category-heading">{c.label}</div>
                  ) : (
                    <button
                      key={c.id}
                      type="button"
                      className="of-app-launcher__category"
                      data-active={category === c.id || undefined}
                      onClick={() => setCategory(c.id)}
                    >
                      <span>{c.label}</span>
                      <span className="of-app-launcher__category-count">{categoryCounts[c.id] ?? 0}</span>
                    </button>
                  ),
                )}
                <div className="of-app-launcher__category-heading of-app-launcher__category-heading--inline">
                  PROMOTED APPS
                  <button type="button" className="of-app-launcher__promote-add">
                    <Glyph name="check" size={12} />
                    <span>Add</span>
                  </button>
                </div>
                {promotedApps.length === 0 && (
                  <div className="of-app-launcher__category-empty">No promoted apps</div>
                )}
                {promotedApps.map((app) => (
                  <Link
                    key={`promoted-${app.id}`}
                    to={app.href}
                    className="of-app-launcher__category"
                    onClick={() => setLauncherOpen(false)}
                  >
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                      <Glyph name={app.icon} size={13} tone={app.iconTone} />
                      {app.name}
                    </span>
                  </Link>
                ))}
              </div>

              <div className="of-app-launcher__catalog">
                {visibleApps.length === 0 ? (
                  <div className="of-app-launcher__empty">No applications matched this search.</div>
                ) : (
                  groupedApps.map((group) => (
                    <div key={group.id} className="of-app-launcher__group">
                      <div className="of-app-launcher__group-title">{group.label}</div>
                      {group.apps.map((app) => {
                        const isFav = favorites.includes(app.id);
                        return (
                          <Link
                            key={app.id}
                            to={app.href}
                            className="of-app-launcher__item"
                            data-selected={hoveredAppId === app.id || undefined}
                            data-active={isActive(app.href, pathname) || undefined}
                            data-favorite={isFav || undefined}
                            onMouseEnter={() => setHoveredAppId(app.id)}
                            onFocus={() => setHoveredAppId(app.id)}
                            onClick={() => setLauncherOpen(false)}
                          >
                            <span
                              className="of-app-launcher__item-icon"
                              style={{
                                background: `${app.iconTone}28`,
                                color: app.iconTone,
                              }}
                            >
                              <Glyph name={app.icon} size={16} tone={app.iconTone} />
                            </span>
                            <span className="of-app-launcher__item-copy">
                              <span className="of-app-launcher__item-name">{app.name}</span>
                              <span className="of-app-launcher__item-description">{app.description}</span>
                            </span>
                            <button
                              type="button"
                              className="of-app-launcher__item-fav"
                              data-favorite={isFav || undefined}
                              aria-pressed={isFav}
                              aria-label={isFav ? `Remove ${app.name} from favorites` : `Add ${app.name} to favorites`}
                              title={isFav ? 'Remove from favorites' : 'Add to favorites'}
                              onClick={(e) => {
                                e.preventDefault();
                                e.stopPropagation();
                                toggleFavorite(app.id);
                              }}
                            >
                              <Glyph name={isFav ? 'star-filled' : 'star'} size={14} />
                            </button>
                          </Link>
                        );
                      })}
                    </div>
                  ))
                )}
              </div>

              <div className="of-app-launcher__detail">
                {hoveredApp ? (
                  <>
                    <div
                      className="of-app-launcher__detail-icon"
                      style={{
                        background: `${hoveredApp.iconTone}28`,
                        color: hoveredApp.iconTone,
                      }}
                    >
                      <Glyph name={hoveredApp.icon} size={20} tone={hoveredApp.iconTone} />
                    </div>
                    <div className="of-app-launcher__detail-copy">
                      <div className="of-app-launcher__detail-badge">
                        {CATEGORIES.find((c) => c.id === hoveredApp.category)?.label ?? ''}
                      </div>
                      <h2>{hoveredApp.name}</h2>
                      <p>{hoveredApp.description}</p>
                    </div>
                    <div className="of-app-launcher__actions">
                      <Link
                        to={hoveredApp.href}
                        className="of-app-launcher__button of-app-launcher__button--primary"
                        onClick={() => setLauncherOpen(false)}
                      >
                        Open
                      </Link>
                    </div>
                  </>
                ) : (
                  <div className="of-app-launcher__detail-empty">
                    <span className="of-app-launcher__detail-empty-icon" aria-hidden>
                      <Glyph name="cube" size={28} tone="rgba(255, 255, 255, 0.18)" />
                    </span>
                    <p>Hover on an application to see details</p>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
