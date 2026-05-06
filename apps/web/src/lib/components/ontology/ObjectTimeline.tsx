import { useEffect, useMemo, useState } from 'react';

import {
  listObjectRevisions,
  restoreObjectRevision,
  type ObjectInstance,
  type ObjectRevision,
} from '@/lib/api/ontology';

interface ObjectTimelineProps {
  typeId: string;
  objectId: string;
  limit?: number;
  onRestore?: (object: ObjectInstance) => void;
}

function formatJson(value: unknown) {
  try { return JSON.stringify(value ?? {}, null, 2); }
  catch { return String(value); }
}

const OP_TONE: Record<string, { background: string; color: string }> = {
  insert: { background: '#022c22', color: '#86efac' },
  update: { background: '#78350f', color: '#fde68a' },
  delete: { background: '#7f1d1d', color: '#fecaca' },
};

export function ObjectTimeline({ typeId, objectId, limit = 100, onRestore }: ObjectTimelineProps) {
  const [revisions, setRevisions] = useState<ObjectRevision[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<number | null>(null);
  const [restoring, setRestoring] = useState(false);

  async function reload() {
    setLoading(true);
    setError('');
    try {
      const r = await listObjectRevisions(typeId, objectId, { limit });
      const data = r.data ?? [];
      setRevisions(data);
      if (data.length && selected === null) setSelected(data[0].revision_number);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void reload(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [typeId, objectId]);

  const sorted = useMemo(() => [...revisions].sort((a, b) => b.revision_number - a.revision_number), [revisions]);
  const diffPair = useMemo(() => {
    if (selected === null) return null;
    const idx = sorted.findIndex((r) => r.revision_number === selected);
    if (idx < 0) return null;
    return { current: sorted[idx], previous: sorted[idx + 1] ?? null };
  }, [sorted, selected]);

  async function restore() {
    if (selected === null) return;
    setRestoring(true);
    try {
      const r = await restoreObjectRevision(typeId, objectId, selected);
      onRestore?.(r.object);
      await reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setRestoring(false);
    }
  }

  return (
    <section style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 240px) minmax(0, 1fr)', padding: 12, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, color: '#e2e8f0' }}>
      <div>
        <h3 style={{ margin: 0, fontSize: 14 }}>Revisions ({revisions.length})</h3>
        {loading && <p style={{ color: '#94a3b8', fontSize: 11, fontStyle: 'italic', marginTop: 8 }}>Loading…</p>}
        {error && <p style={{ color: '#fca5a5', fontSize: 11, marginTop: 8 }}>{error}</p>}
        <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 8, maxHeight: 480, overflow: 'auto' }}>
          {sorted.map((r) => {
            const tone = OP_TONE[r.operation] ?? { background: '#334155', color: '#cbd5e1' };
            const isSelected = r.revision_number === selected;
            return (
              <li key={r.id}>
                <button
                  type="button"
                  onClick={() => setSelected(r.revision_number)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 8,
                    border: `1px solid ${isSelected ? '#3b82f6' : '#1f2937'}`,
                    background: isSelected ? '#1e293b' : 'transparent',
                    borderRadius: 4,
                    cursor: 'pointer',
                    color: 'inherit',
                    fontSize: 11,
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6 }}>
                    <strong>v{r.revision_number}</strong>
                    <span style={{ ...tone, padding: '1px 6px', borderRadius: 999, fontSize: 9, textTransform: 'uppercase' }}>{r.operation}</span>
                  </div>
                  <p style={{ margin: '2px 0 0', color: '#94a3b8', fontSize: 10 }}>
                    {new Date(r.written_at).toLocaleString()} · {r.changed_by.slice(0, 8)}
                  </p>
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {diffPair ? (
          <>
            <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
              <h4 style={{ margin: 0, fontSize: 13 }}>
                Diff: v{diffPair.previous?.revision_number ?? '∅'} → v{diffPair.current.revision_number}
              </h4>
              <button type="button" onClick={() => void restore()} disabled={restoring || selected === null} className="of-button" style={{ fontSize: 11 }}>
                {restoring ? 'Restoring…' : 'Restore to this revision'}
              </button>
            </header>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <div>
                <p className="of-eyebrow" style={{ fontSize: 10 }}>Previous</p>
                <pre style={{ background: '#020617', color: '#cbd5e1', padding: 8, borderRadius: 4, fontSize: 10, overflow: 'auto', maxHeight: 360 }}>
                  {diffPair.previous ? formatJson(diffPair.previous.properties) : 'No previous revision (initial insert).'}
                </pre>
              </div>
              <div>
                <p className="of-eyebrow" style={{ fontSize: 10 }}>Current</p>
                <pre style={{ background: '#020617', color: '#86efac', padding: 8, borderRadius: 4, fontSize: 10, overflow: 'auto', maxHeight: 360 }}>
                  {formatJson(diffPair.current.properties)}
                </pre>
              </div>
            </div>
          </>
        ) : (
          <p style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 12 }}>Select a revision to see the diff.</p>
        )}
      </div>
    </section>
  );
}
