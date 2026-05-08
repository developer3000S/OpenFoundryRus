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
        minHeight: 190,
        flexDirection: 'column',
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        boxShadow: 'var(--shadow-panel)',
        padding: 10,
      }}
    >
      <header style={{ marginBottom: 8, display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <h3 className="of-heading-sm" style={{ fontSize: 14 }}>
              {widget.title}
            </h3>
            <span
              className="of-chip"
              style={{ fontSize: 10, letterSpacing: 0, textTransform: 'uppercase' }}
            >
              {widget.type}
            </span>
          </div>
          <p className="of-text-muted" style={{ fontSize: 12, marginTop: 3 }}>
            {widget.description}
          </p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
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
            style={{ minHeight: 26, fontSize: 11 }}
          >
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ marginBottom: 8, padding: '7px 8px', borderRadius: 'var(--radius-sm)', fontSize: 12 }}>
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
