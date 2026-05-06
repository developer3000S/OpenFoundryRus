import { useEffect, useState } from 'react';

interface CustomMetadataPanelProps {
  initial: Record<string, string>;
  saving?: boolean;
  error?: string;
  onSave: (next: Record<string, string>) => void | Promise<void>;
}

interface Pair { key: string; value: string }

export function CustomMetadataPanel({ initial, saving = false, error = '', onSave }: CustomMetadataPanelProps) {
  const [pairs, setPairs] = useState<Pair[]>(() => Object.entries(initial).map(([k, v]) => ({ key: k, value: v })));

  useEffect(() => {
    setPairs(Object.entries(initial).map(([k, v]) => ({ key: k, value: v })));
  }, [initial]);

  function patch(idx: number, p: Partial<Pair>) {
    setPairs((prev) => prev.map((pp, i) => (i === idx ? { ...pp, ...p } : pp)));
  }
  function add() {
    setPairs((prev) => [...prev, { key: '', value: '' }]);
  }
  function remove(idx: number) {
    setPairs((prev) => prev.filter((_, i) => i !== idx));
  }

  async function submit() {
    const next: Record<string, string> = {};
    for (const p of pairs) {
      const k = p.key.trim();
      if (!k) continue;
      next[k] = p.value;
    }
    await onSave(next);
  }

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Custom metadata</div>
          <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Key / value attributes</h2>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>Free-form metadata indexed by the catalog and surfaced in search.</p>
        </div>
        <button type="button" onClick={() => void submit()} disabled={saving} className="of-button of-button--primary">
          {saving ? 'Saving…' : 'Save'}
        </button>
      </header>

      <div style={{ display: 'grid', gap: 6 }}>
        {pairs.map((p, idx) => (
          <div key={idx} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 6 }}>
            <input aria-label="Key" value={p.key} onChange={(e) => patch(idx, { key: e.target.value })} placeholder="key" className="of-input" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }} />
            <input aria-label="Value" value={p.value} onChange={(e) => patch(idx, { value: e.target.value })} placeholder="value" className="of-input" />
            <button type="button" onClick={() => remove(idx)} aria-label="Remove pair" className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca', fontSize: 11 }}>×</button>
          </div>
        ))}
        <button type="button" onClick={add} className="of-button" style={{ borderStyle: 'dashed', fontSize: 12 }}>+ Add metadata entry</button>
      </div>

      {error && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 12, fontSize: 13 }}>{error}</div>}
    </section>
  );
}
