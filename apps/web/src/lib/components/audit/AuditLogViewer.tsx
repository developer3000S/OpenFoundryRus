import type {
  AuditEvent,
  AuditEventStatus,
  AuditSeverity,
  ClassificationCatalogEntry,
  ClassificationLevel,
} from '@/lib/api/audit';

export interface EventFilterDraft {
  source_service: string;
  subject_id: string;
  classification: string;
}

export interface EventDraft {
  source_service: string;
  channel: string;
  actor: string;
  action: string;
  resource_type: string;
  resource_id: string;
  status: AuditEventStatus;
  severity: AuditSeverity;
  classification: ClassificationLevel;
  subject_id: string;
  ip_address: string;
  location: string;
  labels_text: string;
  metadata_text: string;
  retention_days: string;
}

interface Props {
  events: AuditEvent[];
  classifications: ClassificationCatalogEntry[];
  filters: EventFilterDraft;
  draft: EventDraft;
  busy?: boolean;
  onFilterChange: (patch: Partial<EventFilterDraft>) => void;
  onApplyFilters: () => void;
  onDraftChange: (patch: Partial<EventDraft>) => void;
  onAppendEvent: () => void;
  showProbeForm?: boolean;
}

const STATUSES: AuditEventStatus[] = ['success', 'failure', 'denied'];
const SEVERITIES: AuditSeverity[] = ['low', 'medium', 'high', 'critical'];

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

export function AuditLogViewer({
  events,
  classifications,
  filters,
  draft,
  busy = false,
  onFilterChange,
  onApplyFilters,
  onDraftChange,
  onAppendEvent,
  showProbeForm = true,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#0369a1' }}>
            Audit Log Viewer
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Append-only events with filters and manual probes
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Filter by service, subject, or classification, and append probe events to validate the chain end to end.
          </p>
        </div>
        {showProbeForm && (
          <button type="button" onClick={onAppendEvent} disabled={busy} className="of-button of-button--primary" style={{ background: '#0ea5e9' }}>
            Append event
          </button>
        )}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: showProbeForm ? 'minmax(0, 1.02fr) minmax(0, 0.98fr)' : '1fr', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))' }}>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Source service</span>
              <input
                value={filters.source_service}
                onChange={(e) => onFilterChange({ source_service: e.target.value })}
                className="of-input"
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Subject ID</span>
              <input
                value={filters.subject_id}
                onChange={(e) => onFilterChange({ subject_id: e.target.value })}
                className="of-input"
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Classification</span>
              <select
                value={filters.classification}
                onChange={(e) => onFilterChange({ classification: e.target.value })}
                className="of-input"
              >
                <option value="">All</option>
                {classifications.map((option) => (
                  <option key={option.classification} value={option.classification}>
                    {option.classification}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <button type="button" onClick={onApplyFilters} disabled={busy} className="of-button" style={{ width: 'fit-content' }}>
            Apply filters
          </button>

          <div style={{ display: 'grid', gap: 8 }}>
            {events.map((event) => (
              <div key={event.id} className="of-panel" style={{ padding: 12 }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>
                      #{event.sequence} · {event.action}
                    </p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {event.source_service} · {event.actor} · {event.resource_type}:{event.resource_id}
                    </p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      event.severity === 'critical'
                        ? { background: '#fef2f2', color: '#b91c1c' }
                        : { background: 'var(--bg-subtle)', color: 'var(--text-muted)' }
                    }
                  >
                    {event.severity}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <span className="of-chip">{event.status}</span>
                  <span className="of-chip">{event.classification}</span>
                  {event.labels.map((label) => (
                    <span key={label} className="of-chip" style={{ background: '#dbeafe', color: '#0369a1' }}>
                      {label}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        {showProbeForm && (
          <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#7dd3fc' }}>
              Manual event probe
            </p>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              {(['source_service', 'channel', 'actor', 'action', 'resource_type', 'resource_id'] as const).map((field) => (
                <label key={field} style={{ fontSize: 13, color: '#f5f5f4' }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.replace(/_/g, ' ')}</span>
                  <input
                    value={draft[field]}
                    onChange={(e) => onDraftChange({ [field]: e.target.value } as Partial<EventDraft>)}
                    style={darkInput}
                  />
                </label>
              ))}
              <label style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Status</span>
                <select
                  value={draft.status}
                  onChange={(e) => onDraftChange({ status: e.target.value as AuditEventStatus })}
                  style={darkInput}
                >
                  {STATUSES.map((status) => (
                    <option key={status} value={status}>
                      {status}
                    </option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Severity</span>
                <select
                  value={draft.severity}
                  onChange={(e) => onDraftChange({ severity: e.target.value as AuditSeverity })}
                  style={darkInput}
                >
                  {SEVERITIES.map((severity) => (
                    <option key={severity} value={severity}>
                      {severity}
                    </option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Classification</span>
                <select
                  value={draft.classification}
                  onChange={(e) => onDraftChange({ classification: e.target.value as ClassificationLevel })}
                  style={darkInput}
                >
                  {classifications.map((option) => (
                    <option key={option.classification} value={option.classification}>
                      {option.classification}
                    </option>
                  ))}
                </select>
              </label>
              {(['subject_id', 'ip_address', 'location'] as const).map((field) => (
                <label key={field} style={{ fontSize: 13, color: '#f5f5f4' }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.replace(/_/g, ' ')}</span>
                  <input
                    value={draft[field]}
                    onChange={(e) => onDraftChange({ [field]: e.target.value } as Partial<EventDraft>)}
                    style={darkInput}
                  />
                </label>
              ))}
              <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Labels</span>
                <input
                  value={draft.labels_text}
                  onChange={(e) => onDraftChange({ labels_text: e.target.value })}
                  style={darkInput}
                />
              </label>
              <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Metadata JSON</span>
                <textarea
                  value={draft.metadata_text}
                  onChange={(e) => onDraftChange({ metadata_text: e.target.value })}
                  style={{ ...darkInput, minHeight: 90, fontFamily: 'var(--font-mono)', fontSize: 11, color: '#7dd3fc', resize: 'vertical' }}
                />
              </label>
              <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Retention days</span>
                <input
                  value={draft.retention_days}
                  onChange={(e) => onDraftChange({ retention_days: e.target.value })}
                  style={darkInput}
                />
              </label>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
