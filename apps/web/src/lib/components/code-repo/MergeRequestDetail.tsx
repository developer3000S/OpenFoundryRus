import type { CiRun, MergeRequestDetail as MergeRequestDetailModel, MergeRequestStatus } from '@/lib/api/code-repos';

export interface CommentDraft {
  author: string;
  body: string;
  file_path: string;
  line_number: string;
  resolved: boolean;
}

interface Props {
  detail: MergeRequestDetailModel | null;
  draft: CommentDraft;
  busy?: boolean;
  mergeBlockers: string[];
  latestSourceCi: CiRun | null;
  targetBranchProtected: boolean;
  onDraftChange: (patch: Partial<CommentDraft>) => void;
  onCreateComment: () => void;
  onStatusChange: (status: MergeRequestStatus) => void;
  onReviewerStateChange: (reviewer: string, approved: boolean, state: string) => void;
  onMerge: () => void;
}

const STATUSES: MergeRequestStatus[] = ['open', 'closed'];

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

export function MergeRequestDetail({
  detail,
  draft,
  busy = false,
  mergeBlockers,
  latestSourceCi,
  targetBranchProtected,
  onDraftChange,
  onCreateComment,
  onStatusChange,
  onReviewerStateChange,
  onMerge,
}: Props) {
  const mergeEnabled = Boolean(detail) && mergeBlockers.length === 0 && !busy;

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#4338ca' }}>
            Review Detail
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Inline comments, approvals, and lifecycle state
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Track reviewer decisions, CI readiness, and branch protection gates before merging for real.
          </p>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {STATUSES.map((status) => {
            const active = detail?.merge_request.status === status;
            return (
              <button
                key={status}
                type="button"
                onClick={() => onStatusChange(status)}
                disabled={busy || !detail}
                className={active ? 'of-button of-button--primary' : 'of-button'}
                style={active ? { background: '#4f46e5' } : { borderColor: '#c7d2fe', color: '#4338ca' }}
              >
                {status}
              </button>
            );
          })}
          <button
            type="button"
            onClick={onMerge}
            disabled={!mergeEnabled}
            className={mergeEnabled ? 'of-button of-button--primary' : 'of-button'}
            style={mergeEnabled ? { background: '#059669' } : { borderColor: '#a7f3d0', color: '#047857' }}
          >
            Merge now
          </button>
        </div>
      </div>

      {detail ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.98fr) minmax(0, 1.02fr)', marginTop: 18 }}>
          <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
            <div className="of-panel" style={{ padding: 14 }}>
              <p style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-strong)' }}>{detail.merge_request.title}</p>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                {detail.merge_request.source_branch} → {detail.merge_request.target_branch} · {detail.merge_request.author}
              </p>
              <p style={{ marginTop: 10, fontSize: 13, color: 'var(--text-default)' }}>{detail.merge_request.description}</p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 12 }}>
                <span className="of-chip" style={{ background: '#eef2ff', color: '#4338ca' }}>
                  Approvals {detail.approval_count}/{detail.merge_request.approvals_required}
                </span>
                <span className="of-chip">Threads {detail.thread_count}</span>
                <span className="of-chip">Changed files {detail.merge_request.changed_files}</span>
                <span className="of-chip">{targetBranchProtected ? 'Protected target' : 'Writable target'}</span>
              </div>
            </div>

            <div className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Merge policy</p>
                  <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    Protected targets require approvals and a green CI run on the latest source head.
                  </p>
                </div>
                <span
                  className="of-chip"
                  style={
                    mergeBlockers.length === 0
                      ? { background: '#ecfdf5', color: '#047857', textTransform: 'uppercase', letterSpacing: '0.16em' }
                      : { background: '#fffbeb', color: '#b45309', textTransform: 'uppercase', letterSpacing: '0.16em' }
                  }
                >
                  {mergeBlockers.length === 0 ? 'Ready' : 'Blocked'}
                </span>
              </div>

              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 12 }}>
                <div className="of-panel-muted" style={{ padding: 12 }}>
                  <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
                    Latest source CI
                  </div>
                  {latestSourceCi ? (
                    <>
                      <div style={{ marginTop: 6, fontSize: 13, fontWeight: 500, color: 'var(--text-strong)' }}>{latestSourceCi.status}</div>
                      <div style={{ marginTop: 4, fontSize: 11, color: 'var(--text-muted)' }}>
                        {latestSourceCi.branch_name} · {latestSourceCi.commit_sha}
                      </div>
                    </>
                  ) : (
                    <>
                      <div style={{ marginTop: 6, fontSize: 13, fontWeight: 500, color: 'var(--text-strong)' }}>No runs yet</div>
                      <div style={{ marginTop: 4, fontSize: 11, color: 'var(--text-muted)' }}>
                        Trigger CI or push a commit to produce a status check.
                      </div>
                    </>
                  )}
                </div>
                <div className="of-panel-muted" style={{ padding: 12 }}>
                  <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
                    Reviewer state
                  </div>
                  <div style={{ marginTop: 6, fontSize: 13, fontWeight: 500, color: 'var(--text-strong)' }}>
                    {detail.approval_count} approved
                  </div>
                  <div style={{ marginTop: 4, fontSize: 11, color: 'var(--text-muted)' }}>
                    {detail.merge_request.reviewers.length} assigned reviewer(s)
                  </div>
                </div>
              </div>

              {mergeBlockers.length > 0 && (
                <div style={{ display: 'grid', gap: 6, marginTop: 12 }}>
                  {mergeBlockers.map((blocker, index) => (
                    <div
                      key={index}
                      style={{
                        borderRadius: 16,
                        border: '1px solid #fde68a',
                        background: '#fffbeb',
                        padding: '8px 12px',
                        fontSize: 13,
                        color: '#92400e',
                      }}
                    >
                      {blocker}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="of-panel" style={{ padding: 14 }}>
              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Reviewers</p>
              <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                {detail.merge_request.reviewers.map((reviewer) => (
                  <div key={reviewer.reviewer} className="of-panel-muted" style={{ padding: 10 }}>
                    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <div>
                        <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{reviewer.reviewer}</p>
                        <p style={{ fontSize: 11, color: 'var(--text-muted)' }}>{reviewer.state}</p>
                      </div>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        <button
                          type="button"
                          onClick={() => onReviewerStateChange(reviewer.reviewer, true, 'approved')}
                          disabled={busy}
                          className="of-button"
                          style={
                            reviewer.approved
                              ? { background: '#059669', color: '#f8fafc', borderColor: '#059669', fontSize: 11 }
                              : { borderColor: '#a7f3d0', color: '#047857', fontSize: 11 }
                          }
                        >
                          Approve
                        </button>
                        <button
                          type="button"
                          onClick={() => onReviewerStateChange(reviewer.reviewer, false, 'changes_requested')}
                          disabled={busy}
                          className="of-button"
                          style={
                            reviewer.state === 'changes_requested'
                              ? { background: '#dc2626', color: '#f8fafc', borderColor: '#dc2626', fontSize: 11 }
                              : { borderColor: '#fecaca', color: '#b91c1c', fontSize: 11 }
                          }
                        >
                          Request changes
                        </button>
                        <button
                          type="button"
                          onClick={() => onReviewerStateChange(reviewer.reviewer, false, 'commented')}
                          disabled={busy}
                          className="of-button"
                          style={
                            reviewer.state === 'commented'
                              ? { background: '#0c0a09', color: '#f8fafc', borderColor: '#0c0a09', fontSize: 11 }
                              : { fontSize: 11 }
                          }
                        >
                          Comment only
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            <div style={{ display: 'grid', gap: 8 }}>
              {detail.comments.map((comment) => (
                <div key={comment.id} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{comment.author}</p>
                      <p style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                        {comment.file_path || 'general comment'}
                        {comment.line_number !== null && <> · line {comment.line_number}</>}
                      </p>
                    </div>
                    <span
                      className="of-chip"
                      style={
                        comment.resolved
                          ? { background: '#ecfdf5', color: '#047857' }
                          : { background: '#fffbeb', color: '#b45309' }
                      }
                    >
                      {comment.resolved ? 'Resolved' : 'Open'}
                    </span>
                  </div>
                  <p style={{ marginTop: 8, fontSize: 13, color: 'var(--text-default)' }}>{comment.body}</p>
                </div>
              ))}
            </div>
          </div>

          <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#c7d2fe' }}>
              Add inline comment
            </p>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Author</span>
                <input value={draft.author} onChange={(e) => onDraftChange({ author: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>File path</span>
                <input value={draft.file_path} onChange={(e) => onDraftChange({ file_path: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Line number</span>
                <input
                  value={draft.line_number}
                  onChange={(e) => onDraftChange({ line_number: e.target.value })}
                  style={darkInput}
                />
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
                  fontSize: 13,
                }}
              >
                <input type="checkbox" checked={draft.resolved} onChange={(e) => onDraftChange({ resolved: e.target.checked })} />
                Mark resolved
              </label>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Comment</span>
                <textarea
                  value={draft.body}
                  onChange={(e) => onDraftChange({ body: e.target.value })}
                  style={{ ...darkInput, minHeight: 110, resize: 'vertical' }}
                />
              </label>
            </div>
            <button
              type="button"
              onClick={onCreateComment}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 14, background: '#6366f1', color: '#0c0a09' }}
            >
              Add comment
            </button>
          </div>
        </div>
      ) : (
        <div
          style={{
            marginTop: 18,
            border: '1px dashed var(--border-default)',
            borderRadius: 16,
            padding: 32,
            textAlign: 'center',
            fontSize: 13,
            color: 'var(--text-muted)',
          }}
        >
          Select a merge request to inspect reviewers and inline comments.
        </div>
      )}
    </section>
  );
}
