import { useEffect, useRef, useState } from 'react';

import { listEvaluationsForRule, listMonitorRules, type MonitorEvaluation, type MonitorRule } from '@/lib/api/monitoring';
import { getStreamMetrics, listCheckpoints, type Checkpoint, type StreamMetricsResponse, type TopologyDefinition } from '@/lib/api/streaming';

import { JobGraph } from './JobGraph';

interface Props {
  streamId: string;
  topology?: TopologyDefinition | null;
}

const PERF_HISTORY_MAX = 60;

interface PerfHistory {
  t: number[];
  ingested: number[];
  output: number[];
  lag: number[];
  utilization: number[];
}

function checkpointStatusClass(status: string): string {
  const upper = status.toUpperCase();
  if (upper === 'COMPLETED' || upper === 'SUCCESS' || upper === 'OK') return 'ok';
  if (upper === 'FAILED' || upper === 'TIMED_OUT') return 'fail';
  return 'pending';
}

const BADGE_STYLES: Record<string, React.CSSProperties> = {
  ok: { background: '#d4f4dd', color: '#1f5631' },
  fail: { background: '#fde7e9', color: '#720010', fontWeight: 600 },
  pending: { background: '#fff3cd', color: '#5b4500' },
};

export function JobDetails({ streamId, topology = null }: Props) {
  const [perfWindow, setPerfWindow] = useState<'5m' | '30m' | string>('5m');
  const [metrics, setMetrics] = useState<StreamMetricsResponse | null>(null);
  const [checkpoints, setCheckpoints] = useState<Checkpoint[]>([]);
  const [monitors, setMonitors] = useState<Array<{ rule: MonitorRule; firing: boolean }>>([]);
  const perfRef = useRef<HTMLDivElement | null>(null);
  const perfChart = useRef<{ setOption: (o: unknown) => void; dispose: () => void } | null>(null);
  const historyRef = useRef<PerfHistory>({ t: [], ingested: [], output: [], lag: [], utilization: [] });

  async function refreshMetrics() {
    try {
      const next = await getStreamMetrics(streamId, perfWindow);
      setMetrics(next);
      const h = historyRef.current;
      h.t.push(Date.now());
      h.ingested.push(next.records_ingested);
      h.output.push(next.records_output);
      h.lag.push(next.total_lag);
      h.utilization.push(next.utilization_pct * 100);
      if (h.t.length > PERF_HISTORY_MAX) {
        h.t.shift(); h.ingested.shift(); h.output.shift(); h.lag.shift(); h.utilization.shift();
      }
      void renderChart();
    } catch (err) {
      console.warn('refreshMetrics failed', err);
    }
  }

  async function refreshCheckpoints() {
    if (!topology) return;
    try {
      const res = await listCheckpoints(topology.id, 20);
      setCheckpoints(res.data);
    } catch (err) {
      console.warn('refreshCheckpoints failed', err);
    }
  }

  async function refreshMonitors() {
    try {
      const list = await listMonitorRules({ resource_rid: streamId, resource_type: 'STREAMING_DATASET' });
      const enriched = await Promise.all(list.data.map(async (rule) => {
        try {
          const evals = await listEvaluationsForRule(rule.id, 1);
          const last: MonitorEvaluation | undefined = evals.data[0];
          return { rule, firing: !!last?.fired };
        } catch {
          return { rule, firing: false };
        }
      }));
      setMonitors(enriched);
    } catch (err) {
      console.warn('refreshMonitors failed', err);
    }
  }

  async function renderChart() {
    if (!perfRef.current) return;
    if (!perfChart.current) {
      const echarts = await import('echarts');
      perfChart.current = echarts.init(perfRef.current, undefined, { renderer: 'canvas' }) as unknown as { setOption: (o: unknown) => void; dispose: () => void };
    }
    const h = historyRef.current;
    const labels = h.t.map((t) => new Date(t).toLocaleTimeString());
    perfChart.current.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['Ingested', 'Output', 'Lag (ms)', 'Utilization %'] },
      grid: { top: 40, left: 60, right: 60, bottom: 30 },
      xAxis: { type: 'category', data: labels },
      yAxis: [{ type: 'value', name: 'records' }, { type: 'value', name: '%', max: 100, position: 'right' }],
      series: [
        { name: 'Ingested', type: 'line', smooth: true, data: h.ingested },
        { name: 'Output', type: 'line', smooth: true, data: h.output },
        { name: 'Lag (ms)', type: 'line', smooth: true, data: h.lag },
        { name: 'Utilization %', type: 'line', smooth: true, yAxisIndex: 1, data: h.utilization },
      ],
    });
  }

  function changeWindow(w: string) {
    setPerfWindow(w);
    historyRef.current = { t: [], ingested: [], output: [], lag: [], utilization: [] };
  }

  useEffect(() => {
    void refreshMetrics();
    void refreshCheckpoints();
    void refreshMonitors();
    const perfTimer = setInterval(() => void refreshMetrics(), 10_000);
    const cpTimer = setInterval(() => void refreshCheckpoints(), 10_000);
    const monTimer = setInterval(() => void refreshMonitors(), 30_000);
    return () => {
      clearInterval(perfTimer);
      clearInterval(cpTimer);
      clearInterval(monTimer);
      perfChart.current?.dispose();
      perfChart.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamId, topology?.id, perfWindow]);

  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: 6, padding: 16 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <h3 style={{ margin: 0, fontSize: 16 }}>Job Graph</h3>
        </header>
        {topology ? <JobGraph topologyId={topology.id} /> : <p style={{ color: '#666' }}>Select a related topology to render its live job graph.</p>}
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: 6, padding: 16 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <h3 style={{ margin: 0, fontSize: 16 }}>Last checkpoints</h3>
          <small style={{ color: '#666' }}>Auto-refresh every 10 s · last 20</small>
        </header>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
          <thead>
            <tr><th style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #eee' }}>ID</th><th style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #eee' }}>Started</th><th style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #eee' }}>Status</th><th style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #eee' }}>Duration</th><th style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #eee' }}>Trigger</th></tr>
          </thead>
          <tbody>
            {checkpoints.length === 0 ? (
              <tr><td colSpan={5} style={{ color: '#666', padding: '6px 8px' }}>No checkpoints yet.</td></tr>
            ) : (
              checkpoints.map((cp) => {
                const cls = checkpointStatusClass(cp.status);
                return (
                  <tr key={cp.id}>
                    <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}><code>{cp.id.slice(0, 8)}</code></td>
                    <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{new Date(cp.created_at).toLocaleTimeString()}</td>
                    <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>
                      <span style={{ display: 'inline-block', padding: '1px 8px', borderRadius: 3, fontSize: 12, ...BADGE_STYLES[cls] }}>{cp.status}</span>
                    </td>
                    <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{cp.duration_ms} ms</td>
                    <td style={{ padding: '6px 8px', borderBottom: '1px solid #eee' }}>{cp.trigger}</td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: 6, padding: 16 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <h3 style={{ margin: 0, fontSize: 16 }}>Performance</h3>
          <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
            <button type="button" onClick={() => changeWindow('5m')} style={{ padding: '3px 10px', border: '1px solid #ddd', borderRadius: 4, cursor: 'pointer', background: perfWindow === '5m' ? '#246' : '#fff', color: perfWindow === '5m' ? '#fff' : 'inherit' }}>5m</button>
            <button type="button" onClick={() => changeWindow('30m')} style={{ padding: '3px 10px', border: '1px solid #ddd', borderRadius: 4, cursor: 'pointer', background: perfWindow === '30m' ? '#246' : '#fff', color: perfWindow === '30m' ? '#fff' : 'inherit' }}>30m</button>
            <input type="text" placeholder="custom (e.g. 600s)" value={perfWindow !== '5m' && perfWindow !== '30m' ? perfWindow : ''} onChange={(e) => { const v = e.target.value.trim(); if (v) changeWindow(v); }} style={{ padding: '3px 6px', fontSize: 13, border: '1px solid #ddd', borderRadius: 4 }} />
          </div>
        </header>
        {metrics && (
          <dl style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '8px 16px', margin: 0 }}>
            <div><dt style={{ fontSize: 11, color: '#666', textTransform: 'uppercase' }}>Ingested</dt><dd style={{ margin: 0, fontWeight: 600, fontSize: 18 }}>{metrics.records_ingested.toLocaleString()}</dd></div>
            <div><dt style={{ fontSize: 11, color: '#666', textTransform: 'uppercase' }}>Output</dt><dd style={{ margin: 0, fontWeight: 600, fontSize: 18 }}>{metrics.records_output.toLocaleString()}</dd></div>
            <div><dt style={{ fontSize: 11, color: '#666', textTransform: 'uppercase' }}>Throughput</dt><dd style={{ margin: 0, fontWeight: 600, fontSize: 18 }}>{metrics.total_throughput.toFixed(2)} r/s</dd></div>
            <div><dt style={{ fontSize: 11, color: '#666', textTransform: 'uppercase' }}>Lag</dt><dd style={{ margin: 0, fontWeight: 600, fontSize: 18 }}>{metrics.total_lag.toLocaleString()} ms</dd></div>
            <div><dt style={{ fontSize: 11, color: '#666', textTransform: 'uppercase' }}>Utilization</dt><dd style={{ margin: 0, fontWeight: 600, fontSize: 18 }}>{(metrics.utilization_pct * 100).toFixed(1)} %</dd></div>
          </dl>
        )}
        <div ref={perfRef} style={{ width: '100%', height: 240, marginTop: 8 }} />
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: 6, padding: 16 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <h3 style={{ margin: 0, fontSize: 16 }}>Active monitors</h3>
          <a href="/control-panel/data-health" style={{ color: '#246', textDecoration: 'none', fontSize: 14 }}>+ Add monitor</a>
        </header>
        <ul style={{ margin: 0, padding: 0, listStyle: 'none', display: 'grid', gap: 6 }}>
          {monitors.length === 0 ? (
            <li style={{ color: '#666' }}>No monitors configured for this stream. Use Data Health to add one.</li>
          ) : (
            monitors.map((m) => {
              const cls = m.firing ? 'fail' : 'ok';
              return (
                <li key={m.rule.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 10px', border: '1px solid #eee', borderRadius: 4 }}>
                  <span>
                    <strong>{m.rule.monitor_kind}</strong>
                    <small style={{ display: 'block', color: '#666', fontWeight: 'normal' }}>{m.rule.comparator} {m.rule.threshold} · {m.rule.window_seconds}s · <em>{m.rule.severity}</em></small>
                  </span>
                  <span style={{ display: 'inline-block', padding: '1px 8px', borderRadius: 3, fontSize: 12, ...BADGE_STYLES[cls] }}>{m.firing ? 'FIRING' : 'ok'}</span>
                </li>
              );
            })
          )}
        </ul>
      </div>
    </section>
  );
}
