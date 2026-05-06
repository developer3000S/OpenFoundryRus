import { useEffect, useState } from 'react';

import { previewScheduleWindows, type PipelineScheduleConfig, type ScheduleWindow } from '@/lib/api/pipelines';
import {
  AUTO_PAUSED_REASON,
  getSchedule,
  listScheduleRuns,
  listScheduleVersions,
  pauseSchedule,
  resumeSchedule,
  type Schedule,
  type ScheduleRun,
  type ScheduleVersion,
} from '@/lib/api/schedules';
import { Tabs } from '@/lib/components/Tabs';

interface ScheduleConfigProps {
  pipelineId?: string;
  scheduleRid?: string;
  config: PipelineScheduleConfig;
  readOnly?: boolean;
  onChange: (next: PipelineScheduleConfig) => void;
}

const RUN_OUTCOME_TONE: Record<string, { background: string; color: string }> = {
  SUCCEEDED: { background: '#166534', color: '#d1fae5' },
  IGNORED: { background: '#334155', color: '#cbd5e1' },
  FAILED: { background: '#991b1b', color: '#fee2e2' },
};

export function ScheduleConfig({ pipelineId, scheduleRid, config, readOnly = false, onChange }: ScheduleConfigProps) {
  const [preview, setPreview] = useState<ScheduleWindow[]>([]);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [tab, setTab] = useState<'config' | 'history' | 'versions'>('config');

  // governance state
  const [schedule, setSchedule] = useState<Schedule | null>(null);
  const [runs, setRuns] = useState<ScheduleRun[]>([]);
  const [versions, setVersions] = useState<ScheduleVersion[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!config.enabled || !config.cron || !pipelineId) {
      setPreview([]);
      return;
    }
    let cancelled = false;
    setPreviewLoading(true);
    setPreviewError(null);
    const startAt = new Date().toISOString();
    const endAt = new Date(Date.now() + 14 * 24 * 3600 * 1000).toISOString();
    previewScheduleWindows({
      target_kind: 'pipeline',
      target_id: pipelineId,
      start_at: startAt,
      end_at: endAt,
      limit: 5,
    })
      .then((res) => { if (!cancelled) setPreview(res.data ?? []); })
      .catch((cause: unknown) => { if (!cancelled) setPreviewError(cause instanceof Error ? cause.message : String(cause)); })
      .finally(() => { if (!cancelled) setPreviewLoading(false); });
    return () => { cancelled = true; };
  }, [config.enabled, config.cron, pipelineId]);

  useEffect(() => {
    if (!scheduleRid) return;
    let cancelled = false;
    Promise.all([
      getSchedule(scheduleRid),
      listScheduleRuns(scheduleRid, { limit: 50 }),
      listScheduleVersions(scheduleRid, { limit: 20 }),
    ])
      .then(([s, r, v]) => {
        if (cancelled) return;
        setSchedule(s);
        setRuns(r.data ?? []);
        setVersions(v.data ?? []);
      })
      .catch((cause: unknown) => { if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause)); });
    return () => { cancelled = true; };
  }, [scheduleRid]);

  async function pause() {
    if (!scheduleRid) return;
    setBusy(true);
    try {
      await pauseSchedule(scheduleRid, 'Manual pause');
      const s = await getSchedule(scheduleRid);
      setSchedule(s);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  }
  async function resume() {
    if (!scheduleRid) return;
    setBusy(true);
    try {
      await resumeSchedule(scheduleRid);
      const s = await getSchedule(scheduleRid);
      setSchedule(s);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      {scheduleRid && schedule && schedule.paused_reason === AUTO_PAUSED_REASON && (
        <div style={{ background: '#78350f', color: '#fde68a', padding: 10, borderRadius: 6, fontSize: 12 }}>
          Schedule auto-paused after consecutive failures.
          <button type="button" onClick={() => void resume()} disabled={busy} className="of-button" style={{ marginLeft: 8, fontSize: 11 }}>
            Resume
          </button>
        </div>
      )}

      {scheduleRid && (
        <Tabs tabs={['config', 'history', 'versions'] as const} active={tab} onChange={setTab} />
      )}

      {(!scheduleRid || tab === 'config') && (
        <div style={{ display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={(e) => onChange({ ...config, enabled: e.target.checked })}
              disabled={readOnly}
            />
            Enabled
          </label>
          <label style={{ fontSize: 13 }}>
            Cron expression
            <input
              value={config.cron ?? ''}
              onChange={(e) => onChange({ ...config, cron: e.target.value || null })}
              disabled={readOnly || !config.enabled}
              placeholder="0 */15 * * * *"
              className="of-input"
              style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
            />
          </label>

          {config.enabled && config.cron && (
            <section>
              <p className="of-eyebrow" style={{ fontSize: 10, marginBottom: 4 }}>Next windows</p>
              {previewLoading && <p style={{ color: '#94a3b8', fontSize: 11, margin: 0 }}>Computing…</p>}
              {previewError && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{previewError}</p>}
              {!previewLoading && !previewError && (
                <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 2 }}>
                  {preview.map((w, i) => (
                    <li key={i} style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                      {new Date(w.scheduled_for).toLocaleString()}
                    </li>
                  ))}
                  {preview.length === 0 && <li style={{ color: '#94a3b8', fontSize: 11 }}>No windows.</li>}
                </ul>
              )}
            </section>
          )}

          {scheduleRid && schedule && (
            <div style={{ marginTop: 8, display: 'flex', gap: 6 }}>
              {schedule.paused ? (
                <button type="button" onClick={() => void resume()} disabled={busy} className="of-button">Resume</button>
              ) : (
                <button type="button" onClick={() => void pause()} disabled={busy} className="of-button">Pause</button>
              )}
            </div>
          )}
        </div>
      )}

      {scheduleRid && tab === 'history' && (
        <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, maxHeight: 360, overflow: 'auto' }}>
          {runs.map((r) => (
            <li key={r.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 12, padding: '6px 8px', background: '#1e293b', borderRadius: 4, fontSize: 11 }}>
              <span style={{ ...(RUN_OUTCOME_TONE[r.outcome] ?? RUN_OUTCOME_TONE.IGNORED), padding: '2px 8px', borderRadius: 999, fontWeight: 500 }}>
                {r.outcome}
              </span>
              <span>{new Date(r.triggered_at).toLocaleString()}</span>
              <span className="of-text-muted">{r.build_rid ? r.build_rid.slice(0, 12) + '…' : '—'}</span>
            </li>
          ))}
          {runs.length === 0 && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>No runs yet.</li>}
        </ul>
      )}

      {scheduleRid && tab === 'versions' && (
        <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, maxHeight: 360, overflow: 'auto' }}>
          {versions.map((v) => (
            <li key={v.id} style={{ padding: '6px 8px', background: '#1e293b', borderRadius: 4, fontSize: 11 }}>
              <strong>v{v.version}</strong>
              <span className="of-text-muted" style={{ marginLeft: 8 }}>
                · {new Date(v.edited_at).toLocaleString()} · {v.edited_by}
              </span>
              {v.comment && <p style={{ margin: '4px 0 0', fontSize: 11 }}>{v.comment}</p>}
            </li>
          ))}
          {versions.length === 0 && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>No version history.</li>}
        </ul>
      )}

      {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
    </section>
  );
}
