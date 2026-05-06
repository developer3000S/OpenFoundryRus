import type { AuditEvent } from '@/lib/api/audit';

interface Props {
  events: AuditEvent[];
}

export function AuditTimeline({ events }: Props) {
  const timeline = [...events].reverse();
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#7c3aed' }}>
          Audit Timeline
        </p>
        <h3 className="of-heading-md" style={{ marginTop: 6 }}>
          Hash chain and sequence integrity
        </h3>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Visualize the append-only chain from oldest to newest entry hash.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 10, marginTop: 18 }}>
        {timeline.map((event) => (
          <div key={event.id} className="of-panel-muted" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
              <div>
                <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Sequence {event.sequence}</p>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {event.action} · {new Date(event.occurred_at).toLocaleString()}
                </p>
              </div>
              <span className="of-chip" style={{ background: '#f5f3ff', color: '#7c3aed' }}>
                {event.channel}
              </span>
            </div>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr', marginTop: 12 }}>
              <div className="of-panel" style={{ padding: 10 }}>
                <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
                  Previous hash
                </p>
                <p style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-default)', wordBreak: 'break-all' }}>
                  {event.previous_hash}
                </p>
              </div>
              <div style={{ borderRadius: 16, padding: 10, background: '#0c0a09', color: '#f5f5f4' }}>
                <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#c4b5fd' }}>
                  Entry hash
                </p>
                <p style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, wordBreak: 'break-all' }}>
                  {event.entry_hash}
                </p>
              </div>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
