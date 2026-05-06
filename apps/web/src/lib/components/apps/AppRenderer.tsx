import { useEffect, useMemo, useState } from 'react';

import type { AppDefinition, AppPage, AppWidget, WidgetEvent } from '@/lib/api/apps';

interface AppRendererProps {
  app: AppDefinition;
  mode?: 'builder' | 'published';
}

export function AppRenderer({ app, mode = 'published' }: AppRendererProps) {
  const visiblePages = useMemo(() => app.pages.filter((p) => p.visible !== false), [app]);
  const [activePageId, setActivePageId] = useState<string>('');
  const [banner, setBanner] = useState<string>('');
  const [filter, setFilter] = useState('');
  const [params, setParams] = useState<Record<string, string>>({});

  useEffect(() => {
    setActivePageId(visiblePages[0]?.id ?? '');
    setBanner('');
    setFilter('');
    setParams({});
  }, [app.id, visiblePages]);

  const activePage = visiblePages.find((p) => p.id === activePageId) ?? visiblePages[0] ?? null;

  const themeVars = useMemo(() => {
    const t = app.theme as unknown as Record<string, string | number | undefined>;
    const obj: Record<string, string> = {};
    if (t.primary_color) obj['--app-primary'] = String(t.primary_color);
    if (t.background_color) obj['--app-background'] = String(t.background_color);
    if (t.surface_color) obj['--app-surface'] = String(t.surface_color);
    if (t.text_color) obj['--app-text'] = String(t.text_color);
    if (t.border_radius) obj['--app-radius'] = `${t.border_radius}px`;
    return obj;
  }, [app.theme]);

  function handleAction(event: WidgetEvent) {
    const config = (event.config ?? {}) as Record<string, unknown>;
    if (event.action === 'navigate') {
      const target = String(config.page_id ?? config.page_path ?? config.path ?? '');
      const page = app.pages.find((p) => p.id === target || p.path === target);
      if (page) {
        setActivePageId(page.id);
        setBanner(`Navigated to ${page.name}`);
        return;
      }
      if (mode === 'published' && target.startsWith('/')) {
        window.location.href = target;
      }
      return;
    }
    if (event.action === 'open_link') {
      const url = String(config.url ?? '');
      if (!url) return;
      if (mode === 'builder') {
        setBanner(`Preview would open ${url}`);
        return;
      }
      if (url.startsWith('/')) {
        window.location.href = url;
      } else {
        window.open(url, '_blank', 'noopener,noreferrer');
      }
      return;
    }
    if (event.action === 'filter') {
      const value = typeof config.value === 'string' ? config.value : '';
      setFilter(value);
      setBanner(`Filter applied: ${value || '(cleared)'}`);
      return;
    }
    setBanner(`Action "${event.action}" — not handled in MVP runtime.`);
  }

  return (
    <div
      style={{
        ...(themeVars as React.CSSProperties),
        background: 'var(--app-background, #0f172a)',
        color: 'var(--app-text, #f1f5f9)',
        borderRadius: 'var(--app-radius, 12px)',
        padding: 16,
        display: 'grid',
        gap: 16,
      }}
    >
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 20 }}>{app.name}</h2>
          {app.description && <p style={{ margin: 0, opacity: 0.7, fontSize: 13 }}>{app.description}</p>}
        </div>
        <div style={{ fontSize: 11, opacity: 0.6 }}>
          {mode === 'builder' ? 'Builder preview' : 'Published runtime'} · v{app.published_version_id?.slice(0, 8) ?? '—'}
        </div>
      </header>

      {visiblePages.length > 1 && (
        <nav style={{ display: 'flex', gap: 4, borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
          {visiblePages.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => setActivePageId(p.id)}
              style={{
                padding: '8px 16px',
                background: 'transparent',
                border: 'none',
                cursor: 'pointer',
                color: 'inherit',
                borderBottom: activePageId === p.id ? '2px solid var(--app-primary, #3b82f6)' : '2px solid transparent',
                fontSize: 13,
                fontWeight: activePageId === p.id ? 600 : 400,
              }}
            >
              {p.name}
            </button>
          ))}
        </nav>
      )}

      {banner && (
        <div style={{ padding: 8, background: 'rgba(59,130,246,0.1)', borderRadius: 6, fontSize: 12 }}>{banner}</div>
      )}

      {activePage ? (
        <PageRenderer page={activePage} onAction={handleAction} filter={filter} params={params} setParams={setParams} />
      ) : (
        <p style={{ opacity: 0.6 }}>No pages.</p>
      )}
    </div>
  );
}

function PageRenderer({
  page,
  onAction,
  filter,
  params,
  setParams,
}: {
  page: AppPage;
  onAction: (event: WidgetEvent) => void;
  filter: string;
  params: Record<string, string>;
  setParams: (next: Record<string, string>) => void;
}) {
  const columns = page.layout?.columns ?? 12;

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, 1fr)`,
        gap: page.layout?.gap ?? '1rem',
        maxWidth: page.layout?.max_width ?? '1280px',
      }}
    >
      {page.widgets.map((w) => (
        <div
          key={w.id}
          style={{
            gridColumn: `span ${Math.min(w.position?.width ?? columns, columns)}`,
            gridRow: `span ${w.position?.height ?? 1}`,
          }}
        >
          <WidgetRenderer widget={w} onAction={onAction} filter={filter} params={params} setParams={setParams} />
        </div>
      ))}
      {page.widgets.length === 0 && (
        <p style={{ gridColumn: '1 / -1', opacity: 0.6, fontSize: 13 }}>No widgets on this page.</p>
      )}
    </div>
  );
}

function WidgetRenderer({
  widget,
  onAction,
  filter,
  params,
  setParams,
}: {
  widget: AppWidget;
  onAction: (event: WidgetEvent) => void;
  filter: string;
  params: Record<string, string>;
  setParams: (next: Record<string, string>) => void;
}) {
  const props = (widget.props ?? {}) as Record<string, unknown>;
  const card: React.CSSProperties = {
    background: 'var(--app-surface, #1e293b)',
    borderRadius: 'var(--app-radius, 8px)',
    padding: 14,
    height: '100%',
    overflow: 'hidden',
  };

  switch (widget.widget_type) {
    case 'text':
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          <div style={{ fontSize: 13, lineHeight: 1.5, whiteSpace: 'pre-wrap' }}>
            {String(props.body ?? props.content ?? widget.description ?? '')}
          </div>
        </div>
      );
    case 'metric': {
      const value = props.value ?? '—';
      return (
        <div style={card}>
          <p style={{ margin: 0, fontSize: 11, opacity: 0.6, textTransform: 'uppercase' }}>{widget.title || 'Metric'}</p>
          <p style={{ margin: '4px 0 0', fontSize: 28, fontWeight: 700 }}>{String(value)}</p>
          {props.delta !== undefined && (
            <p style={{ margin: '4px 0 0', fontSize: 11, opacity: 0.7 }}>Δ {String(props.delta)}</p>
          )}
        </div>
      );
    }
    case 'button':
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {(widget.events ?? []).map((event) => (
              <button
                key={event.id}
                type="button"
                onClick={() => onAction(event)}
                style={{
                  padding: '6px 14px',
                  background: 'var(--app-primary, #3b82f6)',
                  color: '#fff',
                  border: 'none',
                  borderRadius: 6,
                  cursor: 'pointer',
                  fontSize: 12,
                }}
              >
                {event.label || event.trigger || event.action}
              </button>
            ))}
            {(widget.events ?? []).length === 0 && (
              <span style={{ fontSize: 12, opacity: 0.6 }}>{String(props.label ?? 'No events configured')}</span>
            )}
          </div>
        </div>
      );
    case 'image':
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          {typeof props.url === 'string' && (
            <img src={props.url} alt={String(props.alt ?? widget.title)} style={{ maxWidth: '100%', borderRadius: 4 }} />
          )}
        </div>
      );
    case 'form': {
      const fields = Array.isArray(props.fields) ? (props.fields as Array<Record<string, unknown>>) : [];
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          <div style={{ display: 'grid', gap: 6 }}>
            {fields.map((f, i) => {
              const name = String(f.name ?? `field_${i}`);
              return (
                <label key={i} style={{ fontSize: 12 }}>
                  {String(f.label ?? name)}
                  <input
                    value={params[name] ?? ''}
                    onChange={(e) => setParams({ ...params, [name]: e.target.value })}
                    style={{ marginTop: 4, width: '100%', padding: 4, fontSize: 12, background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.15)', borderRadius: 4, color: 'inherit' }}
                  />
                </label>
              );
            })}
          </div>
        </div>
      );
    }
    case 'iframe':
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          {typeof props.src === 'string' ? (
            <iframe src={props.src} title={widget.title || 'iframe'} style={{ width: '100%', height: 300, border: 'none', borderRadius: 4 }} />
          ) : (
            <span style={{ opacity: 0.6, fontSize: 12 }}>iframe missing src</span>
          )}
        </div>
      );
    case 'container':
      return (
        <div style={card}>
          {widget.title && <h3 style={{ margin: '0 0 8px', fontSize: 14 }}>{widget.title}</h3>}
          <div style={{ display: 'grid', gap: 8 }}>
            {(widget.children ?? []).map((child) => (
              <WidgetRenderer key={child.id} widget={child} onAction={onAction} filter={filter} params={params} setParams={setParams} />
            ))}
          </div>
        </div>
      );
    case 'table':
    case 'chart':
    case 'map':
    case 'scenario':
    case 'agent':
    case 'media_preview':
    case 'media_uploader':
    case 'object':
    default:
      return (
        <div style={card}>
          <p style={{ margin: 0, fontSize: 11, opacity: 0.6, textTransform: 'uppercase' }}>{widget.widget_type}</p>
          {widget.title && <h3 style={{ margin: '4px 0', fontSize: 14 }}>{widget.title}</h3>}
          {widget.description && <p style={{ margin: '4px 0', fontSize: 12, opacity: 0.7 }}>{widget.description}</p>}
          <pre
            style={{
              marginTop: 8,
              padding: 8,
              background: 'rgba(0,0,0,0.3)',
              borderRadius: 4,
              fontSize: 10,
              fontFamily: 'ui-monospace, monospace',
              overflow: 'auto',
              maxHeight: 160,
            }}
          >
            {JSON.stringify({ props, binding: widget.binding }, null, 2)}
          </pre>
          <p style={{ margin: '4px 0 0', fontSize: 10, opacity: 0.5 }}>
            Specialized widget runtime not yet ported to React. Configuration is read-only here.
          </p>
        </div>
      );
  }
}
