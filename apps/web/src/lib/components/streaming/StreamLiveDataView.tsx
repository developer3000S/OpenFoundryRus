import { useEffect, useRef, useState } from 'react';

import { previewStream, type PreviewRow } from '@/lib/api/streaming';

interface Props {
  streamId: string;
}

type Mode = 'live' | 'recent' | 'historical';

function modeToServerMode(m: Mode): 'oldest' | 'hot_only' | 'cold_only' {
  if (m === 'live') return 'hot_only';
  if (m === 'recent') return 'oldest';
  return 'cold_only';
}

const BADGE_LIVE: React.CSSProperties = { background: '#d4f4dd', color: '#1f5631' };
const BADGE_ARCHIVED: React.CSSProperties = { background: '#e5e7eb', color: '#4b5563' };

export function StreamLiveDataView({ streamId }: Props) {
  const [mode, setMode] = useState<Mode>('live');
  const [limit] = useState(50);
  const [rows, setRows] = useState<PreviewRow[]>([]);
  const [aggregateSource, setAggregateSource] = useState<'hot' | 'cold' | 'hybrid'>('hot');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const socketRef = useRef<WebSocket | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  async function refresh(currentMode: Mode) {
    setLoading(true);
    setError('');
    try {
      const res = await previewStream(streamId, { mode: modeToServerMode(currentMode), limit });
      setRows(res.data);
      setAggregateSource(res.source);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  function closeSocket() {
    if (socketRef.current) {
      try { socketRef.current.close(); } catch { /* swallow */ }
      socketRef.current = null;
    }
  }

  function stopPolling() {
    if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null; }
  }

  function openSocket() {
    closeSocket();
    try {
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const host = window.location.host;
      const sock = new WebSocket(`${proto}://${host}/api/v1/streaming/streams/${streamId}/live`);
      sock.onmessage = (ev) => {
        try {
          const incoming = JSON.parse(ev.data) as PreviewRow;
          const tagged: PreviewRow = { ...incoming, source: 'hot' };
          setRows((prev) => [tagged, ...prev].slice(0, limit));
          setAggregateSource('hot');
        } catch (err) {
          console.warn('live frame parse failed', err);
        }
      };
      sock.onerror = (ev) => { console.warn('live socket error', ev); };
      socketRef.current = sock;
    } catch (err) {
      console.warn('live socket open failed', err);
    }
  }

  useEffect(() => {
    closeSocket();
    stopPolling();
    void refresh(mode);
    if (mode === 'live') {
      openSocket();
      pollRef.current = setInterval(() => void refresh(mode), 5_000);
    } else if (mode === 'recent') {
      pollRef.current = setInterval(() => void refresh(mode), 15_000);
    }
    return () => { closeSocket(); stopPolling(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, streamId]);

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        {(['live', 'recent', 'historical'] as Mode[]).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            style={{ background: mode === m ? '#246' : '#f3f4f6', color: mode === m ? '#fff' : 'inherit', border: '1px solid #ddd', borderRadius: 4, padding: '4px 10px', cursor: 'pointer', fontSize: 13, textTransform: 'capitalize' }}
          >
            {m}
          </button>
        ))}
        <span style={{ marginLeft: 'auto', fontSize: 12, color: '#555' }}>
          source: <strong>{aggregateSource}</strong>
        </span>
      </div>

      {error && <p style={{ color: '#b00' }}>{error}</p>}
      {loading && rows.length === 0 && <p style={{ color: '#666' }}>Loading…</p>}

      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
        <thead>
          <tr><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Time</th><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Sequence</th><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Source</th><th style={{ textAlign: 'left', padding: '6px 10px', borderBottom: '1px solid #eee' }}>Payload</th></tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr><td colSpan={4} style={{ color: '#666', padding: '6px 10px' }}>No records.</td></tr>
          ) : (
            rows.map((row, idx) => (
              <tr key={idx}>
                <td style={{ padding: '6px 10px', borderBottom: '1px solid #eee', verticalAlign: 'top' }}>{new Date(row.event_time).toLocaleString()}</td>
                <td style={{ padding: '6px 10px', borderBottom: '1px solid #eee', verticalAlign: 'top' }}>{row.sequence_no ?? '—'}</td>
                <td style={{ padding: '6px 10px', borderBottom: '1px solid #eee', verticalAlign: 'top' }}>
                  <span style={{ padding: '1px 8px', borderRadius: 3, fontSize: 11, fontWeight: 600, ...(row.source === 'hot' ? BADGE_LIVE : BADGE_ARCHIVED) }}>
                    {row.source === 'hot' ? 'live' : 'archived'}
                  </span>
                </td>
                <td style={{ padding: '6px 10px', borderBottom: '1px solid #eee', verticalAlign: 'top' }}>
                  <pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontFamily: 'ui-monospace, monospace' }}>{JSON.stringify(row.payload, null, 2)}</pre>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </section>
  );
}
