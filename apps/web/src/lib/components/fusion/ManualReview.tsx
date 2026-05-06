import type { ClusterDetail, ReviewQueueItem } from '@/lib/api/fusion';

export interface ReviewDraft {
  decision: string;
  reviewed_by: string;
  notes: string;
}

interface Props {
  reviewQueue: ReviewQueueItem[];
  selectedClusterId: string;
  clusterDetail: ClusterDetail | null;
  draft: ReviewDraft;
  busy?: boolean;
  onSelectCluster?: (clusterId: string) => void;
  onDraftChange?: (draft: ReviewDraft) => void;
  onSubmit?: () => void;
}

const fieldStyle: React.CSSProperties = {
  borderRadius: 16,
  border: '1px solid var(--border-default)',
  background: 'var(--bg-elevated)',
  padding: '10px 14px',
};

export function ManualReview({
  reviewQueue,
  selectedClusterId,
  clusterDetail,
  draft,
  busy = false,
  onSelectCluster,
  onDraftChange,
  onSubmit,
}: Props) {
  function patch<K extends keyof ReviewDraft>(key: K, value: ReviewDraft[K]) {
    onDraftChange?.({ ...draft, [key]: value });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow">Manual Review</p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Human-in-the-loop decisions for uncertain matches
          </h2>
        </div>
        <button
          type="button"
          onClick={() => onSubmit?.()}
          disabled={busy || !selectedClusterId}
          className="of-button of-button--primary"
        >
          Submit review
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.8fr) minmax(0, 1.2fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 10 }}>
          {reviewQueue.length === 0 ? (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              No pending reviews right now.
            </div>
          ) : (
            reviewQueue.map((item) => {
              const active = selectedClusterId === item.cluster_id;
              return (
                <button
                  key={item.id}
                  type="button"
                  disabled={busy}
                  onClick={() => onSelectCluster?.(item.cluster_id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 14,
                    border: `1px solid ${active ? '#fb7185' : 'var(--border-default)'}`,
                    background: active ? '#fff1f2' : 'var(--bg-subtle)',
                    borderRadius: 16,
                    cursor: busy ? 'not-allowed' : 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{item.cluster_id}</div>
                      <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                        {item.severity} · {item.recommended_action}
                      </div>
                    </div>
                    <span className="of-chip" style={{ textTransform: 'uppercase', letterSpacing: '0.18em' }}>
                      {item.status}
                    </span>
                  </div>
                </button>
              );
            })
          )}
        </div>

        <div
          style={{
            display: 'grid',
            gap: 12,
            borderRadius: 24,
            border: '1px solid var(--border-default)',
            background: 'linear-gradient(135deg, #fff1f2 0%, var(--bg-elevated) 100%)',
            padding: 16,
          }}
        >
          <div className="of-eyebrow">Review Decision</div>
          {clusterDetail?.review_item && (
            <div
              style={{
                borderRadius: 16,
                border: '1px solid #fecdd3',
                background: 'var(--bg-elevated)',
                padding: 14,
                fontSize: 13,
                color: 'var(--text-default)',
              }}
            >
              <p style={{ fontWeight: 600 }}>{clusterDetail.review_item.recommended_action}</p>
              <p style={{ marginTop: 8 }}>{clusterDetail.review_item.rationale.join(' | ')}</p>
            </div>
          )}
          <label style={fieldStyle}>
            <div className="of-eyebrow">Decision</div>
            <select
              value={draft.decision}
              onChange={(e) => patch('decision', e.target.value)}
              style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
            >
              <option value="confirm_match">Confirm match</option>
              <option value="manually_resolved">Manual override</option>
              <option value="split_cluster">Split cluster</option>
              <option value="reject_match">Reject match</option>
            </select>
          </label>
          <label style={fieldStyle}>
            <div className="of-eyebrow">Reviewed By</div>
            <input
              value={draft.reviewed_by}
              onChange={(e) => patch('reviewed_by', e.target.value)}
              style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
            />
          </label>
          <label style={fieldStyle}>
            <div className="of-eyebrow">Notes</div>
            <textarea
              value={draft.notes}
              onChange={(e) => patch('notes', e.target.value)}
              style={{ marginTop: 6, width: '100%', height: 100, background: 'transparent', fontSize: 13, border: 'none', outline: 'none', resize: 'vertical' }}
            />
          </label>
        </div>
      </div>
    </section>
  );
}
