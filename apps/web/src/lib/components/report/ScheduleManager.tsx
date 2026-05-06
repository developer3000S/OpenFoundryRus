import type { ScheduleBoard } from '@/lib/api/reports';

interface ScheduleManagerProps {
  board: ScheduleBoard | null;
  selectedReportId: string;
  busy?: boolean;
  onSelectReport: (reportId: string) => void;
  onGenerate: () => void;
}

export function ScheduleManager({
  board,
  selectedReportId,
  busy = false,
  onSelectReport,
  onGenerate,
}: ScheduleManagerProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            Schedule manager
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Upcoming runs and recent deliveries
          </h3>
        </div>
        <button
          type="button"
          className="of-btn of-btn-primary"
          onClick={onGenerate}
          disabled={busy || !selectedReportId}
        >
          Run selected report
        </button>
      </div>

      {board ? (
        <>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)', marginTop: 20 }}>
            {[
              { label: 'Active schedules', value: board.active_schedules, tone: 'of-status-warning' },
              { label: 'Paused drafts', value: board.paused_reports, tone: '' },
              {
                label: 'Recent executions',
                value: board.recent_executions.length,
                tone: 'of-status-success',
              },
            ].map((card) => (
              <div key={card.label} className={`of-panel-muted ${card.tone}`} style={{ padding: 16 }}>
                <p className="of-eyebrow">{card.label}</p>
                <p style={{ marginTop: 12, fontSize: 28, fontWeight: 600, color: 'var(--text-strong)' }}>
                  {card.value}
                </p>
              </div>
            ))}
          </div>

          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 20 }}>
            <div className="of-panel-muted" style={{ padding: 16 }}>
              <p style={{ fontWeight: 600, fontSize: 14 }}>Upcoming queue</p>
              <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                {board.upcoming.map((run) => (
                  <button
                    key={run.report_id}
                    type="button"
                    onClick={() => onSelectReport(run.report_id)}
                    style={{
                      display: 'flex',
                      alignItems: 'flex-start',
                      justifyContent: 'space-between',
                      gap: 12,
                      width: '100%',
                      textAlign: 'left',
                      padding: '12px 16px',
                      border: '1px solid var(--border-default)',
                      background: '#fff',
                      borderRadius: 'var(--radius-md)',
                      cursor: 'pointer',
                    }}
                  >
                    <div>
                      <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{run.report_name}</p>
                      <p className="of-text-muted" style={{ fontSize: 13 }}>
                        {run.cadence} • {run.generator_kind} • {run.recipient_count} targets
                      </p>
                    </div>
                    <p style={{ textAlign: 'right', fontSize: 13, color: 'var(--text-muted)' }}>
                      {new Date(run.next_run_at).toLocaleString()}
                    </p>
                  </button>
                ))}
              </div>
            </div>

            <div className="of-panel-muted" style={{ padding: 16 }}>
              <p style={{ fontWeight: 600, fontSize: 14 }}>Last five executions</p>
              <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                {board.recent_executions.map((execution) => (
                  <div
                    key={execution.id}
                    style={{
                      padding: '12px 16px',
                      border: '1px solid var(--border-default)',
                      background: '#fff',
                      borderRadius: 'var(--radius-md)',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                      <div>
                        <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{execution.report_name}</p>
                        <p className="of-text-muted" style={{ fontSize: 13 }}>
                          {execution.generator_kind} • {execution.triggered_by}
                        </p>
                      </div>
                      <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                        {new Date(execution.generated_at).toLocaleString()}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </>
      ) : (
        <p className="of-text-muted" style={{ marginTop: 20, fontSize: 13 }}>
          Schedule telemetry will appear after the first load.
        </p>
      )}
    </section>
  );
}
