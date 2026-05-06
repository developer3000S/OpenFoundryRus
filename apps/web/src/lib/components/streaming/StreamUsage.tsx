import { useEffect, useMemo, useRef, useState } from 'react';

import { getStreamUsage, type UsageResponse } from '@/lib/api/streaming';

interface Props {
  streamId: string;
  costFactor?: number;
}

const DEFAULT_COST = Number((import.meta as { env?: Record<string, string> }).env?.VITE_COMPUTE_SECONDS_TO_COST_FACTOR ?? '0.0001');

function rangeFromLookback(lb: '24h' | '7d' | '30d'): { from: string; to: string } {
  const to = new Date();
  const from = new Date(to);
  if (lb === '24h') from.setHours(from.getHours() - 24);
  if (lb === '7d') from.setDate(from.getDate() - 7);
  if (lb === '30d') from.setDate(from.getDate() - 30);
  return { from: from.toISOString(), to: to.toISOString() };
}

export function StreamUsage({ streamId, costFactor = DEFAULT_COST }: Props) {
  const [usage, setUsage] = useState<UsageResponse | null>(null);
  const [group, setGroup] = useState<'hour' | 'day'>('hour');
  const [lookback, setLookback] = useState<'24h' | '7d' | '30d'>('24h');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const computeRef = useRef<HTMLDivElement | null>(null);
  const recordsRef = useRef<HTMLDivElement | null>(null);
  const computeChart = useRef<{ setOption: (o: unknown) => void; dispose: () => void } | null>(null);
  const recordsChart = useRef<{ setOption: (o: unknown) => void; dispose: () => void } | null>(null);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const range = rangeFromLookback(lookback);
      const next = await getStreamUsage(streamId, { ...range, group });
      setUsage(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    return () => { computeChart.current?.dispose(); computeChart.current = null; recordsChart.current?.dispose(); recordsChart.current = null; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamId, lookback, group]);

  useEffect(() => {
    if (!usage) return;
    let cancelled = false;
    (async () => {
      const echarts = await import('echarts');
      if (cancelled) return;
      if (!computeChart.current && computeRef.current) {
        computeChart.current = echarts.init(computeRef.current, undefined, { renderer: 'canvas' }) as unknown as { setOption: (o: unknown) => void; dispose: () => void };
      }
      if (!recordsChart.current && recordsRef.current) {
        recordsChart.current = echarts.init(recordsRef.current, undefined, { renderer: 'canvas' }) as unknown as { setOption: (o: unknown) => void; dispose: () => void };
      }
      const labels = usage.buckets.map((b) => new Date(b.bucket_start).toLocaleString());
      computeChart.current?.setOption({
        tooltip: { trigger: 'axis' },
        grid: { top: 30, left: 60, right: 30, bottom: 30 },
        xAxis: { type: 'category', data: labels },
        yAxis: { type: 'value', name: 'compute s' },
        series: [{ type: 'line', smooth: true, name: `Compute s / ${group}`, data: usage.buckets.map((b) => b.compute_seconds) }],
      });
      recordsChart.current?.setOption({
        tooltip: { trigger: 'axis' },
        grid: { top: 30, left: 60, right: 30, bottom: 30 },
        xAxis: { type: 'category', data: labels },
        yAxis: { type: 'value', name: 'records' },
        series: [{ type: 'bar', name: `Records / ${group}`, data: usage.buckets.map((b) => b.records_processed) }],
      });
    })();
    return () => { cancelled = true; };
  }, [usage, group]);

  const estimatedCost = useMemo(() => (usage ? usage.total_compute_seconds * costFactor : 0), [usage, costFactor]);

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <h3 style={{ margin: 0 }}>Compute usage</h3>
        <div style={{ display: 'flex', gap: 8 }}>
          <select value={lookback} onChange={(e) => setLookback(e.target.value as '24h' | '7d' | '30d')} className="of-input">
            <option value="24h">Last 24 h</option>
            <option value="7d">Last 7 d</option>
            <option value="30d">Last 30 d</option>
          </select>
          <select value={group} onChange={(e) => setGroup(e.target.value as 'hour' | 'day')} className="of-input">
            <option value="hour">Group: hour</option>
            <option value="day">Group: day</option>
          </select>
        </div>
      </header>

      <div style={{ padding: '8px 12px', borderRadius: 4, background: '#eef5ff', borderLeft: '4px solid #2a6df0', color: '#1a3a8a', fontSize: 13 }} role="note">
        <strong>Approximated cost</strong> — this is a projection based on a <code>compute_seconds_to_cost_factor</code> of {costFactor.toFixed(6)} USD/s. Actual billing may differ.
      </div>

      {loading && <p>Loading…</p>}
      {error && <p style={{ color: '#b00' }}>{error}</p>}

      {usage && (
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <tbody>
            <tr><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Window</th><td style={{ padding: '6px 10px', borderBottom: '1px solid #eee' }}>{lookback}</td></tr>
            <tr><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Total compute seconds</th><td style={{ padding: '6px 10px', borderBottom: '1px solid #eee' }}>{usage.total_compute_seconds.toFixed(2)}</td></tr>
            <tr><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Total records processed</th><td style={{ padding: '6px 10px', borderBottom: '1px solid #eee' }}>{usage.total_records_processed.toLocaleString()}</td></tr>
            <tr><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Estimated cost</th><td style={{ padding: '6px 10px', borderBottom: '1px solid #eee' }}>${estimatedCost.toFixed(4)} (USD)</td></tr>
          </tbody>
        </table>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        <div ref={computeRef} style={{ height: 220, minWidth: 0 }} />
        <div ref={recordsRef} style={{ height: 220, minWidth: 0 }} />
      </div>
    </section>
  );
}
