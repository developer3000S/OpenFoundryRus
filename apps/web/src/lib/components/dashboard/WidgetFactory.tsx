import { useEffect, useRef, useState } from 'react';

import { executeQuery, type QueryResult } from '@/lib/api/queries';
import {
  applyDashboardQueryTemplate,
  formatDashboardTimestamp,
  type DashboardFilterState,
  type DashboardWidget,
} from '@/lib/utils/dashboards';

import { ChartWidget } from './ChartWidget';
import { KPIWidget } from './KPIWidget';
import { TableWidget } from './TableWidget';

interface WidgetFactoryProps {
  widget: DashboardWidget;
  filters: DashboardFilterState;
}

export function WidgetFactory({ widget, filters }: WidgetFactoryProps) {
  const [result, setResult] = useState<QueryResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [lastLoadedAt, setLastLoadedAt] = useState<string | null>(null);
  const requestRef = useRef(0);

  const renderedSql = applyDashboardQueryTemplate(widget.query.sql, filters);
  const requestKey = `${widget.id}:${widget.query.limit}:${renderedSql}`;

  async function loadData() {
    requestRef.current += 1;
    const requestId = requestRef.current;
    setLoading(true);
    setError('');

    try {
      const next = await executeQuery(renderedSql, widget.query.limit);
      if (requestId !== requestRef.current) return;
      setResult(next);
      setLastLoadedAt(new Date().toISOString());
    } catch (err) {
      if (requestId !== requestRef.current) return;
      setResult(null);
      setError(err instanceof Error ? err.message : 'Widget query failed');
    } finally {
      if (requestId === requestRef.current) setLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [requestKey]);

  return (
    <article
      style={{
        display: 'flex',
        height: '100%',
        minHeight: 220,
        flexDirection: 'column',
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        boxShadow: 'var(--shadow-panel)',
        padding: 16,
      }}
    >
      <header style={{ marginBottom: 16, display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <h3 className="of-heading-sm" style={{ fontSize: 16 }}>
              {widget.title}
            </h3>
            <span
              className="of-chip"
              style={{ fontSize: 10, letterSpacing: '0.2em', textTransform: 'uppercase' }}
            >
              {widget.type}
            </span>
          </div>
          <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
            {widget.description}
          </p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {lastLoadedAt && (
            <span className="of-text-soft" style={{ fontSize: 11 }}>
              {formatDashboardTimestamp(lastLoadedAt)}
            </span>
          )}
          <button
            type="button"
            className="of-btn"
            onClick={() => void loadData()}
            disabled={loading}
            style={{ minHeight: 30, fontSize: 12 }}
          >
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ marginBottom: 12, padding: '8px 12px', borderRadius: 'var(--radius-sm)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ minHeight: 0, flex: 1 }}>
        {widget.type === 'chart' ? (
          <ChartWidget widget={widget} result={result} />
        ) : widget.type === 'table' ? (
          <TableWidget widget={widget} result={result} globalSearch={filters.search} />
        ) : (
          <KPIWidget widget={widget} result={result} />
        )}
      </div>
    </article>
  );
}
