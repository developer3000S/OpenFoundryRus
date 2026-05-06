import { useEffect, useMemo, useState } from 'react';

import { CytoscapeCanvas } from '@/lib/components/CytoscapeCanvas';
import {
  getDatasetLineage,
  getDatasetLineageImpact,
  type LineageGraph,
  type LineageImpactAnalysis,
} from '@/lib/api/pipelines';

interface LineageViewProps {
  datasetId: string;
  onSelect?: (id: string, kind: string) => void;
}

export function LineageView({ datasetId, onSelect }: LineageViewProps) {
  const [graph, setGraph] = useState<LineageGraph | null>(null);
  const [impact, setImpact] = useState<LineageImpactAnalysis | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!datasetId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([
      getDatasetLineage(datasetId),
      getDatasetLineageImpact(datasetId).catch(() => null),
    ])
      .then(([g, imp]) => {
        if (!cancelled) {
          setGraph(g);
          setImpact(imp);
        }
      })
      .catch((cause: unknown) => { if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause)); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetId]);

  const elements = useMemo(() => {
    if (!graph) return [];
    return [
      ...graph.nodes.map((n) => ({ data: { id: n.id, label: n.label || n.id, kind: n.kind, marking: n.marking } })),
      ...graph.edges.map((e) => ({ data: { id: e.id, source: e.source, target: e.target, relation: e.relation_kind } })),
    ];
  }, [graph]);

  const stylesheet = useMemo(
    () => [
      { selector: 'node', style: { label: 'data(label)', 'font-size': 10, color: '#e2e8f0', 'background-color': '#1e3a8a', 'text-valign': 'bottom', 'text-margin-y': 6, width: 32, height: 32 } },
      { selector: 'node[kind = "dataset"]', style: { 'background-color': '#1d4ed8', shape: 'round-rectangle' } },
      { selector: 'node[kind = "pipeline"]', style: { 'background-color': '#7c3aed', shape: 'diamond' } },
      { selector: 'node[kind = "workflow"]', style: { 'background-color': '#0d9488', shape: 'hexagon' } },
      { selector: `node[id = "${datasetId}"]`, style: { 'background-color': '#f59e0b', 'border-width': 2, 'border-color': '#fbbf24' } },
      { selector: 'edge', style: { width: 1.5, 'line-color': '#475569', 'target-arrow-color': '#475569', 'target-arrow-shape': 'triangle', 'curve-style': 'bezier' } },
    ],
    [datasetId],
  );

  return (
    <section className="of-panel" style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Lineage</h3>
        {impact && (
          <span className="of-text-muted" style={{ fontSize: 11 }}>
            ↑ {impact.upstream.length} upstream · ↓ {impact.downstream.length} downstream
          </span>
        )}
      </header>
      {loading && <p className="of-text-muted" style={{ fontSize: 12, fontStyle: 'italic', margin: 0 }}>Loading lineage…</p>}
      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}
      {graph && graph.nodes.length === 0 && !loading && (
        <p className="of-text-muted" style={{ fontSize: 12, fontStyle: 'italic', margin: 0 }}>No lineage data for this dataset yet.</p>
      )}
      {graph && graph.nodes.length > 0 && (
        <div style={{ border: '1px solid var(--border-default)', borderRadius: 12, background: '#0b1220', minHeight: 360 }}>
          <CytoscapeCanvas
            elements={elements}
            stylesheet={stylesheet as unknown as import('cytoscape').StylesheetStyle[]}
            layout={{ name: 'breadthfirst', directed: true, padding: 20, spacingFactor: 1.4 } as import('cytoscape').LayoutOptions}
            height={360}
            onReady={(cy) => {
              cy.removeListener('tap');
              cy.on('tap', 'node', (evt) => {
                const id = evt.target.id();
                const kind = evt.target.data('kind') as string;
                onSelect?.(id, kind);
              });
            }}
          />
        </div>
      )}
    </section>
  );
}
