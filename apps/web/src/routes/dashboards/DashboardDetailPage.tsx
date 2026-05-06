import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import { DashboardGrid } from '@/lib/components/dashboard/DashboardGrid';
import { FilterBar } from '@/lib/components/dashboard/FilterBar';
import { WidgetConfig } from '@/lib/components/dashboard/WidgetConfig';
import { executeQuery } from '@/lib/api/queries';
import { dashboards, useDashboards } from '@/lib/stores/dashboards';
import {
  applyDashboardQueryTemplate,
  cloneDashboard,
  createDefaultFilters,
  createWidget,
  deserializeDashboardSnapshot,
  duplicateDashboardDefinition,
  formatDashboardTimestamp,
  resolveDateRange,
  serializeDashboardSnapshot,
  type DashboardDefinition,
  type DashboardFilterState,
  type DashboardWidget,
  type DashboardWidgetLayout,
} from '@/lib/utils/dashboards';
import { buildTableLines, downloadStructuredPdf, type PdfSection } from '@/lib/utils/pdf';

export function DashboardDetailPage() {
  const { id } = useParams<{ id: string }>();
  const dashboardId = id ?? '';
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const dashboardItems = useDashboards();

  const [loading, setLoading] = useState(true);
  const [dashboard, setDashboard] = useState<DashboardDefinition | null>(null);
  const [sharedDashboard, setSharedDashboard] = useState<DashboardDefinition | null>(null);
  const [filters, setFilters] = useState<DashboardFilterState>(createDefaultFilters());
  const [editMode, setEditMode] = useState(false);
  const [widgetEditorOpen, setWidgetEditorOpen] = useState(false);
  const [editorWidget, setEditorWidget] = useState<DashboardWidget | null>(null);
  const [feedback, setFeedback] = useState('');
  const [feedbackTone, setFeedbackTone] = useState<'success' | 'error'>('success');
  const [exportingPdf, setExportingPdf] = useState(false);

  const activeDashboard = dashboard ?? sharedDashboard;
  const readOnlySnapshot = sharedDashboard !== null && dashboard === null;

  // Initial load: try local store, fall back to ?snapshot= URL param.
  useEffect(() => {
    dashboards.restore();
    const local = dashboards.getById(dashboardId);
    if (local) {
      setDashboard(cloneDashboard(local));
      setSharedDashboard(null);
      setLoading(false);
      return;
    }
    const snapshotParam = searchParams.get('snapshot');
    if (snapshotParam) {
      try {
        setSharedDashboard(cloneDashboard(deserializeDashboardSnapshot(snapshotParam)));
      } catch {
        setSharedDashboard(null);
      }
    }
    setLoading(false);
  }, [dashboardId, searchParams]);

  // Re-sync when the underlying store changes (e.g. after `dashboards.save`).
  useEffect(() => {
    if (!dashboardId) return;
    const fresh = dashboardItems.find((entry) => entry.id === dashboardId);
    if (fresh) {
      setDashboard((prev) => (prev && prev.updatedAt === fresh.updatedAt ? prev : cloneDashboard(fresh)));
    }
  }, [dashboardItems, dashboardId]);

  function persist(next: DashboardDefinition) {
    setDashboard(cloneDashboard(dashboards.save(next)));
  }

  function updateMetadata(field: 'name' | 'description', value: string) {
    setDashboard((current) => (current ? { ...current, [field]: value } : current));
  }

  function commitMetadata() {
    if (dashboard) persist(dashboard);
  }

  function reorderWidgets(fromIndex: number, toIndex: number) {
    if (!dashboard) return;
    const next = { ...dashboard, widgets: [...dashboard.widgets] };
    const [moved] = next.widgets.splice(fromIndex, 1);
    next.widgets.splice(toIndex, 0, moved);
    persist(next);
  }

  function openNewWidget() {
    if (!dashboard) return;
    setEditorWidget(createWidget('chart'));
    setWidgetEditorOpen(true);
  }

  function openWidgetEditor(widget: DashboardWidget) {
    setEditorWidget(cloneDashboard(widget));
    setWidgetEditorOpen(true);
  }

  function saveWidget(widget: DashboardWidget) {
    if (!dashboard) return;
    const existingIndex = dashboard.widgets.findIndex((entry) => entry.id === widget.id);
    const nextWidgets =
      existingIndex >= 0
        ? dashboard.widgets.map((entry, index) => (index === existingIndex ? widget : entry))
        : [...dashboard.widgets, widget];
    persist({ ...dashboard, widgets: nextWidgets });
  }

  function deleteWidget(widgetId: string) {
    if (!dashboard) return;
    if (!window.confirm('Delete this widget?')) return;
    persist({ ...dashboard, widgets: dashboard.widgets.filter((widget) => widget.id !== widgetId) });
  }

  function duplicateWidget(widgetId: string) {
    if (!dashboard) return;
    const index = dashboard.widgets.findIndex((widget) => widget.id === widgetId);
    if (index < 0) return;
    const source = dashboard.widgets[index];
    const copy = cloneDashboard({ ...source, id: crypto.randomUUID(), title: `${source.title} Copy` });
    const nextWidgets = [...dashboard.widgets];
    nextWidgets.splice(index + 1, 0, copy);
    persist({ ...dashboard, widgets: nextWidgets });
  }

  function resizeWidget(widgetId: string, layout: DashboardWidgetLayout) {
    if (!dashboard) return;
    persist({
      ...dashboard,
      widgets: dashboard.widgets.map((widget) => (widget.id === widgetId ? { ...widget, layout } : widget)),
    });
  }

  function applyFilters(nextFilters: DashboardFilterState) {
    setFilters(nextFilters);
  }

  function resetFilters() {
    setFilters(createDefaultFilters());
  }

  async function duplicateDashboard() {
    if (!activeDashboard) return;
    const duplicate = dashboard
      ? dashboards.duplicate(dashboard.id)
      : dashboards.save(duplicateDashboardDefinition(activeDashboard));
    if (!duplicate) return;
    navigate(`/dashboards/${duplicate.id}`);
  }

  async function shareDashboard() {
    if (!activeDashboard || typeof window === 'undefined') return;
    const snapshot = serializeDashboardSnapshot(activeDashboard);
    const shareUrl = `${window.location.origin}/dashboards/${activeDashboard.id}?snapshot=${snapshot}`;
    try {
      await navigator.clipboard.writeText(shareUrl);
      setFeedbackTone('success');
      setFeedback('Share link copied to clipboard.');
    } catch {
      setFeedbackTone('success');
      setFeedback(shareUrl);
    }
  }

  async function exportDashboardPdf() {
    if (!activeDashboard) return;

    setExportingPdf(true);
    setFeedback('');

    try {
      const resolvedRange = resolveDateRange(filters.dateRange);
      const widgetSections = await Promise.all(
        activeDashboard.widgets.map(async (widget, index) => {
          const renderedSql = applyDashboardQueryTemplate(widget.query.sql, filters);
          try {
            const result = await executeQuery(renderedSql, Math.min(widget.query.limit, 16));
            return {
              heading: `${index + 1}. ${widget.title}`,
              lines: [
                { text: `${widget.type.toUpperCase()} widget`, style: 'muted' },
                widget.description || 'No widget description provided.',
                `Rows: ${result.total_rows} total, ${result.rows.length} previewed.`,
                `Execution time: ${result.execution_time_ms} ms`,
                ...buildTableLines(
                  result.columns.map((column) => column.name),
                  result.rows,
                  8,
                  6,
                ),
              ],
            } satisfies PdfSection;
          } catch (cause) {
            return {
              heading: `${index + 1}. ${widget.title}`,
              lines: [
                { text: `${widget.type.toUpperCase()} widget`, style: 'muted' },
                widget.description || 'No widget description provided.',
                `Query execution failed: ${cause instanceof Error ? cause.message : 'Unknown error'}`,
              ],
            } satisfies PdfSection;
          }
        }),
      );

      downloadStructuredPdf({
        fileName: `${activeDashboard.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-dashboard.pdf`,
        title: activeDashboard.name,
        subtitle: activeDashboard.description || 'OpenFoundry dashboard PDF snapshot',
        metadata: [
          `Generated at ${new Date().toISOString()}`,
          `Filters: search="${filters.search || 'none'}", range=${resolvedRange.label}`,
          `Widgets: ${activeDashboard.widgets.length}`,
          `Mode: ${readOnlySnapshot ? 'shared snapshot' : 'editable dashboard'}`,
          typeof window === 'undefined' ? '' : `URL: ${window.location.href}`,
        ].filter(Boolean),
        sections: [
          {
            heading: 'Dashboard context',
            lines: [
              `Widget count: ${activeDashboard.widgets.length}`,
              `Search filter: ${filters.search || 'None'}`,
              `Date window: ${resolvedRange.label}`,
              `Updated at: ${formatDashboardTimestamp(activeDashboard.updatedAt)}`,
            ],
          },
          ...widgetSections,
        ],
      });

      setFeedbackTone('success');
      setFeedback('PDF snapshot downloaded.');
    } catch (cause) {
      setFeedbackTone('error');
      setFeedback(cause instanceof Error ? cause.message : 'Unable to export dashboard PDF');
    } finally {
      setExportingPdf(false);
    }
  }

  function importSnapshot() {
    if (!sharedDashboard) return;
    const imported = dashboards.save({
      ...duplicateDashboardDefinition(sharedDashboard),
      name: `${sharedDashboard.name} Imported`,
    });
    navigate(`/dashboards/${imported.id}`);
  }

  function closeWidgetEditor() {
    setEditorWidget(null);
    setWidgetEditorOpen(false);
  }

  // Memoise the widgets array passed down so child memoisation works.
  const widgets = useMemo(() => activeDashboard?.widgets ?? [], [activeDashboard]);

  if (loading) {
    return (
      <section className="of-page" style={{ display: 'flex', justifyContent: 'center', padding: 80, color: 'var(--text-muted)' }}>
        Loading dashboard…
      </section>
    );
  }

  if (!activeDashboard) {
    return (
      <section className="of-page" style={{ display: 'flex', justifyContent: 'center' }}>
        <div className="of-panel" style={{ maxWidth: 560, padding: 40, textAlign: 'center' }}>
          <h1 className="of-heading-lg">Dashboard not found</h1>
          <p className="of-text-muted" style={{ marginTop: 12 }}>
            The requested dashboard does not exist locally and no share snapshot was provided.
          </p>
          <Link
            to="/dashboards"
            className="of-btn of-btn-primary"
            style={{ display: 'inline-flex', marginTop: 24 }}
          >
            Back to dashboards
          </Link>
        </div>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720, display: 'grid', gap: 12 }}>
            <Link to="/dashboards" className="of-link" style={{ fontSize: 13 }}>
              ← Back to dashboards
            </Link>

            {dashboard ? (
              <>
                <input
                  type="text"
                  value={dashboard.name}
                  onChange={(e) => updateMetadata('name', e.target.value)}
                  onBlur={commitMetadata}
                  style={{
                    width: '100%',
                    background: 'transparent',
                    fontSize: 28,
                    fontWeight: 700,
                    letterSpacing: '-0.02em',
                    color: 'var(--text-strong)',
                    border: 0,
                    outline: 'none',
                  }}
                />
                <textarea
                  rows={2}
                  value={dashboard.description}
                  onChange={(e) => updateMetadata('description', e.target.value)}
                  onBlur={commitMetadata}
                  style={{
                    width: '100%',
                    resize: 'none',
                    background: 'transparent',
                    fontSize: 14,
                    lineHeight: 1.7,
                    color: 'var(--text-muted)',
                    border: 0,
                    outline: 'none',
                  }}
                />
              </>
            ) : (
              <div>
                <p className="of-eyebrow" style={{ color: 'var(--status-warning)' }}>
                  Shared snapshot
                </p>
                <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
                  {activeDashboard.name}
                </h1>
                <p className="of-text-muted" style={{ marginTop: 12 }}>
                  {activeDashboard.description}
                </p>
              </div>
            )}

            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              <span className="of-chip">{activeDashboard.widgets.length} widgets</span>
              <span className="of-chip">Updated {formatDashboardTimestamp(activeDashboard.updatedAt)}</span>
            </div>
          </div>

          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {dashboard ? (
              <>
                <button
                  type="button"
                  className={editMode ? 'of-btn of-btn-primary' : 'of-btn'}
                  onClick={() => setEditMode((value) => !value)}
                >
                  {editMode ? 'Done layout' : 'Edit layout'}
                </button>
                <button type="button" className="of-btn of-btn-primary" onClick={openNewWidget}>
                  Add widget
                </button>
              </>
            ) : (
              <button type="button" className="of-btn of-btn-primary" onClick={importSnapshot}>
                Save copy
              </button>
            )}
            <button type="button" className="of-btn" onClick={duplicateDashboard}>
              Duplicate
            </button>
            <button type="button" className="of-btn" onClick={exportDashboardPdf} disabled={exportingPdf}>
              {exportingPdf ? 'Exporting PDF…' : 'Export PDF'}
            </button>
            <button type="button" className="of-btn" onClick={shareDashboard}>
              Share
            </button>
          </div>
        </div>

        {readOnlySnapshot && (
          <div
            className="of-status-warning"
            style={{ marginTop: 20, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
          >
            This view comes from a snapshot link. Save a copy to persist edits locally.
          </div>
        )}

        {feedback && (
          <div
            className={feedbackTone === 'error' ? 'of-status-danger' : 'of-status-success'}
            style={{ marginTop: 20, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
          >
            {feedback}
          </div>
        )}
      </div>

      <FilterBar
        search={filters.search}
        dateRange={filters.dateRange}
        onApply={applyFilters}
        onReset={resetFilters}
      />

      <DashboardGrid
        widgets={widgets}
        filters={filters}
        editing={editMode && !readOnlySnapshot}
        onReorder={reorderWidgets}
        onEditWidget={openWidgetEditor}
        onDeleteWidget={deleteWidget}
        onDuplicateWidget={duplicateWidget}
        onResizeWidget={resizeWidget}
      />

      <WidgetConfig
        open={widgetEditorOpen}
        initialWidget={editorWidget}
        onSave={saveWidget}
        onClose={closeWidgetEditor}
      />
    </section>
  );
}
