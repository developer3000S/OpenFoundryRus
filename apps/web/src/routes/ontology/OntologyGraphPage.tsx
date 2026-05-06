import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { CytoscapeCanvas } from '@/lib/components/CytoscapeCanvas';
import { getOntologyGraph, listObjectTypes, type GraphResponse, type ObjectType } from '@/lib/api/ontology';

type Mode = 'schema' | 'object';

export function OntologyGraphPage() {
  const [types, setTypes] = useState<ObjectType[]>([]);
  const [graph, setGraph] = useState<GraphResponse | null>(null);
  const [mode, setMode] = useState<Mode>('schema');
  const [rootObjectId, setRootObjectId] = useState('');
  const [rootTypeId, setRootTypeId] = useState('');
  const [depth, setDepth] = useState(2);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    listObjectTypes({ per_page: 200 })
      .then((res) => setTypes(res.data))
      .catch(() => {});
  }, []);

  async function loadGraph() {
    setBusy(true);
    setError('');
    try {
      const res = await getOntologyGraph({
        root_object_id: mode === 'object' ? rootObjectId || undefined : undefined,
        root_type_id: mode === 'schema' ? rootTypeId || undefined : undefined,
        depth,
        limit: 60,
      });
      setGraph(res);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load graph');
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void loadGraph();
  }, []);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/ontology" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology</Link>
      <header>
        <h1 className="of-heading-xl">Ontology graph</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
          Schema (object types + link types) or object-rooted graph view.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <select value={mode} onChange={(e) => setMode(e.target.value as Mode)} className="of-input">
            <option value="schema">schema</option>
            <option value="object">object</option>
          </select>
          {mode === 'schema' ? (
            <select value={rootTypeId} onChange={(e) => setRootTypeId(e.target.value)} className="of-input">
              <option value="">— all types —</option>
              {types.map((t) => (
                <option key={t.id} value={t.id}>{t.display_name}</option>
              ))}
            </select>
          ) : (
            <input value={rootObjectId} onChange={(e) => setRootObjectId(e.target.value)} placeholder="object id" className="of-input" />
          )}
          <input type="number" min={1} max={6} value={depth} onChange={(e) => setDepth(Number(e.target.value) || 2)} className="of-input" style={{ width: 80 }} />
          <button type="button" onClick={() => void loadGraph()} disabled={busy} className="of-button of-button--primary">Load</button>
        </div>
      </section>

      {graph && <OntologyGraphView graph={graph} />}
    </section>
  );
}

function OntologyGraphView({ graph }: { graph: GraphResponse }) {
  const elements = useMemo(() => {
    const nodes = graph.nodes.map((n) => ({
      data: {
        id: n.id,
        label: n.label,
        kind: n.kind,
        color: n.color || '#4d8cf0',
      },
    }));
    const edges = graph.edges.map((e) => ({
      data: { id: e.id, source: e.source, target: e.target, label: e.label || e.kind },
    }));
    return [...nodes, ...edges];
  }, [graph]);

  const stylesheet = useMemo(
    () => [
      {
        selector: 'node',
        style: {
          'background-color': 'data(color)',
          label: 'data(label)',
          color: '#f1f5f9',
          'text-valign': 'center',
          'text-halign': 'center',
          'text-wrap': 'wrap',
          'text-max-width': '100px',
          'font-size': 11,
          'font-weight': 600,
          width: 100,
          height: 40,
          shape: 'round-rectangle',
          'border-color': '#475569',
          'border-width': 2,
        },
      },
      {
        selector: 'node:selected',
        style: { 'border-color': '#fbbf24', 'border-width': 4 },
      },
      {
        selector: 'edge',
        style: {
          width: 1.5,
          'line-color': '#475569',
          'target-arrow-color': '#475569',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
          label: 'data(label)',
          'font-size': 9,
          color: '#94a3b8',
          'text-rotation': 'autorotate',
          'text-background-color': '#0b1220',
          'text-background-opacity': 0.85,
          'text-background-padding': '2px',
        },
      },
    ],
    [],
  );

  return (
    <section className="of-panel" style={{ padding: 16 }}>
      <p className="of-eyebrow">
        {graph.nodes.length} nodes · {graph.edges.length} edges
        {graph.summary && (
          <>
            {' '}· hops: {graph.summary.max_hops_reached}
            {graph.summary.boundary_crossings > 0 && ` · ${graph.summary.boundary_crossings} boundary crossings`}
            {graph.summary.sensitive_objects > 0 && ` · ${graph.summary.sensitive_objects} sensitive`}
          </>
        )}
      </p>
      <div style={{ marginTop: 8, border: '1px solid var(--border-default)', borderRadius: 12, background: '#0b1220' }}>
        <CytoscapeCanvas
          elements={elements}
          stylesheet={stylesheet as unknown as import('cytoscape').StylesheetStyle[]}
          height={520}
        />
      </div>
    </section>
  );
}
