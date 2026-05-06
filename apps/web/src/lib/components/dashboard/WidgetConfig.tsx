import { useEffect, useState } from 'react';

import {
  cloneDashboard,
  createWidget,
  type DashboardWidget,
  type DashboardWidgetType,
} from '@/lib/utils/dashboards';

interface WidgetConfigProps {
  open: boolean;
  initialWidget: DashboardWidget | null;
  onSave?: (widget: DashboardWidget) => void;
  onClose?: () => void;
}

export function WidgetConfig({ open, initialWidget, onSave, onClose }: WidgetConfigProps) {
  const [draft, setDraft] = useState<DashboardWidget | null>(null);
  const [seriesColumnsInput, setSeriesColumnsInput] = useState('');

  useEffect(() => {
    const next = initialWidget ? cloneDashboard(initialWidget) : null;
    setDraft(next);
    setSeriesColumnsInput(next && next.type === 'chart' ? next.seriesColumns.join(', ') : '');
  }, [initialWidget]);

  if (!open || !draft) return null;

  function patchDraft(patch: Partial<DashboardWidget>) {
    setDraft((current) => (current ? ({ ...current, ...patch } as DashboardWidget) : current));
  }

  function patchLayout(patch: Partial<DashboardWidget['layout']>) {
    setDraft((current) =>
      current ? ({ ...current, layout: { ...current.layout, ...patch } } as DashboardWidget) : current,
    );
  }

  function patchQuery(patch: Partial<DashboardWidget['query']>) {
    setDraft((current) =>
      current ? ({ ...current, query: { ...current.query, ...patch } } as DashboardWidget) : current,
    );
  }

  function switchType(type: DashboardWidgetType) {
    if (!draft || draft.type === type) return;
    const template = createWidget(type);
    const nextDraft = {
      ...template,
      id: draft.id,
      title: draft.title,
      description: draft.description,
      query: draft.query,
      layout: draft.layout,
    } as DashboardWidget;
    setDraft(nextDraft);
    setSeriesColumnsInput(nextDraft.type === 'chart' ? nextDraft.seriesColumns.join(', ') : '');
  }

  function save() {
    if (!draft) return;
    let final = cloneDashboard(draft);
    if (final.type === 'chart') {
      final.seriesColumns = seriesColumnsInput
        .split(',')
        .map((value) => value.trim())
        .filter(Boolean);
    }
    onSave?.(final);
    onClose?.();
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 50,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'rgba(15, 23, 42, 0.6)',
        padding: 16,
      }}
    >
      <div
        className="of-panel"
        style={{ width: '100%', maxWidth: 720, maxHeight: '90vh', overflow: 'auto', padding: 24 }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, marginBottom: 24 }}>
          <div>
            <p className="of-eyebrow">Widget editor</p>
            <h2 className="of-heading-lg" style={{ marginTop: 4 }}>
              Configure widget
            </h2>
          </div>
          <button type="button" className="of-btn" onClick={() => onClose?.()}>
            Close
          </button>
        </div>

        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
          <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
            <span style={{ display: 'block', marginBottom: 6 }}>Title</span>
            <input
              type="text"
              className="of-input"
              value={draft.title}
              onChange={(e) => patchDraft({ title: e.target.value })}
            />
          </label>

          <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
            <span style={{ display: 'block', marginBottom: 6 }}>Widget type</span>
            <select
              className="of-select"
              value={draft.type}
              onChange={(e) => switchType(e.target.value as DashboardWidgetType)}
            >
              <option value="chart">Chart</option>
              <option value="table">Table</option>
              <option value="kpi">KPI</option>
            </select>
          </label>
        </div>

        <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginTop: 16 }}>
          <span style={{ display: 'block', marginBottom: 6 }}>Description</span>
          <textarea
            className="of-textarea"
            rows={2}
            value={draft.description}
            onChange={(e) => patchDraft({ description: e.target.value })}
          />
        </label>

        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr 2fr', marginTop: 16 }}>
          <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
            <span style={{ display: 'block', marginBottom: 6 }}>Columns</span>
            <input
              type="number"
              className="of-input"
              min={1}
              max={12}
              value={draft.layout.colSpan}
              onChange={(e) => patchLayout({ colSpan: Number(e.target.value) })}
            />
          </label>
          <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
            <span style={{ display: 'block', marginBottom: 6 }}>Rows</span>
            <input
              type="number"
              className="of-input"
              min={1}
              max={4}
              value={draft.layout.rowSpan}
              onChange={(e) => patchLayout({ rowSpan: Number(e.target.value) })}
            />
          </label>
          <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
            <span style={{ display: 'block', marginBottom: 6 }}>Query limit</span>
            <input
              type="number"
              className="of-input"
              min={1}
              max={1000}
              value={draft.query.limit}
              onChange={(e) => patchQuery({ limit: Number(e.target.value) })}
            />
          </label>
        </div>

        <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginTop: 16 }}>
          <span style={{ display: 'block', marginBottom: 6 }}>SQL query</span>
          <textarea
            className="of-textarea"
            rows={8}
            value={draft.query.sql}
            onChange={(e) => patchQuery({ sql: e.target.value })}
            style={{ fontFamily: 'var(--font-mono)', background: '#0f172a', color: '#e2e8f0' }}
          />
        </label>

        <div
          style={{
            marginTop: 12,
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
            padding: '8px 12px',
            fontSize: 12,
            color: 'var(--text-muted)',
          }}
        >
          Available placeholders: <code>{'{{search}}'}</code>, <code>{'{{date_from}}'}</code>,{' '}
          <code>{'{{date_to}}'}</code>
        </div>

        {draft.type === 'chart' && (
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 24 }}>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Chart type</span>
              <select
                className="of-select"
                value={draft.chartType}
                onChange={(e) => patchDraft({ chartType: e.target.value as typeof draft.chartType } as Partial<DashboardWidget>)}
              >
                <option value="bar">Bar</option>
                <option value="line">Line</option>
                <option value="area">Area</option>
                <option value="pie">Pie</option>
                <option value="scatter">Scatter</option>
              </select>
            </label>

            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Category column</span>
              <input
                type="text"
                className="of-input"
                value={draft.categoryColumn}
                onChange={(e) => patchDraft({ categoryColumn: e.target.value } as Partial<DashboardWidget>)}
              />
            </label>

            <label style={{ display: 'block', fontSize: 13, fontWeight: 500, gridColumn: '1 / -1' }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Series columns</span>
              <input
                type="text"
                className="of-input"
                value={seriesColumnsInput}
                onChange={(e) => setSeriesColumnsInput(e.target.value)}
                placeholder="ingested, published"
              />
            </label>

            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={draft.stacked}
                onChange={(e) => patchDraft({ stacked: e.target.checked } as Partial<DashboardWidget>)}
              />
              Stack series
            </label>
          </div>
        )}

        {draft.type === 'table' && (
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)', marginTop: 24 }}>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Page size</span>
              <input
                type="number"
                className="of-input"
                min={3}
                max={50}
                value={draft.pageSize}
                onChange={(e) => patchDraft({ pageSize: Number(e.target.value) } as Partial<DashboardWidget>)}
              />
            </label>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Default sort column</span>
              <input
                type="text"
                className="of-input"
                value={draft.defaultSortColumn}
                onChange={(e) => patchDraft({ defaultSortColumn: e.target.value } as Partial<DashboardWidget>)}
              />
            </label>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Sort direction</span>
              <select
                className="of-select"
                value={draft.defaultSortDirection}
                onChange={(e) => patchDraft({ defaultSortDirection: e.target.value as 'asc' | 'desc' } as Partial<DashboardWidget>)}
              >
                <option value="asc">Ascending</option>
                <option value="desc">Descending</option>
              </select>
            </label>
          </div>
        )}

        {draft.type === 'kpi' && (
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 24 }}>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Value column</span>
              <input
                type="text"
                className="of-input"
                value={draft.valueColumn}
                onChange={(e) => patchDraft({ valueColumn: e.target.value } as Partial<DashboardWidget>)}
              />
            </label>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Delta column</span>
              <input
                type="text"
                className="of-input"
                value={draft.deltaColumn}
                onChange={(e) => patchDraft({ deltaColumn: e.target.value } as Partial<DashboardWidget>)}
              />
            </label>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Sparkline column</span>
              <input
                type="text"
                className="of-input"
                value={draft.sparklineColumn}
                onChange={(e) => patchDraft({ sparklineColumn: e.target.value } as Partial<DashboardWidget>)}
              />
            </label>
            <label style={{ display: 'block', fontSize: 13, fontWeight: 500 }}>
              <span style={{ display: 'block', marginBottom: 6 }}>Value format</span>
              <select
                className="of-select"
                value={draft.valueFormat}
                onChange={(e) => patchDraft({ valueFormat: e.target.value as typeof draft.valueFormat } as Partial<DashboardWidget>)}
              >
                <option value="number">Number</option>
                <option value="currency">Currency</option>
                <option value="percent">Percent</option>
              </select>
            </label>
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 12, marginTop: 32 }}>
          <button type="button" className="of-btn" onClick={() => onClose?.()}>
            Cancel
          </button>
          <button
            type="button"
            className="of-btn of-btn-primary"
            onClick={save}
            disabled={!draft.title.trim() || !draft.query.sql.trim()}
          >
            Save widget
          </button>
        </div>
      </div>
    </div>
  );
}
