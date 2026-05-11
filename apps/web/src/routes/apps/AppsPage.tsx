import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';

import {
  createApp,
  createAppFromTemplate,
  deleteApp,
  getApp,
  getSlatePackage,
  importSlatePackage,
  listAppTemplates,
  listAppVersions,
  listApps,
  listWidgetCatalog,
  previewApp,
  publishApp,
  updateApp,
  type AppDefinition,
  type AppPage,
  type AppPreviewResponse,
  type AppSummary,
  type AppTemplate,
  type AppVersion,
  type AppWidget,
  type ImportSlatePackageParams,
  type SlatePackageResponse,
  type WidgetCatalogItem,
} from '@/lib/api/apps';
import { AppRenderer } from '@/lib/components/apps/AppRenderer';
import {
  PageCanvas,
  PageInspector,
  PagesOutline,
  WidgetInspector,
  commitPages,
  defaultPage,
  defaultWidget,
  groupWidgetsBySection,
  makeId,
  parsePages,
} from '@/lib/components/apps/AppPagesEditor';
import { CreateAppModal } from '@/lib/components/apps/CreateAppModal';
import { ImportSlateModal } from '@/lib/components/apps/ImportSlateModal';
import { PublishAppModal, type PublishAppDraft } from '@/lib/components/apps/PublishAppModal';
import { ThemePanel } from '@/lib/components/apps/ThemePanel';
import { WidgetCatalog, getWidgetCatalogItems } from '@/lib/components/apps/WidgetCatalog';
import { WorkshopToolbar } from '@/lib/components/apps/WorkshopToolbar';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { ConfirmDialog } from '@/lib/components/ConfirmDialog';
import { Drawer } from '@/lib/components/ui/Drawer';
import { Glyph } from '@/lib/components/ui/Glyph';

interface Draft {
  id?: string;
  name: string;
  slug: string;
  description: string;
  status: string;
  template_key: string;
  pages_json: string;
  settings_json: string;
  theme_json: string;
}

type SecondaryPanel = null | 'settings' | 'theme' | 'versions' | 'slate' | 'preview' | 'add-widget';

const EMPTY_THEME = {
  name: 'Workshop App',
  primary_color: '#2d72d2',
  accent_color: '#0f766e',
  background_color: '#f8fafc',
  surface_color: '#ffffff',
  text_color: '#0f172a',
  heading_font: 'Inter',
  body_font: 'Inter',
  border_radius: 8,
  logo_url: null,
};

const EMPTY_SETTINGS = {
  home_page_id: 'page-home',
  navigation_style: 'tabs',
  max_width: '1280px',
  show_branding: true,
  custom_css: null,
  builder_experience: 'workshop',
  ontology_source_type_id: null,
  object_set_variables: [] as Array<unknown>,
  consumer_mode: {
    enabled: false,
    allow_guest_access: false,
    portal_title: null,
    portal_subtitle: null,
    primary_cta_label: null,
    primary_cta_url: null,
  },
  interactive_workshop: {
    enabled: false,
    title: null,
    subtitle: null,
    briefing_template: null,
    primary_scenario_widget_id: null,
    primary_agent_widget_id: null,
    suggested_questions: [] as string[],
    scenario_presets: [] as Array<unknown>,
  },
  workshop_header: { title: null, icon: null, color: null },
  slate: {
    enabled: false,
    framework: 'react',
    package_name: '@open-foundry/workshop-app',
    entry_file: 'src/App.tsx',
    sdk_import: '@open-foundry/sdk/react',
    workspace: {
      enabled: false,
      repository_id: null,
      layout: 'single-pane',
      runtime: 'node',
      dev_command: 'pnpm dev',
      preview_command: 'pnpm preview',
      files: [],
    },
    quiver_embed: {
      enabled: false,
      primary_type_id: null,
      secondary_type_id: null,
      join_field: null,
      secondary_join_field: null,
      date_field: null,
      metric_field: null,
      group_field: null,
      selected_group: null,
    },
  },
};

function emptyDraft(): Draft {
  return {
    name: 'New app',
    slug: '',
    description: '',
    status: 'draft',
    template_key: '',
    pages_json: JSON.stringify(
      [
        {
          id: 'page-home',
          name: 'Home',
          path: '/',
          description: '',
          layout: { kind: 'grid', columns: 12, gap: '1rem', max_width: '1280px' },
          visible: true,
          widgets: [],
        },
      ],
      null,
      2,
    ),
    settings_json: JSON.stringify(EMPTY_SETTINGS, null, 2),
    theme_json: JSON.stringify(EMPTY_THEME, null, 2),
  };
}

function applyDefinition(definition: AppDefinition): Draft {
  return {
    id: definition.id,
    name: definition.name,
    slug: definition.slug,
    description: definition.description,
    status: definition.status,
    template_key: definition.template_key ?? '',
    pages_json: JSON.stringify(definition.pages, null, 2),
    settings_json: JSON.stringify(definition.settings, null, 2),
    theme_json: JSON.stringify(definition.theme, null, 2),
  };
}

function parseDraftDefinition(draft: Draft): AppDefinition {
  return {
    id: draft.id ?? 'local-draft',
    name: draft.name,
    slug: draft.slug || 'local-draft',
    description: draft.description,
    status: draft.status,
    pages: JSON.parse(draft.pages_json) as AppDefinition['pages'],
    settings: JSON.parse(draft.settings_json) as AppDefinition['settings'],
    theme: JSON.parse(draft.theme_json) as AppDefinition['theme'],
    template_key: draft.template_key || null,
    created_by: null,
    published_version_id: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
}

function toErrorMessage(cause: unknown, fallback: string) {
  return cause instanceof Error ? cause.message : fallback;
}

function downloadJson(filename: string, payload: unknown) {
  if (typeof document === 'undefined') return false;
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
  return true;
}

function safeParse<T>(json: string, fallback: T): T {
  try {
    return JSON.parse(json) as T;
  } catch {
    return fallback;
  }
}

export function AppsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const selectedParam = searchParams.get('selected') ?? '';

  const [apps, setApps] = useState<AppSummary[]>([]);
  const [templates, setTemplates] = useState<AppTemplate[]>([]);
  const [widgetCatalog, setWidgetCatalog] = useState<WidgetCatalogItem[]>([]);
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [versions, setVersions] = useState<AppVersion[]>([]);
  const [slatePackage, setSlatePackage] = useState<SlatePackageResponse | null>(null);
  const [preview, setPreview] = useState<AppPreviewResponse | null>(null);
  const [localPreview, setLocalPreview] = useState<AppDefinition | null>(null);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [feedback, setFeedback] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [secondaryPanel, setSecondaryPanel] = useState<SecondaryPanel>(null);

  const [selectedPageId, setSelectedPageId] = useState('');
  const [selectedWidgetId, setSelectedWidgetId] = useState('');

  const isBuilder = Boolean(selectedParam);

  // ---------- data loading ----------------------------------------------------
  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [appResponse, templateResponse, catalogResponse] = await Promise.all([
        listApps({ per_page: 200, status: statusFilter === 'all' ? undefined : statusFilter })
          .catch(() => ({ data: [] as AppSummary[], total: 0 })),
        listAppTemplates().catch(() => ({ data: [] as AppTemplate[] })),
        listWidgetCatalog().catch(() => [] as WidgetCatalogItem[]),
      ]);
      setApps(appResponse.data);
      setTemplates(templateResponse.data);
      setWidgetCatalog(catalogResponse);
    } catch (cause) {
      setError(toErrorMessage(cause, 'Failed to load apps'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void refresh(); }, []);

  useEffect(() => {
    if (!selectedParam || apps.length === 0) return;
    if (draft.id && (draft.id === selectedParam || draft.slug === selectedParam)) return;
    const match = apps.find((app) => app.id === selectedParam || app.slug === selectedParam);
    if (match) void loadApp(match.id, false);
  }, [apps, selectedParam, draft.id, draft.slug]);

  function setSelectedInUrl(value: string) {
    const next = new URLSearchParams(searchParams);
    if (value) next.set('selected', value);
    else next.delete('selected');
    setSearchParams(next, { replace: true });
  }

  async function loadApp(id: string, syncUrl = true) {
    setError('');
    setFeedback('');
    try {
      const definition = await getApp(id);
      setDraft(applyDefinition(definition));
      setPreview(null);
      setLocalPreview(null);
      setSlatePackage(null);
      setVersions([]);
      setSelectedWidgetId('');
      setSelectedPageId(definition.pages[0]?.id ?? '');
      if (syncUrl) setSelectedInUrl(definition.slug || definition.id);
    } catch (cause) {
      setError(toErrorMessage(cause, 'Failed to load app'));
    }
  }

  async function loadVersions(appId: string) {
    try { setVersions((await listAppVersions(appId)).data); }
    catch (cause) { setError(toErrorMessage(cause, 'Failed to load versions')); }
  }

  async function loadSlate(appId: string) {
    try { setSlatePackage(await getSlatePackage(appId)); }
    catch (cause) { setError(toErrorMessage(cause, 'Slate package failed')); }
  }

  async function persistDraft() {
    const pages = JSON.parse(draft.pages_json) as AppDefinition['pages'];
    const settings = JSON.parse(draft.settings_json) as AppDefinition['settings'];
    const theme = JSON.parse(draft.theme_json) as AppDefinition['theme'];
    const payload = {
      name: draft.name.trim(),
      slug: draft.slug.trim() || undefined,
      description: draft.description,
      status: draft.status,
      pages,
      settings,
      theme,
    };
    if (!payload.name) throw new Error('App name is required.');

    const definition = draft.id
      ? await updateApp(draft.id, payload)
      : await createApp(payload);
    setDraft(applyDefinition(definition));
    setSelectedInUrl(definition.slug || definition.id);
    return definition;
  }

  async function save() {
    setBusy(true); setError(''); setFeedback('');
    try {
      const definition = await persistDraft();
      await refresh();
      setFeedback(`Saved ${definition.name}.`);
    } catch (cause) {
      setError(toErrorMessage(cause, 'Save failed'));
    } finally {
      setBusy(false);
    }
  }

  async function createFromModal(input: { name: string; slug?: string; description?: string; template_key?: string }) {
    setBusy(true); setError(''); setFeedback('');
    try {
      const definition = input.template_key
        ? await createAppFromTemplate(input)
        : await createApp({
          name: input.name,
          slug: input.slug,
          description: input.description,
          status: 'draft',
          pages: JSON.parse(emptyDraft().pages_json) as AppDefinition['pages'],
          settings: EMPTY_SETTINGS as unknown as AppDefinition['settings'],
          theme: EMPTY_THEME as unknown as AppDefinition['theme'],
        });
      setDraft(applyDefinition(definition));
      setSelectedInUrl(definition.slug || definition.id);
      setSelectedPageId(definition.pages[0]?.id ?? '');
      setSelectedWidgetId('');
      setCreateOpen(false);
      await refresh();
      setFeedback(`Created ${definition.name}.`);
    } catch (cause) {
      setError(toErrorMessage(cause, 'Create failed'));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!draft.id) return;
    setBusy(true); setError(''); setFeedback('');
    try {
      await deleteApp(draft.id);
      setDraft(emptyDraft());
      setPreview(null);
      setLocalPreview(null);
      setSlatePackage(null);
      setVersions([]);
      setConfirmDelete(false);
      setSelectedInUrl('');
      await refresh();
      setFeedback('App deleted.');
    } catch (cause) {
      setError(toErrorMessage(cause, 'Delete failed'));
    } finally {
      setBusy(false);
    }
  }

  async function publish(notes: string) {
    if (!draft.id) return;
    setBusy(true); setError(''); setFeedback('');
    try {
      await publishApp(draft.id, { notes: notes || undefined });
      await loadVersions(draft.id);
      await loadApp(draft.id, false);
      await refresh();
      setPublishOpen(false);
      setFeedback('Published a new app version.');
    } catch (cause) {
      setError(toErrorMessage(cause, 'Publish failed'));
    } finally {
      setBusy(false);
    }
  }

  async function openPublishModal() {
    setError('');
    if (draft.id) await loadVersions(draft.id);
    setPublishOpen(true);
  }

  async function runPreview() {
    setBusy(true); setError(''); setFeedback('');
    try {
      if (draft.id) {
        const definition = await persistDraft();
        const response = await previewApp(definition.id);
        setPreview(response);
        setLocalPreview(null);
        await refresh();
      } else {
        setPreview(null);
        setLocalPreview(parseDraftDefinition(draft));
      }
      setSecondaryPanel('preview');
    } catch (cause) {
      setError(toErrorMessage(cause, 'Preview failed'));
    } finally {
      setBusy(false);
    }
  }

  async function exportSlate() {
    if (!draft.id) return;
    setBusy(true); setError(''); setFeedback('');
    try {
      const pkg = await getSlatePackage(draft.id);
      setSlatePackage(pkg);
      const didDownload = downloadJson(`${pkg.app_slug || draft.slug || 'app'}-slate-package.json`, pkg);
      setFeedback(didDownload ? 'Slate package exported.' : 'Slate package loaded.');
    } catch (cause) {
      setError(toErrorMessage(cause, 'Export failed'));
    } finally {
      setBusy(false);
    }
  }

  async function runImportSlate(body: ImportSlatePackageParams) {
    if (!draft.id) return;
    setBusy(true); setError(''); setFeedback('');
    try {
      const response = await importSlatePackage(draft.id, body);
      setDraft(applyDefinition(response.app));
      setSlatePackage(response.slate_package);
      setImportOpen(false);
      setFeedback('Slate package imported.');
    } catch (cause) {
      setError(toErrorMessage(cause, 'Import failed'));
    } finally {
      setBusy(false);
    }
  }

  // ---------- pages mutations -------------------------------------------------
  const { pages } = useMemo(() => parsePages(draft.pages_json), [draft.pages_json]);
  const catalog = useMemo(() => getWidgetCatalogItems(widgetCatalog), [widgetCatalog]);
  const selectedPage = pages.find((page) => page.id === selectedPageId) ?? pages[0] ?? null;
  const selectedWidget = selectedPage?.widgets.find((widget) => widget.id === selectedWidgetId) ?? null;

  useEffect(() => {
    if (pages.length === 0) {
      if (selectedPageId) setSelectedPageId('');
      if (selectedWidgetId) setSelectedWidgetId('');
      return;
    }
    if (!pages.some((page) => page.id === selectedPageId)) {
      setSelectedPageId(pages[0].id);
      setSelectedWidgetId('');
    }
  }, [pages, selectedPageId, selectedWidgetId]);

  useEffect(() => {
    if (!selectedPage || !selectedWidgetId) return;
    if (!selectedPage.widgets.some((widget) => widget.id === selectedWidgetId)) {
      setSelectedWidgetId('');
    }
  }, [selectedPage, selectedWidgetId]);

  function patchPagesJson(updater: (pages: AppPage[]) => AppPage[]) {
    const next = updater(pages);
    commitPages((value) => setDraft((current) => ({ ...current, pages_json: value })), next);
  }

  function patchPage(id: string, patch: Partial<AppPage>) {
    patchPagesJson((current) => current.map((page) => (page.id === id ? { ...page, ...patch } : page)));
  }

  function patchWidget(pageId: string, widgetId: string, patch: Partial<AppWidget>) {
    patchPagesJson((current) => current.map((page) => (
      page.id === pageId
        ? { ...page, widgets: page.widgets.map((widget) => (widget.id === widgetId ? { ...widget, ...patch } : widget)) }
        : page
    )));
  }

  function addPage() {
    const next = defaultPage();
    patchPagesJson((current) => [...current, next]);
    setSelectedPageId(next.id);
    setSelectedWidgetId('');
  }

  function duplicatePage(page: AppPage) {
    const next = {
      ...page,
      id: makeId('page'),
      name: `${page.name} copy`,
      path: `${page.path.replace(/\/$/, '')}-copy`,
      widgets: page.widgets.map((widget) => ({ ...widget, id: makeId('widget') })),
    };
    patchPagesJson((current) => [...current, next]);
    setSelectedPageId(next.id);
    setSelectedWidgetId('');
  }

  function deletePage(id: string) {
    const remaining = pages.filter((page) => page.id !== id);
    patchPagesJson(() => remaining);
    if (selectedPageId === id) {
      setSelectedPageId(remaining[0]?.id ?? '');
      setSelectedWidgetId('');
    }
  }

  function addSection(pageId: string) {
    const page = pages.find((entry) => entry.id === pageId);
    if (!page) return;
    const nextY = page.widgets.reduce((max, widget) => Math.max(max, (widget.position?.y ?? 0) + (widget.position?.height ?? 1)), 0);
    const placeholder: AppWidget = {
      id: makeId('widget'),
      widget_type: 'text',
      title: 'Section',
      description: '',
      position: { x: 0, y: nextY, width: page.layout?.columns ?? 12, height: 2 },
      props: { content: '### Section\nDrop widgets here.' },
      binding: null,
      events: [],
      children: [],
    };
    patchPagesJson((current) => current.map((entry) => (entry.id === pageId ? { ...entry, widgets: [...entry.widgets, placeholder] } : entry)));
    setSelectedWidgetId(placeholder.id);
  }

  function addWidget(pageId: string, item: WidgetCatalogItem, atY?: number) {
    const widget = defaultWidget(item);
    if (typeof atY === 'number') widget.position = { ...widget.position, y: atY };
    patchPagesJson((current) => current.map((page) => (page.id === pageId ? { ...page, widgets: [...page.widgets, widget] } : page)));
    setSelectedWidgetId(widget.id);
    setSecondaryPanel(null);
  }

  function duplicateWidget(pageId: string, widget: AppWidget) {
    const copy = { ...widget, id: makeId('widget'), title: `${widget.title || widget.widget_type} copy` };
    patchPagesJson((current) => current.map((page) => (page.id === pageId ? { ...page, widgets: [...page.widgets, copy] } : page)));
    setSelectedWidgetId(copy.id);
  }

  function deleteWidget(pageId: string, widgetId: string) {
    patchPagesJson((current) => current.map((page) => (
      page.id === pageId ? { ...page, widgets: page.widgets.filter((widget) => widget.id !== widgetId) } : page
    )));
    if (selectedWidgetId === widgetId) setSelectedWidgetId('');
  }

  function applyLayoutTemplate(pageId: string, widgets: AppWidget[]) {
    patchPagesJson((current) => current.map((page) => (page.id === pageId ? { ...page, widgets } : page)));
    setSelectedWidgetId('');
  }

  // ---------- derived ---------------------------------------------------------
  const filteredApps = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return apps.filter((app) => {
      const inStatus = statusFilter === 'all' || app.status === statusFilter;
      const haystack = `${app.name} ${app.slug} ${app.description} ${app.template_key ?? ''}`.toLowerCase();
      return inStatus && (!needle || haystack.includes(needle));
    });
  }, [apps, search, statusFilter]);

  const publishDraft = useMemo<PublishAppDraft | null>(() => {
    if (!draft.id) return null;
    return {
      id: draft.id,
      name: draft.name,
      slug: draft.slug,
      description: draft.description,
      status: draft.status,
      pagesJson: draft.pages_json,
      settingsJson: draft.settings_json,
      themeJson: draft.theme_json,
    };
  }, [draft]);

  const previewDefinition = preview?.app ?? localPreview;

  // Derived counts for the left outline
  const settings = useMemo(() => safeParse<Partial<AppDefinition['settings']>>(draft.settings_json, {}), [draft.settings_json]);
  const objectVarCount = Array.isArray(settings.object_set_variables) ? settings.object_set_variables.length : 0;
  const buttonWidgetCount = useMemo(() => pages.reduce((sum, page) => sum + page.widgets.filter((w) => w.widget_type === 'button').length, 0), [pages]);
  const totalWidgetCount = useMemo(() => pages.reduce((sum, page) => sum + page.widgets.length, 0), [pages]);
  const sectionCount = useMemo(() => (selectedPage ? groupWidgetsBySection(selectedPage.widgets).length : 0), [selectedPage]);

  // ---------- render ---------------------------------------------------------
  if (!isBuilder) {
    return (
      <Gallery
        apps={filteredApps}
        templates={templates}
        loading={loading}
        busy={busy}
        error={error}
        feedback={feedback}
        search={search}
        setSearch={setSearch}
        statusFilter={statusFilter}
        setStatusFilter={setStatusFilter}
        onRefresh={() => void refresh()}
        onCreateFromTemplate={(templateKey) => {
          setCreateOpen(true);
          setDraft((current) => ({ ...current, template_key: templateKey }));
        }}
        onNew={() => setCreateOpen(true)}
        onSelect={(id, slug) => setSelectedInUrl(slug || id)}
        createOpen={createOpen}
        setCreateOpen={setCreateOpen}
        createFromModal={createFromModal}
      />
    );
  }

  return (
    <section
      className="of-page"
      style={{
        display: 'grid',
        gridTemplateRows: 'auto 1fr',
        gap: 0,
        padding: 0,
        minHeight: 'calc(100vh - var(--app-shell-header-height, 56px))',
      }}
    >
      <WorkshopToolbar
        appName={draft.name || 'Untitled app'}
        status={draft.status}
        versionLabel={`v${(versions[0]?.version_number ?? 0) + (draft.id ? 0.1 : 0)}`}
        savedAt={draft.id ? new Date().toISOString() : null}
        branchName="Main"
        busy={busy}
        canPublish={Boolean(draft.id)}
        isPublished={draft.status === 'published'}
        hasApp={Boolean(draft.id)}
        onBack={() => {
          setSelectedInUrl('');
          setDraft(emptyDraft());
          setPreview(null);
          setLocalPreview(null);
          setSlatePackage(null);
          setVersions([]);
        }}
        onSave={() => void save()}
        onPublish={() => void openPublishModal()}
        onPreview={() => void runPreview()}
        onOpenRuntime={() => { if (draft.slug) navigate(`/apps/runtime/${draft.slug}`); }}
        onShare={() => setShareOpen(true)}
        sectionControls={(
          <SectionToolbar
            sectionsCount={sectionCount}
            widgetsCount={selectedPage?.widgets.length ?? 0}
            onAddSection={() => selectedPage && addSection(selectedPage.id)}
            onAddWidget={() => setSecondaryPanel('add-widget')}
            onOpenSettings={() => setSecondaryPanel('settings')}
            onOpenTheme={() => setSecondaryPanel('theme')}
            onOpenVersions={() => { setSecondaryPanel('versions'); if (draft.id) void loadVersions(draft.id); }}
            onOpenSlate={() => { setSecondaryPanel('slate'); if (draft.id) void loadSlate(draft.id); }}
            onDeleteApp={() => setConfirmDelete(true)}
            disableDelete={!draft.id || busy}
            disableWidgetActions={!selectedWidget}
            onDuplicateWidget={() => { if (selectedPage && selectedWidget) duplicateWidget(selectedPage.id, selectedWidget); }}
            onDeleteWidget={() => { if (selectedPage && selectedWidget) deleteWidget(selectedPage.id, selectedWidget.id); }}
          />
        )}
      />

      {(error || feedback) ? (
        <div
          className={error ? 'of-status-danger' : 'of-status-success'}
          style={{ padding: '8px 12px', fontSize: 12, borderBottom: '1px solid var(--border-subtle)' }}
        >
          {error || feedback}
        </div>
      ) : null}

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(220px, 240px) minmax(0, 1fr) minmax(280px, 320px)',
          gap: 0,
          minHeight: 0,
        }}
      >
        <BuilderLeftPane
          appName={draft.name || 'Untitled app'}
          description={draft.description}
          objectTypeCount={objectVarCount}
          actionCount={buttonWidgetCount}
          functionCount={0}
          totalWidgetCount={totalWidgetCount}
          pages={pages}
          selectedPageId={selectedPage?.id ?? ''}
          selectedWidgetId={selectedWidgetId}
          onSelectPage={(id) => { setSelectedPageId(id); setSelectedWidgetId(''); }}
          onSelectWidget={(_, widgetId) => setSelectedWidgetId(widgetId)}
          onAddPage={addPage}
          onDuplicatePage={duplicatePage}
          onDeletePage={deletePage}
        />

        <main style={{ background: 'var(--bg-canvas, #f1f4f9)', overflow: 'auto', minWidth: 0 }}>
          <PageCanvas
            page={selectedPage}
            catalog={catalog}
            selectedWidgetId={selectedWidgetId}
            onSelectWidget={setSelectedWidgetId}
            onAddSection={() => selectedPage && addSection(selectedPage.id)}
            onAddWidgetToSection={() => setSecondaryPanel('add-widget')}
            onApplyTemplate={(widgets) => selectedPage && applyLayoutTemplate(selectedPage.id, widgets)}
          />
        </main>

        <aside
          style={{
            background: 'var(--bg-panel)',
            borderLeft: '1px solid var(--border-subtle)',
            overflow: 'auto',
            minWidth: 0,
            display: 'grid',
            alignContent: 'start',
          }}
        >
          {selectedWidget && selectedPage ? (
            <WidgetInspector
              widget={selectedWidget}
              catalog={catalog}
              onPatch={(patch) => patchWidget(selectedPage.id, selectedWidget.id, patch)}
              onDuplicate={() => duplicateWidget(selectedPage.id, selectedWidget)}
              onDelete={() => deleteWidget(selectedPage.id, selectedWidget.id)}
            />
          ) : selectedPage ? (
            <PageInspector
              page={selectedPage}
              onPatch={(patch) => patchPage(selectedPage.id, patch)}
              onDuplicate={() => duplicatePage(selectedPage)}
              onDelete={() => deletePage(selectedPage.id)}
              disableDelete={pages.length <= 1}
            />
          ) : (
            <div style={{ padding: 14 }}>
              <p className="of-text-muted" style={{ margin: 0 }}>Add a page to start building.</p>
            </div>
          )}
        </aside>
      </div>

      <PublishAppModal
        open={publishOpen}
        publishing={busy}
        latestVersion={versions[0] ?? null}
        app={publishDraft}
        error={error}
        onClose={() => setPublishOpen(false)}
        onPublish={publish}
      />
      <CreateAppModal
        open={createOpen}
        busy={busy}
        templates={templates}
        onClose={() => setCreateOpen(false)}
        onCreate={createFromModal}
      />
      <ImportSlateModal
        open={importOpen}
        busy={busy}
        initialBody={slatePackage ? JSON.stringify(slatePackage, null, 2) : ''}
        onClose={() => setImportOpen(false)}
        onImport={runImportSlate}
      />
      <ConfirmDialog
        open={confirmDelete}
        title="Delete app"
        message={`Delete ${draft.name}? Published runtime links for this slug will stop resolving.`}
        confirmLabel="Delete app"
        danger
        busy={busy}
        onConfirm={() => void remove()}
        onCancel={() => setConfirmDelete(false)}
      />
      <ConfirmDialog
        open={shareOpen}
        title="Share app"
        message={
          draft.id
            ? `Runtime URL: /apps/runtime/${draft.slug || 'new-app'}\nDraft preview is gated by app.read; published URL is gated by app.public.read.`
            : 'Save the app first to generate a share link.'
        }
        confirmLabel="Copy runtime URL"
        onConfirm={() => {
          if (typeof navigator !== 'undefined' && draft.slug) {
            void navigator.clipboard?.writeText(`${window.location.origin}/apps/runtime/${draft.slug}`).catch(() => undefined);
          }
          setShareOpen(false);
        }}
        onCancel={() => setShareOpen(false)}
      />

      <Drawer
        open={secondaryPanel === 'add-widget'}
        title="Add widget"
        width="420px"
        onClose={() => setSecondaryPanel(null)}
      >
        <WidgetCatalog
          items={catalog}
          onSelect={(item) => {
            if (selectedPage) addWidget(selectedPage.id, item);
          }}
        />
      </Drawer>

      <Drawer
        open={secondaryPanel === 'settings'}
        title="App settings"
        width="560px"
        onClose={() => setSecondaryPanel(null)}
      >
        <SettingsDrawer draft={draft} setDraft={setDraft} />
      </Drawer>

      <Drawer
        open={secondaryPanel === 'theme'}
        title="App theme"
        width="560px"
        onClose={() => setSecondaryPanel(null)}
      >
        <ThemePanel
          value={draft.theme_json}
          pagesJson={draft.pages_json}
          onChange={(next) => setDraft((current) => ({ ...current, theme_json: next }))}
        />
      </Drawer>

      <Drawer
        open={secondaryPanel === 'versions'}
        title="Versions"
        width="480px"
        onClose={() => setSecondaryPanel(null)}
      >
        <VersionsList versions={versions} appId={draft.id} onRefresh={() => draft.id && loadVersions(draft.id)} />
      </Drawer>

      <Drawer
        open={secondaryPanel === 'slate'}
        title="Slate package"
        width="560px"
        onClose={() => setSecondaryPanel(null)}
      >
        <SlatePanel
          appId={draft.id}
          slatePackage={slatePackage}
          busy={busy}
          onExport={() => void exportSlate()}
          onImport={() => setImportOpen(true)}
          onRefresh={() => draft.id && loadSlate(draft.id)}
        />
      </Drawer>

      <Drawer
        open={secondaryPanel === 'preview'}
        title="Preview"
        width="720px"
        onClose={() => setSecondaryPanel(null)}
      >
        <PreviewPanel
          app={previewDefinition}
          embedUrl={preview?.embed.url}
          widgetCatalogCount={preview?.widget_catalog.length ?? widgetCatalog.length}
          onPreview={() => void runPreview()}
          busy={busy}
        />
      </Drawer>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Gallery (no app selected)
// ---------------------------------------------------------------------------

interface GalleryProps {
  apps: AppSummary[];
  templates: AppTemplate[];
  loading: boolean;
  busy: boolean;
  error: string;
  feedback: string;
  search: string;
  setSearch: (next: string) => void;
  statusFilter: string;
  setStatusFilter: (next: string) => void;
  onRefresh: () => void;
  onCreateFromTemplate: (templateKey: string) => void;
  onNew: () => void;
  onSelect: (id: string, slug: string) => void;
  createOpen: boolean;
  setCreateOpen: (open: boolean) => void;
  createFromModal: (input: { name: string; slug?: string; description?: string; template_key?: string }) => Promise<void>;
}

function Gallery({
  apps,
  templates,
  loading,
  busy,
  error,
  feedback,
  search,
  setSearch,
  statusFilter,
  setStatusFilter,
  onRefresh,
  onCreateFromTemplate,
  onNew,
  onSelect,
  createOpen,
  setCreateOpen,
  createFromModal,
}: GalleryProps) {
  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header className="of-panel" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '12px 16px', flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0 }}>
          <p className="of-eyebrow" style={{ margin: 0 }}>APP-001 · Workshop</p>
          <h1 className="of-heading-md" style={{ margin: '2px 0 0' }}>Workshop apps</h1>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
            Build operational apps backed by the Foundry ontology and your datasets.
          </p>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          <button type="button" className="of-button" onClick={onRefresh} disabled={loading || busy}>
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
          <button type="button" className="of-button of-button--primary" onClick={onNew} disabled={busy}>
            <Glyph name="plus" size={14} />
            <span style={{ marginLeft: 4 }}>New app</span>
          </button>
        </div>
      </header>

      {(error || feedback) ? (
        <div
          className={error ? 'of-status-danger' : 'of-status-success'}
          style={{ padding: '8px 12px', fontSize: 12, borderRadius: 'var(--radius-sm)' }}
        >
          {error || feedback}
        </div>
      ) : null}

      {templates.length > 0 ? (
        <section className="of-panel" style={{ padding: 16 }}>
          <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 10 }}>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>From a template</p>
              <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>Start from a curated layout.</p>
            </div>
          </header>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
              gap: 10,
            }}
          >
            {templates.map((template) => (
              <button
                key={template.key}
                type="button"
                onClick={() => onCreateFromTemplate(template.key)}
                style={{
                  display: 'grid',
                  gap: 6,
                  padding: 12,
                  textAlign: 'left',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: 'var(--radius-md)',
                  background: 'var(--bg-panel-muted)',
                  color: 'var(--text-default)',
                  cursor: 'pointer',
                  minHeight: 120,
                }}
              >
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <Glyph name="view-grid" size={14} />
                  <strong style={{ color: 'var(--text-strong)' }}>{template.name || template.key}</strong>
                </span>
                <span className="of-text-muted" style={{ fontSize: 12, lineHeight: 1.4 }}>
                  {template.description || 'Pre-configured pages, widgets and theme.'}
                </span>
                <span className="of-chip" style={{ marginTop: 'auto', minHeight: 20, fontSize: 11 }}>{template.key}</span>
              </button>
            ))}
          </div>
        </section>
      ) : null}

      <section className="of-panel" style={{ padding: 16 }}>
        <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: 12, flexWrap: 'wrap' }}>
          <div>
            <p className="of-eyebrow" style={{ margin: 0 }}>Recent apps</p>
            <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
              {apps.length} apps · click to open the builder.
            </p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search apps"
              className="of-input"
              style={{ width: 220 }}
            />
            <div className="of-pill-toggle" aria-label="Status filter">
              {['all', 'draft', 'published', 'archived'].map((status) => (
                <button
                  key={status}
                  type="button"
                  data-active={statusFilter === status}
                  onClick={() => setStatusFilter(status)}
                  style={{ textTransform: 'capitalize' }}
                >
                  {status}
                </button>
              ))}
            </div>
          </div>
        </header>

        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
            gap: 12,
          }}
        >
          {apps.map((app) => (
            <button
              key={app.id}
              type="button"
              onClick={() => onSelect(app.id, app.slug)}
              style={{
                display: 'grid',
                gap: 8,
                padding: 14,
                textAlign: 'left',
                border: '1px solid var(--border-subtle)',
                borderRadius: 'var(--radius-md)',
                background: 'var(--bg-panel)',
                color: 'var(--text-default)',
                cursor: 'pointer',
                minHeight: 150,
                transition: 'box-shadow 120ms ease',
              }}
            >
              <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
                  <Glyph name="app" size={14} />
                  <strong style={{ color: 'var(--text-strong)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{app.name}</strong>
                </span>
                <span className={`of-chip ${app.status === 'published' ? 'of-chip-active' : ''}`} style={{ minHeight: 20, fontSize: 11 }}>
                  {app.status}
                </span>
              </span>
              <span className="of-text-muted" style={{ fontSize: 12, lineHeight: 1.4 }}>
                {app.description || `/${app.slug}`}
              </span>
              <span style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 'auto' }}>
                <span className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>{app.page_count} pages</span>
                <span className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>{app.widget_count} widgets</span>
                {app.template_key ? <span className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>{app.template_key}</span> : null}
              </span>
            </button>
          ))}
          {apps.length === 0 ? (
            <div className="of-panel-muted" style={{ padding: 18, gridColumn: '1 / -1' }}>
              <p className="of-text-muted" style={{ margin: 0 }}>
                {loading ? 'Loading apps…' : 'No apps match the current filters.'}
              </p>
            </div>
          ) : null}
        </div>
      </section>

      <CreateAppModal
        open={createOpen}
        busy={busy}
        templates={templates}
        onClose={() => setCreateOpen(false)}
        onCreate={createFromModal}
      />
    </section>
  );
}

// ---------------------------------------------------------------------------
// Builder left pane: Overview / Object types / Capabilities / Layout / History
// ---------------------------------------------------------------------------

interface BuilderLeftPaneProps {
  appName: string;
  description: string;
  objectTypeCount: number;
  actionCount: number;
  functionCount: number;
  totalWidgetCount: number;
  pages: AppPage[];
  selectedPageId: string;
  selectedWidgetId: string;
  onSelectPage: (id: string) => void;
  onSelectWidget: (pageId: string, widgetId: string) => void;
  onAddPage: () => void;
  onDuplicatePage: (page: AppPage) => void;
  onDeletePage: (id: string) => void;
}

function BuilderLeftPane({
  appName,
  description,
  objectTypeCount,
  actionCount,
  functionCount,
  totalWidgetCount,
  pages,
  selectedPageId,
  selectedWidgetId,
  onSelectPage,
  onSelectWidget,
  onAddPage,
  onDuplicatePage,
  onDeletePage,
}: BuilderLeftPaneProps) {
  return (
    <aside
      style={{
        background: 'var(--bg-panel)',
        borderRight: '1px solid var(--border-subtle)',
        overflow: 'auto',
        display: 'grid',
        alignContent: 'start',
        gap: 14,
        padding: '12px 8px',
      }}
    >
      <section style={{ padding: '0 8px' }}>
        <p className="of-eyebrow" style={{ margin: 0 }}>Overview</p>
        <h2 className="of-heading-sm" style={{ margin: '4px 0 0' }}>{appName}</h2>
        <p className="of-text-muted" style={{ margin: '6px 0 0', fontSize: 12, lineHeight: 1.4 }}>
          {description || 'Enter the purpose of the app'}
        </p>
      </section>

      <CountSection icon="object" label="Object types" count={objectTypeCount} />
      <section style={{ padding: '0 8px' }}>
        <p className="of-eyebrow" style={{ margin: 0 }}>Capabilities</p>
        <ul style={{ listStyle: 'none', padding: 0, margin: '6px 0 0', display: 'grid', gap: 4 }}>
          <CapabilityRow icon="run" label="Actions" value={actionCount} />
          <CapabilityRow icon="code" label="Functions" value={functionCount} />
        </ul>
      </section>

      <PagesOutline
        pages={pages}
        selectedPageId={selectedPageId}
        selectedWidgetId={selectedWidgetId}
        onSelectPage={onSelectPage}
        onSelectWidget={onSelectWidget}
        onAddPage={onAddPage}
        onDuplicatePage={onDuplicatePage}
        onDeletePage={onDeletePage}
      />

      <section style={{ padding: '8px', borderTop: '1px solid var(--border-subtle)' }}>
        <p className="of-eyebrow" style={{ margin: 0 }}>History</p>
        <p className="of-text-muted" style={{ margin: '6px 0 0', fontSize: 11 }}>
          {totalWidgetCount} widgets across {pages.length} pages.
        </p>
      </section>
    </aside>
  );
}

function CountSection({ icon, label, count }: { icon: 'object' | 'list' | 'cube'; label: string; count: number }) {
  return (
    <section style={{ padding: '0 8px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <p className="of-eyebrow" style={{ margin: 0 }}>{label}</p>
        <span className="of-chip" style={{ minHeight: 18, fontSize: 11 }}>{count}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 0', color: 'var(--text-muted)' }}>
        <Glyph name={icon} size={12} />
        <span style={{ fontSize: 12 }}>
          {count === 0 ? 'None added yet' : `${count} configured`}
        </span>
        <button type="button" className="of-button of-button--ghost" style={{ marginLeft: 'auto', minHeight: 22, padding: '0 4px' }} aria-label={`Add ${label}`}>
          <Glyph name="plus" size={12} />
        </button>
      </div>
    </section>
  );
}

function CapabilityRow({ icon, label, value }: { icon: 'run' | 'code'; label: string; value: number }) {
  return (
    <li style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0' }}>
      <Glyph name={icon} size={12} />
      <span style={{ flex: 1, fontSize: 12 }}>{label}</span>
      <span className="of-chip" style={{ minHeight: 18, fontSize: 11 }}>{value}</span>
    </li>
  );
}

// ---------------------------------------------------------------------------
// Section toolbar (under the WorkshopToolbar)
// ---------------------------------------------------------------------------

interface SectionToolbarProps {
  sectionsCount: number;
  widgetsCount: number;
  onAddSection: () => void;
  onAddWidget: () => void;
  onOpenSettings: () => void;
  onOpenTheme: () => void;
  onOpenVersions: () => void;
  onOpenSlate: () => void;
  onDeleteApp: () => void;
  disableDelete: boolean;
  disableWidgetActions: boolean;
  onDuplicateWidget: () => void;
  onDeleteWidget: () => void;
}

function SectionToolbar({
  sectionsCount,
  widgetsCount,
  onAddSection,
  onAddWidget,
  onOpenSettings,
  onOpenTheme,
  onOpenVersions,
  onOpenSlate,
  onDeleteApp,
  disableDelete,
  disableWidgetActions,
  onDuplicateWidget,
  onDeleteWidget,
}: SectionToolbarProps) {
  return (
    <>
      <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', paddingRight: 4 }}>SECTION</span>
      <button type="button" className="of-button" onClick={onAddSection}>
        <Glyph name="plus" size={11} />
        <span style={{ marginLeft: 4 }}>Add section inside</span>
      </button>
      <button type="button" className="of-button of-button--primary" onClick={onAddWidget}>
        <Glyph name="plus" size={11} />
        <span style={{ marginLeft: 4 }}>Add widget</span>
      </button>

      <span style={{ width: 1, height: 18, background: 'var(--border-default)', margin: '0 4px' }} />

      <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', paddingRight: 4 }}>SHIFT</span>
      <button type="button" className="of-button" disabled>Above</button>
      <button type="button" className="of-button" disabled>Below</button>
      <button type="button" className="of-button" disabled>Left</button>
      <button type="button" className="of-button" disabled>Right</button>
      <button type="button" className="of-button" disabled>Split section</button>
      <button type="button" className="of-button" disabled>Add header</button>

      <span style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 6 }}>
        <span className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>{sectionsCount} sections</span>
        <span className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>{widgetsCount} widgets</span>
        <span style={{ width: 1, height: 18, background: 'var(--border-default)' }} />
        <button type="button" className="of-button of-button--ghost" title="Duplicate widget" onClick={onDuplicateWidget} disabled={disableWidgetActions} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="duplicate" size={12} />
        </button>
        <button type="button" className="of-button of-button--ghost" title="Delete widget" onClick={onDeleteWidget} disabled={disableWidgetActions} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="trash" size={12} />
        </button>
        <span style={{ width: 1, height: 18, background: 'var(--border-default)' }} />
        <button type="button" className="of-button of-button--ghost" title="App settings" onClick={onOpenSettings} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="settings" size={12} />
        </button>
        <button type="button" className="of-button of-button--ghost" title="Theme" onClick={onOpenTheme} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="star" size={12} />
        </button>
        <button type="button" className="of-button of-button--ghost" title="Versions" onClick={onOpenVersions} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="history" size={12} />
        </button>
        <button type="button" className="of-button of-button--ghost" title="Slate package" onClick={onOpenSlate} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="code" size={12} />
        </button>
        <button type="button" className="of-button of-btn-danger" title="Delete app" onClick={onDeleteApp} disabled={disableDelete} style={{ minHeight: 24, padding: '0 6px' }}>
          <Glyph name="trash" size={12} />
        </button>
      </span>
    </>
  );
}

// ---------------------------------------------------------------------------
// Drawer panels
// ---------------------------------------------------------------------------

function SettingsDrawer({ draft, setDraft }: { draft: Draft; setDraft: Dispatch<SetStateAction<Draft>> }) {
  return (
    <div style={{ display: 'grid', gap: 14 }}>
      <section style={{ display: 'grid', gap: 8 }}>
        <p className="of-eyebrow" style={{ margin: 0 }}>App definition</p>
        <label style={{ display: 'grid', gap: 4, fontSize: 12, fontWeight: 600 }}>
          Name
          <input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} className="of-input" />
        </label>
        <label style={{ display: 'grid', gap: 4, fontSize: 12, fontWeight: 600 }}>
          Slug
          <input value={draft.slug} onChange={(event) => setDraft((current) => ({ ...current, slug: event.target.value }))} className="of-input" />
        </label>
        <label style={{ display: 'grid', gap: 4, fontSize: 12, fontWeight: 600 }}>
          Status
          <select value={draft.status} onChange={(event) => setDraft((current) => ({ ...current, status: event.target.value }))} className="of-input">
            <option value="draft">draft</option>
            <option value="published">published</option>
            <option value="archived">archived</option>
          </select>
        </label>
        <label style={{ display: 'grid', gap: 4, fontSize: 12, fontWeight: 600 }}>
          Description
          <textarea
            value={draft.description}
            onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))}
            rows={3}
            className="of-input"
            style={{ resize: 'vertical' }}
          />
        </label>
      </section>

      <details>
        <summary style={{ cursor: 'pointer', fontSize: 12, color: 'var(--text-muted)' }}>Raw settings JSON</summary>
        <div style={{ marginTop: 8 }}>
          <JsonEditor
            value={draft.settings_json}
            onChange={(next) => setDraft((current) => ({ ...current, settings_json: next }))}
            minHeight={320}
          />
        </div>
      </details>
    </div>
  );
}

function VersionsList({ versions, appId, onRefresh }: { versions: AppVersion[]; appId?: string; onRefresh: () => void }) {
  if (!appId) return <p className="of-text-muted" style={{ margin: 0 }}>Save the app before publishing versions.</p>;
  return (
    <div style={{ display: 'grid', gap: 10 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10 }}>
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>{versions.length} release snapshots</p>
        <button type="button" className="of-button" onClick={onRefresh}>Refresh</button>
      </div>
      <div style={{ display: 'grid', gap: 8 }}>
        {versions.map((version) => (
          <article key={version.id} className="of-panel-muted" style={{ display: 'grid', gap: 6, padding: 12 }}>
            <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
              <strong style={{ color: 'var(--text-strong)' }}>v{version.version_number}</strong>
              <span className={`of-chip ${version.published_at ? 'of-chip-active' : ''}`}>{version.status}</span>
            </div>
            <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>
              {version.published_at ? `Published ${version.published_at.slice(0, 16)}` : `Created ${version.created_at.slice(0, 16)}`}
            </p>
            {version.notes ? <p style={{ margin: 0, fontSize: 13 }}>{version.notes}</p> : null}
          </article>
        ))}
        {versions.length === 0 ? <p className="of-text-muted" style={{ margin: 0 }}>No versions yet.</p> : null}
      </div>
    </div>
  );
}

function SlatePanel({
  appId,
  slatePackage,
  busy,
  onExport,
  onImport,
  onRefresh,
}: {
  appId?: string;
  slatePackage: SlatePackageResponse | null;
  busy: boolean;
  onExport: () => void;
  onImport: () => void;
  onRefresh: () => void;
}) {
  if (!appId) return <p className="of-text-muted" style={{ margin: 0 }}>Save the app before importing or exporting Slate packages.</p>;
  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Round-trip pro-code packages for this app.</p>
      <div style={{ display: 'flex', gap: 7, flexWrap: 'wrap' }}>
        <button type="button" className="of-button" onClick={onRefresh} disabled={busy}>Load package</button>
        <button type="button" className="of-button" onClick={onImport} disabled={busy}>Import Slate</button>
        <button type="button" className="of-button of-button--primary" onClick={onExport} disabled={busy}>Export Slate</button>
      </div>
      {slatePackage ? (
        <pre
          className="of-scrollbar"
          style={{
            margin: 0,
            padding: 10,
            background: 'var(--bg-subtle)',
            fontSize: 11,
            fontFamily: 'var(--font-mono)',
            border: '1px solid var(--border-subtle)',
            borderRadius: 'var(--radius-md)',
            overflow: 'auto',
            maxHeight: 360,
          }}
        >
          {JSON.stringify(slatePackage, null, 2)}
        </pre>
      ) : (
        <p className="of-text-muted" style={{ margin: 0 }}>Load or export the Slate package to inspect generated files.</p>
      )}
    </div>
  );
}

function PreviewPanel({
  app,
  embedUrl,
  widgetCatalogCount,
  onPreview,
  busy,
}: {
  app: AppDefinition | null;
  embedUrl?: string;
  widgetCatalogCount: number;
  onPreview: () => void;
  busy: boolean;
}) {
  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>
          Catalog contains {widgetCatalogCount} widget definitions.
        </p>
        <button type="button" className="of-button of-button--primary" onClick={onPreview} disabled={busy}>
          {busy ? 'Rendering…' : 'Render preview'}
        </button>
      </div>
      {embedUrl ? (
        <section className="of-panel-muted" style={{ display: 'flex', justifyContent: 'space-between', gap: 12, padding: 10 }}>
          <span className="of-text-muted">Embed URL</span>
          <code>{embedUrl}</code>
        </section>
      ) : null}
      {app ? (
        <AppRenderer app={app} mode="builder" />
      ) : (
        <div className="of-panel-muted" style={{ display: 'grid', minHeight: 220, placeItems: 'center', padding: 16 }}>
          <p className="of-text-muted" style={{ margin: 0 }}>Run preview to render the current draft.</p>
        </div>
      )}
    </div>
  );
}
