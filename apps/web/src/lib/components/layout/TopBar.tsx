import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';

import { auth, useCurrentUser, useIsAuthenticated } from '@/lib/stores/auth';
import { ontologySearch } from '@/lib/stores/ontologySearch';
import { createTranslator, getLocaleLabel, setLocale, useCurrentLocale, type AppLocale } from '@/lib/i18n/store';
import type { MessageKey } from '@/lib/i18n/messages';
import { Glyph } from '@/lib/components/ui/Glyph';

import { NotificationBell } from './NotificationBell';

const TITLE_MAP: Record<string, MessageKey> = {
  '/': 'nav.home',
  '/apps': 'nav.applications',
  '/dashboards': 'nav.recent',
  '/datasets': 'nav.datasets',
  '/ml': 'nav.training',
  '/notebooks': 'nav.workshop',
  '/object-explorer': 'nav.objectExplorer',
  '/object-monitors': 'nav.notifications',
  '/ontology': 'nav.ontology',
  '/ontology-manager': 'nav.ontologyManager',
  '/pipelines': 'nav.pipelineBuilder',
  '/projects': 'nav.projects',
  '/queries': 'nav.queries',
  '/reports': 'nav.files',
  '/search': 'nav.search',
  '/settings': 'nav.account',
  '/control-panel': 'common.controlPanel',
};

export function TopBar() {
  const locale = useCurrentLocale();
  const t = useMemo(() => createTranslator(locale), [locale]);
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const isAuthenticated = useIsAuthenticated();
  const user = useCurrentUser();

  const [createMenuOpen, setCreateMenuOpen] = useState(false);
  const [createSearch, setCreateSearch] = useState('');
  const [createCategory, setCreateCategory] = useState<'all' | 'integration'>('all');
  const createMenuRef = useRef<HTMLDivElement | null>(null);

  const supported: AppLocale[] = ['en', 'es'];

  const pageTitle = useMemo(() => {
    const sorted = Object.keys(TITLE_MAP).sort((a, b) => b.length - a.length);
    const match = sorted.find((key) => pathname === key || pathname.startsWith(`${key}/`));
    return match ? t(TITLE_MAP[match]) : t('topbar.pageDefault');
  }, [pathname, t]);

  const isPipelineVisible = useMemo(() => {
    const term = createSearch.trim().toLowerCase();
    const matchesCategory = createCategory === 'all' || createCategory === 'integration';
    const title = t('nav.pipelineBuilder').toLowerCase();
    const description = t('topbar.pipelineBuilderDescription').toLowerCase();
    return matchesCategory && (!term || title.includes(term) || description.includes(term));
  }, [createSearch, createCategory, t]);

  function closeCreateMenu() {
    setCreateMenuOpen(false);
    setCreateSearch('');
    setCreateCategory('all');
  }

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (!createMenuOpen || !createMenuRef.current) return;
      if (createMenuRef.current.contains(e.target as Node)) return;
      closeCreateMenu();
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && createMenuOpen) closeCreateMenu();
    }
    window.addEventListener('click', onClick);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('click', onClick);
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [createMenuOpen]);

  function handleLogout() {
    auth.logout();
    navigate('/auth/login');
  }

  return (
    <header className="of-topbar">
      <div className="of-topbar__crumbs">
        <div className="of-topbar__trail">
          <span className="of-topbar__crumb-icon"><Glyph name="folder" size={13} /></span>
          <span className="of-topbar__crumb">{t('topbar.workspace')}</span>
        </div>
        <span aria-hidden="true"><Glyph name="chevron-right" size={11} /></span>
        <div className="of-topbar__trail of-topbar__trail--current">
          <span className="of-topbar__crumb">{pageTitle}</span>
        </div>
      </div>

      <div className="of-topbar__actions">
        <div className="of-topbar__create-menu" ref={createMenuRef}>
          <button
            type="button"
            className="of-topbar__action of-topbar__action--primary"
            aria-haspopup="dialog"
            aria-expanded={createMenuOpen}
            onClick={() => setCreateMenuOpen((v) => !v)}
          >
            <Glyph name="plus" size={14} />
            <span>{t('topbar.createNew')}</span>
            <Glyph name="chevron-down" size={12} />
          </button>

          {createMenuOpen && (
            <div className="of-topbar__create-panel" role="dialog" aria-label={t('topbar.createNew')}>
              <label className="of-topbar__create-search">
                <span className="of-topbar__create-search-icon"><Glyph name="search" size={14} /></span>
                <input
                  type="search"
                  value={createSearch}
                  onChange={(e) => setCreateSearch(e.target.value)}
                  placeholder={t('topbar.createSearchPlaceholder')}
                  aria-label={t('topbar.createSearchPlaceholder')}
                />
              </label>

              <div className="of-topbar__create-body">
                <div className="of-topbar__create-categories">
                  <button
                    type="button"
                    className="of-topbar__create-category"
                    data-active={createCategory === 'all' || undefined}
                    onClick={() => setCreateCategory('all')}
                  >
                    {t('topbar.createCategoryAll')}
                  </button>
                  <button
                    type="button"
                    className="of-topbar__create-category"
                    data-active={createCategory === 'integration' || undefined}
                    onClick={() => setCreateCategory('integration')}
                  >
                    {t('topbar.createCategoryIntegration')}
                  </button>
                </div>

                <div className="of-topbar__create-results">
                  {isPipelineVisible ? (
                    <button
                      type="button"
                      className="of-topbar__create-item"
                      onClick={() => { closeCreateMenu(); navigate('/pipelines/new'); }}
                    >
                      <span className="of-topbar__create-item-icon"><Glyph name="graph" size={17} /></span>
                      <span className="of-topbar__create-item-copy">
                        <span className="of-topbar__create-item-title">{t('nav.pipelineBuilder')}</span>
                        <span className="of-topbar__create-item-description">{t('topbar.pipelineBuilderDescription')}</span>
                      </span>
                      <span className="of-topbar__create-item-arrow"><Glyph name="chevron-right" size={14} /></span>
                    </button>
                  ) : (
                    <div className="of-topbar__create-empty">{t('topbar.createEmpty')}</div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>

        <label className="of-topbar__action">
          <span>{t('topbar.userLanguage')}</span>
          <select
            className="bg-transparent text-[11px] font-semibold outline-none"
            value={locale}
            onChange={(e) => setLocale(e.target.value as AppLocale)}
          >
            {supported.map((loc) => (
              <option key={loc} value={loc}>{getLocaleLabel(loc, locale)}</option>
            ))}
          </select>
        </label>

        <button
          type="button"
          className="of-topbar__action"
          onClick={() => ontologySearch.open()}
          aria-label="Search ontology (⌘K)"
          title="Search ontology (⌘K)"
        >
          <Glyph name="search" size={14} />
          <span>Search</span>
          <kbd className="of-topbar__kbd">⌘K</kbd>
        </button>

        <Link to="/apps" className="of-topbar__action">
          <Glyph name="cube" size={14} />
          <span>{t('nav.applications')}</span>
        </Link>

        {isAuthenticated ? (
          <>
            <NotificationBell />
            <div className="of-topbar__user">
              <span className="of-topbar__avatar">OF</span>
              <div className="min-w-0">
                <div className="truncate text-[12px] font-semibold text-[var(--text-strong)]">
                  {user?.name ?? t('topbar.operator')}
                </div>
                <div className="truncate text-[11px] text-[var(--text-muted)]">{t('topbar.workspaceSession')}</div>
              </div>
            </div>
            <button type="button" className="of-topbar__action" onClick={handleLogout} aria-label={t('common.logout')}>
              <Glyph name="logout" size={14} />
            </button>
          </>
        ) : (
          <Link to="/auth/login" className="of-topbar__action">{t('common.login')}</Link>
        )}
      </div>
    </header>
  );
}
