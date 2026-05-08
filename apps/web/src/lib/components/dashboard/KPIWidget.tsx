import { useMemo } from 'react';

import { EChartCanvas } from '@components/EChartCanvas';
import type { QueryResult } from '@/lib/api/queries';
import {
  formatMetricValue,
  parseSparklineSeries,
  toNumber,
  type DashboardKpiWidget,
} from '@/lib/utils/dashboards';

interface KPIWidgetProps {
  widget: DashboardKpiWidget;
  result: QueryResult | null;
}

export function KPIWidget({ widget, result }: KPIWidgetProps) {
  const firstRow = result?.rows[0] ?? null;
  const columns = result?.columns ?? [];

  const columnIndex = (name: string) => columns.findIndex((column) => column.name === name);

  const value = firstRow ? firstRow[columnIndex(widget.valueColumn)] : null;
  const delta = firstRow ? toNumber(firstRow[columnIndex(widget.deltaColumn)]) : null;
  const sparkline = firstRow ? parseSparklineSeries(firstRow[columnIndex(widget.sparklineColumn)]) : [];

  const sparklineOptions = useMemo(() => {
    if (sparkline.length === 0) return null;
    return {
      animation: false,
      grid: { left: 0, right: 0, top: 0, bottom: 0 },
      xAxis: { type: 'category', show: false, data: sparkline.map((_, index) => index) },
      yAxis: { type: 'value', show: false },
      series: [
        {
          type: 'line',
          smooth: true,
          data: sparkline,
          showSymbol: false,
          lineStyle: { color: '#2d72d2', width: 2 },
          areaStyle: { color: 'rgba(45, 114, 210, 0.12)' },
        },
      ],
    };
  }, [sparkline]);

  return (
    <div
      style={{
        display: 'flex',
        height: '100%',
        minHeight: 170,
        flexDirection: 'column',
        justifyContent: 'space-between',
        gap: 14,
        background: '#fff',
        padding: 0,
        borderRadius: 'var(--radius-md)',
      }}
    >
      <div>
        <div className="of-eyebrow">Current Value</div>
        <div
          style={{
            marginTop: 8,
            fontSize: 30,
            fontWeight: 600,
            color: 'var(--text-strong)',
            letterSpacing: 0,
          }}
        >
          {formatMetricValue(value, widget.valueFormat)}
        </div>
        {delta !== null && (
          <div
            className={`of-chip ${delta >= 0 ? 'of-status-success' : 'of-status-danger'}`}
            style={{ marginTop: 8 }}
          >
            <span>{delta >= 0 ? '▲' : '▼'}</span>
            <span>{Math.abs(delta).toFixed(1)}%</span>
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 8 }}>
        <div className="of-eyebrow">Trend</div>
        {sparklineOptions ? (
          <EChartCanvas options={sparklineOptions} style={{ height: 72 }} />
        ) : (
          <div
            style={{
              height: 72,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 12,
              color: 'var(--text-soft)',
              border: '1px dashed var(--border-default)',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            No sparkline data
          </div>
        )}
      </div>
    </div>
  );
}
