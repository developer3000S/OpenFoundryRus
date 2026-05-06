import { useEffect, useMemo, useRef, useState } from 'react';

export interface LiveLogEntry {
  sequence: number;
  ts: string;
  level: 'TRACE' | 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'FATAL';
  message: string;
  params?: Record<string, unknown> | null;
}

export type LiveLogMode = 'live' | 'historical';
export type LevelFilter = LiveLogEntry['level'];

interface LiveLogViewerProps {
  jobRid: string;
  mode?: LiveLogMode;
}

const ALL_LEVELS: LevelFilter[] = ['TRACE', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'];

const COLOR: Record<LevelFilter, string> = {
  INFO: '#3B82F6',
  WARN: '#F59E0B',
  ERROR: '#EF4444',
  FATAL: '#EF4444',
  DEBUG: '#6B7280',
  TRACE: '#6B7280',
};

export function LiveLogViewer({ jobRid, mode = 'live' }: LiveLogViewerProps) {
  const [entries, setEntries] = useState<LiveLogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [initializingSecondsRemaining, setInitializingSecondsRemaining] = useState<number | null>(null);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [activeLevels, setActiveLevels] = useState<Set<LevelFilter>>(new Set(ALL_LEVELS));
  const [search, setSearch] = useState('');
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const lastSequenceRef = useRef(0);
  const esRef = useRef<EventSource | null>(null);

  function appendEntry(entry: LiveLogEntry) {
    setEntries((prev) => {
      const next = [...prev, entry];
      if (next.length > 5000) return next.slice(-3000);
      return next;
    });
    if (entry.sequence > lastSequenceRef.current) lastSequenceRef.current = entry.sequence;
  }

  useEffect(() => {
    if (mode !== 'live' || paused) return;
    const url = `/api/v1/jobs/${encodeURIComponent(jobRid)}/logs/stream?from_sequence=${lastSequenceRef.current}`;
    let es: EventSource;
    try {
      es = new EventSource(url, { withCredentials: true });
    } catch (err) {
      setConnectionError(String(err));
      return;
    }
    esRef.current = es;
    setConnectionError(null);

    es.addEventListener('heartbeat', (event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data) as { phase: string; delay_remaining_seconds: number };
        if (payload.phase === 'initializing') setInitializingSecondsRemaining(payload.delay_remaining_seconds);
      } catch { /* ignore */ }
    });
    es.addEventListener('log', (event) => {
      try {
        const entry = JSON.parse((event as MessageEvent).data) as LiveLogEntry;
        if (paused) return;
        setInitializingSecondsRemaining(null);
        appendEntry(entry);
      } catch { /* ignore */ }
    });
    es.onerror = () => {
      setConnectionError('Disconnected — auto-retry on reconnect.');
      es.close();
      esRef.current = null;
    };
    return () => {
      es.close();
      esRef.current = null;
    };
  }, [jobRid, mode, paused]);

  useEffect(() => {
    if (mode !== 'historical') return;
    let cancelled = false;
    fetch(`/api/v1/jobs/${encodeURIComponent(jobRid)}/logs?limit=1000`, { credentials: 'include' })
      .then(async (res) => {
        if (!res.ok) {
          if (!cancelled) setConnectionError(`failed to load history: ${res.status}`);
          return;
        }
        const body = (await res.json()) as { data: LiveLogEntry[] };
        if (cancelled) return;
        const data = body.data ?? [];
        setEntries(data);
        if (data.length > 0) lastSequenceRef.current = data[data.length - 1].sequence;
      })
      .catch((cause) => { if (!cancelled) setConnectionError(String(cause)); });
    return () => { cancelled = true; };
  }, [jobRid, mode]);

  function toggleLevel(level: LevelFilter) {
    setActiveLevels((prev) => {
      const next = new Set(prev);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next;
    });
  }

  function toggleExpanded(seq: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(seq)) next.delete(seq);
      else next.add(seq);
      return next;
    });
  }

  const visible = useMemo(
    () =>
      entries.filter((e) => {
        if (!activeLevels.has(e.level)) return false;
        if (search.trim() && !e.message.toLowerCase().includes(search.trim().toLowerCase())) return false;
        return true;
      }),
    [entries, activeLevels, search],
  );

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 12, background: '#0f172a', border: '1px solid #1f2937', borderRadius: 6, color: '#e2e8f0' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <h4 style={{ margin: 0, fontSize: 13 }}>Live logs <code style={{ fontSize: 11, color: '#94a3b8' }}>{jobRid.slice(0, 32)}</code></h4>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
          {mode === 'live' && (
            paused ? (
              <button type="button" onClick={() => setPaused(false)} className="of-button" style={{ fontSize: 11 }}>Resume</button>
            ) : (
              <button type="button" onClick={() => setPaused(true)} className="of-button" style={{ fontSize: 11 }}>Pause</button>
            )
          )}
          <input type="search" value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search log message" className="of-input" style={{ fontSize: 11, width: 180 }} />
        </div>
      </header>
      <p style={{ margin: 0, fontSize: 11, color: '#94a3b8', fontStyle: 'italic' }}>
        Live logs are streamed in real-time. Time range filters do not apply.
      </p>
      {mode === 'live' && initializingSecondsRemaining !== null && initializingSecondsRemaining > 0 && (
        <p style={{ margin: 0, fontSize: 11, color: '#fcd34d' }}>
          Initializing — logs will appear in ~{initializingSecondsRemaining}s
        </p>
      )}
      {connectionError && <p style={{ margin: 0, fontSize: 11, color: '#fca5a5' }}>{connectionError}</p>}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {ALL_LEVELS.map((level) => {
          const active = activeLevels.has(level);
          return (
            <button
              key={level}
              type="button"
              onClick={() => toggleLevel(level)}
              style={{
                fontSize: 10,
                padding: '2px 8px',
                borderRadius: 999,
                border: 'none',
                background: active ? COLOR[level] : '#1e293b',
                color: active ? '#fff' : '#94a3b8',
                cursor: 'pointer',
              }}
            >
              {level}
            </button>
          );
        })}
      </div>

      <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 2, maxHeight: 320, overflow: 'auto', fontFamily: 'var(--font-mono)' }}>
        {visible.map((entry) => (
          <li key={entry.sequence} style={{ padding: '2px 4px', display: 'grid', gridTemplateColumns: '90px 56px 1fr', gap: 6, fontSize: 11 }}>
            <span style={{ color: '#94a3b8' }} title={entry.ts}>{new Date(entry.ts).toISOString().slice(11, 23)}</span>
            <span style={{ color: COLOR[entry.level] }}>{entry.level}</span>
            <span style={{ color: '#e2e8f0' }}>
              {entry.message}
              {entry.params && (
                <button
                  type="button"
                  onClick={() => toggleExpanded(entry.sequence)}
                  style={{ marginLeft: 8, fontSize: 10, color: '#60a5fa', background: 'transparent', border: 'none', cursor: 'pointer', padding: 0 }}
                >
                  {expanded.has(entry.sequence) ? '▾ Hide JSON' : '▸ Format as JSON'}
                </button>
              )}
              {entry.params && expanded.has(entry.sequence) && (
                <pre style={{ marginTop: 4, padding: 6, background: '#020617', color: '#cbd5e1', borderRadius: 4, fontSize: 10, overflow: 'auto', maxHeight: 200 }}>
                  {JSON.stringify(entry.params, null, 2)}
                </pre>
              )}
            </span>
          </li>
        ))}
        {visible.length === 0 && <li style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 11, padding: 8 }}>No log entries.</li>}
      </ul>
    </section>
  );
}
