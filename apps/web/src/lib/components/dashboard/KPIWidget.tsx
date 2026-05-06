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
          lineStyle: { color: '#0f766e', width: 3 },
          areaStyle: { color: 'rgba(15, 118, 110, 0.18)' },
        },
      ],
    };
  }, [sparkline]);

  return (
    <div
      style={{
        display: 'flex',
        height: '100%',
        minHeight: 200,
        flexDirection: 'column',
        justifyContent: 'space-between',
        gap: 24,
        background: 'radial-gradient(circle at top left, rgba(15, 118, 110, 0.16), transparent 55%)',
        padding: 4,
        borderRadius: 'var(--radius-md)',
      }}
    >
      <div>
        <div className="of-eyebrow">Current Value</div>
        <div
          style={{
            marginTop: 12,
            fontSize: 36,
            fontWeight: 600,
            color: 'var(--text-strong)',
            letterSpacing: '-0.02em',
          }}
        >
          {formatMetricValue(value, widget.valueFormat)}
        </div>
        {delta !== null && (
          <div
            className={`of-chip ${delta >= 0 ? 'of-status-success' : 'of-status-danger'}`}
            style={{ marginTop: 12 }}
          >
            <span>{delta >= 0 ? '▲' : '▼'}</span>
            <span>{Math.abs(delta).toFixed(1)}%</span>
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 8 }}>
        <div className="of-eyebrow">Sparkline</div>
        {sparklineOptions ? (
          <EChartCanvas options={sparklineOptions} style={{ height: 80 }} />
        ) : (
          <div
            style={{
              height: 80,
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
