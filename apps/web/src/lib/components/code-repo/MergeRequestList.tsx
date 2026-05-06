import type { MergeRequestDefinition } from '@/lib/api/code-repos';

export interface MergeRequestDraft {
  title: string;
  description: string;
  source_branch: string;
  target_branch: string;
  author: string;
  labels_text: string;
  reviewers_text: string;
  approvals_required: string;
  changed_files: string;
}

interface Props {
  mergeRequests: MergeRequestDefinition[];
  selectedMergeRequestId: string;
  branchOptions: string[];
  draft: MergeRequestDraft;
  busy?: boolean;
  onSelectMergeRequest: (mergeRequestId: string) => void;
  onDraftChange: (patch: Partial<MergeRequestDraft>) => void;
  onCreateMergeRequest: () => void;
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

export function MergeRequestList({
  mergeRequests,
  selectedMergeRequestId,
  branchOptions,
  draft,
  busy = false,
  onSelectMergeRequest,
  onDraftChange,
  onCreateMergeRequest,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#a21caf' }}>
            Merge Requests
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Review queues, labels, and approvals
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Open review flows between feature and target branches, then select one to inspect comments and status.
          </p>
        </div>
        <button type="button" onClick={onCreateMergeRequest} disabled={busy} className="of-button of-button--primary" style={{ background: '#a21caf' }}>
          Create MR
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 8 }}>
          {mergeRequests.map((mergeRequest) => {
            const active = selectedMergeRequestId === mergeRequest.id;
            return (
              <button
                key={mergeRequest.id}
                type="button"
                onClick={() => onSelectMergeRequest(mergeRequest.id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 14,
                  border: `1px solid ${active ? '#a21caf' : 'var(--border-default)'}`,
                  background: active ? '#fdf4ff' : 'var(--bg-subtle)',
                  borderRadius: 16,
                  cursor: 'pointer',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{mergeRequest.title}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {mergeRequest.source_branch} → {mergeRequest.target_branch}
                    </p>
                  </div>
                  <span className="of-chip" style={{ background: '#fdf4ff', color: '#a21caf', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                    {mergeRequest.status}
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  {mergeRequest.labels.map((label) => (
                    <span key={label} className="of-chip">
                      {label}
                    </span>
                  ))}
                </div>
              </button>
            );
          })}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
          <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#f0abfc' }}>
            New merge request
          </p>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Title</span>
              <input value={draft.title} onChange={(e) => onDraftChange({ title: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Description</span>
              <textarea
                value={draft.description}
                onChange={(e) => onDraftChange({ description: e.target.value })}
                style={{ ...darkInput, minHeight: 80, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Source</span>
              <select value={draft.source_branch} onChange={(e) => onDraftChange({ source_branch: e.target.value })} style={darkInput}>
                {branchOptions.map((branch) => (
                  <option key={branch} value={branch}>
                    {branch}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Target</span>
              <select value={draft.target_branch} onChange={(e) => onDraftChange({ target_branch: e.target.value })} style={darkInput}>
                {branchOptions.map((branch) => (
                  <option key={branch} value={branch}>
                    {branch}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Author</span>
              <input value={draft.author} onChange={(e) => onDraftChange({ author: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Reviewers</span>
              <input
                value={draft.reviewers_text}
                onChange={(e) => onDraftChange({ reviewers_text: e.target.value })}
                placeholder="Elena, Marco"
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Labels</span>
              <input
                value={draft.labels_text}
                onChange={(e) => onDraftChange({ labels_text: e.target.value })}
                placeholder="preview, widget, release"
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Approvals required</span>
              <input
                value={draft.approvals_required}
                onChange={(e) => onDraftChange({ approvals_required: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Changed files</span>
              <input
                value={draft.changed_files}
                onChange={(e) => onDraftChange({ changed_files: e.target.value })}
                style={darkInput}
              />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
