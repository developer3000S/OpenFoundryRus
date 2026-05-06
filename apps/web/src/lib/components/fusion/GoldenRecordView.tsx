import type { ClusterDetail, GoldenRecord } from '@/lib/api/fusion';

interface Props {
  goldenRecords: GoldenRecord[];
  clusterDetail: ClusterDetail | null;
}

export function GoldenRecordView({ goldenRecords, clusterDetail }: Props) {
  const activeGoldenRecord = clusterDetail?.golden_record ?? goldenRecords[0] ?? null;

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow">Golden Records</p>
        <h2 className="of-heading-md" style={{ marginTop: 6 }}>
          Canonical identities and provenance trails
        </h2>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.8fr) minmax(0, 1.2fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 10 }}>
          {goldenRecords.length === 0 ? (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              Golden records appear here after a resolution run.
            </div>
          ) : (
            goldenRecords.map((goldenRecord) => {
              const active = activeGoldenRecord?.id === goldenRecord.id;
              return (
                <div
                  key={goldenRecord.id}
                  style={{
                    padding: 14,
                    border: `1px solid ${active ? '#06b6d4' : 'var(--border-default)'}`,
                    background: active ? '#ecfeff' : 'var(--bg-subtle)',
                    borderRadius: 16,
                  }}
                >
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{goldenRecord.title}</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                    completeness {goldenRecord.completeness_score.toFixed(2)} · confidence {goldenRecord.confidence_score.toFixed(2)}
                  </div>
                </div>
              );
            })
          )}
        </div>

        <div>
          {activeGoldenRecord ? (
            <div className="of-panel-muted" style={{ padding: 16 }}>
              <div className="of-eyebrow">Selected Golden Record</div>
              <h3 className="of-heading-md" style={{ marginTop: 6 }}>
                {activeGoldenRecord.title}
              </h3>
              <pre
                style={{
                  marginTop: 16,
                  overflowX: 'auto',
                  background: '#0c0a09',
                  color: '#67e8f9',
                  padding: 14,
                  borderRadius: 16,
                  fontSize: 11,
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {JSON.stringify(activeGoldenRecord.canonical_values, null, 2)}
              </pre>
              <div style={{ display: 'grid', gap: 8, marginTop: 14 }}>
                {activeGoldenRecord.provenance.map((item, index) => (
                  <div key={index} className="of-panel" style={{ padding: 12, fontSize: 13 }}>
                    {item.field}: {item.source} · {item.external_id} · {item.strategy}
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              Select or generate a golden record to inspect canonical values.
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
