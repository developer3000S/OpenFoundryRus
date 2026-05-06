import { useEffect, useRef, useState } from 'react';

import { deployTopologyToFlink, getTopologyJobGraph, type FlinkJobGraph } from '@/lib/api/streaming';

interface Props {
  topologyId: string;
}

export function JobGraph({ topologyId }: Props) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const cyRef = useRef<{ destroy: () => void } | null>(null);
  const [graph, setGraph] = useState<FlinkJobGraph | null>(null);
  const [loading, setLoading] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const next = await getTopologyJobGraph(topologyId);
      setGraph(next);
      setInfo(next.message ?? null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  async function deploy() {
    setDeploying(true);
    setError(null);
    try {
      const resp = await deployTopologyToFlink(topologyId);
      let msg = resp.message;
      if (resp.sql_warnings.length > 0) msg += ` — warnings: ${resp.sql_warnings.join('; ')}`;
      setInfo(msg);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setDeploying(false);
    }
  }

  useEffect(() => {
    void load();
    return () => { cyRef.current?.destroy(); cyRef.current = null; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [topologyId]);

  useEffect(() => {
    if (!hostRef.current || !graph) return;
    let cancelled = false;
    (async () => {
      cyRef.current?.destroy();
      const cytoscape = (await import('cytoscape')).default;
      if (cancelled || !hostRef.current) return;
      cyRef.current = cytoscape({
        container: hostRef.current,
        elements: [
          ...graph.vertices.map((v) => ({
            data: { id: v.id, label: `${v.name ?? v.id}\n(p=${v.parallelism ?? '?'})`, status: v.status ?? 'UNKNOWN' },
          })),
          ...graph.edges.map((e, idx) => ({ data: { id: `e${idx}`, source: e.source, target: e.target } })),
        ],
        style: [
          { selector: 'node', style: { label: 'data(label)', 'text-wrap': 'wrap', 'font-size': 10, color: '#e2e8f0', 'background-color': '#1e3a8a', 'text-valign': 'bottom', 'text-margin-y': 6, shape: 'round-rectangle', width: 60, height: 36 } },
          { selector: 'node[status = "RUNNING"]', style: { 'background-color': '#16a34a' } },
          { selector: 'node[status = "FAILED"]', style: { 'background-color': '#dc2626' } },
          { selector: 'edge', style: { width: 1.5, 'line-color': '#475569', 'target-arrow-color': '#475569', 'target-arrow-shape': 'triangle', 'curve-style': 'bezier' } },
        ],
        layout: { name: 'breadthfirst', directed: true, padding: 20, spacingFactor: 1.4 },
      });
    })();
    return () => { cancelled = true; };
  }, [graph]);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0, fontSize: 15, color: '#e2e8f0' }}>Flink job graph</h3>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" onClick={() => void load()} disabled={loading} className="of-button" style={{ fontSize: 12 }}>
            {loading ? 'Loading…' : 'Refresh'}
          </button>
          <button type="button" onClick={() => void deploy()} disabled={deploying} className="of-button" style={{ fontSize: 12 }}>
            {deploying ? 'Deploying…' : 'Deploy to Flink'}
          </button>
        </div>
      </header>
      {error && <p style={{ color: '#fca5a5', fontSize: 13, margin: 0 }}>{error}</p>}
      {info && <p style={{ color: '#fde68a', fontSize: 13, margin: 0 }}>{info}</p>}
      {graph?.job_id && (
        <p style={{ color: '#94a3b8', fontSize: 12, margin: 0 }}>
          Job ID: <code style={{ color: '#cbd5e1' }}>{graph.job_id}</code>
        </p>
      )}
      <div ref={hostRef} style={{ width: '100%', height: 360, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6 }} />
    </section>
  );
}
