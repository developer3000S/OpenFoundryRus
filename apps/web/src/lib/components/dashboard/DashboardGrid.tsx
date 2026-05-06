import { useState } from 'react';

import {
  type DashboardFilterState,
  type DashboardWidget,
  type DashboardWidgetLayout,
} from '@/lib/utils/dashboards';

import { WidgetFactory } from './WidgetFactory';

interface DashboardGridProps {
  widgets: DashboardWidget[];
  filters: DashboardFilterState;
  editing?: boolean;
  onReorder?: (fromIndex: number, toIndex: number) => void;
  onEditWidget?: (widget: DashboardWidget) => void;
  onDeleteWidget?: (widgetId: string) => void;
  onDuplicateWidget?: (widgetId: string) => void;
  onResizeWidget?: (widgetId: string, layout: DashboardWidgetLayout) => void;
}

export function DashboardGrid({
  widgets,
  filters,
  editing = false,
  onReorder,
  onEditWidget,
  onDeleteWidget,
  onDuplicateWidget,
  onResizeWidget,
}: DashboardGridProps) {
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [overIndex, setOverIndex] = useState<number | null>(null);

  function handleDragStart(index: number) {
    setDragIndex(index);
    setOverIndex(index);
  }

  function handleDrop(e: React.DragEvent, targetIndex: number) {
    e.preventDefault();
    if (dragIndex === null || dragIndex === targetIndex) {
      setDragIndex(null);
      setOverIndex(null);
      return;
    }
    onReorder?.(dragIndex, targetIndex);
    setDragIndex(null);
    setOverIndex(null);
  }

  function updateLayout(widget: DashboardWidget, nextLayout: DashboardWidgetLayout) {
    onResizeWidget?.(widget.id, nextLayout);
  }

  if (widgets.length === 0) {
    return (
      <div
        style={{
          display: 'flex',
          minHeight: 280,
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px dashed var(--border-default)',
          borderRadius: 'var(--radius-md)',
          background: 'var(--bg-panel-muted)',
          fontSize: 13,
          color: 'var(--text-muted)',
        }}
      >
        Add your first widget to start composing a dashboard.
      </div>
    );
  }

  return (
    <div className="dashboard-grid">
      {widgets.map((widget, index) => {
        const colSpan = Math.min(Math.max(widget.layout.colSpan, 1), 12);
        const colSpanMd = Math.min(Math.max(widget.layout.colSpan, 1), 6);
        const rowSpan = Math.min(Math.max(widget.layout.rowSpan, 1), 4);
        return (
          <div
            key={widget.id}
            className={`dashboard-grid__item ${editing ? 'dashboard-grid__item--editing' : ''} ${
              overIndex === index ? 'dashboard-grid__item--drop' : ''
            }`}
            style={
              {
                '--col-span': colSpan,
                '--col-span-md': colSpanMd,
                '--col-span-sm': 1,
                '--row-span': rowSpan,
              } as React.CSSProperties
            }
            role="group"
            draggable={editing}
            onDragStart={() => handleDragStart(index)}
            onDragOver={(e) => {
              if (editing) {
                e.preventDefault();
                setOverIndex(index);
              }
            }}
            onDragLeave={() => {
              if (overIndex === index) setOverIndex(null);
            }}
            onDragEnd={() => {
              setDragIndex(null);
              setOverIndex(null);
            }}
            onDrop={(e) => handleDrop(e, index)}
          >
            {editing && (
              <>
                <div className="dashboard-grid__toolbar">
                  <span className="dashboard-grid__drag">Drag</span>
                  <div className="dashboard-grid__toolbar-actions">
                    <button type="button" title="Edit widget" onClick={() => onEditWidget?.(widget)}>
                      Edit
                    </button>
                    <button type="button" title="Duplicate widget" onClick={() => onDuplicateWidget?.(widget.id)}>
                      Copy
                    </button>
                    <button type="button" title="Delete widget" onClick={() => onDeleteWidget?.(widget.id)}>
                      Delete
                    </button>
                  </div>
                </div>
                <div className="dashboard-grid__resize">
                  <button
                    type="button"
                    title="Narrower"
                    onClick={() => updateLayout(widget, { ...widget.layout, colSpan: Math.max(1, widget.layout.colSpan - 1) })}
                  >
                    W-
                  </button>
                  <button
                    type="button"
                    title="Wider"
                    onClick={() => updateLayout(widget, { ...widget.layout, colSpan: Math.min(12, widget.layout.colSpan + 1) })}
                  >
                    W+
                  </button>
                  <button
                    type="button"
                    title="Shorter"
                    onClick={() => updateLayout(widget, { ...widget.layout, rowSpan: Math.max(1, widget.layout.rowSpan - 1) })}
                  >
                    H-
                  </button>
                  <button
                    type="button"
                    title="Taller"
                    onClick={() => updateLayout(widget, { ...widget.layout, rowSpan: Math.min(4, widget.layout.rowSpan + 1) })}
                  >
                    H+
                  </button>
                </div>
              </>
            )}

            <WidgetFactory widget={widget} filters={filters} />
          </div>
        );
      })}
    </div>
  );
}
