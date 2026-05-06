import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listSchedules, type Schedule } from '@/lib/api/schedules';

interface DatasetSchedulesTabProps {
  datasetRid: string;
}

function summarize(s: Schedule): string {
  const kind = s.trigger.kind as Record<string, unknown>;
  if ('time' in kind) return `Time · ${(kind.time as { cron: string }).cron}`;
  if ('event' in kind) return `Event · ${(kind.event as { type: string }).type}`;
  if ('compound' in kind) return `Compound · ${(kind.compound as { op: string }).op}`;
  return 'unknown';
}

export function DatasetSchedulesTab({ datasetRid }: DatasetSchedulesTabProps) {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listSchedules({ files: [datasetRid] })
      .then((res) => { if (!cancelled) setSchedules(res.data); })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : String(cause));
          setSchedules([]);
        }
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetRid]);

  return (
    <section className="of-panel" style={{ padding: 12, display: 'grid', gap: 8 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
        <h3 style={{ margin: 0, fontSize: 13 }}>Schedules</h3>
        <a
          href={`/schedules/new?event_target=${encodeURIComponent(datasetRid)}`}
          className="of-button"
          style={{ fontSize: 11, padding: '4px 10px' }}
        >
          + Schedule
        </a>
      </header>
      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}
      {loading ? (
        <p className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 12, margin: 0 }}>Loading…</p>
      ) : schedules.length === 0 ? (
        <p className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 12, margin: 0 }}>No schedules reference this dataset yet.</p>
      ) : (
        <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: 4 }}>
          {schedules.map((s) => (
            <li
              key={s.rid}
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr auto auto',
                gap: 8,
                alignItems: 'baseline',
                background: 'var(--bg-subtle)',
                padding: '6px 8px',
                borderRadius: 4,
                fontSize: 12,
              }}
            >
              <Link to={`/schedules/${s.rid}`} style={{ color: '#93c5fd', textDecoration: 'underline' }}>{s.name}</Link>
              <code style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{summarize(s)}</code>
              <span style={{ color: '#fcd34d' }}>{s.paused ? '⏸︎' : '▶︎'}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
