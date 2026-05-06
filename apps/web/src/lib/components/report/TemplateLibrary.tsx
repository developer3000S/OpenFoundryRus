import type { ReportCatalog } from '@/lib/api/reports';

interface TemplateLibraryProps {
  catalog: ReportCatalog | null;
}

export function TemplateLibrary({ catalog }: TemplateLibraryProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            Generator catalog
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Templates, engines, and delivery channels
          </h3>
        </div>
        <p className="of-text-muted" style={{ maxWidth: 400, textAlign: 'right', fontSize: 13 }}>
          PDF and PPTX decks are simulated through the control plane, while HTML and CSV remain
          ideal for quick preview loops.
        </p>
      </div>

      {catalog ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 20 }}>
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <p style={{ fontWeight: 600, fontSize: 14 }}>Generators</p>
            {catalog.generators.map((generator) => (
              <div
                key={generator.kind}
                style={{
                  border: '1px solid var(--border-default)',
                  background: '#fff',
                  borderRadius: 'var(--radius-md)',
                  padding: 16,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <p style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {generator.display_name}
                    </p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      Engine: {generator.engine}
                    </p>
                  </div>
                  <span
                    className="of-chip of-status-warning"
                    style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 11, fontWeight: 600 }}
                  >
                    {generator.kind}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                  {generator.extensions.map((ext) => (
                    <span key={ext} className="of-chip" style={{ fontSize: 11 }}>
                      .{ext}
                    </span>
                  ))}
                </div>
                <ul style={{ marginTop: 12, paddingLeft: 16, fontSize: 13, color: 'var(--text-muted)' }}>
                  {generator.capabilities.map((cap) => (
                    <li key={cap}>{cap}</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <p style={{ fontWeight: 600, fontSize: 14 }}>Distribution channels</p>
            {catalog.delivery_channels.map((channel) => (
              <div
                key={channel.channel}
                style={{
                  border: '1px solid var(--border-default)',
                  background: '#fff',
                  borderRadius: 'var(--radius-md)',
                  padding: 16,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <p style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {channel.display_name}
                    </p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {channel.description}
                    </p>
                  </div>
                  <span className="of-chip" style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 11, fontWeight: 600 }}>
                    {channel.channel}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                  {channel.configuration_fields.map((field) => (
                    <span key={field} className="of-chip of-status-success" style={{ fontSize: 11 }}>
                      {field}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="of-text-muted" style={{ marginTop: 20, fontSize: 13 }}>
          Catalog data will appear once the report service responds.
        </p>
      )}
    </section>
  );
}
