import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  getRuntime,
  getLiveTail,
  listTopologies,
  runTopology,
  type LiveTailResponse,
  type TopologyDefinition,
  type TopologyRuntimeSnapshot,
} from '@/lib/api/streaming';

export function StreamingDetailPage() {
  const { id = '' } = useParams();
  const [topology, setTopology] = useState<TopologyDefinition | null>(null);
  const [runtime, setRuntime] = useState<TopologyRuntimeSnapshot | null>(null);
  const [liveTail, setLiveTail] = useState<LiveTailResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [topRes, runRes, tailRes] = await Promise.all([
          listTopologies(),
          getRuntime(id).catch(() => null),
          getLiveTail().catch(() => null),
        ]);
        if (cancelled) return;
        setTopology(topRes.data.find((t) => t.id === id) ?? null);
        setRuntime(runRes);
        setLiveTail(tailRes);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load topology');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [id]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/streaming" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Streaming
      </Link>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {topology ? (
        <>
          <header>
            <h1 className="of-heading-xl">{topology.name}</h1>
            <p className="of-text-muted" style={{ marginTop: 4 }}>
              {topology.description}
            </p>
          </header>
          <button
            type="button"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await runTopology(topology.id);
                setRuntime(await getRuntime(topology.id));
              } catch (cause) {
                setError(cause instanceof Error ? cause.message : 'Run failed');
              } finally {
                setBusy(false);
              }
            }}
            className="of-button of-button--primary"
            style={{ width: 'fit-content' }}
          >
            Run topology
          </button>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Topology JSON</p>
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(topology, null, 2)}
            </pre>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Runtime snapshot</p>
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(runtime, null, 2)}
            </pre>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Live tail</p>
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(liveTail, null, 2)}
            </pre>
          </section>
        </>
      ) : (
        <p className="of-text-muted">Loading topology…</p>
      )}
    </section>
  );
}
