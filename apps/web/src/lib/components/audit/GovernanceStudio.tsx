import type {
  CompliancePostureOverview,
  GovernanceTemplate,
  GovernanceTemplateApplication,
  SensitiveDataScanResponse,
} from '@/lib/api/audit';

export interface GovernanceTemplateDraft {
  scope: string;
  updated_by: string;
  scan_text: string;
}

interface Props {
  templates: GovernanceTemplate[];
  applications: GovernanceTemplateApplication[];
  posture: CompliancePostureOverview | null;
  scanResult: SensitiveDataScanResponse | null;
  draft: GovernanceTemplateDraft;
  busy?: boolean;
  onDraftChange: (patch: Partial<GovernanceTemplateDraft>) => void;
  onApplyTemplate: (slug: string) => void;
  onScan: () => void;
}

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

export function GovernanceStudio({
  templates,
  applications,
  posture,
  scanResult,
  draft,
  busy = false,
  onDraftChange,
  onApplyTemplate,
  onScan,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#047857' }}>
            Governance Studio
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Templates, compliance posture, and SDS remediation
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Apply governance baselines by scope, monitor coverage by standard, and run sensitive-data scans from one surface.
          </p>
        </div>
        <div style={{ borderRadius: 16, padding: 14, background: '#ecfdf5', color: '#064e3b' }}>
          <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em' }}>Template apps</p>
          <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>
            {posture?.active_template_application_count ?? applications.length}
          </p>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.1fr) minmax(0, 0.9fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 12 }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            {(posture?.standards ?? []).map((standard) => (
              <div key={standard.standard} className="of-panel-muted" style={{ padding: 14 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                  <div style={{ fontSize: 13, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-strong)' }}>
                    {standard.standard}
                  </div>
                  <div style={{ borderRadius: 999, padding: '4px 10px', fontSize: 11, fontWeight: 600, background: '#0c0a09', color: '#f5f5f4' }}>
                    {standard.coverage_score}%
                  </div>
                </div>
                <p className="of-text-muted" style={{ marginTop: 8, fontSize: 11 }}>
                  {standard.evidence_summary}
                </p>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <span className="of-chip">{standard.applied_scope_count} scopes</span>
                  <span className="of-chip">{standard.active_policy_count} policies</span>
                  <span className="of-chip">{standard.checkpoint_prompt_count} prompts</span>
                </div>
              </div>
            ))}
          </div>

          <div className="of-panel-muted" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Applied governance templates</p>
              <p className="of-eyebrow">Auditable by scope</p>
            </div>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {applications.map((application) => (
                <div key={application.id} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{application.template_name}</p>
                      <p className="of-text-muted" style={{ fontSize: 13 }}>
                        {application.scope} · {application.applied_by}
                      </p>
                    </div>
                    <span className="of-chip" style={{ background: '#ecfdf5', color: '#047857', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                      {application.default_report_standard}
                    </span>
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                    {application.standards.map((standard) => (
                      <span key={standard} className="of-chip">
                        {standard}
                      </span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 12 }}>
          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#6ee7b7' }}>
              Apply template
            </p>
            <div style={{ display: 'grid', gap: 12, marginTop: 14 }}>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Scope / project</span>
                <input value={draft.scope} onChange={(e) => onDraftChange({ scope: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Applied by</span>
                <input value={draft.updated_by} onChange={(e) => onDraftChange({ updated_by: e.target.value })} style={darkInput} />
              </label>
            </div>
          </div>

          <div style={{ display: 'grid', gap: 8 }}>
            {templates.map((template) => (
              <div key={template.slug} style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 500 }}>{template.name}</p>
                    <p style={{ marginTop: 4, fontSize: 13, color: '#a8a29e' }}>{template.summary}</p>
                  </div>
                  <button
                    type="button"
                    onClick={() => onApplyTemplate(template.slug)}
                    disabled={busy}
                    className="of-button of-button--primary"
                    style={{ background: '#34d399', color: '#0c0a09', fontSize: 11 }}
                  >
                    Apply
                  </button>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  {template.standards.map((standard) => (
                    <span key={standard} className="of-chip" style={{ background: '#292524', color: '#d6d3d1' }}>
                      {standard}
                    </span>
                  ))}
                  <span className="of-chip" style={{ background: '#022c22', color: '#6ee7b7' }}>
                    report {template.default_report_standard}
                  </span>
                </div>
                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 10 }}>
                  <div>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#a8a29e' }}>
                      Checkpoint prompts
                    </p>
                    <ul style={{ margin: '8px 0 0', paddingLeft: 16, fontSize: 11, color: '#d6d3d1', display: 'grid', gap: 4 }}>
                      {template.checkpoint_prompts.map((prompt) => (
                        <li key={prompt}>{prompt}</li>
                      ))}
                    </ul>
                  </div>
                  <div>
                    <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#a8a29e' }}>
                      SDS remediations
                    </p>
                    <ul style={{ margin: '8px 0 0', paddingLeft: 16, fontSize: 11, color: '#d6d3d1', display: 'grid', gap: 4 }}>
                      {template.sds_remediations.map((remediation) => (
                        <li key={remediation}>{remediation}</li>
                      ))}
                    </ul>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#6ee7b7' }}>
              Sensitive Data Scanner
            </p>
            <textarea
              value={draft.scan_text}
              onChange={(e) => onDraftChange({ scan_text: e.target.value })}
              style={{ ...darkInput, marginTop: 10, minHeight: 100, resize: 'vertical' }}
            />
            <div style={{ marginTop: 10, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <p style={{ fontSize: 11, color: '#a8a29e' }}>Run a remediation-oriented scan over sample payloads.</p>
              <button
                type="button"
                onClick={onScan}
                disabled={busy}
                className="of-button"
                style={{ borderColor: '#34d399', color: '#6ee7b7', background: 'transparent', fontSize: 13 }}
              >
                Scan
              </button>
            </div>
            {scanResult && (
              <div style={{ marginTop: 14, borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#0c0a09' }}>
                <p style={{ fontWeight: 500 }}>Risk score {scanResult.risk_score}</p>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  {scanResult.findings.map((finding, index) => (
                    <span key={index} className="of-chip" style={{ background: '#450a0a', color: '#fda4af' }}>
                      {finding.kind} × {finding.match_count}
                    </span>
                  ))}
                </div>
                <p style={{ marginTop: 8, fontSize: 11, color: '#a8a29e' }}>{scanResult.redacted_content}</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
