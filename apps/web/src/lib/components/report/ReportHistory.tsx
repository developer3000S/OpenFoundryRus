import type { ReportExecution } from '@/lib/api/reports';

interface ReportHistoryProps {
  history: ReportExecution[];
  onSelectExecution: (executionId: string) => void;
}

export function ReportHistory({ history, onSelectExecution }: ReportHistoryProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            History
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Execution timeline
          </h3>
        </div>
        <p className="of-text-muted" style={{ fontSize: 13 }}>
          Select a run to inspect its preview and artifact metadata.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 12, marginTop: 20 }}>
        {history.length > 0 ? (
          history.map((execution) => (
            <button
              key={execution.id}
              type="button"
              onClick={() => onSelectExecution(execution.id)}
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                justifyContent: 'space-between',
                gap: 12,
                width: '100%',
                textAlign: 'left',
                padding: 16,
                border: '1px solid var(--border-default)',
                background: 'var(--bg-panel-muted)',
                borderRadius: 'var(--radius-md)',
                cursor: 'pointer',
              }}
            >
              <div>
                <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{execution.report_name}</p>
                <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                  {execution.generator_kind} • {execution.status} •{' '}
                  {execution.metrics.row_count.toLocaleString()} rows
                </p>
              </div>
              <div style={{ textAlign: 'right' }}>
                <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                  {new Date(execution.generated_at).toLocaleString()}
                </p>
                <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                  {execution.metrics.duration_ms} ms
                </p>
              </div>
            </button>
          ))
        ) : (
          <div
            style={{
              border: '1px dashed var(--border-default)',
              borderRadius: 'var(--radius-md)',
              padding: 32,
              textAlign: 'center',
              fontSize: 13,
              color: 'var(--text-muted)',
            }}
          >
            No executions recorded yet for the selected report.
          </div>
        )}
      </div>
    </section>
  );
}
