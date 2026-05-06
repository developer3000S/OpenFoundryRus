import type {
  FusionJob,
  FusionOverview,
  MatchRule,
  MergeStrategy,
  RunResolutionJobResponse,
} from '@/lib/api/fusion';

export interface JobDraft {
  name: string;
  description: string;
  status: string;
  entity_type: string;
  match_rule_id: string;
  merge_strategy_id: string;
  source_labels_text: string;
  record_count: number;
  review_sampling_rate: number;
}

interface Props {
  overview: FusionOverview | null;
  jobs: FusionJob[];
  rules: MatchRule[];
  mergeStrategies: MergeStrategy[];
  draft: JobDraft;
  lastRun: RunResolutionJobResponse | null;
  selectedJobId: string;
  busy?: boolean;
  onSelectJob?: (jobId: string) => void;
  onDraftChange?: (draft: JobDraft) => void;
  onSave?: () => void;
  onRun?: () => void;
  onReset?: () => void;
}

const fieldStyle: React.CSSProperties = {
  borderRadius: 16,
  border: '1px solid var(--border-default)',
  background: 'var(--bg-subtle)',
  padding: '10px 14px',
};

const STAT_CARDS: Array<{ key: keyof FusionOverview; label: string; dark?: boolean }> = [
  { key: 'rule_count', label: 'Rules', dark: true },
  { key: 'active_job_count', label: 'Active Jobs' },
  { key: 'completed_job_count', label: 'Completed' },
  { key: 'cluster_count', label: 'Clusters' },
  { key: 'pending_review_count', label: 'Review Queue' },
  { key: 'golden_record_count', label: 'Golden Records' },
  { key: 'auto_merged_cluster_count', label: 'Auto-Merged' },
];

export function ResolutionResults({
  overview,
  jobs,
  rules,
  mergeStrategies,
  draft,
  lastRun,
  selectedJobId,
  busy = false,
  onSelectJob,
  onDraftChange,
  onSave,
  onRun,
  onReset,
}: Props) {
  function patch<K extends keyof JobDraft>(key: K, value: JobDraft[K]) {
    onDraftChange?.({ ...draft, [key]: value });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow">Resolution Jobs</p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Run blocking, scoring, clustering, and golden record generation
          </h2>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <button type="button" onClick={() => onReset?.()} disabled={busy} className="of-button">
            New job
          </button>
          <button
            type="button"
            onClick={() => onSave?.()}
            disabled={busy}
            className="of-button"
            style={{ borderColor: '#fcd34d', color: '#b45309' }}
          >
            Create job
          </button>
          <button
            type="button"
            onClick={() => onRun?.()}
            disabled={busy || !selectedJobId}
            className="of-button of-button--primary"
          >
            Run selected
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', marginTop: 18 }}>
        {STAT_CARDS.map((card) => {
          const value = overview ? (overview as unknown as Record<string, number>)[card.key] ?? 0 : 0;
          if (card.dark) {
            return (
              <div key={card.key} style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
                <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#94a3b8' }}>
                  {card.label}
                </div>
                <div style={{ marginTop: 8, fontSize: 26, fontWeight: 600 }}>{value}</div>
              </div>
            );
          }
          return (
            <div key={card.key} className="of-panel-muted" style={{ padding: 14 }}>
              <div className="of-eyebrow">{card.label}</div>
              <div style={{ marginTop: 8, fontSize: 26, fontWeight: 600, color: 'var(--text-strong)' }}>{value}</div>
            </div>
          );
        })}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.8fr) minmax(0, 1.2fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 10 }}>
          {jobs.length === 0 ? (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, color: 'var(--text-muted)' }}>
              No jobs yet.
            </div>
          ) : (
            jobs.map((job) => {
              const active = selectedJobId === job.id;
              return (
                <button
                  key={job.id}
                  type="button"
                  onClick={() => onSelectJob?.(job.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 14,
                    border: `1px solid ${active ? '#f59e0b' : 'var(--border-default)'}`,
                    background: active ? '#fffbeb' : 'var(--bg-subtle)',
                    borderRadius: 16,
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{job.name}</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                    {job.status} · {job.metrics.cluster_count} clusters · {job.metrics.review_pairs} review pairs
                  </div>
                  <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                    {job.last_run_summary}
                  </p>
                </button>
              );
            })
          )}
        </div>

        <div style={{ display: 'grid', gap: 12 }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Name</div>
              <input
                value={draft.name}
                onChange={(e) => patch('name', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Entity Type</div>
              <input
                value={draft.entity_type}
                onChange={(e) => patch('entity_type', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
          </div>

          <label style={fieldStyle}>
            <div className="of-eyebrow">Description</div>
            <textarea
              value={draft.description}
              onChange={(e) => patch('description', e.target.value)}
              style={{ marginTop: 6, width: '100%', height: 80, background: 'transparent', fontSize: 13, border: 'none', outline: 'none', resize: 'vertical' }}
            />
          </label>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Match Rule</div>
              <select
                value={draft.match_rule_id}
                onChange={(e) => patch('match_rule_id', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              >
                <option value="">Select match rule</option>
                {rules.map((rule) => (
                  <option key={rule.id} value={rule.id}>
                    {rule.name}
                  </option>
                ))}
              </select>
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Merge Strategy</div>
              <select
                value={draft.merge_strategy_id}
                onChange={(e) => patch('merge_strategy_id', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              >
                <option value="">Select merge strategy</option>
                {mergeStrategies.map((strategy) => (
                  <option key={strategy.id} value={strategy.id}>
                    {strategy.name}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))' }}>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Source Labels</div>
              <input
                value={draft.source_labels_text}
                onChange={(e) => patch('source_labels_text', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Record Count</div>
              <input
                type="number"
                value={String(draft.record_count)}
                onChange={(e) => patch('record_count', Number(e.target.value) || 12)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Review Sampling Rate</div>
              <input
                type="number"
                step={0.01}
                value={String(draft.review_sampling_rate)}
                onChange={(e) => patch('review_sampling_rate', Number(e.target.value) || 0.25)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
          </div>

          {lastRun && (
            <div
              style={{
                borderRadius: 24,
                border: '1px dashed #fcd34d',
                background: 'rgba(254, 243, 199, 0.5)',
                padding: 16,
              }}
            >
              <div className="of-eyebrow" style={{ color: '#b45309' }}>
                Latest Resolution Run
              </div>
              <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', marginTop: 10, fontSize: 13 }}>
                <div className="of-panel" style={{ padding: '8px 12px' }}>Clusters {lastRun.cluster_ids.length}</div>
                <div className="of-panel" style={{ padding: '8px 12px' }}>Golden {lastRun.golden_record_ids.length}</div>
                <div className="of-panel" style={{ padding: '8px 12px' }}>Review {lastRun.review_queue_item_ids.length}</div>
              </div>
              <p className="of-text-muted" style={{ marginTop: 10, fontSize: 13 }}>
                Executed at {new Date(lastRun.executed_at).toLocaleString()}
              </p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
