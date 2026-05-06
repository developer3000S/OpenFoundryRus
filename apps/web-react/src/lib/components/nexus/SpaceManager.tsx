import type { NexusSpace, PeerOrganization } from '@/lib/api/nexus';

export interface SpaceDraft {
  slug: string;
  display_name: string;
  description: string;
  space_kind: string;
  owner_peer_id: string;
  region: string;
  member_peer_ids: string[];
  governance_tags_text: string;
  status: string;
}

interface Props {
  spaces: NexusSpace[];
  peers: PeerOrganization[];
  draft: SpaceDraft;
  busy?: boolean;
  onDraftChange: (patch: Partial<SpaceDraft>) => void;
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

export function SpaceManager({ spaces, peers, draft, busy = false, onDraftChange, onCreate }: Props) {
  function handleMembersChange(event: React.ChangeEvent<HTMLSelectElement>) {
    const selected = Array.from(event.currentTarget.selectedOptions)
      .map((option) => option.value)
      .filter(Boolean);
    onDraftChange({ member_peer_ids: selected });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#0369a1' }}>
            Space Manager
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Private and shared spaces
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Model operational boundaries for multi-org ecosystems, ownership, members and governance tags.
          </p>
        </div>
        <button type="button" onClick={onCreate} disabled={busy} className="of-button of-button--primary" style={{ background: '#0284c7' }}>
          Create space
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 10 }}>
          {spaces.map((space) => (
            <div key={space.id} className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{space.display_name}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {space.slug} · {space.space_kind} · {space.region}
                  </p>
                </div>
                <span
                  className="of-chip"
                  style={
                    space.status === 'active'
                      ? { background: '#ecfdf5', color: '#047857' }
                      : { background: '#fffbeb', color: '#b45309' }
                  }
                >
                  {space.status}
                </span>
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                {space.description}
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                {space.governance_tags.map((tag) => (
                  <span key={tag} className="of-chip" style={{ background: '#dbeafe', color: '#0369a1' }}>
                    {tag}
                  </span>
                ))}
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 11 }}>
                Members {space.member_peer_ids.length}
              </p>
            </div>
          ))}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Slug</span>
              <input value={draft.slug} onChange={(e) => onDraftChange({ slug: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Display name</span>
              <input
                value={draft.display_name}
                onChange={(e) => onDraftChange({ display_name: e.target.value })}
                style={darkInput}
              />
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
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Kind</span>
              <select
                value={draft.space_kind}
                onChange={(e) => onDraftChange({ space_kind: e.target.value })}
                style={darkInput}
              >
                <option value="private">private</option>
                <option value="shared">shared</option>
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Region</span>
              <input value={draft.region} onChange={(e) => onDraftChange({ region: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Owner peer</span>
              <select
                value={draft.owner_peer_id}
                onChange={(e) => onDraftChange({ owner_peer_id: e.target.value })}
                style={darkInput}
              >
                <option value="">Local host</option>
                {peers.map((peer) => (
                  <option key={peer.id} value={peer.id}>
                    {peer.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Status</span>
              <select value={draft.status} onChange={(e) => onDraftChange({ status: e.target.value })} style={darkInput}>
                <option value="draft">draft</option>
                <option value="active">active</option>
                <option value="paused">paused</option>
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Members</span>
              <select multiple onChange={handleMembersChange} style={{ ...darkInput, minHeight: 120 }}>
                {peers.map((peer) => (
                  <option key={peer.id} value={peer.id} selected={draft.member_peer_ids.includes(peer.id)}>
                    {peer.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Governance tags</span>
              <input
                value={draft.governance_tags_text}
                onChange={(e) => onDraftChange({ governance_tags_text: e.target.value })}
                style={darkInput}
              />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
