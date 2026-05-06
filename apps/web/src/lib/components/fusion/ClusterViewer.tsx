import type { ClusterDetail, ResolvedCluster } from '@/lib/api/fusion';

interface Props {
  clusters: ResolvedCluster[];
  selectedClusterId: string;
  clusterDetail: ClusterDetail | null;
  busy?: boolean;
  onSelectCluster?: (clusterId: string) => void;
}

export function ClusterViewer({ clusters, selectedClusterId, clusterDetail, busy = false, onSelectCluster }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow">Cluster Viewer</p>
        <h2 className="of-heading-md" style={{ marginTop: 6 }}>
          Transitive clusters, pair evidence, and confidence
        </h2>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.75fr) minmax(0, 1.25fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 10 }}>
          {clusters.length === 0 ? (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              Run a resolution job to generate clusters.
            </div>
          ) : (
            clusters.map((cluster) => {
              const active = selectedClusterId === cluster.id;
              return (
                <button
                  key={cluster.id}
                  type="button"
                  disabled={busy}
                  onClick={() => onSelectCluster?.(cluster.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 14,
                    border: `1px solid ${active ? '#06b6d4' : 'var(--border-default)'}`,
                    background: active ? '#ecfeff' : 'var(--bg-subtle)',
                    borderRadius: 16,
                    cursor: busy ? 'not-allowed' : 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                        {cluster.records.length} records
                      </div>
                      <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                        {cluster.status} · confidence {cluster.confidence_score.toFixed(2)}
                      </div>
                    </div>
                    {cluster.requires_review && (
                      <span
                        className="of-chip"
                        style={{ background: '#fef3c7', color: '#b45309', textTransform: 'uppercase', letterSpacing: '0.18em' }}
                      >
                        Review
                      </span>
                    )}
                  </div>
                </button>
              );
            })
          )}
        </div>

        <div style={{ display: 'grid', gap: 12 }}>
          {clusterDetail ? (
            <div className="of-panel-muted" style={{ padding: 16 }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <div className="of-eyebrow">Selected Cluster</div>
                  <h3 className="of-heading-md" style={{ marginTop: 6 }}>
                    {clusterDetail.cluster.cluster_key}
                  </h3>
                </div>
                <div className="of-chip">{clusterDetail.cluster.status}</div>
              </div>

              <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', marginTop: 16 }}>
                <div>
                  <div className="of-eyebrow">Records</div>
                  <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                    {clusterDetail.cluster.records.map((record) => (
                      <div key={record.record_id} className="of-panel" style={{ padding: 12 }}>
                        <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                          {record.display_name}
                        </div>
                        <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                          {record.source} · {record.external_id} · confidence {record.confidence.toFixed(2)}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <div>
                  <div className="of-eyebrow">Pair Evidence</div>
                  <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                    {clusterDetail.cluster.evidence.length === 0 ? (
                      <p className="of-text-muted" style={{ fontSize: 13 }}>
                        No pair evidence captured for this cluster.
                      </p>
                    ) : (
                      clusterDetail.cluster.evidence.map((evidence, index) => (
                        <div key={index} className="of-panel" style={{ padding: 12 }}>
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, fontSize: 13 }}>
                            <span>
                              {evidence.left_record_id} ↔ {evidence.right_record_id}
                            </span>
                            <span style={{ fontWeight: 600 }}>{evidence.final_score.toFixed(2)}</span>
                          </div>
                          <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                            {evidence.explanation}
                          </p>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              Select a cluster to inspect its records and pairwise evidence.
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
