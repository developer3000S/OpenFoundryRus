import { useEffect, useMemo, useRef } from 'react';

import { EChartCanvas } from '@components/EChartCanvas';

const PALETTE = ['#0f766e', '#0369a1', '#f97316', '#be123c', '#7c3aed'];

function toNumber(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const numeric = Number(value.replace(/,/g, ''));
    return Number.isFinite(numeric) ? numeric : 0;
  }
  return 0;
}

interface EChartViewProps {
  rows: Array<Record<string, unknown>>;
  categoryKey: string;
  valueKeys: string[];
  mode?: 'bar' | 'line' | 'area' | 'pie';
  emptyLabel?: string;
  onCategoryClick?: (value: string) => void;
}

export function EChartView({
  rows,
  categoryKey,
  valueKeys,
  mode = 'bar',
  emptyLabel = 'No data available for this view.',
  onCategoryClick,
}: EChartViewProps) {
  const onCategoryClickRef = useRef(onCategoryClick);
  useEffect(() => {
    onCategoryClickRef.current = onCategoryClick;
  }, [onCategoryClick]);

  const options = useMemo(() => {
    if (rows.length === 0 || !categoryKey || valueKeys.length === 0) return null;

    const categories = rows.map((row, index) => String(row[categoryKey] ?? `Row ${index + 1}`));

    if (mode === 'pie') {
      const valueKey = valueKeys[0];
      return {
        color: PALETTE,
        tooltip: { trigger: 'item' },
        legend: { bottom: 0, textStyle: { color: '#64748b' } },
        series: [
          {
            type: 'pie',
            radius: ['36%', '72%'],
            data: rows.map((row, index) => ({
              name: String(row[categoryKey] ?? `Row ${index + 1}`),
              value: toNumber(row[valueKey]),
            })),
          },
        ],
      };
    }

    return {
      color: PALETTE,
      tooltip: { trigger: 'axis' },
      legend: { top: 0, textStyle: { color: '#64748b' } },
      grid: { left: 12, right: 12, top: 32, bottom: 12, containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: mode === 'bar',
        data: categories,
        axisLabel: { color: '#64748b' },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: '#64748b' },
        splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.15)' } },
      },
      series: valueKeys.map((valueKey) => ({
        name: valueKey,
        type: mode === 'area' ? 'line' : mode,
        smooth: mode !== 'bar',
        areaStyle: mode === 'area' ? { opacity: 0.18 } : undefined,
        data: rows.map((row) => toNumber(row[valueKey])),
      })),
    };
  }, [rows, categoryKey, valueKeys, mode]);

  if (!options) {
    return (
      <div
        style={{
          display: 'flex',
          minHeight: 280,
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px dashed var(--border-default)',
          borderRadius: 'var(--radius-md)',
          color: 'var(--text-muted)',
          fontSize: 13,
        }}
      >
        {emptyLabel}
      </div>
    );
  }

  return (
    <EChartCanvas
      options={options}
      style={{ height: '100%', minHeight: 280 }}
      onReady={(chart) => {
        chart.on('click', (params) => {
          if (params?.name != null) {
            onCategoryClickRef.current?.(String(params.name));
          }
        });
      }}
    />
  );
}
