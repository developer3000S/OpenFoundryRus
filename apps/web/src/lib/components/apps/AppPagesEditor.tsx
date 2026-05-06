import { useMemo, useState } from 'react';

import { JsonEditor } from '@/lib/components/JsonEditor';
import type { AppPage, AppWidget } from '@/lib/api/apps';

interface AppPagesEditorProps {
  pagesJson: string;
  onChange: (next: string) => void;
}

function makeId(prefix: string) {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 10_000)}`;
}

function defaultWidget(widget_type: string): AppWidget {
  return {
    id: makeId('widget'),
    widget_type,
    title: widget_type,
    description: '',
    position: { x: 0, y: 0, width: 6, height: 4 },
    props: {},
    binding: null,
    events: [],
    children: [],
  };
}

function defaultPage(): AppPage {
  return {
    id: makeId('page'),
    name: 'New page',
    path: '/new',
    description: '',
    layout: { kind: 'grid', columns: 12, gap: '1rem', max_width: '1280px' },
    widgets: [],
    visible: true,
  };
}

export function AppPagesEditor({ pagesJson, onChange }: AppPagesEditorProps) {
  const pages: AppPage[] = useMemo(() => {
    try { return JSON.parse(pagesJson) as AppPage[]; }
    catch { return []; }
  }, [pagesJson]);

  const [selectedPageId, setSelectedPageId] = useState<string>(pages[0]?.id ?? '');
  const [selectedWidgetId, setSelectedWidgetId] = useState<string>('');

  const selectedPage = pages.find((p) => p.id === selectedPageId) ?? null;
  const selectedWidget = selectedPage?.widgets.find((w) => w.id === selectedWidgetId) ?? null;

  function commit(nextPages: AppPage[]) {
    onChange(JSON.stringify(nextPages, null, 2));
  }

  function patchPage(id: string, patch: Partial<AppPage>) {
    commit(pages.map((p) => (p.id === id ? { ...p, ...patch } : p)));
  }

  function patchWidget(pageId: string, widgetId: string, patch: Partial<AppWidget>) {
    commit(pages.map((p) =>
      p.id === pageId
        ? { ...p, widgets: p.widgets.map((w) => (w.id === widgetId ? { ...w, ...patch } : w)) }
        : p,
    ));
  }

  function addPage() {
    const np = defaultPage();
    commit([...pages, np]);
    setSelectedPageId(np.id);
    setSelectedWidgetId('');
  }

  function deletePage(id: string) {
    commit(pages.filter((p) => p.id !== id));
    if (selectedPageId === id) setSelectedPageId('');
  }

  function addWidget(pageId: string, widget_type: string) {
    const w = defaultWidget(widget_type);
    commit(pages.map((p) => (p.id === pageId ? { ...p, widgets: [...p.widgets, w] } : p)));
    setSelectedWidgetId(w.id);
  }

  function deleteWidget(pageId: string, widgetId: string) {
    commit(pages.map((p) => (p.id === pageId ? { ...p, widgets: p.widgets.filter((w) => w.id !== widgetId) } : p)));
    if (selectedWidgetId === widgetId) setSelectedWidgetId('');
  }

  return (
    <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 220px) minmax(0, 220px) minmax(0, 1fr)' }}>
      <section className="of-panel" style={{ padding: 12 }}>
        <p className="of-eyebrow">Pages ({pages.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
          {pages.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => { setSelectedPageId(p.id); setSelectedWidgetId(''); }}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 8,
                  borderRadius: 6,
                  border: `1px solid ${selectedPageId === p.id ? '#1d4ed8' : 'var(--border-default)'}`,
                  background: selectedPageId === p.id ? '#eff6ff' : 'transparent',
                  cursor: 'pointer',
                  fontSize: 12,
                }}
              >
                <strong>{p.name}</strong>
                <p className="of-text-muted" style={{ fontSize: 10, margin: 0 }}>{p.path} · {p.widgets.length}w</p>
              </button>
            </li>
          ))}
          {pages.length === 0 && <li className="of-text-muted">No pages.</li>}
        </ul>
        <button type="button" onClick={addPage} className="of-button" style={{ marginTop: 8, fontSize: 11 }}>+ Page</button>
      </section>

      <section className="of-panel" style={{ padding: 12 }}>
        <p className="of-eyebrow">Widgets {selectedPage ? `(${selectedPage.widgets.length})` : ''}</p>
        {selectedPage ? (
          <>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
              {selectedPage.widgets.map((w) => (
                <li key={w.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedWidgetId(w.id)}
                    style={{
                      width: '100%',
                      textAlign: 'left',
                      padding: 8,
                      borderRadius: 6,
                      border: `1px solid ${selectedWidgetId === w.id ? '#1d4ed8' : 'var(--border-default)'}`,
                      background: selectedWidgetId === w.id ? '#eff6ff' : 'transparent',
                      cursor: 'pointer',
                      fontSize: 12,
                    }}
                  >
                    <strong>{w.title || w.widget_type}</strong>
                    <p className="of-text-muted" style={{ fontSize: 10, margin: 0 }}>{w.widget_type}</p>
                  </button>
                </li>
              ))}
              {selectedPage.widgets.length === 0 && <li className="of-text-muted">No widgets.</li>}
            </ul>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
              {['text', 'table', 'chart', 'metric', 'object', 'agent', 'scenario', 'iframe'].map((t) => (
                <button key={t} type="button" onClick={() => addWidget(selectedPage.id, t)} className="of-button" style={{ fontSize: 10, padding: '2px 8px' }}>
                  + {t}
                </button>
              ))}
            </div>
          </>
        ) : (
          <p className="of-text-muted" style={{ fontSize: 12 }}>Pick a page first.</p>
        )}
      </section>

      <section className="of-panel" style={{ padding: 12 }}>
        {selectedWidget && selectedPage ? (
          <div style={{ display: 'grid', gap: 8 }}>
            <p className="of-eyebrow">Widget {selectedWidget.id.slice(0, 12)}…</p>
            <label style={{ fontSize: 13 }}>
              Title
              <input value={selectedWidget.title} onChange={(e) => patchWidget(selectedPage.id, selectedWidget.id, { title: e.target.value })} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Widget type
              <input value={selectedWidget.widget_type} onChange={(e) => patchWidget(selectedPage.id, selectedWidget.id, { widget_type: e.target.value })} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <input value={selectedWidget.description} onChange={(e) => patchWidget(selectedPage.id, selectedWidget.id, { description: e.target.value })} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <div style={{ display: 'grid', gap: 6, gridTemplateColumns: 'repeat(4, 1fr)' }}>
              {(['x', 'y', 'width', 'height'] as const).map((field) => (
                <label key={field} style={{ fontSize: 11 }}>
                  {field}
                  <input
                    type="number"
                    value={selectedWidget.position[field]}
                    onChange={(e) => patchWidget(selectedPage.id, selectedWidget.id, {
                      position: { ...selectedWidget.position, [field]: Number(e.target.value) || 0 },
                    })}
                    className="of-input"
                    style={{ marginTop: 4 }}
                  />
                </label>
              ))}
            </div>
            <JsonEditor
              label="Props"
              value={JSON.stringify(selectedWidget.props, null, 2)}
              onChange={(text) => {
                try { patchWidget(selectedPage.id, selectedWidget.id, { props: JSON.parse(text) }); }
                catch { /* JsonEditor surfaces error */ }
              }}
              minHeight={120}
            />
            <JsonEditor
              label="Binding"
              value={JSON.stringify(selectedWidget.binding ?? null, null, 2)}
              onChange={(text) => {
                try { patchWidget(selectedPage.id, selectedWidget.id, { binding: JSON.parse(text) }); }
                catch { /* JsonEditor surfaces error */ }
              }}
              minHeight={80}
            />
            <div>
              <button type="button" onClick={() => deleteWidget(selectedPage.id, selectedWidget.id)} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                Delete widget
              </button>
            </div>
          </div>
        ) : selectedPage ? (
          <div style={{ display: 'grid', gap: 8 }}>
            <p className="of-eyebrow">Page {selectedPage.id.slice(0, 12)}…</p>
            <label style={{ fontSize: 13 }}>
              Name
              <input value={selectedPage.name} onChange={(e) => patchPage(selectedPage.id, { name: e.target.value })} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Path
              <input value={selectedPage.path} onChange={(e) => patchPage(selectedPage.id, { path: e.target.value })} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <textarea value={selectedPage.description} onChange={(e) => patchPage(selectedPage.id, { description: e.target.value })} rows={3} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
              <input type="checkbox" checked={selectedPage.visible} onChange={(e) => patchPage(selectedPage.id, { visible: e.target.checked })} />
              Visible
            </label>
            <JsonEditor
              label="Layout"
              value={JSON.stringify(selectedPage.layout, null, 2)}
              onChange={(text) => {
                try { patchPage(selectedPage.id, { layout: JSON.parse(text) }); }
                catch { /* JsonEditor surfaces error */ }
              }}
              minHeight={100}
            />
            <div>
              <button type="button" onClick={() => deletePage(selectedPage.id)} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                Delete page
              </button>
            </div>
          </div>
        ) : (
          <p className="of-text-muted">Pick a page or add one.</p>
        )}
      </section>
    </div>
  );
}
