import type { AnomalyAlert, AuditOverview, CollectorStatus, ComplianceReport } from '@/lib/api/audit';

interface Props {
  overview: AuditOverview | null;
  collectors: CollectorStatus[];
  anomalies: AnomalyAlert[];
  reports: ComplianceReport[];
}

const STATS: Array<{ key: string; label: string; bg: string; color: string }> = [
  { key: 'event_count', label: 'Events', bg: '#0c0a09', color: '#5eead4' },
  { key: 'critical_event_count', label: 'Critical', bg: '#fef2f2', color: '#b91c1c' },
  { key: 'collector_count', label: 'Collectors', bg: '#f0f9ff', color: '#0369a1' },
  { key: 'active_policy_count', label: 'Policies', bg: '#fffbeb', color: '#b45309' },
  { key: 'anomaly_count', label: 'Anomalies', bg: '#f5f3ff', color: '#6d28d9' },
  { key: 'gdpr_subject_count', label: 'GDPR Subjects', bg: '#ecfdf5', color: '#047857' },
];

export function ComplianceDashboard({ overview, collectors, anomalies, reports }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#0f766e' }}>
          Compliance Dashboard
        </p>
        <h2 className="of-heading-md" style={{ marginTop: 6 }}>
          Integrity, anomalies, collectors, and evidence packs
        </h2>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Monitor the append-only audit chain, NATS collector health, critical events, and compliance exports.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', marginTop: 18 }}>
        {STATS.map((stat) => {
          const value = overview ? (overview as unknown as Record<string, number>)[stat.key] ?? 0 : 0;
          return (
            <div key={stat.key} style={{ borderRadius: 16, padding: 14, background: stat.bg, color: stat.color }}>
              <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>{stat.label}</p>
              <p style={{ marginTop: 8, fontSize: 22, fontWeight: 600 }}>{value}</p>
            </div>
          );
        })}
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Collector health</p>
            <p className="of-eyebrow">NATS subjects</p>
          </div>
          <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
            {collectors.map((collector) => (
              <div key={collector.subject} className="of-panel" style={{ padding: 12 }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 500 }}>{collector.service_name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>{collector.subject}</p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      collector.connected
                        ? { background: '#ecfdf5', color: '#047857' }
                        : { background: '#fffbeb', color: '#b45309' }
                    }
                  >
                    {collector.health}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <span className="of-chip">Backlog {collector.backlog_depth}</span>
                  <span className="of-chip">Next pull {new Date(collector.next_pull_at).toLocaleTimeString()}</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 12 }}>
          <div>
            <p style={{ fontWeight: 600 }}>Anomaly alerts</p>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {anomalies.map((anomaly, index) => (
                <div key={index} style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 500 }}>{anomaly.title}</p>
                      <p style={{ marginTop: 4, fontSize: 13, color: '#a8a29e' }}>{anomaly.description}</p>
                    </div>
                    <span className="of-chip" style={{ background: '#fda4af', color: '#881337' }}>
                      {anomaly.severity}
                    </span>
                  </div>
                  <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>{anomaly.recommended_action}</p>
                </div>
              ))}
            </div>
          </div>

          <div>
            <p style={{ fontWeight: 600 }}>Recent reports</p>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {reports.slice(0, 3).map((report) => (
                <div key={report.id} style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 500 }}>{report.title}</p>
                      <p style={{ fontSize: 11, color: '#a8a29e' }}>
                        {report.standard} · {report.scope}
                      </p>
                    </div>
                    <span className="of-chip" style={{ background: '#5eead4', color: '#134e4a' }}>
                      {report.status}
                    </span>
                  </div>
                  <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>{report.control_summary}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
