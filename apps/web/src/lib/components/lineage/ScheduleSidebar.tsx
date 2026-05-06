import { useEffect, useState } from 'react';

import { listSchedules, type Schedule } from '@/lib/api/schedules';

interface Props {
  selectedDatasetRids: string[];
  onCreateForDataset?: (datasetRid: string) => void;
}

function summarizeTrigger(s: Schedule): string {
  const k = s.trigger.kind as Record<string, { cron?: string; type?: string; target_rid?: string; op?: string; components?: unknown[] }>;
  if ('time' in k) return `Time · ${k.time.cron ?? ''}`;
  if ('event' in k) return `Event · ${k.event.type ?? ''} ${k.event.target_rid ?? ''}`;
  if ('compound' in k) return `Compound · ${k.compound.op ?? ''}(${k.compound.components?.length ?? 0})`;
  return 'unknown';
}

export function ScheduleSidebar({ selectedDatasetRids, onCreateForDataset }: Props) {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [expandedRid, setExpandedRid] = useState<string | null>(null);
  const [dragHover, setDragHover] = useState(false);

  async function refresh() {
    if (selectedDatasetRids.length === 0) {
      setSchedules([]);
      return;
    }
    setLoading(true);
    setErrorMsg(null);
    try {
      const res = await listSchedules({ files: selectedDatasetRids });
      setSchedules(res.data);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
      setSchedules([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedDatasetRids]);

  return (
    <aside
      role="complementary"
      aria-label="Schedules touching the current lineage selection"
      onDragOver={(e) => { e.preventDefault(); setDragHover(true); }}
      onDragLeave={() => setDragHover(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragHover(false);
        const datasetRid = e.dataTransfer?.getData('application/x-dataset-rid');
        if (datasetRid && onCreateForDataset) onCreateForDataset(datasetRid);
      }}
      style={{ ...sidebarStyle, background: dragHover ? '#111827' : '#0b1220', borderColor: dragHover ? '#38bdf8' : '#1f2937' }}
    >
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <h3 style={{ margin: 0, fontSize: 13 }}>Manage Schedules</h3>
        <span style={hintStyle}>{schedules.length} schedule{schedules.length === 1 ? '' : 's'}</span>
      </header>

      {errorMsg && <p role="alert" style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{errorMsg}</p>}

      {dragHover && (
        <p style={{ background: '#064e3b', color: '#6ee7b7', padding: '6px 8px', borderRadius: 4, fontSize: 11, textAlign: 'center', margin: 0 }}>
          Drop the dataset to create a new schedule with an Event trigger.
        </p>
      )}

      {selectedDatasetRids.length === 0 ? (
        <p style={hintStyle}>Select datasets in the lineage canvas to see schedules touching them.</p>
      ) : loading ? (
        <p style={hintStyle}>Loading…</p>
      ) : schedules.length === 0 ? (
        <p style={hintStyle}>No schedules touch the current selection.</p>
      ) : (
        <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 4, maxHeight: 480, overflowY: 'auto' }}>
          {schedules.map((s) => {
            const expanded = expandedRid === s.rid;
            return (
              <li key={s.rid} style={{ background: '#111827', borderRadius: 4 }}>
                <button
                  type="button"
                  onClick={() => setExpandedRid(expanded ? null : s.rid)}
                  aria-expanded={expanded}
                  style={toggleStyle}
                >
                  <span style={{ color: '#f1f5f9', fontWeight: 500 }}>{s.name}</span>
                  <span style={{ color: '#94a3b8', fontFamily: "ui-monospace, 'SF Mono', Consolas, monospace", fontSize: 11 }}>{summarizeTrigger(s)}</span>
                  <span style={{ color: '#fcd34d' }}>{s.paused ? '⏸︎' : '▶︎'}</span>
                </button>
                {expanded && (
                  <div style={{ padding: '6px 8px 8px', borderTop: '1px solid #1f2937' }}>
                    <p style={hintStyle}>
                      Open <a href={`/schedules/${s.rid}`} style={{ color: '#93c5fd' }}>full editor</a> to change trigger, target, or governance settings. Use <a href={`/build-schedules/${s.rid}`} style={{ color: '#93c5fd' }}>Metrics</a> for run history + version diff.
                    </p>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </aside>
  );
}

const sidebarStyle: React.CSSProperties = { display: 'flex', flexDirection: 'column', gap: 8, padding: 12, border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0', width: 320, boxSizing: 'border-box', transition: 'background 0.1s ease, border-color 0.1s ease' };
const hintStyle: React.CSSProperties = { color: '#94a3b8', fontSize: 11, fontStyle: 'italic', margin: 0 };
const toggleStyle: React.CSSProperties = { width: '100%', background: 'transparent', border: 'none', color: '#cbd5e1', padding: '6px 8px', cursor: 'pointer', display: 'grid', gridTemplateColumns: '1fr auto auto', gap: 8, alignItems: 'baseline', textAlign: 'left', fontSize: 12 };
