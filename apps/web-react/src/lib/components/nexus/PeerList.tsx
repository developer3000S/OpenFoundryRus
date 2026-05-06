import type { PeerOrganization } from '@/lib/api/nexus';

export interface PeerDraft {
  slug: string;
  display_name: string;
  organization_type: string;
  region: string;
  endpoint_url: string;
  auth_mode: string;
  trust_level: string;
  public_key_fingerprint: string;
  shared_scopes_text: string;
  admin_contacts_text: string;
}

interface Props {
  peers: PeerOrganization[];
  draft: PeerDraft;
  busy?: boolean;
  onDraftChange: (patch: Partial<PeerDraft>) => void;
  onCreate: () => void;
  onAuthenticate: (peerId: string) => void;
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

export function PeerList({ peers, draft, busy = false, onDraftChange, onCreate, onAuthenticate }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#047857' }}>
            Peer List
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Partner registration and authentication
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Register cross-org peers, capture trust posture, and trigger an authentication handshake.
          </p>
        </div>
        <button
          type="button"
          onClick={onCreate}
          disabled={busy}
          className="of-button of-button--primary"
          style={{ background: '#059669' }}
        >
          Register peer
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {peers.map((peer) => (
            <div key={peer.id} className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{peer.display_name}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {peer.slug} · {peer.organization_type} · {peer.region} · {peer.auth_mode}
                  </p>
                </div>
                <span
                  className="of-chip"
                  style={
                    peer.status === 'authenticated'
                      ? { background: '#ecfdf5', color: '#047857' }
                      : { background: '#fffbeb', color: '#b45309' }
                  }
                >
                  {peer.status}
                </span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                {peer.shared_scopes.map((scope) => (
                  <span key={scope} className="of-chip">
                    {scope}
                  </span>
                ))}
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 11 }}>
                Lifecycle {peer.lifecycle_stage}
              </p>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                Contacts {peer.admin_contacts.join(', ') || 'n/a'}
              </p>
              <div style={{ marginTop: 10, display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, fontSize: 11 }}>
                <span className="of-text-muted">{peer.public_key_fingerprint}</span>
                <button
                  type="button"
                  onClick={() => onAuthenticate(peer.id)}
                  disabled={busy || peer.status === 'authenticated'}
                  className="of-button"
                  style={{ fontSize: 12, color: '#047857', borderColor: '#a7f3d0' }}
                >
                  Authenticate
                </button>
              </div>
            </div>
          ))}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            {[
              { key: 'slug' as const, label: 'Slug' },
              { key: 'display_name' as const, label: 'Display name' },
              { key: 'organization_type' as const, label: 'Organization type' },
              { key: 'region' as const, label: 'Region' },
              { key: 'auth_mode' as const, label: 'Auth mode' },
            ].map((field) => (
              <label key={field.key} style={{ fontSize: 13, color: '#f5f5f4' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>{field.label}</span>
                <input
                  value={draft[field.key]}
                  onChange={(e) => onDraftChange({ [field.key]: e.target.value } as Partial<PeerDraft>)}
                  style={darkInput}
                />
              </label>
            ))}
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Endpoint URL</span>
              <input
                value={draft.endpoint_url}
                onChange={(e) => onDraftChange({ endpoint_url: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Trust level</span>
              <input value={draft.trust_level} onChange={(e) => onDraftChange({ trust_level: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Fingerprint</span>
              <input
                value={draft.public_key_fingerprint}
                onChange={(e) => onDraftChange({ public_key_fingerprint: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Shared scopes</span>
              <input
                value={draft.shared_scopes_text}
                onChange={(e) => onDraftChange({ shared_scopes_text: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Admin contacts</span>
              <input
                value={draft.admin_contacts_text}
                onChange={(e) => onDraftChange({ admin_contacts_text: e.target.value })}
                style={darkInput}
              />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
