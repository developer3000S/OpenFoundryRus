import type { AuditPolicy, ClassificationCatalogEntry, ClassificationLevel } from '@/lib/api/audit';

export interface PolicyDraft {
  id?: string;
  name: string;
  description: string;
  scope: string;
  classification: ClassificationLevel;
  retention_days: string;
  legal_hold: boolean;
  purge_mode: string;
  active: boolean;
  rules_text: string;
  updated_by: string;
}

interface Props {
  policies: AuditPolicy[];
  classifications: ClassificationCatalogEntry[];
  selectedPolicyId: string;
  draft: PolicyDraft;
  busy?: boolean;
  onSelectPolicy: (policyId: string) => void;
  onDraftChange: (patch: Partial<PolicyDraft>) => void;
  onSave: () => void;
  onReset: () => void;
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

export function PolicyManager({
  policies,
  classifications,
  selectedPolicyId,
  draft,
  busy = false,
  onSelectPolicy,
  onDraftChange,
  onSave,
  onReset,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            Policy Manager
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Retention TTL, legal hold, and purge modes
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Manage audit retention policies by scope and sensitivity class.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={onReset} disabled={busy} className="of-button">
            New policy
          </button>
          <button type="button" onClick={onSave} disabled={busy} className="of-button of-button--primary" style={{ background: '#d97706' }}>
            {draft.id ? 'Update policy' : 'Create policy'}
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {policies.map((policy) => {
            const active = selectedPolicyId === policy.id;
            return (
              <button
                key={policy.id}
                type="button"
                onClick={() => onSelectPolicy(policy.id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 14,
                  border: `1px solid ${active ? '#d97706' : 'var(--border-default)'}`,
                  background: active ? '#fffbeb' : 'var(--bg-elevated)',
                  borderRadius: 16,
                  cursor: 'pointer',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{policy.name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {policy.scope} · {policy.updated_by}
                    </p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      policy.active
                        ? { background: '#ecfdf5', color: '#047857' }
                        : { background: 'var(--bg-subtle)', color: 'var(--text-muted)' }
                    }
                  >
                    {policy.active ? 'Active' : 'Paused'}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <span className="of-chip">{policy.classification}</span>
                  <span className="of-chip">{policy.retention_days} days</span>
                  <span className="of-chip">{policy.purge_mode}</span>
                </div>
              </button>
            );
          })}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Name</span>
              <input value={draft.name} onChange={(e) => onDraftChange({ name: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Description</span>
              <textarea
                value={draft.description}
                onChange={(e) => onDraftChange({ description: e.target.value })}
                style={{ ...darkInput, minHeight: 90, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Scope</span>
              <input value={draft.scope} onChange={(e) => onDraftChange({ scope: e.target.value })} style={darkInput} />
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
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Retention days</span>
              <input
                value={draft.retention_days}
                onChange={(e) => onDraftChange({ retention_days: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Purge mode</span>
              <input value={draft.purge_mode} onChange={(e) => onDraftChange({ purge_mode: e.target.value })} style={darkInput} />
            </label>
            <label
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 8,
                padding: '10px 14px',
                borderRadius: 16,
                border: '1px solid #44403c',
                background: '#1c1917',
                color: '#f5f5f4',
                fontSize: 13,
              }}
            >
              <input type="checkbox" checked={draft.legal_hold} onChange={(e) => onDraftChange({ legal_hold: e.target.checked })} />
              Legal hold
            </label>
            <label
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 8,
                padding: '10px 14px',
                borderRadius: 16,
                border: '1px solid #44403c',
                background: '#1c1917',
                color: '#f5f5f4',
                fontSize: 13,
              }}
            >
              <input type="checkbox" checked={draft.active} onChange={(e) => onDraftChange({ active: e.target.checked })} />
              Policy active
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Rules</span>
              <textarea
                value={draft.rules_text}
                onChange={(e) => onDraftChange({ rules_text: e.target.value })}
                style={{ ...darkInput, minHeight: 90, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Updated by</span>
              <input value={draft.updated_by} onChange={(e) => onDraftChange({ updated_by: e.target.value })} style={darkInput} />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
