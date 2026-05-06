import type { AuditBridgeSummary, NexusOverview, ReplicationPlan } from '@/lib/api/nexus';

interface Props {
  overview: NexusOverview | null;
  auditBridge: AuditBridgeSummary | null;
  replicationPlans: ReplicationPlan[];
}

const STATS: Array<{ key: keyof NexusOverview; label: string; bg: string; color: string }> = [
  { key: 'peer_count', label: 'Peers', bg: '#0c0a09', color: '#67e8f9' },
  { key: 'active_peer_count', label: 'Authenticated', bg: '#ecfdf5', color: '#047857' },
  { key: 'contract_count', label: 'Contracts', bg: '#f0f9ff', color: '#0369a1' },
  { key: 'share_count', label: 'Shares', bg: '#fdf4ff', color: '#a21caf' },
  { key: 'private_space_count', label: 'Private Spaces', bg: '#ecfeff', color: '#0e7490' },
  { key: 'shared_space_count', label: 'Shared Spaces', bg: '#eef2ff', color: '#4338ca' },
  { key: 'replication_ready_count', label: 'Replication Ready', bg: '#fffbeb', color: '#b45309' },
  { key: 'encrypted_share_count', label: 'Encrypted', bg: '#f5f3ff', color: '#6d28d9' },
];

export function SharingDashboard({ overview, auditBridge, replicationPlans }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#0e7490' }}>
          Nexus Dashboard
        </p>
        <h2 className="of-heading-md" style={{ marginTop: 6 }}>
          Cross-org trust, contracts, replication, and audit exchange
        </h2>
        <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
          Monitor partner authentication, data sharing posture, selective replication readiness, and cross-org audit cursors.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', marginTop: 18 }}>
        {STATS.map((stat) => {
          const value = overview ? (overview as unknown as Record<string, number>)[stat.key as string] ?? 0 : 0;
          return (
            <div key={stat.key as string} style={{ borderRadius: 16, background: stat.bg, color: stat.color, padding: 14 }}>
              <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>{stat.label}</p>
              <p style={{ marginTop: 8, fontSize: 22, fontWeight: 600 }}>{value}</p>
            </div>
          );
        })}
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <p style={{ fontWeight: 600 }}>Audit bridge</p>
            <span
              className="of-chip"
              style={
                auditBridge?.bridge_status === 'healthy'
                  ? { background: '#ecfdf5', color: '#047857' }
                  : { background: '#fffbeb', color: '#b45309' }
              }
            >
              {auditBridge?.bridge_status ?? 'pending'}
            </span>
          </div>
          <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
            {(auditBridge?.entries ?? []).map((entry) => (
              <div key={entry.share_id} className="of-panel" style={{ padding: 14 }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{entry.dataset_name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {entry.peer_name} · {entry.contract_name}
                    </p>
                  </div>
                  <span className="of-chip">{entry.status}</span>
                </div>
                <p className="of-text-muted" style={{ marginTop: 8, fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                  {entry.audit_cursor}
                </p>
              </div>
            ))}
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
          <p style={{ fontWeight: 600 }}>Selective replication plans</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
            {replicationPlans.map((plan) => (
              <div key={plan.share_id} style={{ borderRadius: 16, padding: 14, border: '1px solid #44403c', background: '#1c1917' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 500 }}>{plan.dataset_name}</p>
                    <p style={{ marginTop: 4, fontSize: 13, color: '#a8a29e' }}>
                      {plan.mode} · backlog {plan.backlog_rows}
                    </p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      plan.encrypted
                        ? { background: '#67e8f9', color: '#083344' }
                        : { background: '#fda4af', color: '#881337' }
                    }
                  >
                    {plan.status}
                  </span>
                </div>
                <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>
                  Filter {JSON.stringify(plan.selective_filter)}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
