import type { GeneratorKind, ReportDefinition } from '@/lib/api/reports';

export interface ReportDraft {
  id?: string;
  name: string;
  description: string;
  owner: string;
  generator_kind: GeneratorKind;
  dataset_name: string;
  active: boolean;
  tags_text: string;
  schedule_text: string;
  template_text: string;
  recipients_text: string;
}

const GENERATORS: GeneratorKind[] = ['pdf', 'excel', 'csv', 'html', 'pptx'];

interface ReportDesignerProps {
  reports: ReportDefinition[];
  selectedReportId: string;
  draft: ReportDraft;
  busy?: boolean;
  onSelect: (reportId: string) => void;
  onDraftChange: (patch: Partial<ReportDraft>) => void;
  onSave: () => void;
  onReset: () => void;
}

export function ReportDesigner({
  reports,
  selectedReportId,
  draft,
  busy = false,
  onSelect,
  onDraftChange,
  onSave,
  onReset,
}: ReportDesignerProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            Report designer
          </p>
          <h2 className="of-heading-md" style={{ marginTop: 4 }}>
            Definitions, template payloads, and scheduling
          </h2>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Select an existing report or shape a new one with generator, template, schedule, and
            delivery bindings.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="of-btn" onClick={onReset} disabled={busy}>
            New draft
          </button>
          <button type="button" className="of-btn of-btn-primary" onClick={onSave} disabled={busy}>
            {draft.id ? 'Update report' : 'Create report'}
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1.15fr 0.85fr', marginTop: 16 }}>
        <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
          <label style={{ display: 'block', fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
            Existing definitions
            <select
              className="of-select"
              value={selectedReportId}
              onChange={(e) => onSelect(e.target.value)}
              style={{ marginTop: 8, fontSize: 13 }}
            >
              <option value="">Create a new definition</option>
              {reports.map((report) => (
                <option key={report.id} value={report.id}>
                  {report.name}
                </option>
              ))}
            </select>
          </label>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <Field label="Name">
              <input
                className="of-input"
                value={draft.name}
                onChange={(e) => onDraftChange({ name: e.target.value })}
              />
            </Field>
            <Field label="Owner">
              <input
                className="of-input"
                value={draft.owner}
                onChange={(e) => onDraftChange({ owner: e.target.value })}
              />
            </Field>
            <Field label="Description" fullWidth>
              <textarea
                className="of-textarea"
                value={draft.description}
                onChange={(e) => onDraftChange({ description: e.target.value })}
                style={{ minHeight: 96 }}
              />
            </Field>
            <Field label="Generator">
              <select
                className="of-select"
                value={draft.generator_kind}
                onChange={(e) => onDraftChange({ generator_kind: e.target.value as GeneratorKind })}
              >
                {GENERATORS.map((generator) => (
                  <option key={generator} value={generator}>
                    {generator.toUpperCase()}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Dataset">
              <input
                className="of-input"
                value={draft.dataset_name}
                onChange={(e) => onDraftChange({ dataset_name: e.target.value })}
              />
            </Field>
            <Field label="Tags" fullWidth>
              <input
                className="of-input"
                value={draft.tags_text}
                onChange={(e) => onDraftChange({ tags_text: e.target.value })}
                placeholder="executive, weekly, revenue"
              />
            </Field>
            <label style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', border: '1px solid var(--border-default)', background: '#fff', borderRadius: 'var(--radius-md)', fontSize: 13, gridColumn: '1 / -1' }}>
              <input
                type="checkbox"
                checked={draft.active}
                onChange={(e) => onDraftChange({ active: e.target.checked })}
              />
              <span>Definition active</span>
            </label>
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
              Structured payloads
            </p>
            <p style={{ marginTop: 8, fontSize: 13, color: '#d6d3d1' }}>
              Use JSON to control template sections, schedule cadence, and distribution recipients.
            </p>
          </div>
          {(['template_text', 'schedule_text', 'recipients_text'] as const).map((field) => (
            <label key={field} style={{ display: 'block', fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 8, fontWeight: 500, color: '#f5f5f4' }}>
                {field === 'template_text' ? 'Template JSON' : field === 'schedule_text' ? 'Schedule JSON' : 'Recipients JSON'}
              </span>
              <textarea
                value={draft[field]}
                onChange={(e) => onDraftChange({ [field]: e.target.value } as Partial<ReportDraft>)}
                style={{
                  minHeight: field === 'template_text' ? 160 : 112,
                  width: '100%',
                  border: '1px solid #292524',
                  background: '#1c1917',
                  color: '#fde68a',
                  padding: '12px 16px',
                  borderRadius: 'var(--radius-md)',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 12,
                  outline: 'none',
                  resize: 'vertical',
                }}
              />
            </label>
          ))}
        </div>
      </div>
    </section>
  );
}

function Field({ label, children, fullWidth }: { label: string; children: React.ReactNode; fullWidth?: boolean }) {
  return (
    <label style={{ display: 'block', fontSize: 13, gridColumn: fullWidth ? '1 / -1' : undefined }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      {children}
    </label>
  );
}
