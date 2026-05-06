import type { MatchRule } from '@/lib/api/fusion';

export interface MatchRuleDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  entity_type: string;
  strategy_type: string;
  key_fields_text: string;
  window_size: number;
  bucket_count: number;
  review_threshold: number;
  auto_merge_threshold: number;
  conditions_text: string;
}

interface Props {
  rules: MatchRule[];
  draft: MatchRuleDraft;
  busy?: boolean;
  onSelect?: (ruleId: string) => void;
  onDraftChange?: (draft: MatchRuleDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

const fieldStyle: React.CSSProperties = {
  borderRadius: 16,
  border: '1px solid var(--border-default)',
  background: 'var(--bg-subtle)',
  padding: '10px 14px',
};

export function MatchRuleBuilder({ rules, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  function patch<K extends keyof MatchRuleDraft>(key: K, value: MatchRuleDraft[K]) {
    onDraftChange?.({ ...draft, [key]: value });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow">Match Rule Builder</p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Deterministic, fuzzy, and phonetic matching rules
          </h2>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={() => onReset?.()} disabled={busy} className="of-button">
            New
          </button>
          <button type="button" onClick={() => onSave?.()} disabled={busy} className="of-button of-button--primary">
            Save
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.8fr) minmax(0, 1.2fr)', marginTop: 18 }}>
        <div style={{ display: 'grid', gap: 10 }}>
          {rules.length === 0 ? (
            <div
              style={{
                border: '1px dashed var(--border-default)',
                borderRadius: 16,
                padding: 18,
                fontSize: 13,
                color: 'var(--text-muted)',
              }}
            >
              No rules defined yet.
            </div>
          ) : (
            rules.map((rule) => {
              const active = draft.id === rule.id;
              return (
                <button
                  key={rule.id}
                  type="button"
                  onClick={() => onSelect?.(rule.id)}
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
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{rule.name}</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                    {rule.entity_type} · {rule.blocking_strategy.strategy_type}
                  </div>
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

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))' }}>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Blocking</div>
              <input
                value={draft.strategy_type}
                onChange={(e) => patch('strategy_type', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Key Fields</div>
              <input
                value={draft.key_fields_text}
                onChange={(e) => patch('key_fields_text', e.target.value)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Window</div>
              <input
                type="number"
                value={String(draft.window_size)}
                onChange={(e) => patch('window_size', Number(e.target.value) || 4)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Buckets</div>
              <input
                type="number"
                value={String(draft.bucket_count)}
                onChange={(e) => patch('bucket_count', Number(e.target.value) || 24)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Review Threshold</div>
              <input
                type="number"
                step={0.01}
                value={String(draft.review_threshold)}
                onChange={(e) => patch('review_threshold', Number(e.target.value) || 0.76)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
            <label style={fieldStyle}>
              <div className="of-eyebrow">Auto-Merge Threshold</div>
              <input
                type="number"
                step={0.01}
                value={String(draft.auto_merge_threshold)}
                onChange={(e) => patch('auto_merge_threshold', Number(e.target.value) || 0.9)}
                style={{ marginTop: 6, width: '100%', background: 'transparent', fontSize: 13, border: 'none', outline: 'none' }}
              />
            </label>
          </div>

          <label
            style={{
              borderRadius: 16,
              border: '1px dashed #fcd34d',
              background: 'rgba(254, 243, 199, 0.5)',
              padding: '10px 14px',
            }}
          >
            <div className="of-eyebrow" style={{ color: '#b45309' }}>
              Conditions JSON
            </div>
            <textarea
              value={draft.conditions_text}
              onChange={(e) => patch('conditions_text', e.target.value)}
              style={{
                marginTop: 6,
                width: '100%',
                height: 220,
                background: 'transparent',
                fontFamily: 'var(--font-mono)',
                fontSize: 12,
                border: 'none',
                outline: 'none',
                resize: 'vertical',
              }}
            />
          </label>
        </div>
      </div>
    </section>
  );
}
