import type { PeerOrganization, SharingContract } from '@/lib/api/nexus';

export interface ContractDraft {
  id?: string;
  peer_id: string;
  name: string;
  description: string;
  dataset_locator: string;
  allowed_purposes_text: string;
  data_classes_text: string;
  residency_region: string;
  query_template: string;
  max_rows_per_query: string;
  replication_mode: string;
  encryption_profile: string;
  retention_days: string;
  status: string;
  expires_at: string;
}

interface Props {
  contracts: SharingContract[];
  peers: PeerOrganization[];
  selectedContractId: string;
  draft: ContractDraft;
  busy?: boolean;
  onSelect: (contractId: string) => void;
  onDraftChange: (patch: Partial<ContractDraft>) => void;
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

export function ContractManager({
  contracts,
  peers,
  selectedContractId,
  draft,
  busy = false,
  onSelect,
  onDraftChange,
  onSave,
  onReset,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#0369a1' }}>
            Contract Manager
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Sharing contracts, residency, and access terms
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Define what is shared, for which purpose, under what residency, encryption, and retention constraints.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={onReset} disabled={busy} className="of-button">
            New contract
          </button>
          <button type="button" onClick={onSave} disabled={busy} className="of-button of-button--primary" style={{ background: '#0284c7' }}>
            {draft.id ? 'Update contract' : 'Create contract'}
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {contracts.map((contract) => {
            const active = selectedContractId === contract.id;
            return (
              <button
                key={contract.id}
                type="button"
                onClick={() => onSelect(contract.id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 14,
                  border: `1px solid ${active ? '#0284c7' : 'var(--border-default)'}`,
                  background: active ? '#f0f9ff' : 'var(--bg-elevated)',
                  borderRadius: 16,
                  cursor: 'pointer',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{contract.name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {contract.dataset_locator}
                    </p>
                  </div>
                  <span
                    className="of-chip"
                    style={
                      contract.status === 'active'
                        ? { background: '#ecfdf5', color: '#047857' }
                        : { background: '#fffbeb', color: '#b45309' }
                    }
                  >
                    {contract.status}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  {contract.allowed_purposes.map((purpose) => (
                    <span key={purpose} className="of-chip">
                      {purpose}
                    </span>
                  ))}
                </div>
              </button>
            );
          })}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Peer</span>
              <select value={draft.peer_id} onChange={(e) => onDraftChange({ peer_id: e.target.value })} style={darkInput}>
                <option value="">Select peer</option>
                {peers.map((peer) => (
                  <option key={peer.id} value={peer.id}>
                    {peer.display_name}
                  </option>
                ))}
              </select>
            </label>
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
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Dataset locator</span>
              <input
                value={draft.dataset_locator}
                onChange={(e) => onDraftChange({ dataset_locator: e.target.value })}
                style={darkInput}
              />
            </label>
            {[
              { key: 'allowed_purposes_text' as const, label: 'Allowed purposes' },
              { key: 'data_classes_text' as const, label: 'Data classes' },
              { key: 'residency_region' as const, label: 'Residency region' },
              { key: 'status' as const, label: 'Status' },
            ].map((field) => (
              <label key={field.key} style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.label}</span>
                <input
                  value={draft[field.key]}
                  onChange={(e) => onDraftChange({ [field.key]: e.target.value } as Partial<ContractDraft>)}
                  style={darkInput}
                />
              </label>
            ))}
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Query template</span>
              <textarea
                value={draft.query_template}
                onChange={(e) => onDraftChange({ query_template: e.target.value })}
                style={{ ...darkInput, minHeight: 90, fontFamily: 'var(--font-mono)', fontSize: 11, resize: 'vertical' }}
              />
            </label>
            {[
              { key: 'max_rows_per_query' as const, label: 'Max rows' },
              { key: 'replication_mode' as const, label: 'Replication mode' },
              { key: 'encryption_profile' as const, label: 'Encryption profile' },
              { key: 'retention_days' as const, label: 'Retention days' },
            ].map((field) => (
              <label key={field.key} style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.label}</span>
                <input
                  value={draft[field.key]}
                  onChange={(e) => onDraftChange({ [field.key]: e.target.value } as Partial<ContractDraft>)}
                  style={darkInput}
                />
              </label>
            ))}
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Expires at</span>
              <input
                type="datetime-local"
                value={draft.expires_at}
                onChange={(e) => onDraftChange({ expires_at: e.target.value })}
                style={darkInput}
              />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
