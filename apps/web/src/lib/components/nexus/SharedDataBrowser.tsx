import type { AuditBridgeSummary, FederatedQueryResult, ReplicationPlan, ShareDetail } from '@/lib/api/nexus';

export interface QueryDraft {
  share_id: string;
  sql: string;
  purpose: string;
  limit: string;
}

interface Props {
  shares: ShareDetail[];
  selectedShareId: string;
  selectedShare: ShareDetail | null;
  replicationPlans: ReplicationPlan[];
  auditBridge: AuditBridgeSummary | null;
  queryDraft: QueryDraft;
  queryResult: FederatedQueryResult | null;
  busy?: boolean;
  onSelectShare: (shareId: string) => void;
  onQueryDraftChange: (patch: Partial<QueryDraft>) => void;
  onRunQuery: () => void;
}

export function SharedDataBrowser({
  shares,
  selectedShareId,
  selectedShare,
  replicationPlans,
  auditBridge,
  queryDraft,
  queryResult,
  busy = false,
  onSelectShare,
  onQueryDraftChange,
  onRunQuery,
}: Props) {
  const selectedPlan = replicationPlans.find((plan) => plan.share_id === selectedShareId) ?? null;
  const bridgeEntry = auditBridge?.entries.find((entry) => entry.share_id === selectedShareId) ?? null;

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#7c3aed' }}>
          Shared Data Browser
        </p>
        <h3 className="of-heading-md" style={{ marginTop: 6 }}>
          Federated query, schema review, and encryption posture
        </h3>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Browse shared datasets, inspect compatibility and transport posture, and run federated preview queries without copying the source.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.82fr) minmax(0, 1.18fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {shares.map((item) => {
            const active = selectedShareId === item.share.id;
            const allEncrypted = item.encryption.encrypted_in_transit && item.encryption.encrypted_at_rest;
            return (
              <button
                key={item.share.id}
                type="button"
                onClick={() => onSelectShare(item.share.id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 14,
                  border: `1px solid ${active ? '#7c3aed' : 'var(--border-default)'}`,
                  background: active ? '#f5f3ff' : 'var(--bg-elevated)',
                  borderRadius: 16,
                  cursor: 'pointer',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{item.share.dataset_name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {item.share.replication_mode} · {item.access_grant?.query_template ?? 'No grant'}
                    </p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      allEncrypted
                        ? { background: '#ecfeff', color: '#0e7490' }
                        : { background: '#fef2f2', color: '#b91c1c' }
                    }
                  >
                    {item.encryption.profile}
                  </span>
                </div>
              </button>
            );
          })}
        </div>

        <div style={{ display: 'grid', gap: 12 }}>
          {selectedShare && (
            <>
              <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
                  <div style={{ borderRadius: 16, padding: 14, border: '1px solid #44403c', background: '#1c1917' }}>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#c4b5fd' }}>
                      Schema compatibility
                    </p>
                    <p style={{ marginTop: 8, fontSize: 13 }}>{selectedShare.compatibility.summary}</p>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                      {selectedShare.compatibility.missing_fields.map((field) => (
                        <span key={field} className="of-chip" style={{ background: '#fcd34d', color: '#78350f' }}>
                          Missing {field}
                        </span>
                      ))}
                      {selectedShare.compatibility.type_mismatches.map((mismatch) => (
                        <span key={mismatch} className="of-chip" style={{ background: '#fda4af', color: '#881337' }}>
                          {mismatch}
                        </span>
                      ))}
                    </div>
                  </div>

                  <div style={{ borderRadius: 16, padding: 14, border: '1px solid #44403c', background: '#1c1917' }}>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#67e8f9' }}>
                      Encryption posture
                    </p>
                    <p style={{ marginTop: 8, fontSize: 13 }}>
                      {selectedShare.encryption.transport_cipher} · {selectedShare.encryption.at_rest_cipher}
                    </p>
                    <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>
                      {selectedShare.encryption.key_version} · {selectedShare.encryption.recommendation}
                    </p>
                  </div>
                </div>

                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 12 }}>
                  <div style={{ borderRadius: 16, padding: 14, border: '1px solid #44403c', background: '#1c1917' }}>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#a8a29e' }}>
                      Replication plan
                    </p>
                    <p style={{ marginTop: 8, fontSize: 13 }}>
                      {selectedPlan?.status ?? 'pending'} · backlog {selectedPlan?.backlog_rows ?? 0}
                    </p>
                    <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>
                      Filter {selectedPlan ? JSON.stringify(selectedPlan.selective_filter) : 'n/a'}
                    </p>
                  </div>
                  <div style={{ borderRadius: 16, padding: 14, border: '1px solid #44403c', background: '#1c1917' }}>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#a8a29e' }}>
                      Audit bridge
                    </p>
                    <p style={{ marginTop: 8, fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                      {bridgeEntry?.audit_cursor ?? 'cursor/pending'}
                    </p>
                    <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>
                      {bridgeEntry?.contract_name ?? 'No audit evidence linked yet'}
                    </p>
                  </div>
                </div>
              </div>

              <div className="of-panel-muted" style={{ padding: 14 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Federated query preview</p>
                  <button type="button" onClick={onRunQuery} disabled={busy} className="of-button of-button--primary" style={{ background: '#7c3aed' }}>
                    Run query
                  </button>
                </div>

                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 12 }}>
                  <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                    <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>SQL</span>
                    <textarea
                      value={queryDraft.sql}
                      onChange={(e) => onQueryDraftChange({ sql: e.target.value })}
                      className="of-input"
                      style={{ minHeight: 80, fontFamily: 'var(--font-mono)', fontSize: 11, resize: 'vertical' }}
                    />
                  </label>
                  <label style={{ fontSize: 13 }}>
                    <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Purpose</span>
                    <input
                      value={queryDraft.purpose}
                      onChange={(e) => onQueryDraftChange({ purpose: e.target.value })}
                      className="of-input"
                    />
                  </label>
                  <label style={{ fontSize: 13 }}>
                    <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Limit</span>
                    <input
                      value={queryDraft.limit}
                      onChange={(e) => onQueryDraftChange({ limit: e.target.value })}
                      className="of-input"
                    />
                  </label>
                </div>

                {queryResult && (
                  <div style={{ marginTop: 14, overflow: 'hidden', border: '1px solid var(--border-default)', borderRadius: 16 }}>
                    <div style={{ borderBottom: '1px solid var(--border-default)', padding: '8px 14px', fontSize: 13, color: 'var(--text-muted)' }}>
                      {queryResult.source_peer} · {queryResult.dataset_name}
                    </div>
                    <div style={{ overflowX: 'auto' }}>
                      <table style={{ minWidth: '100%', fontSize: 13, textAlign: 'left' }}>
                        <thead style={{ background: 'var(--bg-subtle)' }}>
                          <tr>
                            {queryResult.columns.map((column) => (
                              <th key={column} style={{ padding: '10px 14px', fontWeight: 500, color: 'var(--text-muted)' }}>
                                {column}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {queryResult.rows.map((row, index) => (
                            <tr key={index} style={{ borderTop: '1px solid var(--border-default)' }}>
                              {queryResult.columns.map((column) => (
                                <td key={column} style={{ padding: '10px 14px', color: 'var(--text-default)' }}>
                                  {String(row[column] ?? '')}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </section>
  );
}
