import type {
  ComplianceReport,
  ComplianceStandard,
  GdprEraseResponse,
  GdprExportPayload,
} from '@/lib/api/audit';

export interface ReportDraft {
  standard: ComplianceStandard;
  title: string;
  scope: string;
  window_start: string;
  window_end: string;
}

export interface GdprDraft {
  subject_id: string;
  portable_format: string;
  hard_delete: boolean;
  legal_hold: boolean;
}

interface Props {
  reports: ComplianceReport[];
  reportDraft: ReportDraft;
  gdprDraft: GdprDraft;
  exportPayload: GdprExportPayload | null;
  eraseResponse: GdprEraseResponse | null;
  busy?: boolean;
  onReportDraftChange: (patch: Partial<ReportDraft>) => void;
  onGdprDraftChange: (patch: Partial<GdprDraft>) => void;
  onGenerateReport: () => void;
  onExportSubject: () => void;
  onEraseSubject: () => void;
}

const STANDARDS: ComplianceStandard[] = ['soc2', 'iso27001', 'hipaa', 'gdpr', 'itar'];

const darkInput: React.CSSProperties = {
  width: '100%',
  borderRadius: 16,
  border: '1px solid #44403c',
  background: '#1c1917',
  padding: '10px 14px',
  color: '#f5f5f4',
  fontSize: 13,
  outline: 'none',
};

export function ExportWizard({
  reports,
  reportDraft,
  gdprDraft,
  exportPayload,
  eraseResponse,
  busy = false,
  onReportDraftChange,
  onGdprDraftChange,
  onGenerateReport,
  onExportSubject,
  onEraseSubject,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#a21caf' }}>
            Export Wizard
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Compliance evidence and GDPR workflows
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Generate standard-specific evidence packs, export subject data, or request erasure/masking flows.
          </p>
        </div>
        <button type="button" onClick={onGenerateReport} disabled={busy} className="of-button of-button--primary" style={{ background: '#a21caf' }}>
          Generate report
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
          <p className="of-eyebrow">Compliance report</p>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Standard</span>
              <select
                value={reportDraft.standard}
                onChange={(e) => onReportDraftChange({ standard: e.target.value as ComplianceStandard })}
                className="of-input"
              >
                {STANDARDS.map((standard) => (
                  <option key={standard} value={standard}>
                    {standard}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Scope</span>
              <input
                value={reportDraft.scope}
                onChange={(e) => onReportDraftChange({ scope: e.target.value })}
                className="of-input"
              />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Title</span>
              <input
                value={reportDraft.title}
                onChange={(e) => onReportDraftChange({ title: e.target.value })}
                className="of-input"
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Window start</span>
              <input
                type="datetime-local"
                value={reportDraft.window_start}
                onChange={(e) => onReportDraftChange({ window_start: e.target.value })}
                className="of-input"
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Window end</span>
              <input
                type="datetime-local"
                value={reportDraft.window_end}
                onChange={(e) => onReportDraftChange({ window_end: e.target.value })}
                className="of-input"
              />
            </label>
          </div>

          <div className="of-panel" style={{ padding: 12 }}>
            <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Recent evidence packs</p>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {reports.map((report) => (
                <div key={report.id} className="of-panel-muted" style={{ padding: 12 }}>
                  <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{report.title}</p>
                  <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    {report.standard} · {report.artifact.file_name}
                  </p>
                  <p className="of-text-muted" style={{ marginTop: 6, fontSize: 11 }}>
                    {report.control_summary}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 12 }}>
          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#f0abfc' }}>
              GDPR actions
            </p>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Subject ID</span>
                <input
                  value={gdprDraft.subject_id}
                  onChange={(e) => onGdprDraftChange({ subject_id: e.target.value })}
                  style={darkInput}
                />
              </label>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Portable format</span>
                <input
                  value={gdprDraft.portable_format}
                  onChange={(e) => onGdprDraftChange({ portable_format: e.target.value })}
                  style={darkInput}
                />
              </label>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, padding: '10px 14px', borderRadius: 16, border: '1px solid #44403c', background: '#1c1917', fontSize: 13 }}>
                <input
                  type="checkbox"
                  checked={gdprDraft.hard_delete}
                  onChange={(e) => onGdprDraftChange({ hard_delete: e.target.checked })}
                />
                Hard delete
              </label>
              <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, padding: '10px 14px', borderRadius: 16, border: '1px solid #44403c', background: '#1c1917', fontSize: 13 }}>
                <input
                  type="checkbox"
                  checked={gdprDraft.legal_hold}
                  onChange={(e) => onGdprDraftChange({ legal_hold: e.target.checked })}
                />
                Legal hold
              </label>
            </div>
            <div style={{ marginTop: 14, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              <button type="button" onClick={onExportSubject} disabled={busy} className="of-button of-button--primary" style={{ background: '#06b6d4', color: '#0c0a09' }}>
                Export subject
              </button>
              <button type="button" onClick={onEraseSubject} disabled={busy} className="of-button of-button--primary" style={{ background: '#f43f5e' }}>
                Erase subject
              </button>
            </div>
          </div>

          {exportPayload && (
            <div style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
              <p style={{ fontWeight: 500 }}>Portable export</p>
              <p style={{ marginTop: 6, fontSize: 13, color: '#a8a29e' }}>
                {exportPayload.subject_id} · {exportPayload.event_count} events · {exportPayload.portable_format}
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                {exportPayload.resources.map((resource) => (
                  <span key={resource} className="of-chip" style={{ background: '#292524', color: '#d6d3d1' }}>
                    {resource}
                  </span>
                ))}
              </div>
            </div>
          )}

          {eraseResponse && (
            <div style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
              <p style={{ fontWeight: 500 }}>Erasure response</p>
              <p style={{ marginTop: 6, fontSize: 13, color: '#a8a29e' }}>
                {eraseResponse.status} · {eraseResponse.masked_event_count} events · legal hold {eraseResponse.legal_hold ? 'enabled' : 'disabled'}
              </p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
