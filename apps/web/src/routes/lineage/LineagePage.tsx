import { useCallback, useEffect, useMemo, useState } from 'react';
import type { Core, ElementDefinition, EventObject, StylesheetStyle } from 'cytoscape';

import { CytoscapeCanvas } from '@components/CytoscapeCanvas';
import { loadJobSpecStatus } from '@/lib/api/datasets';
import {
  getDatasetLineageImpact,
  getFullLineage,
  triggerLineageBuilds,
  type LineageBuildResult,
  type LineageGraph,
  type LineageImpactAnalysis,
  type LineageNode,
} from '@/lib/api/pipelines';
import { notifications } from '@stores/notifications';

// Foundry doc § "Job graph compilation":
//   * dataset icon GREY  ⇒ no JobSpec on master
//   * dataset icon BLUE  ⇒ JobSpec is defined on master
const KIND_PALETTE: Record<string, string> = {
  dataset: '#94a3b8',
  pipeline: '#2563eb',
  workflow: '#d97706',
};
const DATASET_WITH_MASTER = '#2563eb';
const DATASET_WITHOUT_MASTER = '#94a3b8';

const MARKING_PALETTE: Record<string, string> = {
  public: '#a3a3a3',
  confidential: '#f97316',
  pii: '#ef4444',
};

const STYLESHEET: StylesheetStyle[] = [
  {
    selector: 'node',
    style: {
      'background-color': 'data(color)',
      label: 'data(label)',
      color: '#e5e7eb',
      'text-wrap': 'wrap',
      'text-max-width': '140',
      'text-valign': 'bottom',
      'text-margin-y': 10,
      'font-size': 11,
      'font-weight': 600,
      width: 42,
      height: 42,
      'border-width': 3,
      'border-color': 'data(borderColor)',
    },
  },
  {
    selector: 'edge',
    style: {
      width: 2,
      label: 'data(relation)',
      'font-size': 9,
      color: '#cbd5e1',
      'text-background-color': '#0f172a',
      'text-background-opacity': 0.7,
      'text-background-padding': '2',
      'line-color': 'data(color)',
      'target-arrow-color': 'data(color)',
      'target-arrow-shape': 'triangle',
      'curve-style': 'bezier',
    },
  },
  {
    selector: ':selected',
    style: {
      'overlay-opacity': 0,
      'border-width': 5,
    },
  },
];

const LAYOUT = { name: 'breadthfirst', directed: true, spacingFactor: 1.45 } as const;

function markingColor(marking: string) {
  return MARKING_PALETTE[marking] ?? '#a3a3a3';
}

export function LineagePage() {
  const [graph, setGraph] = useState<LineageGraph | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedNode, setSelectedNode] = useState<LineageNode | null>(null);
  const [impact, setImpact] = useState<LineageImpactAnalysis | null>(null);
  const [impactLoading, setImpactLoading] = useState(false);
  const [building, setBuilding] = useState(false);
  const [buildResult, setBuildResult] = useState<LineageBuildResult | null>(null);
  const [acknowledgeSensitiveLineage, setAcknowledgeSensitiveLineage] = useState(false);
  const [jobSpecByDatasetId, setJobSpecByDatasetId] = useState<Record<string, boolean>>({});

  const loadGraph = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const fresh = await getFullLineage();
      const datasetIds = fresh.nodes.filter((n) => n.kind === 'dataset').map((n) => n.id);
      const jobSpecResults = await Promise.allSettled(
        datasetIds.map(async (id) => [id, await loadJobSpecStatus(id)] as const),
      );
      const nextJobSpec: Record<string, boolean> = {};
      for (const r of jobSpecResults) {
        if (r.status === 'fulfilled') {
          const [id, status] = r.value;
          nextJobSpec[id] = status.has_master_jobspec;
        }
      }
      setJobSpecByDatasetId(nextJobSpec);
      setGraph(fresh);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load lineage');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadGraph();
  }, [loadGraph]);

  // Compute Cytoscape elements every time graph or jobspec map changes.
  const elements = useMemo<ElementDefinition[]>(() => {
    if (!graph) return [];
    function nodeColor(kind: string, nodeId: string) {
      if (kind === 'dataset') {
        return jobSpecByDatasetId[nodeId] ? DATASET_WITH_MASTER : DATASET_WITHOUT_MASTER;
      }
      return KIND_PALETTE[kind] ?? '#64748b';
    }
    return [
      ...graph.nodes.map((node) => ({
        data: {
          id: node.id,
          label: node.label,
          kind: node.kind,
          marking: node.marking,
          color: nodeColor(node.kind, node.id),
          borderColor: markingColor(node.marking),
        },
      })),
      ...graph.edges.map((edge) => ({
        data: {
          id: edge.id,
          source: edge.source,
          target: edge.target,
          relation: edge.relation_kind,
          color: markingColor(edge.effective_marking),
        },
      })),
    ];
  }, [graph, jobSpecByDatasetId]);

  const handleCytoscapeReady = useCallback(
    (cy: Core) => {
      cy.on('tap', 'node', (event: EventObject) => {
        const nodeId = String(event.target.id());
        const node = graph?.nodes.find((entry) => entry.id === nodeId) ?? null;
        setSelectedNode(node);
        setImpact(null);
        setBuildResult(null);
        if (node?.kind === 'dataset') {
          void loadImpact(node.id);
        }
      });
    },
    [graph],
  );

  async function loadImpact(datasetId: string) {
    setImpactLoading(true);
    setBuildResult(null);
    setAcknowledgeSensitiveLineage(false);
    try {
      const next = await getDatasetLineageImpact(datasetId);
      setImpact(next);
    } catch (cause) {
      setImpact(null);
      notifications.error(
        cause instanceof Error ? cause.message : 'Failed to load impact analysis',
      );
    } finally {
      setImpactLoading(false);
    }
  }

  async function triggerBuilds() {
    if (!selectedNode || selectedNode.kind !== 'dataset') return;
    setBuilding(true);
    try {
      const next = await triggerLineageBuilds(selectedNode.id, {
        include_workflows: true,
        dry_run: false,
        acknowledge_sensitive_lineage: acknowledgeSensitiveLineage,
        context: { initiated_from: 'lineage-explorer' },
      });
      setBuildResult(next);
      notifications.success(`Triggered ${next.triggered.length} downstream build(s)`);
      await loadImpact(selectedNode.id);
    } catch (cause) {
      notifications.error(cause instanceof Error ? cause.message : 'Failed to trigger builds');
    } finally {
      setBuilding(false);
    }
  }

  const sensitiveCandidateCount =
    impact?.build_candidates.filter((c) => c.requires_acknowledgement).length ?? 0;

  function kindCount(kind: string) {
    return graph?.nodes.filter((node) => node.kind === kind).length ?? 0;
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <h1 className="of-heading-xl">Operational lineage</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Explore dataset, pipeline, and workflow dependencies, inspect propagated markings, and
            trigger downstream rebuilds from the graph.
          </p>
        </div>
        <button type="button" className="of-btn" onClick={() => void loadGraph()}>
          Refresh graph
        </button>
      </header>

      {error && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>{error}</div>}

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, minmax(0, 1fr))' }}>
        {[
          { label: 'Datasets', kind: 'dataset', color: '#0f766e' },
          { label: 'Pipelines', kind: 'pipeline', color: '#2563eb' },
          { label: 'Workflows', kind: 'workflow', color: '#d97706' },
        ].map((card) => (
          <div key={card.kind} className="of-card">
            <p className="of-eyebrow">{card.label}</p>
            <div style={{ marginTop: 8, fontSize: 28, fontWeight: 700, color: card.color }}>
              {kindCount(card.kind)}
            </div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.45fr) minmax(0, 0.95fr)' }}>
        <section className="of-panel" style={{ padding: 20 }}>
          {loading ? (
            <div style={{ padding: 80, textAlign: 'center', color: 'var(--text-muted)' }}>
              Loading lineage graph…
            </div>
          ) : !graph || graph.nodes.length === 0 ? (
            <div style={{ padding: 80, textAlign: 'center', color: 'var(--text-muted)' }}>
              No lineage data yet. Run a pipeline or workflow to populate the graph.
            </div>
          ) : (
            <>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, fontSize: 13, color: 'var(--text-muted)', marginBottom: 16 }}>
                <div>{graph.nodes.length} nodes, {graph.edges.length} relations</div>
                <div style={{ display: 'flex', gap: 12 }}>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    <span style={{ width: 10, height: 10, borderRadius: 999, background: '#0f766e' }} />
                    Dataset
                  </span>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    <span style={{ width: 10, height: 10, borderRadius: 999, background: '#2563eb' }} />
                    Pipeline
                  </span>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    <span style={{ width: 10, height: 10, borderRadius: 999, background: '#d97706' }} />
                    Workflow
                  </span>
                </div>
              </div>

              <CytoscapeCanvas
                elements={elements}
                stylesheet={STYLESHEET}
                layout={LAYOUT}
                height={680}
                onReady={handleCytoscapeReady}
                className="lineage-canvas"
              />
            </>
          )}
        </section>

        <section className="of-panel" style={{ padding: 20, display: 'grid', gap: 16 }}>
          {!selectedNode ? (
            <div style={{ padding: 64, textAlign: 'center', color: 'var(--text-muted)', fontSize: 13 }}>
              Select a node in the graph to inspect metadata, impact, and build candidates.
            </div>
          ) : (
            <>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <p className="of-eyebrow">{selectedNode.kind}</p>
                    <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                      {selectedNode.label}
                    </h2>
                  </div>
                  <span
                    className="of-chip"
                    style={{
                      background: `${markingColor(selectedNode.marking)}22`,
                      color: markingColor(selectedNode.marking),
                      textTransform: 'uppercase',
                      letterSpacing: '0.18em',
                      fontSize: 11,
                      fontWeight: 600,
                    }}
                  >
                    {selectedNode.marking}
                  </span>
                </div>
                <div style={{ marginTop: 12, fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)' }}>
                  {selectedNode.id}
                </div>
              </div>

              <div className="of-panel-muted" style={{ padding: 16 }}>
                <p className="of-eyebrow">Node metadata</p>
                <div style={{ marginTop: 12, display: 'grid', gap: 8, fontSize: 13 }}>
                  {Object.keys(selectedNode.metadata ?? {}).length === 0 ? (
                    <div className="of-text-muted">No metadata captured yet.</div>
                  ) : (
                    Object.entries(selectedNode.metadata ?? {}).map(([key, value]) => (
                      <div key={key} style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                        <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{key}</div>
                        <div style={{ maxWidth: '60%', textAlign: 'right', wordBreak: 'break-word', color: 'var(--text-muted)' }}>
                          {typeof value === 'string' ? value : JSON.stringify(value)}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>

              {selectedNode.kind === 'dataset' && (
                <>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button
                      type="button"
                      className="of-btn"
                      onClick={() => selectedNode?.id && void loadImpact(selectedNode.id)}
                      disabled={impactLoading}
                    >
                      {impactLoading ? 'Refreshing impact…' : 'Refresh impact'}
                    </button>
                    <button
                      type="button"
                      className="of-btn of-btn-primary"
                      onClick={() => void triggerBuilds()}
                      disabled={
                        building ||
                        impactLoading ||
                        (sensitiveCandidateCount > 0 && !acknowledgeSensitiveLineage)
                      }
                    >
                      {building ? 'Triggering builds…' : 'Trigger downstream builds'}
                    </button>
                  </div>

                  {impactLoading ? (
                    <div
                      style={{
                        border: '1px dashed var(--border-default)',
                        borderRadius: 'var(--radius-md)',
                        padding: 32,
                        textAlign: 'center',
                        color: 'var(--text-muted)',
                        fontSize: 13,
                      }}
                    >
                      Loading impact analysis…
                    </div>
                  ) : impact ? (
                    <div style={{ display: 'grid', gap: 16 }}>
                      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)' }}>
                        <div className="of-panel-muted" style={{ padding: 16 }}>
                          <p className="of-eyebrow">Upstream</p>
                          <div style={{ marginTop: 8, fontSize: 22, fontWeight: 600 }}>
                            {impact.upstream.length}
                          </div>
                        </div>
                        <div className="of-panel-muted" style={{ padding: 16 }}>
                          <p className="of-eyebrow">Downstream</p>
                          <div style={{ marginTop: 8, fontSize: 22, fontWeight: 600 }}>
                            {impact.downstream.length}
                          </div>
                        </div>
                        <div className="of-panel-muted" style={{ padding: 16 }}>
                          <p className="of-eyebrow">Propagated marking</p>
                          <div style={{ marginTop: 8, fontSize: 22, fontWeight: 600 }}>
                            {impact.propagated_marking}
                          </div>
                        </div>
                      </div>

                      {sensitiveCandidateCount > 0 && (
                        <label
                          className="of-status-warning"
                          style={{
                            display: 'flex',
                            alignItems: 'flex-start',
                            gap: 12,
                            padding: '10px 14px',
                            borderRadius: 'var(--radius-md)',
                            fontSize: 13,
                          }}
                        >
                          <input
                            type="checkbox"
                            checked={acknowledgeSensitiveLineage}
                            onChange={(e) => setAcknowledgeSensitiveLineage(e.target.checked)}
                            style={{ marginTop: 2 }}
                          />
                          <span>
                            {sensitiveCandidateCount} downstream build candidate(s) inherit
                            confidential or PII lineage. Confirm acknowledgment before dispatching
                            rebuilds.
                          </span>
                        </label>
                      )}

                      <div className="of-panel-muted" style={{ padding: 16 }}>
                        <p className="of-eyebrow">Build candidates</p>
                        <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                          {impact.build_candidates.length === 0 ? (
                            <div className="of-text-muted" style={{ fontSize: 13 }}>
                              No downstream pipelines or workflows are currently reachable from this
                              dataset.
                            </div>
                          ) : (
                            impact.build_candidates.map((candidate) => (
                              <div
                                key={candidate.id}
                                style={{
                                  border: '1px solid var(--border-default)',
                                  borderRadius: 'var(--radius-sm)',
                                  padding: 12,
                                }}
                              >
                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                                  <div>
                                    <div style={{ fontWeight: 500 }}>{candidate.label}</div>
                                    <div className="of-eyebrow" style={{ marginTop: 4 }}>
                                      {candidate.kind} · distance {candidate.distance}
                                    </div>
                                    <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                                      Node marking {candidate.marking} · Effective path marking{' '}
                                      {candidate.effective_marking}
                                    </div>
                                    {candidate.requires_acknowledgement && (
                                      <div style={{ marginTop: 6, fontSize: 12, fontWeight: 500, color: 'var(--status-warning)' }}>
                                        Sensitive lineage acknowledgment required
                                      </div>
                                    )}
                                    {candidate.blocked_reason && (
                                      <div style={{ marginTop: 6, fontSize: 12, color: 'var(--status-danger)' }}>
                                        {candidate.blocked_reason}
                                      </div>
                                    )}
                                  </div>
                                  <div style={{ textAlign: 'right' }}>
                                    <div
                                      style={{
                                        fontSize: 13,
                                        fontWeight: 500,
                                        color: candidate.triggerable ? 'var(--status-success)' : 'var(--text-muted)',
                                      }}
                                    >
                                      {candidate.status ?? 'unknown'}
                                    </div>
                                    <div style={{ fontSize: 11, color: 'var(--text-soft)' }}>
                                      {candidate.effective_marking}
                                    </div>
                                  </div>
                                </div>
                              </div>
                            ))
                          )}
                        </div>
                      </div>

                      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
                        <div className="of-panel-muted" style={{ padding: 16 }}>
                          <p className="of-eyebrow">Upstream path</p>
                          <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                            {impact.upstream.length === 0 ? (
                              <div className="of-text-muted" style={{ fontSize: 13 }}>
                                No upstream dependencies captured yet.
                              </div>
                            ) : (
                              impact.upstream.slice(0, 6).map((item) => (
                                <div
                                  key={item.id}
                                  style={{
                                    border: '1px solid var(--border-default)',
                                    borderRadius: 'var(--radius-sm)',
                                    padding: 12,
                                  }}
                                >
                                  <div style={{ fontWeight: 500 }}>{item.label}</div>
                                  <div className="of-eyebrow" style={{ marginTop: 4 }}>
                                    {item.kind} · distance {item.distance} · node {item.marking} · path {item.effective_marking}
                                  </div>
                                </div>
                              ))
                            )}
                          </div>
                        </div>
                        <div className="of-panel-muted" style={{ padding: 16 }}>
                          <p className="of-eyebrow">Downstream impact</p>
                          <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                            {impact.downstream.length === 0 ? (
                              <div className="of-text-muted" style={{ fontSize: 13 }}>
                                No downstream dependencies captured yet.
                              </div>
                            ) : (
                              impact.downstream.slice(0, 6).map((item) => (
                                <div
                                  key={item.id}
                                  style={{
                                    border: '1px solid var(--border-default)',
                                    borderRadius: 'var(--radius-sm)',
                                    padding: 12,
                                  }}
                                >
                                  <div style={{ fontWeight: 500 }}>{item.label}</div>
                                  <div className="of-eyebrow" style={{ marginTop: 4 }}>
                                    {item.kind} · distance {item.distance} · node {item.marking} · path {item.effective_marking}
                                  </div>
                                </div>
                              ))
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ) : null}

                  {buildResult && (
                    <div className="of-panel-muted" style={{ padding: 16 }}>
                      <p className="of-eyebrow">Last build dispatch</p>
                      <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                        <div className="of-text-muted" style={{ fontSize: 13 }}>
                          {buildResult.triggered.length} triggered · {buildResult.skipped.length} skipped
                        </div>
                        {[...buildResult.triggered, ...buildResult.skipped].map((item) => (
                          <div
                            key={item.id}
                            style={{
                              border: '1px solid var(--border-default)',
                              borderRadius: 'var(--radius-sm)',
                              padding: 12,
                            }}
                          >
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                              <div>
                                <div style={{ fontWeight: 500 }}>{item.label}</div>
                                <div className="of-eyebrow" style={{ marginTop: 4 }}>
                                  {item.kind}
                                </div>
                              </div>
                              <div style={{ textAlign: 'right' }}>
                                <div style={{ fontWeight: 500 }}>{item.status}</div>
                                <div style={{ fontSize: 11, color: 'var(--text-soft)' }}>
                                  {item.run_id ?? item.message ?? 'No extra details'}
                                </div>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </>
              )}
            </>
          )}
        </section>
      </div>
    </section>
  );
}
