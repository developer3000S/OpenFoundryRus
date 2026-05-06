import type { DownloadPayload, ReportExecution } from '@/lib/api/reports';

interface ReportPreviewProps {
  execution: ReportExecution | null;
  download: DownloadPayload | null;
}

export function ReportPreview({ execution, download }: ReportPreviewProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            Preview
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Execution preview and generated artifact
          </h3>
        </div>
        {download && (
          <a className="of-btn" href={download.storage_url} target="_blank" rel="noreferrer">
            Open artifact
          </a>
        )}
      </div>

      {execution ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.15fr) minmax(0, 0.85fr)', marginTop: 20 }}>
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 16 }}>
            <div>
              <p style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-strong)' }}>
                {execution.preview.headline}
              </p>
              <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                {execution.preview.generated_for} • {execution.preview.engine}
              </p>
            </div>

            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)' }}>
              {execution.preview.highlights.map((highlight) => (
                <div key={highlight.label} className="of-panel" style={{ padding: 16 }}>
                  <p className="of-eyebrow">{highlight.label}</p>
                  <p style={{ marginTop: 12, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                    {highlight.value}
                  </p>
                  <p style={{ marginTop: 4, fontSize: 13, color: 'var(--status-success)' }}>
                    {highlight.delta}
                  </p>
                </div>
              ))}
            </div>

            <div style={{ display: 'grid', gap: 12 }}>
              {execution.preview.sections.map((section) => (
                <div key={section.section_id} className="of-panel" style={{ padding: 16 }}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                    <div>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{section.title}</p>
                      <p className="of-text-muted" style={{ fontSize: 13 }}>
                        {section.kind}
                      </p>
                    </div>
                    <p style={{ maxWidth: 320, textAlign: 'right', fontSize: 13, color: 'var(--text-muted)' }}>
                      {section.summary}
                    </p>
                  </div>
                  <pre
                    className="of-scrollbar"
                    style={{
                      marginTop: 12,
                      overflowX: 'auto',
                      borderRadius: 'var(--radius-md)',
                      background: '#0c0a09',
                      padding: 12,
                      fontSize: 11,
                      color: '#fde68a',
                      fontFamily: 'var(--font-mono)',
                    }}
                  >
                    {JSON.stringify(section.rows, null, 2)}
                  </pre>
                </div>
              ))}
            </div>
          </div>

          <div
            style={{
              display: 'grid',
              gap: 16,
              padding: 16,
              background: '#0c0a09',
              borderRadius: 'var(--radius-md)',
              color: '#f5f5f4',
            }}
          >
            <div>
              <p className="of-eyebrow" style={{ color: '#fbbf24' }}>
                Artifact
              </p>
              <p style={{ marginTop: 8, fontSize: 16, fontWeight: 600 }}>{execution.artifact.file_name}</p>
              <p style={{ marginTop: 4, fontSize: 13, color: '#d6d3d1' }}>
                {execution.artifact.mime_type} • {execution.artifact.size_bytes.toLocaleString()} bytes
              </p>
            </div>
            <div style={{ display: 'grid', gap: 12 }}>
              <div style={{ border: '1px solid #292524', background: '#1c1917', borderRadius: 'var(--radius-md)', padding: 16 }}>
                <p className="of-eyebrow" style={{ color: '#a8a29e' }}>
                  Generation metrics
                </p>
                <p style={{ marginTop: 12, fontSize: 13, color: '#e7e5e4' }}>
                  {execution.metrics.row_count.toLocaleString()} rows
                </p>
                <p style={{ fontSize: 13, color: '#e7e5e4' }}>{execution.metrics.section_count} sections</p>
                <p style={{ fontSize: 13, color: '#e7e5e4' }}>{execution.metrics.duration_ms} ms</p>
              </div>
              <div style={{ border: '1px solid #292524', background: '#1c1917', borderRadius: 'var(--radius-md)', padding: 16 }}>
                <p className="of-eyebrow" style={{ color: '#a8a29e' }}>
                  Distribution
                </p>
                <div style={{ display: 'grid', gap: 8, marginTop: 12, fontSize: 13, color: '#e7e5e4' }}>
                  {execution.distributions.map((delivery, index) => (
                    <div key={`${delivery.channel}-${index}`}>
                      <p style={{ fontWeight: 500 }}>
                        {delivery.channel} → {delivery.target}
                      </p>
                      <p style={{ color: '#a8a29e' }}>{delivery.detail}</p>
                    </div>
                  ))}
                </div>
              </div>
            </div>
            {download && (
              <div style={{ border: '1px solid #292524', background: '#1c1917', borderRadius: 'var(--radius-md)', padding: 16, fontSize: 13, color: '#d6d3d1' }}>
                <p style={{ fontWeight: 600, color: '#f5f5f4' }}>Download payload</p>
                <p style={{ marginTop: 8 }}>{download.preview_excerpt}</p>
                <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e', wordBreak: 'break-all' }}>
                  {download.storage_url}
                </p>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div
          style={{
            marginTop: 20,
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
            padding: 32,
            textAlign: 'center',
            fontSize: 13,
            color: 'var(--text-muted)',
          }}
        >
          Run a report or pick a previous execution to populate the preview.
        </div>
      )}
    </section>
  );
}
