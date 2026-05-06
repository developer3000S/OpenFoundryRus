import { useMemo } from 'react';

import { EChartCanvas } from '@components/EChartCanvas';
import type { QueryResult } from '@/lib/api/queries';
import { toNumber, type DashboardChartWidget } from '@/lib/utils/dashboards';

const PALETTE = ['#0f766e', '#0369a1', '#c2410c', '#7c3aed', '#be123c'];

function buildOptions(widget: DashboardChartWidget, result: QueryResult | null): unknown {
  if (!result || result.rows.length === 0) return null;

  const columnIndex = (name: string) => result.columns.findIndex((column) => column.name === name);
  const numericColumns = result.columns
    .filter((_, index) => result.rows.some((row) => toNumber(row[index]) !== null))
    .map((column) => column.name);

  const categoryColumn =
    widget.categoryColumn ||
    result.columns.find((column) => !numericColumns.includes(column.name))?.name ||
    result.columns[0]?.name;
  const categoryIdx = columnIndex(categoryColumn);
  const seriesColumns =
    widget.seriesColumns.length > 0
      ? widget.seriesColumns
      : numericColumns.filter((column) => column !== categoryColumn);

  if (widget.chartType === 'pie') {
    const valueColumn = seriesColumns[0] ?? numericColumns[0];
    const valueIdx = columnIndex(valueColumn);
    return {
      color: PALETTE,
      tooltip: { trigger: 'item' },
      legend: { bottom: 0, textStyle: { color: '#64748b' } },
      series: [
        {
          type: 'pie',
          radius: ['36%', '70%'],
          avoidLabelOverlap: true,
          data: result.rows.map((row) => ({
            name: categoryIdx >= 0 ? row[categoryIdx] : valueColumn,
            value: toNumber(row[valueIdx]) ?? 0,
          })),
        },
      ],
    };
  }

  if (widget.chartType === 'scatter') {
    const [xColumn, yColumn] = seriesColumns.slice(0, 2);
    const xIdx = columnIndex(xColumn ?? numericColumns[0]);
    const yIdx = columnIndex(yColumn ?? numericColumns[1] ?? numericColumns[0]);
    return {
      color: PALETTE,
      tooltip: { trigger: 'item' },
      xAxis: { type: 'value', axisLabel: { color: '#64748b' } },
      yAxis: { type: 'value', axisLabel: { color: '#64748b' } },
      series: [
        {
          type: 'scatter',
          symbolSize: 14,
          data: result.rows.map((row) => [toNumber(row[xIdx]) ?? 0, toNumber(row[yIdx]) ?? 0]),
        },
      ],
    };
  }

  const categories =
    categoryIdx >= 0
      ? result.rows.map((row) => row[categoryIdx])
      : result.rows.map((_, index) => `${index + 1}`);

  return {
    color: PALETTE,
    tooltip: { trigger: 'axis' },
    legend: { top: 0, textStyle: { color: '#64748b' } },
    grid: { left: 12, right: 12, top: 32, bottom: 12, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: widget.chartType === 'bar',
      data: categories,
      axisLabel: { color: '#64748b' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#64748b' },
      splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.15)' } },
    },
    series: seriesColumns.map((seriesColumn) => {
      const idx = columnIndex(seriesColumn);
      const type = widget.chartType === 'area' ? 'line' : widget.chartType;
      return {
        name: seriesColumn,
        type,
        stack: widget.stacked ? 'total' : undefined,
        smooth: type === 'line',
        areaStyle: widget.chartType === 'area' ? { opacity: 0.18 } : undefined,
        emphasis: { focus: 'series' },
        data: result.rows.map((row) => toNumber(row[idx]) ?? 0),
      };
    }),
  };
}

interface ChartWidgetProps {
  widget: DashboardChartWidget;
  result: QueryResult | null;
}

export function ChartWidget({ widget, result }: ChartWidgetProps) {
  const options = useMemo(() => buildOptions(widget, result), [widget, result]);

  if (!options) {
    return (
      <div
        style={{
          display: 'flex',
          height: '100%',
          minHeight: 240,
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px dashed var(--border-default)',
          borderRadius: 'var(--radius-md)',
          color: 'var(--text-muted)',
          fontSize: 13,
        }}
      >
        Run the widget query to render chart data.
      </div>
    );
  }

  return <EChartCanvas options={options} style={{ height: '100%', minHeight: 240 }} />;
}
