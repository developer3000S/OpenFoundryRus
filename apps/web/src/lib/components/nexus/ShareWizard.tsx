import type { NexusSpace, PeerOrganization, ShareDetail, SharingContract } from '@/lib/api/nexus';

export interface ShareDraft {
  contract_id: string;
  provider_peer_id: string;
  consumer_peer_id: string;
  provider_space_id: string;
  consumer_space_id: string;
  dataset_name: string;
  selector_text: string;
  provider_schema_text: string;
  consumer_schema_text: string;
  sample_rows_text: string;
  replication_mode: string;
}

interface Props {
  shares: ShareDetail[];
  peers: PeerOrganization[];
  contracts: SharingContract[];
  spaces: NexusSpace[];
  draft: ShareDraft;
  busy?: boolean;
  onDraftChange: (patch: Partial<ShareDraft>) => void;
  onCreate: () => void;
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

export function ShareWizard({ shares, peers, contracts, spaces, draft, busy = false, onDraftChange, onCreate }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#a21caf' }}>
            Share Wizard
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Selective replication and dataset exchange
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Create cross-org shares, limit rows by selector, and define provider vs consumer schemas before federation.
          </p>
        </div>
        <button type="button" onClick={onCreate} disabled={busy} className="of-button of-button--primary" style={{ background: '#a21caf' }}>
          Create share
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)', marginTop: 18 }}>
        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Contract</span>
              <select value={draft.contract_id} onChange={(e) => onDraftChange({ contract_id: e.target.value })} style={darkInput}>
                <option value="">Select contract</option>
                {contracts.map((contract) => (
                  <option key={contract.id} value={contract.id}>
                    {contract.name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Provider peer</span>
              <select
                value={draft.provider_peer_id}
                onChange={(e) => onDraftChange({ provider_peer_id: e.target.value })}
                style={darkInput}
              >
                <option value="">Select provider</option>
                {peers.map((peer) => (
                  <option key={peer.id} value={peer.id}>
                    {peer.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Consumer peer</span>
              <select
                value={draft.consumer_peer_id}
                onChange={(e) => onDraftChange({ consumer_peer_id: e.target.value })}
                style={darkInput}
              >
                <option value="">Select consumer</option>
                {peers.map((peer) => (
                  <option key={peer.id} value={peer.id}>
                    {peer.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Provider space</span>
              <select
                value={draft.provider_space_id}
                onChange={(e) => onDraftChange({ provider_space_id: e.target.value })}
                style={darkInput}
              >
                <option value="">No space</option>
                {spaces.map((space) => (
                  <option key={space.id} value={space.id}>
                    {space.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Consumer space</span>
              <select
                value={draft.consumer_space_id}
                onChange={(e) => onDraftChange({ consumer_space_id: e.target.value })}
                style={darkInput}
              >
                <option value="">No space</option>
                {spaces.map((space) => (
                  <option key={space.id} value={space.id}>
                    {space.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Dataset name</span>
              <input
                value={draft.dataset_name}
                onChange={(e) => onDraftChange({ dataset_name: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Replication mode</span>
              <input
                value={draft.replication_mode}
                onChange={(e) => onDraftChange({ replication_mode: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Selector JSON</span>
              <input
                value={draft.selector_text}
                onChange={(e) => onDraftChange({ selector_text: e.target.value })}
                style={{ ...darkInput, fontFamily: 'var(--font-mono)', fontSize: 11 }}
              />
            </label>
            {[
              { key: 'provider_schema_text' as const, label: 'Provider schema JSON' },
              { key: 'consumer_schema_text' as const, label: 'Consumer schema JSON' },
              { key: 'sample_rows_text' as const, label: 'Sample rows JSON' },
            ].map((field) => (
              <label key={field.key} style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.label}</span>
                <textarea
                  value={draft[field.key]}
                  onChange={(e) => onDraftChange({ [field.key]: e.target.value } as Partial<ShareDraft>)}
                  style={{ ...darkInput, minHeight: 90, fontFamily: 'var(--font-mono)', fontSize: 11, resize: 'vertical' }}
                />
              </label>
            ))}
          </div>
        </div>

        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {shares.map((item) => (
            <div key={item.share.id} className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{item.share.dataset_name}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {item.share.replication_mode} · {item.share.status}
                  </p>
                </div>
                <span
                  className="of-chip"
                  style={
                    item.compatibility.compatible
                      ? { background: '#ecfdf5', color: '#047857' }
                      : { background: '#fffbeb', color: '#b45309' }
                  }
                >
                  {item.compatibility.compatible ? 'Compatible' : 'Review'}
                </span>
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 11 }}>
                Selector {JSON.stringify(item.share.selector)}
              </p>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                Spaces {item.share.provider_space_id ?? 'none'} → {item.share.consumer_space_id ?? 'none'}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
