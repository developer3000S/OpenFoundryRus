import { useEffect, useMemo, useState } from 'react';

import {
  getObjectView,
  listActionTypes,
  listObjects,
  listObjectTypes,
  listProperties,
  type ActionType,
  type ObjectInstance,
  type ObjectType,
  type ObjectViewResponse,
  type Property,
} from '@/lib/api/ontology';

type ViewMode = 'standard' | 'configured';
type FormFactor = 'full' | 'panel';
type EditorTab = 'editor' | 'versions' | 'publish';
type SectionKind = 'summary' | 'properties' | 'links' | 'timeline' | 'actions' | 'graph' | 'comments' | 'apps';

interface ObjectViewSection {
  id: string;
  title: string;
  kind: SectionKind;
  description: string;
}

interface ObjectViewSidebarLink {
  id: string;
  label: string;
  href: string;
}

interface ConfiguredObjectView {
  mode: 'configured';
  form_factor: FormFactor;
  title_template: string;
  subtitle_property: string;
  prominent_properties: string[];
  panel_properties: string[];
  sections: ObjectViewSection[];
  sidebar_links: ObjectViewSidebarLink[];
  comments_enabled: boolean;
  branch_label: string;
  auto_publish: boolean;
}

interface ObjectViewVersion {
  id: string;
  object_type_id: string;
  form_factor: FormFactor;
  description: string;
  created_at: string;
  created_by: string;
  published: boolean;
  branch_label: string;
  config: ConfiguredObjectView;
}

interface StoredViewState {
  full: ObjectViewVersion[];
  panel: ObjectViewVersion[];
}

const STORAGE_KEY = 'of.objectViews.versions';

const SECTION_KINDS: Array<{ id: SectionKind; label: string; description: string }> = [
  { id: 'summary', label: 'Summary', description: 'Hero metrics and prominent properties.' },
  { id: 'properties', label: 'Properties', description: 'Object schema fields.' },
  { id: 'links', label: 'Linked objects', description: 'Related entities and previews.' },
  { id: 'timeline', label: 'Timeline', description: 'Activity, comments, runtime events.' },
  { id: 'actions', label: 'Actions', description: 'Applicable actions.' },
  { id: 'graph', label: 'Graph', description: 'Neighborhood and graph context.' },
  { id: 'comments', label: 'Comments', description: 'Notes, handoff, collaboration.' },
  { id: 'apps', label: 'Applications', description: 'Quiver, Map, Rules, workflow links.' },
];

const SIDEBAR_PRESETS: ObjectViewSidebarLink[] = [
  { id: 'quiver', label: 'Quiver', href: '/quiver' },
  { id: 'graph', label: 'Graph', href: '/ontology/graph' },
  { id: 'explorer', label: 'Object Explorer', href: '/object-explorer' },
  { id: 'rules', label: 'Foundry Rules', href: '/foundry-rules' },
  { id: 'set', label: 'Saved lists', href: '/ontology/object-sets' },
];

function defaultConfig(formFactor: FormFactor): ConfiguredObjectView {
  return {
    mode: 'configured',
    form_factor: formFactor,
    title_template: '{{name}}',
    subtitle_property: '',
    prominent_properties: [],
    panel_properties: [],
    sections:
      formFactor === 'full'
        ? [
            { id: crypto.randomUUID(), title: 'Overview', kind: 'summary', description: 'Core identity and metrics.' },
            { id: crypto.randomUUID(), title: 'Properties', kind: 'properties', description: 'Canonical schema fields.' },
            { id: crypto.randomUUID(), title: 'Linked Objects', kind: 'links', description: 'Traverse the neighborhood.' },
            { id: crypto.randomUUID(), title: 'Activity', kind: 'timeline', description: 'Recent events.' },
            { id: crypto.randomUUID(), title: 'Actions', kind: 'actions', description: 'Applicable actions.' },
            { id: crypto.randomUUID(), title: 'Graph', kind: 'graph', description: 'Graph context.' },
          ]
        : [
            { id: crypto.randomUUID(), title: 'Summary', kind: 'summary', description: 'Compact metrics.' },
            { id: crypto.randomUUID(), title: 'Properties', kind: 'properties', description: 'Key fields.' },
            { id: crypto.randomUUID(), title: 'Links', kind: 'links', description: 'Linked objects.' },
          ],
    sidebar_links: SIDEBAR_PRESETS.slice(0, 3),
    comments_enabled: true,
    branch_label: 'draft',
    auto_publish: false,
  };
}

function readVersions(): StoredViewState {
  if (typeof window === 'undefined') return { full: [], panel: [] };
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as StoredViewState) : { full: [], panel: [] };
  } catch {
    return { full: [], panel: [] };
  }
}

function writeVersions(state: StoredViewState) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function ObjectViewsPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [properties, setProperties] = useState<Property[]>([]);
  const [actions, setActions] = useState<ActionType[]>([]);
  const [objects, setObjects] = useState<ObjectInstance[]>([]);
  const [loading, setLoading] = useState(true);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [error, setError] = useState('');

  const [selectedTypeId, setSelectedTypeId] = useState('');
  const [selectedObjectId, setSelectedObjectId] = useState('');
  const [activeMode, setActiveMode] = useState<ViewMode>('configured');
  const [activeFormFactor, setActiveFormFactor] = useState<FormFactor>('full');
  const [activeEditorTab, setActiveEditorTab] = useState<EditorTab>('editor');
  const [versionDescription, setVersionDescription] = useState('');

  const [preview, setPreview] = useState<ObjectViewResponse | null>(null);
  const [config, setConfig] = useState<ConfiguredObjectView>(() => defaultConfig('full'));
  const [versions, setVersions] = useState<StoredViewState>({ full: [], panel: [] });

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError('');
      try {
        const typeRes = await listObjectTypes({ page: 1, per_page: 100 });
        if (cancelled) return;
        setObjectTypes(typeRes.data);
        if (typeRes.data[0]) {
          setSelectedTypeId(typeRes.data[0].id);
        }
        setVersions(readVersions());
      } catch (cause) {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load object types');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!selectedTypeId) return;
    let cancelled = false;
    async function loadType() {
      try {
        const [propRes, objRes, actionRes] = await Promise.all([
          listProperties(selectedTypeId),
          listObjects(selectedTypeId, { page: 1, per_page: 50 }),
          listActionTypes({ object_type_id: selectedTypeId, page: 1, per_page: 50 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 50 })),
        ]);
        if (cancelled) return;
        setProperties(propRes);
        setObjects(objRes.data);
        setActions(actionRes.data);
        if (objRes.data[0]) setSelectedObjectId(objRes.data[0].id);
      } catch (cause) {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load type details');
      }
    }
    void loadType();
    return () => {
      cancelled = true;
    };
  }, [selectedTypeId]);

  useEffect(() => {
    if (!selectedTypeId || !selectedObjectId) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    async function loadPreview() {
      setPreviewLoading(true);
      try {
        const res = await getObjectView(selectedTypeId, selectedObjectId);
        if (!cancelled) setPreview(res);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load preview');
      } finally {
        if (!cancelled) setPreviewLoading(false);
      }
    }
    void loadPreview();
    return () => {
      cancelled = true;
    };
  }, [selectedTypeId, selectedObjectId]);

  const availableVersions = activeFormFactor === 'full' ? versions.full : versions.panel;
  const publishedVersion = availableVersions.find((v) => v.published) ?? null;

  const summaryEntries = useMemo(() => {
    if (!preview) return [];
    return Object.entries(preview.summary)
      .filter(([key]) =>
        activeMode === 'standard'
          ? true
          : (activeFormFactor === 'full' ? config.prominent_properties : config.panel_properties).includes(key),
      )
      .slice(0, activeFormFactor === 'full' ? 8 : 4);
  }, [preview, activeMode, activeFormFactor, config]);

  function persistVersions(next: StoredViewState) {
    setVersions(next);
    writeVersions(next);
  }

  function publishVersion() {
    const newVersion: ObjectViewVersion = {
      id: crypto.randomUUID(),
      object_type_id: selectedTypeId,
      form_factor: activeFormFactor,
      description: versionDescription || `${activeFormFactor} view`,
      created_at: new Date().toISOString(),
      created_by: 'platform-ui',
      published: true,
      branch_label: config.branch_label,
      config: { ...config, form_factor: activeFormFactor },
    };
    const next: StoredViewState = {
      full: activeFormFactor === 'full'
        ? [{ ...newVersion }, ...versions.full.map((v) => ({ ...v, published: false }))]
        : versions.full,
      panel: activeFormFactor === 'panel'
        ? [{ ...newVersion }, ...versions.panel.map((v) => ({ ...v, published: false }))]
        : versions.panel,
    };
    persistVersions(next);
    setVersionDescription('');
  }

  function toggleSection(kind: SectionKind) {
    setConfig((current) => {
      const exists = current.sections.find((s) => s.kind === kind);
      if (exists) {
        return { ...current, sections: current.sections.filter((s) => s.kind !== kind) };
      }
      const meta = SECTION_KINDS.find((s) => s.id === kind);
      return {
        ...current,
        sections: [
          ...current.sections,
          { id: crypto.randomUUID(), title: meta?.label ?? kind, kind, description: meta?.description ?? '' },
        ],
      };
    });
  }

  function togglePropertyInList(list: 'prominent_properties' | 'panel_properties', name: string) {
    setConfig((current) => {
      const exists = current[list].includes(name);
      return {
        ...current,
        [list]: exists ? current[list].filter((p) => p !== name) : [...current[list], name],
      };
    });
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16, padding: 24 }}>
      <header>
        <h1 className="of-heading-xl">Object views</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Configure full-page and side-panel object views per type, preview against real objects, and publish versions
          (localStorage-backed in this slice).
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Object type:
          <select
            value={selectedTypeId}
            onChange={(e) => setSelectedTypeId(e.target.value)}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Object:
          <select
            value={selectedObjectId}
            onChange={(e) => setSelectedObjectId(e.target.value)}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            {objects.map((o) => (
              <option key={o.id} value={o.id}>
                {o.id.slice(0, 8)}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Mode:
          <select
            value={activeMode}
            onChange={(e) => setActiveMode(e.target.value as ViewMode)}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            <option value="standard">Standard</option>
            <option value="configured">Configured</option>
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Form factor:
          <select
            value={activeFormFactor}
            onChange={(e) => {
              const next = e.target.value as FormFactor;
              setActiveFormFactor(next);
              setConfig(defaultConfig(next));
            }}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            <option value="full">Full</option>
            <option value="panel">Panel</option>
          </select>
        </label>
      </div>

      <div style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
        {(['editor', 'versions', 'publish'] as EditorTab[]).map((tab) => {
          const active = activeEditorTab === tab;
          return (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveEditorTab(tab)}
              style={{
                padding: '8px 14px',
                background: 'transparent',
                border: 'none',
                borderBottom: `2px solid ${active ? '#1d4ed8' : 'transparent'}`,
                color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
                textTransform: 'capitalize',
              }}
            >
              {tab}
            </button>
          );
        })}
      </div>

      {activeEditorTab === 'editor' && (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)' }}>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Configure view</p>
            <label style={{ display: 'block', marginTop: 10, fontSize: 13 }}>
              Title template
              <input
                value={config.title_template}
                onChange={(e) => setConfig((c) => ({ ...c, title_template: e.target.value }))}
                className="of-input"
                style={{ marginTop: 4 }}
              />
            </label>
            <label style={{ display: 'block', marginTop: 8, fontSize: 13 }}>
              Subtitle property
              <select
                value={config.subtitle_property}
                onChange={(e) => setConfig((c) => ({ ...c, subtitle_property: e.target.value }))}
                className="of-input"
                style={{ marginTop: 4 }}
              >
                <option value="">—</option>
                {properties.map((p) => (
                  <option key={p.id} value={p.name}>
                    {p.display_name} ({p.name})
                  </option>
                ))}
              </select>
            </label>

            <p className="of-eyebrow" style={{ marginTop: 14 }}>Prominent properties</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 6 }}>
              {properties.map((p) => {
                const active = config.prominent_properties.includes(p.name);
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => togglePropertyInList('prominent_properties', p.name)}
                    className="of-chip"
                    style={active ? { background: '#dbeafe', color: '#1d4ed8' } : {}}
                  >
                    {p.name}
                  </button>
                );
              })}
            </div>

            <p className="of-eyebrow" style={{ marginTop: 14 }}>Panel properties</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 6 }}>
              {properties.map((p) => {
                const active = config.panel_properties.includes(p.name);
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => togglePropertyInList('panel_properties', p.name)}
                    className="of-chip"
                    style={active ? { background: '#dbeafe', color: '#1d4ed8' } : {}}
                  >
                    {p.name}
                  </button>
                );
              })}
            </div>

            <p className="of-eyebrow" style={{ marginTop: 14 }}>Sections</p>
            <div style={{ display: 'grid', gap: 4, marginTop: 6 }}>
              {SECTION_KINDS.map((kind) => {
                const active = config.sections.some((s) => s.kind === kind.id);
                return (
                  <label
                    key={kind.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '6px 10px',
                      borderRadius: 8,
                      border: '1px solid var(--border-default)',
                      fontSize: 13,
                      background: active ? '#eff6ff' : 'transparent',
                    }}
                  >
                    <input type="checkbox" checked={active} onChange={() => toggleSection(kind.id)} />
                    <strong>{kind.label}</strong>
                    <span className="of-text-muted">{kind.description}</span>
                  </label>
                );
              })}
            </div>

            <p className="of-eyebrow" style={{ marginTop: 14 }}>Sidebar links</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 6 }}>
              {SIDEBAR_PRESETS.map((link) => {
                const active = config.sidebar_links.find((l) => l.id === link.id);
                return (
                  <button
                    key={link.id}
                    type="button"
                    onClick={() =>
                      setConfig((c) => ({
                        ...c,
                        sidebar_links: active
                          ? c.sidebar_links.filter((l) => l.id !== link.id)
                          : [...c.sidebar_links, link],
                      }))
                    }
                    className="of-chip"
                    style={active ? { background: '#dbeafe', color: '#1d4ed8' } : {}}
                  >
                    {link.label}
                  </button>
                );
              })}
            </div>

            <label style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 14, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={config.comments_enabled}
                onChange={(e) => setConfig((c) => ({ ...c, comments_enabled: e.target.checked }))}
              />
              Enable comments
            </label>

            <label style={{ display: 'block', marginTop: 8, fontSize: 13 }}>
              Branch label
              <input
                value={config.branch_label}
                onChange={(e) => setConfig((c) => ({ ...c, branch_label: e.target.value }))}
                className="of-input"
                style={{ marginTop: 4 }}
              />
            </label>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Preview</p>
            {previewLoading ? (
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>Loading preview…</p>
            ) : preview ? (
              <>
                <h3 className="of-heading-md" style={{ marginTop: 8 }}>
                  {preview.object.id.slice(0, 8)}
                </h3>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  Type: {preview.object.object_type_id}
                </p>
                <div style={{ display: 'grid', gap: 6, marginTop: 12 }}>
                  {summaryEntries.map(([key, value]) => (
                    <div
                      key={key}
                      className="of-panel-muted"
                      style={{ padding: 10, fontSize: 13 }}
                    >
                      <strong>{key}</strong>: {String(value)}
                    </div>
                  ))}
                </div>
                <p className="of-eyebrow" style={{ marginTop: 14 }}>Sections present</p>
                <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 13 }}>
                  {(activeMode === 'standard'
                    ? ['summary', 'properties', 'links', 'timeline', 'actions', 'graph']
                    : config.sections.map((s) => s.kind)
                  ).map((kind) => (
                    <li key={kind}>{kind}</li>
                  ))}
                </ul>
                <p className="of-eyebrow" style={{ marginTop: 14 }}>Applicable actions</p>
                <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 13 }}>
                  {preview.applicable_actions.map((a) => (
                    <li key={a.id}>
                      {a.display_name} ({a.operation_kind})
                    </li>
                  ))}
                  {preview.applicable_actions.length === 0 && (
                    <li className="of-text-muted">No applicable actions.</li>
                  )}
                </ul>
              </>
            ) : (
              <p className="of-text-muted">Select an object to preview.</p>
            )}
          </section>
        </div>
      )}

      {activeEditorTab === 'versions' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Stored versions ({activeFormFactor})</p>
          {availableVersions.length === 0 ? (
            <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
              No versions stored yet for this form factor.
            </p>
          ) : (
            <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
              {availableVersions.map((v) => (
                <div key={v.id} className="of-panel-muted" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <strong>{v.description}</strong>
                      <p className="of-text-muted" style={{ marginTop: 2, fontSize: 11 }}>
                        {v.branch_label} · {new Date(v.created_at).toLocaleString()} · {v.created_by}
                      </p>
                    </div>
                    {v.published && (
                      <span className="of-chip" style={{ background: '#ecfdf5', color: '#047857' }}>
                        Published
                      </span>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={() => setConfig(v.config)}
                    className="of-button"
                    style={{ marginTop: 8, fontSize: 12 }}
                  >
                    Load
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {activeEditorTab === 'publish' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Publish version</p>
          <label style={{ display: 'block', marginTop: 8, fontSize: 13 }}>
            Description
            <input
              value={versionDescription}
              onChange={(e) => setVersionDescription(e.target.value)}
              className="of-input"
              style={{ marginTop: 4 }}
              placeholder={`${activeFormFactor} view ${new Date().toLocaleDateString()}`}
            />
          </label>
          <button
            type="button"
            onClick={publishVersion}
            className="of-button of-button--primary"
            style={{ marginTop: 8 }}
            disabled={!selectedTypeId}
          >
            Publish current configuration
          </button>
          {publishedVersion && (
            <p className="of-text-muted" style={{ marginTop: 14, fontSize: 13 }}>
              Currently published: <strong>{publishedVersion.description}</strong> ({new Date(publishedVersion.created_at).toLocaleDateString()})
            </p>
          )}
          <p className="of-eyebrow" style={{ marginTop: 14 }}>Generated URLs</p>
          <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 13, fontFamily: 'var(--font-mono)' }}>
            <li>{selectedTypeId && selectedObjectId ? `/object-views?type=${selectedTypeId}&object=${selectedObjectId}&mode=configured&factor=full` : '—'}</li>
            <li>{selectedTypeId && selectedObjectId ? `/object-views?type=${selectedTypeId}&object=${selectedObjectId}&mode=configured&factor=panel` : '—'}</li>
          </ul>
        </section>
      )}

      {loading && <p className="of-text-muted">Loading…</p>}

      {actions.length > 0 && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Action types for this object type</p>
          <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 13 }}>
            {actions.map((a) => (
              <li key={a.id}>
                {a.display_name} — {a.operation_kind}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
