import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';

import { createTranslator, useCurrentLocale } from '@/lib/i18n/store';
import type { MessageKey } from '@/lib/i18n/messages';
import type { GlyphName } from '@/lib/components/ui/Glyph';
import { Glyph } from '@/lib/components/ui/Glyph';

type NavIcon =
  | 'home' | 'search' | 'bell' | 'history' | 'folder' | 'cube'
  | 'database' | 'object' | 'ontology' | 'code' | 'graph'
  | 'help' | 'settings' | 'sparkles' | 'link';

interface NavItem {
  href: string;
  labelKey: MessageKey;
  icon: NavIcon;
  hint?: string;
}

interface LocalizedCopy { en: string; es: string; }

type LauncherCategoryId =
  | 'all' | 'platform' | 'administration' | 'development'
  | 'integration' | 'toolchain' | 'models' | 'ontology' | 'governance';

interface LauncherCategory { id: LauncherCategoryId; label: LocalizedCopy; }

interface LauncherApp {
  id: string;
  href: string;
  icon: NavIcon;
  name: LocalizedCopy;
  description: LocalizedCopy;
  badge: LocalizedCopy;
  categoryIds: LauncherCategoryId[];
  createHref?: string;
  browseHref?: string;
}

interface LauncherText {
  title: string;
  searchPlaceholder: string;
  close: string;
  open: string;
  createNew: string;
  browseCatalog: string;
  favorites: string;
  emptyFavorites: string;
  emptySearch: string;
  searchAriaLabel: string;
}

const WORKSPACE_NAV: NavItem[] = [
  { href: '/', labelKey: 'nav.home', icon: 'home' },
  { href: '/search', labelKey: 'nav.search', icon: 'search', hint: 'Ctrl + J' },
  { href: '/object-monitors', labelKey: 'nav.notifications', icon: 'bell' },
  { href: '/dashboards', labelKey: 'nav.recent', icon: 'history' },
  { href: '/projects', labelKey: 'nav.files', icon: 'folder' },
];

const UTILITY_NAV: NavItem[] = [
  { href: '/developers', labelKey: 'nav.support', icon: 'help' },
  { href: '/settings', labelKey: 'nav.account', icon: 'settings' },
];

const LAUNCHER_COPY: Record<'en' | 'es', LauncherText> = {
  en: {
    title: 'Applications',
    searchPlaceholder: 'Search for applications…',
    close: 'Close applications launcher',
    open: 'Open',
    createNew: 'Create new',
    browseCatalog: 'Browse catalog',
    favorites: 'Favorites & recents',
    emptyFavorites: 'No favorites or recents yet.',
    emptySearch: 'No applications matched this search.',
    searchAriaLabel: 'Search applications',
  },
  es: {
    title: 'Aplicaciones',
    searchPlaceholder: 'Buscar aplicaciones…',
    close: 'Cerrar lanzador de aplicaciones',
    open: 'Abrir',
    createNew: 'Crear nuevo',
    browseCatalog: 'Explorar catálogo',
    favorites: 'Favoritos y recientes',
    emptyFavorites: 'Todavía no hay favoritos ni recientes.',
    emptySearch: 'Ninguna aplicación coincide con esta búsqueda.',
    searchAriaLabel: 'Buscar aplicaciones',
  },
};

const LAUNCHER_CATEGORIES: LauncherCategory[] = [
  { id: 'all', label: { en: 'All apps', es: 'Todas las apps' } },
  { id: 'platform', label: { en: 'Platform apps', es: 'Apps de plataforma' } },
  { id: 'administration', label: { en: 'Administration', es: 'Administración' } },
  { id: 'development', label: { en: 'Application development', es: 'Desarrollo de aplicaciones' } },
  { id: 'integration', label: { en: 'Data integration', es: 'Integración de datos' } },
  { id: 'toolchain', label: { en: 'Developer toolchain', es: 'Toolchain de desarrollo' } },
  { id: 'models', label: { en: 'Models', es: 'Modelos' } },
  { id: 'ontology', label: { en: 'Ontology', es: 'Ontología' } },
  { id: 'governance', label: { en: 'Security & governance', es: 'Seguridad y gobierno' } },
];

const LAUNCHER_APPS: LauncherApp[] = [
  { id: 'datasets', href: '/datasets', icon: 'database',
    name: { en: 'Data Catalog', es: 'Catálogo de datos' },
    description: { en: 'Search datasets by owner, tags, and quality status from one registry.', es: 'Busca datasets por propietario, etiquetas y estado de calidad desde un único registro.' },
    badge: { en: 'Data integration', es: 'Integración de datos' },
    categoryIds: ['all', 'platform', 'integration'], createHref: '/datasets/upload', browseHref: '/datasets' },
  { id: 'data-connection', href: '/data-connection', icon: 'link',
    name: { en: 'Data Connection', es: 'Data Connection' },
    description: { en: 'Sync data from external systems via sources, egress policies, and batch syncs.', es: 'Sincroniza datos de sistemas externos mediante sources, políticas de egress y batch syncs.' },
    badge: { en: 'Data integration', es: 'Integración de datos' },
    categoryIds: ['all', 'integration'], createHref: '/data-connection/new', browseHref: '/data-connection' },
  { id: 'pipelines', href: '/pipelines', icon: 'graph',
    name: { en: 'Pipeline Builder', es: 'Constructor de pipelines' },
    description: { en: 'Design and monitor operational pipelines, runs, and automation jobs.', es: 'Diseña y monitoriza pipelines operativos, ejecuciones y trabajos de automatización.' },
    badge: { en: 'Data integration', es: 'Integración de datos' },
    categoryIds: ['all', 'integration'], createHref: '/pipelines/new', browseHref: '/pipelines' },
  { id: 'builds', href: '/builds', icon: 'graph',
    name: { en: 'Builds', es: 'Builds' },
    description: { en: 'Inspect every build, drill into the job graph, follow live logs.', es: 'Inspecciona cada build, accede al grafo de jobs y sigue logs en vivo.' },
    badge: { en: 'Data integration', es: 'Integración de datos' },
    categoryIds: ['all', 'integration'], browseHref: '/builds' },
  { id: 'lineage', href: '/lineage', icon: 'graph',
    name: { en: 'Data Lineage', es: 'Lineage de datos' },
    description: { en: 'Inspect upstream and downstream dependencies across the data estate.', es: 'Inspecciona dependencias upstream y downstream a través del patrimonio de datos.' },
    badge: { en: 'Ontology', es: 'Ontología' },
    categoryIds: ['all', 'integration', 'ontology'], browseHref: '/lineage' },
  { id: 'object-explorer', href: '/object-explorer', icon: 'object',
    name: { en: 'Object Explorer', es: 'Explorador de objetos' },
    description: { en: 'Explore linked operational entities, activity, and related records.', es: 'Explora entidades operativas vinculadas, actividad y registros relacionados.' },
    badge: { en: 'Platform app', es: 'App de plataforma' },
    categoryIds: ['all', 'platform', 'ontology'], browseHref: '/object-explorer' },
  { id: 'ontology-manager', href: '/ontology-manager', icon: 'ontology',
    name: { en: 'Ontology Manager', es: 'Gestor de ontología' },
    description: { en: 'Shape core object models, semantics, and linked operational concepts.', es: 'Modela objetos clave, semántica y conceptos operativos enlazados.' },
    badge: { en: 'Ontology', es: 'Ontología' },
    categoryIds: ['all', 'platform', 'ontology'], browseHref: '/ontology-manager' },
  { id: 'apps', href: '/apps', icon: 'cube',
    name: { en: 'Workshop App Builder', es: 'Workshop App Builder' },
    description: { en: 'Build internal applications with widgets, templates, runtime previews, and publishing.', es: 'Construye aplicaciones internas con widgets, plantillas, vista previa runtime y publicación.' },
    badge: { en: 'Application development', es: 'Desarrollo de aplicaciones' },
    categoryIds: ['all', 'development', 'toolchain'], createHref: '/apps', browseHref: '/apps' },
  { id: 'notebooks', href: '/notebooks', icon: 'code',
    name: { en: 'Workshop', es: 'Workshop' },
    description: { en: 'Develop notebook-based workflows, analyses, and collaborative experiments.', es: 'Desarrolla workflows basados en notebooks, análisis y experimentos colaborativos.' },
    badge: { en: 'Developer toolchain', es: 'Toolchain de desarrollo' },
    categoryIds: ['all', 'development', 'toolchain'], browseHref: '/notebooks' },
  { id: 'code-repos', href: '/code-repos', icon: 'code',
    name: { en: 'Code Repositories', es: 'Repositorios de código' },
    description: { en: 'Browse repositories, reviews, CI gates, commits, and protected merge flows.', es: 'Explora repositorios, revisiones, puertas CI, commits y flujos de merge protegidos.' },
    badge: { en: 'Developer toolchain', es: 'Toolchain de desarrollo' },
    categoryIds: ['all', 'development', 'toolchain'], browseHref: '/code-repos' },
  { id: 'marketplace', href: '/marketplace', icon: 'folder',
    name: { en: 'Marketplace', es: 'Marketplace' },
    description: { en: 'Discover internal packages, release channels, and one-click rollout bundles.', es: 'Descubre paquetes internos, canales de release y bundles de despliegue con un clic.' },
    badge: { en: 'Application development', es: 'Desarrollo de aplicaciones' },
    categoryIds: ['all', 'development'], browseHref: '/marketplace' },
  { id: 'ai', href: '/ai', icon: 'sparkles',
    name: { en: 'AI Platform', es: 'Plataforma AI' },
    description: { en: 'Manage providers, prompts, guardrails, agents, and copilots from one control plane.', es: 'Gestiona proveedores, prompts, guardrails, agentes y copilots desde un solo plano de control.' },
    badge: { en: 'Models', es: 'Modelos' },
    categoryIds: ['all', 'models'], browseHref: '/ai' },
  { id: 'ml', href: '/ml', icon: 'sparkles',
    name: { en: 'ML Studio', es: 'ML Studio' },
    description: { en: 'Track experiments, training jobs, model versions, and online feature flows.', es: 'Sigue experimentos, trabajos de entrenamiento, versiones de modelo y flujos de features online.' },
    badge: { en: 'Models', es: 'Modelos' },
    categoryIds: ['all', 'models'], browseHref: '/ml' },
  { id: 'streaming', href: '/streaming', icon: 'graph',
    name: { en: 'Streaming', es: 'Streaming' },
    description: { en: 'Operate real-time topologies, windows, joins, and live event tails.', es: 'Opera topologías en tiempo real, ventanas, joins y colas de eventos en vivo.' },
    badge: { en: 'Data integration', es: 'Integración de datos' },
    categoryIds: ['all', 'integration'], browseHref: '/streaming' },
  { id: 'audit', href: '/audit', icon: 'bell',
    name: { en: 'Governance Center', es: 'Centro de gobierno' },
    description: { en: 'Review policies, approvals, applications, and governance templates.', es: 'Revisa políticas, aprobaciones, aplicaciones y plantillas de gobierno.' },
    badge: { en: 'Security & governance', es: 'Seguridad y gobierno' },
    categoryIds: ['all', 'administration', 'governance'], browseHref: '/audit' },
  { id: 'settings', href: '/settings', icon: 'settings',
    name: { en: 'Settings', es: 'Configuración' },
    description: { en: 'Configure identity, language, security posture, and account defaults.', es: 'Configura identidad, idioma, postura de seguridad y valores por defecto de la cuenta.' },
    badge: { en: 'Administration', es: 'Administración' },
    categoryIds: ['all', 'administration', 'governance'], browseHref: '/settings' },
];

function isActive(href: string, pathname: string) {
  return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(`${href}/`);
}

function getPreferredCategory(app: LauncherApp | undefined): LauncherCategoryId {
  return app?.categoryIds[1] ?? app?.categoryIds[0] ?? 'all';
}

export function Sidebar() {
  const locale = useCurrentLocale();
  const t = useMemo(() => createTranslator(locale), [locale]);
  const localize = (copy: LocalizedCopy) => (locale === 'es' ? copy.es : copy.en);
  const { pathname } = useLocation();

  const [open, setOpen] = useState(false);
  const [category, setCategory] = useState<LauncherCategoryId>('all');
  const [selectedAppId, setSelectedAppId] = useState('datasets');
  const [search, setSearch] = useState('');
  const searchRef = useRef<HTMLInputElement | null>(null);

  const launcherText = LAUNCHER_COPY[locale === 'es' ? 'es' : 'en'];

  const hasActiveApplication = LAUNCHER_APPS.some((a) => isActive(a.href, pathname));

  const categoryCounts = useMemo(() =>
    Object.fromEntries(
      LAUNCHER_CATEGORIES.map((c) => [c.id, c.id === 'all' ? LAUNCHER_APPS.length : LAUNCHER_APPS.filter((a) => a.categoryIds.includes(c.id)).length]),
    ) as Record<LauncherCategoryId, number>
  , []);

  const visibleApps = useMemo(() => {
    const term = search.trim().toLowerCase();
    return LAUNCHER_APPS.filter((app) => {
      const matchesCategory = category === 'all' || app.categoryIds.includes(category);
      if (!matchesCategory) return false;
      if (!term) return true;
      return localize(app.name).toLowerCase().includes(term) || localize(app.description).toLowerCase().includes(term);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search, category, locale]);

  const selectedApp = visibleApps.find((a) => a.id === selectedAppId) ?? visibleApps[0] ?? null;

  useEffect(() => {
    if (open) searchRef.current?.focus();
  }, [open]);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && open) setOpen(false);
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [open]);

  function openLauncher() {
    const activeApp = LAUNCHER_APPS.find((a) => isActive(a.href, pathname));
    setCategory(getPreferredCategory(activeApp));
    setSelectedAppId(activeApp?.id ?? LAUNCHER_APPS[0].id);
    setSearch('');
    setOpen(true);
  }

  function toggleLauncher() {
    if (open) setOpen(false); else openLauncher();
  }

  return (
    <>
      <aside className="of-sidebar of-scrollbar">
        <div className="of-sidebar__brand">
          <Link to="/" className="of-sidebar__logo" aria-label={t('nav.home')} title="OpenFoundry">
            <Glyph name="cube" size={18} />
          </Link>
          <span className="of-sidebar__brand-meta" aria-hidden="true">
            <Glyph name={'menu' as GlyphName} size={14} />
          </span>
        </div>

        <nav className="of-sidebar__section" aria-label="Primary">
          {WORKSPACE_NAV.map((item) => (
            <Link
              key={item.href}
              to={item.href}
              className="of-sidebar__link"
              data-active={isActive(item.href, pathname) || undefined}
              title={t(item.labelKey)}
              aria-label={t(item.labelKey)}
            >
              <span className="of-sidebar__icon"><Glyph name={item.icon as GlyphName} size={16} /></span>
              <span className="of-sidebar__label">{t(item.labelKey)}</span>
              {item.hint && <span className="of-sidebar__hint">{item.hint}</span>}
            </Link>
          ))}

          <button
            type="button"
            className="of-sidebar__link of-sidebar__link--button"
            data-active={(hasActiveApplication || open) || undefined}
            data-expanded={open || undefined}
            aria-haspopup="dialog"
            aria-expanded={open}
            onClick={toggleLauncher}
          >
            <span className="of-sidebar__icon"><Glyph name="cube" size={16} /></span>
            <span className="of-sidebar__label">{t('nav.applications')}</span>
            <span className="of-sidebar__caret"><Glyph name={(open ? 'chevron-down' : 'chevron-right') as GlyphName} size={14} /></span>
          </button>
        </nav>

        <div className="of-sidebar__spacer" />

        <nav className="of-sidebar__section of-sidebar__section--footer" aria-label="Utility">
          {UTILITY_NAV.map((item) => (
            <Link
              key={item.href}
              to={item.href}
              className="of-sidebar__link"
              data-active={isActive(item.href, pathname) || undefined}
              title={t(item.labelKey)}
              aria-label={t(item.labelKey)}
            >
              <span className="of-sidebar__icon"><Glyph name={item.icon as GlyphName} size={16} /></span>
              <span className="of-sidebar__label">{t(item.labelKey)}</span>
            </Link>
          ))}
        </nav>
      </aside>

      {open && (
        <div className="of-app-launcher">
          <button type="button" className="of-app-launcher__backdrop" aria-label={launcherText.close} onClick={() => setOpen(false)} />
          <div className="of-app-launcher__surface" role="dialog" aria-modal="true" aria-label={launcherText.title}>
            <div className="of-app-launcher__header">
              <label className="of-app-launcher__search">
                <span className="sr-only">{launcherText.searchAriaLabel}</span>
                <span className="of-app-launcher__search-icon"><Glyph name="search" size={14} /></span>
                <input
                  ref={searchRef}
                  type="search"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={launcherText.searchPlaceholder}
                  aria-label={launcherText.searchAriaLabel}
                />
              </label>
              <button type="button" className="of-app-launcher__close" aria-label={launcherText.close} onClick={() => setOpen(false)}>
                <Glyph name="x" size={14} />
              </button>
            </div>

            <div className="of-app-launcher__body">
              <div className="of-app-launcher__categories">
                {LAUNCHER_CATEGORIES.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    className="of-app-launcher__category"
                    data-active={category === c.id || undefined}
                    onClick={() => setCategory(c.id)}
                  >
                    <span>{localize(c.label)}</span>
                    <span className="of-app-launcher__category-count">{categoryCounts[c.id]}</span>
                  </button>
                ))}
              </div>

              <div className="of-app-launcher__catalog">
                {visibleApps.length === 0 ? (
                  <div className="of-app-launcher__empty">{launcherText.emptySearch}</div>
                ) : (
                  visibleApps.map((app) => (
                    <Link
                      key={app.id}
                      to={app.href}
                      className="of-app-launcher__item"
                      data-selected={selectedApp?.id === app.id || undefined}
                      data-active={isActive(app.href, pathname) || undefined}
                      onMouseEnter={() => setSelectedAppId(app.id)}
                      onFocus={() => setSelectedAppId(app.id)}
                      onClick={() => setOpen(false)}
                    >
                      <span className="of-app-launcher__item-icon"><Glyph name={app.icon as GlyphName} size={16} /></span>
                      <span className="of-app-launcher__item-copy">
                        <span className="of-app-launcher__item-name">{localize(app.name)}</span>
                        <span className="of-app-launcher__item-description">{localize(app.description)}</span>
                      </span>
                    </Link>
                  ))
                )}
              </div>

              <div className="of-app-launcher__detail">
                {selectedApp && (
                  <>
                    <div className="of-app-launcher__detail-icon"><Glyph name={selectedApp.icon as GlyphName} size={18} /></div>
                    <div className="of-app-launcher__detail-copy">
                      <div className="of-app-launcher__detail-badge">{localize(selectedApp.badge)}</div>
                      <h2>{localize(selectedApp.name)}</h2>
                      <p>{localize(selectedApp.description)}</p>
                    </div>
                    <div className="of-app-launcher__actions">
                      <Link to={selectedApp.href} className="of-app-launcher__link" onClick={() => setOpen(false)}>{launcherText.open}</Link>
                      <Link to={selectedApp.createHref ?? selectedApp.href} className="of-app-launcher__button of-app-launcher__button--primary" onClick={() => setOpen(false)}>{launcherText.createNew}</Link>
                      <Link to={selectedApp.browseHref ?? selectedApp.href} className="of-app-launcher__button" onClick={() => setOpen(false)}>{launcherText.browseCatalog}</Link>
                    </div>
                    <div className="of-app-launcher__favorites">
                      <div className="of-app-launcher__favorites-title">{launcherText.favorites}</div>
                      <p>{launcherText.emptyFavorites}</p>
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
