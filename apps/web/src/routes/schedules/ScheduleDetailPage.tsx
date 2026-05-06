import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  getSchedule,
  listScheduleRuns,
  listScheduleVersions,
  pauseSchedule,
  resumeSchedule,
  runScheduleNow,
  setAutoPauseExempt,
  type Schedule,
  type ScheduleRun,
  type ScheduleVersion,
} from '@/lib/api/schedules';

export function ScheduleDetailPage() {
  const { rid = '' } = useParams<{ rid: string }>();
  const [schedule, setSchedule] = useState<Schedule | null>(null);
  const [runs, setRuns] = useState<ScheduleRun[]>([]);
  const [versions, setVersions] = useState<ScheduleVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [pauseReason, setPauseReason] = useState('Manually paused');

  async function refresh() {
    if (!rid) return;
    setLoading(true);
    setError('');
    try {
      const [s, runRes, verRes] = await Promise.all([
        getSchedule(rid),
        listScheduleRuns(rid, { limit: 50 }).catch(() => null),
        listScheduleVersions(rid, { limit: 20 }).catch(() => null),
      ]);
      setSchedule(s);
      setRuns(runRes?.data ?? []);
      setVersions(verRes?.data ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load schedule');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [rid]);

  async function withBusy(fn: () => Promise<unknown>, label: string) {
    setBusy(true);
    setError('');
    try {
      await fn();
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `${label} failed`);
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading schedule…</p>
      </section>
    );
  }

  if (!schedule) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/build-schedules" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Schedules</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Schedule not found'}</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/build-schedules" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Schedules</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">{schedule.name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            <code>{schedule.rid}</code> · paused {schedule.paused ? 'YES' : 'no'}
            {schedule.paused_reason ? ` · ${schedule.paused_reason}` : ''}
          </p>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          <button type="button" onClick={() => void withBusy(() => runScheduleNow(rid), 'Run now')} disabled={busy} className="of-button of-button--primary">
            Run now
          </button>
          {schedule.paused ? (
            <button type="button" onClick={() => void withBusy(() => resumeSchedule(rid), 'Resume')} disabled={busy} className="of-button">
              Resume
            </button>
          ) : (
            <button type="button" onClick={() => void withBusy(() => pauseSchedule(rid, pauseReason), 'Pause')} disabled={busy} className="of-button">
              Pause
            </button>
          )}
          <button
            type="button"
            onClick={() => void withBusy(() => setAutoPauseExempt(rid, !schedule.auto_pause_exempt), 'Toggle auto-pause-exempt')}
            disabled={busy}
            className="of-button"
          >
            {schedule.auto_pause_exempt ? 'Disable auto-pause-exempt' : 'Enable auto-pause-exempt'}
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {!schedule.paused && (
        <section className="of-panel" style={{ padding: 12 }}>
          <label style={{ fontSize: 12 }}>
            Pause reason
            <input
              value={pauseReason}
              onChange={(e) => setPauseReason(e.target.value)}
              className="of-input"
              style={{ marginTop: 4, width: 320 }}
            />
          </label>
        </section>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Schedule (read-only JSON)</p>
        <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 360 }}>
          {JSON.stringify(schedule, null, 2)}
        </pre>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Runs ({runs.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {runs.map((r) => (
            <li key={r.id}>
              {new Date(r.triggered_at).toLocaleString()} · <strong>{r.outcome}</strong>
              {r.build_rid ? ` · build ${r.build_rid}` : ''}
              {r.failure_reason ? ` · ${r.failure_reason}` : ''}
            </li>
          ))}
          {runs.length === 0 && <li className="of-text-muted">No runs yet.</li>}
        </ul>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Versions ({versions.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {versions.map((v) => (
            <li key={v.version}>
              v{v.version} · {new Date(v.edited_at).toLocaleString()} · {v.edited_by}
              {v.comment ? ` · ${v.comment}` : ''}
            </li>
          ))}
          {versions.length === 0 && <li className="of-text-muted">No version history.</li>}
        </ul>
      </section>
    </section>
  );
}
