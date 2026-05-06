import { useEffect, useMemo, useState } from 'react';

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
  publishApp,
  updateApp,
  type AppSummary,
  type AppTemplate,
  type AppVersion,
  type SlatePackageResponse,
} from '@/lib/api/apps';
import { AppPagesEditor } from '@/lib/components/apps/AppPagesEditor';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { Tabs } from '@/lib/components/Tabs';

type Tab = 'definition' | 'pages' | 'settings' | 'theme' | 'versions' | 'slate';

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

const EMPTY_THEME = {
  primary_color: '#2458b8',
  background_color: '#0f172a',
  font_family: 'Inter',
  border_radius: 'medium',
  density: 'comfortable',
  surface_color: '#1e293b',
  text_color: '#f1f5f9',
};

const EMPTY_SETTINGS = {
  consumer_mode: { layout: 'grid' },
  workshop: {},
  slate: {},
  quiver: {},
  object_set_variables: [],
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
        { id: 'page-home', name: 'Home', layout: { columns: 12 }, widgets: [] },
      ],
      null,
      2,
    ),
    settings_json: JSON.stringify(EMPTY_SETTINGS, null, 2),
    theme_json: JSON.stringify(EMPTY_THEME, null, 2),
  };
}

export function AppsPage() {
  const [apps, setApps] = useState<AppSummary[]>([]);
  const [templates, setTemplates] = useState<AppTemplate[]>([]);
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [versions, setVersions] = useState<AppVersion[]>([]);
  const [slatePackage, setSlatePackage] = useState<SlatePackageResponse | null>(null);
  const [tab, setTab] = useState<Tab>('definition');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [publishNotes, setPublishNotes] = useState('');
  const [importBody, setImportBody] = useState(
    JSON.stringify({ files: [], slug: '', sdk_import: '' }, null, 2),
  );

  async function refresh() {
    setError('');
    try {
      const [a, t] = await Promise.all([
        listApps({ search: search || undefined, per_page: 200 }),
        listAppTemplates().catch(() => ({ data: [] as AppTemplate[] })),
      ]);
      setApps(a.data);
      setTemplates(t.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load apps');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function loadApp(id: string) {
    try {
      const def = await getApp(id);
      setDraft({
        id: def.id,
        name: def.name,
        slug: def.slug,
        description: def.description,
        status: def.status,
        template_key: def.template_key ?? '',
        pages_json: JSON.stringify(def.pages, null, 2),
        settings_json: JSON.stringify(def.settings, null, 2),
        theme_json: JSON.stringify(def.theme, null, 2),
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load app');
    }
  }

  async function loadVersions(appId: string) {
    try {
      setVersions((await listAppVersions(appId)).data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load versions');
    }
  }

  async function loadSlate(appId: string) {
    try {
      setSlatePackage(await getSlatePackage(appId));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Slate package failed');
    }
  }

  async function save() {
    setBusy(true);
    setError('');
    try {
      const pages = JSON.parse(draft.pages_json);
      const settings = JSON.parse(draft.settings_json);
      const theme = JSON.parse(draft.theme_json);
      if (draft.id) {
        const updated = await updateApp(draft.id, {
          name: draft.name,
          slug: draft.slug || undefined,
          description: draft.description,
          status: draft.status,
          pages,
          settings,
          theme,
        });
        await loadApp(updated.id);
      } else if (draft.template_key) {
        const created = await createAppFromTemplate({
          name: draft.name,
          slug: draft.slug || undefined,
          description: draft.description,
          template_key: draft.template_key,
        });
        await loadApp(created.id);
      } else {
        const created = await createApp({
          name: draft.name,
          slug: draft.slug || undefined,
          description: draft.description,
          status: draft.status,
          pages,
          settings,
          theme,
        });
        await loadApp(created.id);
      }
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!draft.id) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete app?')) return;
    setBusy(true);
    try {
      await deleteApp(draft.id);
      setDraft(emptyDraft());
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function publish() {
    if (!draft.id) return;
    setBusy(true);
    try {
      await publishApp(draft.id, { notes: publishNotes || undefined });
      await loadVersions(draft.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Publish failed');
    } finally {
      setBusy(false);
    }
  }

  async function runImportSlate() {
    if (!draft.id) return;
    setBusy(true);
    try {
      await importSlatePackage(draft.id, JSON.parse(importBody));
      await loadApp(draft.id);
      await loadSlate(draft.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Import failed');
    } finally {
      setBusy(false);
    }
  }

  const filteredApps = useMemo(() => {
    if (!search) return apps;
    const s = search.toLowerCase();
    return apps.filter((a) => a.name.toLowerCase().includes(s) || a.slug.toLowerCase().includes(s));
  }, [apps, search]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Apps</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Workshop-style app builder. JSON-driven editor for pages, settings, theme. Versions + publish + slate import/export.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.4fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <div style={{ display: 'flex', gap: 6 }}>
            <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search…" className="of-input" />
            <button type="button" onClick={() => void refresh()} className="of-button">Refresh</button>
          </div>
          <button type="button" onClick={() => setDraft(emptyDraft())} className="of-button" style={{ marginTop: 8, fontSize: 12 }}>
            New app
          </button>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {filteredApps.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  onClick={() => void loadApp(a.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 10,
                    borderRadius: 8,
                    border: `1px solid ${draft.id === a.id ? '#1d4ed8' : 'var(--border-default)'}`,
                    background: draft.id === a.id ? '#eff6ff' : 'transparent',
                    cursor: 'pointer',
                    marginBottom: 4,
                  }}
                >
                  <strong>{a.name}</strong>
                  <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>
                    /{a.slug} · {a.status} · {a.page_count}p · {a.widget_count}w
                  </p>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="of-panel" style={{ padding: 16 }}>
          <Tabs
            tabs={['definition', 'pages', 'settings', 'theme', 'versions', 'slate'] as const}
            active={tab}
            onChange={(t) => {
              setTab(t);
              if (t === 'versions' && draft.id) void loadVersions(draft.id);
              if (t === 'slate' && draft.id) void loadSlate(draft.id);
            }}
          />

          {tab === 'definition' && (
            <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
              <label style={{ fontSize: 13 }}>
                Name
                <input value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Slug
                <input value={draft.slug} onChange={(e) => setDraft((d) => ({ ...d, slug: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Description
                <textarea value={draft.description} onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))} rows={3} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Status
                <select value={draft.status} onChange={(e) => setDraft((d) => ({ ...d, status: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                  <option value="draft">draft</option>
                  <option value="published">published</option>
                  <option value="archived">archived</option>
                </select>
              </label>
              {!draft.id && (
                <label style={{ fontSize: 13 }}>
                  Template (optional, creates from template)
                  <select value={draft.template_key} onChange={(e) => setDraft((d) => ({ ...d, template_key: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                    <option value="">— none —</option>
                    {templates.map((t) => (
                      <option key={t.id} value={t.key}>{t.name} ({t.category})</option>
                    ))}
                  </select>
                </label>
              )}
              <div style={{ display: 'flex', gap: 6 }}>
                <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">
                  {draft.id ? 'Update' : 'Create'}
                </button>
                {draft.id && (
                  <>
                    <input value={publishNotes} onChange={(e) => setPublishNotes(e.target.value)} placeholder="publish notes" className="of-input" />
                    <button type="button" onClick={() => void publish()} disabled={busy} className="of-button">Publish</button>
                    <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
                      Delete
                    </button>
                  </>
                )}
              </div>
            </div>
          )}

          {tab === 'pages' && (
            <div style={{ marginTop: 8, display: 'grid', gap: 12 }}>
              <AppPagesEditor
                pagesJson={draft.pages_json}
                onChange={(v) => setDraft((d) => ({ ...d, pages_json: v }))}
              />
              <details>
                <summary style={{ cursor: 'pointer', fontSize: 12, color: 'var(--text-muted)' }}>Raw JSON</summary>
                <div style={{ marginTop: 8 }}>
                  <JsonEditor
                    value={draft.pages_json}
                    onChange={(v) => setDraft((d) => ({ ...d, pages_json: v }))}
                    minHeight={240}
                  />
                </div>
              </details>
            </div>
          )}

          {tab === 'settings' && (
            <div style={{ marginTop: 8 }}>
              <JsonEditor
                value={draft.settings_json}
                onChange={(v) => setDraft((d) => ({ ...d, settings_json: v }))}
                minHeight={360}
              />
            </div>
          )}

          {tab === 'theme' && (
            <div style={{ marginTop: 8 }}>
              <JsonEditor
                value={draft.theme_json}
                onChange={(v) => setDraft((d) => ({ ...d, theme_json: v }))}
                minHeight={240}
              />
            </div>
          )}

          {tab === 'versions' && (
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {versions.map((v) => (
                <li key={v.id}>
                  v{v.version_number} · {v.status} · {v.published_at ? `published ${v.published_at.slice(0, 16)}` : 'unpublished'} · {v.notes || '—'}
                </li>
              ))}
              {versions.length === 0 && <li className="of-text-muted">No versions yet.</li>}
            </ul>
          )}

          {tab === 'slate' && (
            <>
              {slatePackage ? (
                <pre style={{ marginTop: 8, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 280 }}>
                  {JSON.stringify(slatePackage, null, 2)}
                </pre>
              ) : (
                <p className="of-text-muted" style={{ marginTop: 8 }}>Click slate tab to fetch package.</p>
              )}
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Import slate package</p>
              <JsonEditor value={importBody} onChange={setImportBody} minHeight={160} />
              <button type="button" onClick={() => void runImportSlate()} disabled={busy} className="of-button" style={{ marginTop: 6 }}>
                Import
              </button>
            </>
          )}
        </section>
      </div>
    </section>
  );
}
