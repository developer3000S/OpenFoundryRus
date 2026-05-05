import { useMemo, useState } from 'react';

import { EChartCanvas } from '@components/EChartCanvas';

type ChartType = 'line' | 'bar' | 'area' | 'pie' | 'scatter';

const CATEGORIES = ['Q1', 'Q2', 'Q3', 'Q4', 'Q5', 'Q6'];
const SERIES_A = [120, 132, 101, 134, 90, 230];
const SERIES_B = [220, 182, 191, 234, 290, 330];
const PALETTE = ['#0f766e', '#0369a1', '#c2410c', '#7c3aed', '#be123c'];

function buildOptions(chartType: ChartType, stacked: boolean): unknown {
  if (chartType === 'pie') {
    return {
      color: PALETTE,
      tooltip: { trigger: 'item' },
      legend: { bottom: 0, textStyle: { color: '#64748b' } },
      series: [
        {
          type: 'pie',
          radius: ['36%', '70%'],
          avoidLabelOverlap: true,
          data: CATEGORIES.map((name, i) => ({ name, value: SERIES_A[i] })),
        },
      ],
    };
  }

  if (chartType === 'scatter') {
    return {
      color: PALETTE,
      tooltip: { trigger: 'item' },
      xAxis: { type: 'value', axisLabel: { color: '#64748b' } },
      yAxis: { type: 'value', axisLabel: { color: '#64748b' } },
      series: [
        {
          type: 'scatter',
          symbolSize: 14,
          data: SERIES_A.map((x, i) => [x, SERIES_B[i]]),
        },
      ],
    };
  }

  const echartsType = chartType === 'area' ? 'line' : chartType;

  return {
    color: PALETTE,
    tooltip: { trigger: 'axis' },
    legend: { top: 0, textStyle: { color: '#64748b' } },
    grid: { left: 12, right: 12, top: 32, bottom: 12, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: chartType === 'bar',
      data: CATEGORIES,
      axisLabel: { color: '#64748b' },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#64748b' },
      splitLine: { lineStyle: { color: 'rgba(148, 163, 184, 0.15)' } },
    },
    series: [
      {
        name: 'Series A',
        type: echartsType,
        stack: stacked ? 'total' : undefined,
        smooth: echartsType === 'line',
        areaStyle: chartType === 'area' ? { opacity: 0.18 } : undefined,
        emphasis: { focus: 'series' },
        data: SERIES_A,
      },
      {
        name: 'Series B',
        type: echartsType,
        stack: stacked ? 'total' : undefined,
        smooth: echartsType === 'line',
        areaStyle: chartType === 'area' ? { opacity: 0.18 } : undefined,
        emphasis: { focus: 'series' },
        data: SERIES_B,
      },
    ],
  };
}

export function ChartsDemoPage() {
  const [chartType, setChartType] = useState<ChartType>('line');
  const [stacked, setStacked] = useState(false);

  const options = useMemo(() => buildOptions(chartType, stacked), [chartType, stacked]);
  const stackedSupported = chartType !== 'pie' && chartType !== 'scatter';

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Capability validator</p>
        <h1 className="of-heading-xl">ECharts wrapper demo</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 720 }}>
          Validates the <code>&lt;EChartCanvas&gt;</code> wrapper: lazy <code>echarts</code> import,
          init on mount, reactive <code>setOption</code> on prop change, container resize via
          ResizeObserver, and dispose on unmount. This is the same pattern that the full{' '}
          <code>/dashboards</code> migration will reuse for every chart widget.
        </p>
      </header>

      <div className="of-panel" style={{ padding: 20 }}>
        <div className="of-toolbar" style={{ marginBottom: 16 }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
            Chart type
            <select
              className="of-select"
              value={chartType}
              onChange={(e) => setChartType(e.target.value as ChartType)}
              style={{ minWidth: 140 }}
            >
              <option value="line">Line</option>
              <option value="bar">Bar</option>
              <option value="area">Area</option>
              <option value="pie">Pie</option>
              <option value="scatter">Scatter</option>
            </select>
          </label>
          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              fontSize: 13,
              opacity: stackedSupported ? 1 : 0.5,
            }}
          >
            <input
              type="checkbox"
              checked={stackedSupported && stacked}
              onChange={(e) => setStacked(e.target.checked)}
              disabled={!stackedSupported}
            />
            Stacked
          </label>
        </div>

        <EChartCanvas options={options} style={{ height: 360 }} />
      </div>
    </section>
  );
}
