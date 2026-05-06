import type { MergeStrategy } from '@/lib/api/fusion';

export interface MergeStrategyDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  entity_type: string;
  default_strategy: string;
  rules_text: string;
}

interface Props {
  strategies: MergeStrategy[];
  draft: MergeStrategyDraft;
  busy?: boolean;
  onSelect?: (strategyId: string) => void;
  onDraftChange?: (draft: MergeStrategyDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

const fieldStyle: React.CSSProperties = {
  borderRadius: 16,
  border: '1px solid var(--border-default)',
  background: 'var(--bg-subtle)',
  padding: '10px 14px',
};

export function MergePreview({ strategies, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  function patch<K extends keyof MergeStrategyDraft>(key: K, value: MergeStrategyDraft[K]) {
    onDraftChange?.({ ...draft, [key]: value });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow">Merge Strategy</p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Golden record survivorship and field precedence
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
          {strategies.length === 0 ? (
            <div
              style={{
                border: '1px dashed var(--border-default)',
                borderRadius: 16,
                padding: 18,
                fontSize: 13,
                color: 'var(--text-muted)',
              }}
            >
              No merge strategies defined yet.
            </div>
          ) : (
            strategies.map((strategy) => {
              const active = draft.id === strategy.id;
              return (
                <button
                  key={strategy.id}
                  type="button"
                  onClick={() => onSelect?.(strategy.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 14,
                    border: `1px solid ${active ? '#06b6d4' : 'var(--border-default)'}`,
                    background: active ? '#ecfeff' : 'var(--bg-subtle)',
                    borderRadius: 16,
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{strategy.name}</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                    {strategy.default_strategy} · {strategy.rules.length} rules
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
              <div className="of-eyebrow">Default Strategy</div>
              <input
                value={draft.default_strategy}
                onChange={(e) => patch('default_strategy', e.target.value)}
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

          <label
            style={{
              borderRadius: 16,
              border: '1px dashed #67e8f9',
              background: 'rgba(207, 250, 254, 0.5)',
              padding: '10px 14px',
            }}
          >
            <div className="of-eyebrow" style={{ color: '#0e7490' }}>
              Survivorship Rules JSON
            </div>
            <textarea
              value={draft.rules_text}
              onChange={(e) => patch('rules_text', e.target.value)}
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
