import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  createStream,
  createTopology,
  createWindow,
  getOverview,
  listConnectors,
  listStreams,
  listTopologies,
  listWindows,
  runTopology,
  type ConnectorCatalogEntry,
  type StreamDefinition,
  type StreamingOverview,
  type TopologyDefinition,
  type WindowDefinition,
} from '@/lib/api/streaming';

type Tab = 'overview' | 'streams' | 'windows' | 'topologies' | 'connectors';

const TABS: Array<{ id: Tab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'streams', label: 'Streams' },
  { id: 'windows', label: 'Windows' },
  { id: 'topologies', label: 'Topologies' },
  { id: 'connectors', label: 'Connectors' },
];

function formatJson(value: unknown) {
  return JSON.stringify(value, null, 2);
}

function parseJson<T>(value: string, fallback: T): T {
  if (!value.trim()) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    throw new Error('Invalid JSON');
  }
}

export function StreamingPage() {
  const [tab, setTab] = useState<Tab>('overview');
  const [overview, setOverview] = useState<StreamingOverview | null>(null);
  const [streams, setStreams] = useState<StreamDefinition[]>([]);
  const [windows, setWindows] = useState<WindowDefinition[]>([]);
  const [topologies, setTopologies] = useState<TopologyDefinition[]>([]);
  const [connectors, setConnectors] = useState<ConnectorCatalogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const [streamJson, setStreamJson] = useState(
    formatJson({
      name: 'Orders Ingress',
      description: 'Kafka topic carrying customer checkout events.',
      status: 'active',
      retention_hours: 72,
      connector_type: 'kafka',
      endpoint: 'kafka://stream/orders',
      format: 'json',
      schema: {
        fields: [
          { name: 'event_time', data_type: 'timestamp', nullable: false, semantic_role: 'event_time' },
          { name: 'customer_id', data_type: 'string', nullable: false, semantic_role: 'join_key' },
        ],
        primary_key: null,
        watermark_field: 'event_time',
      },
    }),
  );
  const [windowJson, setWindowJson] = useState(
    formatJson({
      name: 'Five Minute Revenue',
      description: 'Tumbling revenue aggregates.',
      status: 'active',
      window_type: 'tumbling',
      duration_seconds: 300,
      slide_seconds: 300,
      session_gap_seconds: 180,
      allowed_lateness_seconds: 30,
      aggregation_keys: ['customer_id'],
      measure_fields: ['amount'],
    }),
  );
  const [topologyJson, setTopologyJson] = useState(
    formatJson({
      name: 'Revenue Anomaly Pipeline',
      description: '',
      status: 'active',
      state_backend: 'rocksdb',
      source_stream_ids: [],
      nodes: [],
      edges: [],
      join_definition: null,
      cep_definition: null,
      backpressure_policy: { kind: 'block' },
      sink_bindings: [],
    }),
  );

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [oRes, sRes, wRes, tRes, cRes] = await Promise.all([
        getOverview(),
        listStreams(),
        listWindows(),
        listTopologies(),
        listConnectors(),
      ]);
      setOverview(oRes);
      setStreams(sRes.data);
      setWindows(wRes.data);
      setTopologies(tRes.data);
      setConnectors(cRes.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load streaming');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function run(label: string, action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `${label} failed`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Streaming</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Streams, windows, and CEP topologies. JSON-driven editors keep parity with the streaming-service contract.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <nav style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
        {TABS.map((t) => {
          const active = tab === t.id;
          return (
            <button
              key={t.id}
              type="button"
              onClick={() => setTab(t.id)}
              style={{
                padding: '8px 14px',
                background: 'transparent',
                border: 'none',
                borderBottom: `2px solid ${active ? '#1d4ed8' : 'transparent'}`,
                color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
              }}
            >
              {t.label}
            </button>
          );
        })}
      </nav>

      {loading && <p className="of-text-muted">Loading…</p>}

      {tab === 'overview' && overview && (
        <section className="of-panel" style={{ padding: 16 }}>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {formatJson(overview)}
          </pre>
        </section>
      )}

      {tab === 'streams' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Streams ({streams.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {streams.map((s) => (
              <li key={s.id}>
                <strong>{s.name}</strong> · {s.source_binding.connector_type} · {s.status}
              </li>
            ))}
          </ul>
          <p className="of-eyebrow" style={{ marginTop: 14 }}>Create stream JSON</p>
          <textarea
            value={streamJson}
            onChange={(e) => setStreamJson(e.target.value)}
            className="of-input"
            style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 220 }}
          />
          <button
            type="button"
            onClick={() =>
              void run('create-stream', async () => {
                await createStream(parseJson(streamJson, {} as Parameters<typeof createStream>[0]));
                await refresh();
              })
            }
            disabled={busy}
            className="of-button of-button--primary"
            style={{ marginTop: 8 }}
          >
            Create stream
          </button>
        </section>
      )}

      {tab === 'windows' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Windows ({windows.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {windows.map((w) => (
              <li key={w.id}>
                <strong>{w.name}</strong> · {w.window_type} · {w.duration_seconds}s
              </li>
            ))}
          </ul>
          <p className="of-eyebrow" style={{ marginTop: 14 }}>Create window JSON</p>
          <textarea
            value={windowJson}
            onChange={(e) => setWindowJson(e.target.value)}
            className="of-input"
            style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 200 }}
          />
          <button
            type="button"
            onClick={() =>
              void run('create-window', async () => {
                await createWindow(parseJson(windowJson, {} as Parameters<typeof createWindow>[0]));
                await refresh();
              })
            }
            disabled={busy}
            className="of-button of-button--primary"
            style={{ marginTop: 8 }}
          >
            Create window
          </button>
        </section>
      )}

      {tab === 'topologies' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Topologies ({topologies.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {topologies.map((t) => (
              <li key={t.id}>
                <Link to={`/streaming/${t.id}`}>
                  <strong>{t.name}</strong>
                </Link>{' '}
                · {t.status} · {t.nodes.length} nodes
                <button
                  type="button"
                  onClick={() => void run('run-topology', async () => { await runTopology(t.id); })}
                  disabled={busy}
                  className="of-button"
                  style={{ marginLeft: 6, fontSize: 11 }}
                >
                  Run
                </button>
              </li>
            ))}
          </ul>
          <p className="of-eyebrow" style={{ marginTop: 14 }}>Create topology JSON</p>
          <textarea
            value={topologyJson}
            onChange={(e) => setTopologyJson(e.target.value)}
            className="of-input"
            style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 240 }}
          />
          <button
            type="button"
            onClick={() =>
              void run('create-topology', async () => {
                await createTopology(parseJson(topologyJson, {} as Parameters<typeof createTopology>[0]));
                await refresh();
              })
            }
            disabled={busy}
            className="of-button of-button--primary"
            style={{ marginTop: 8 }}
          >
            Create topology
          </button>
        </section>
      )}

      {tab === 'connectors' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Connectors ({connectors.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {connectors.map((c, i) => (
              <li key={`${c.connector_type}-${c.endpoint}-${i}`}>
                <strong>{c.connector_type}</strong> · {c.direction} · {c.status} · backlog {c.backlog}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
