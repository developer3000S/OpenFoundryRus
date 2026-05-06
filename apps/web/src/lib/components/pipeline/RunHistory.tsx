import { useEffect, useMemo, useRef, useState } from 'react';

import { abortBuild, listRuns, retryPipelineRun, triggerRun, type PipelineRun } from '@/lib/api/pipelines';
import { RunLogs } from './RunLogs';
import { LiveLogViewer } from './LiveLogViewer';

interface RunHistoryProps {
  pipelineId: string;
  readOnly?: boolean;
}

const LIVE_STATUSES = new Set([
  'running', 'pending', 'BUILD_RUNNING', 'BUILD_QUEUED',
  'BUILD_RESOLUTION', 'BUILD_ABORTING', 'RUN_PENDING', 'ABORT_PENDING',
]);

const PILL: Record<string, { background: string; color: string }> = {
  running: { background: '#1d4ed8', color: '#dbeafe' },
  completed: { background: '#166534', color: '#d1fae5' },
  failed: { background: '#991b1b', color: '#fee2e2' },
  aborted: { background: '#92400e', color: '#fde68a' },
};

function pill(s: string) {
  return PILL[s] ?? { background: '#334155', color: '#cbd5e1' };
}

function fmt(ts: string | null) {
  return ts ? new Date(ts).toLocaleString() : '—';
}

export function RunHistory({ pipelineId, readOnly = false }: RunHistoryProps) {
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  async function reload() {
    setLoading(true);
    setError(null);
    try {
      const res = await listRuns(pipelineId, { per_page: 25 });
      setRuns(res.data ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  async function trigger() {
    setBusy('trigger');
    try {
      await triggerRun(pipelineId);
      await reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  }

  async function retry(runId: string) {
    setBusy(runId);
    try {
      await retryPipelineRun(pipelineId, runId);
      await reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  }

  async function abort(runId: string) {
    if (typeof window !== 'undefined' && !window.confirm('Abort this build?')) return;
    setBusy(runId);
    try {
      await abortBuild(runId);
      await reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(null);
    }
  }

  useEffect(() => {
    if (pipelineId) void reload();
  }, [pipelineId]);

  useEffect(() => {
    const hasRunning = runs.some((r) => r.status === 'running');
    if (hasRunning && !pollRef.current) {
      pollRef.current = setInterval(() => void reload(), 5000);
    } else if (!hasRunning && pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runs]);

  const selectedRun = useMemo(() => runs.find((r) => r.id === selectedRunId) ?? null, [runs, selectedRunId]);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 14, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Build history</h3>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={() => void reload()} disabled={loading} className="of-button" style={{ fontSize: 12 }}>Refresh</button>
          {!readOnly && (
            <button type="button" onClick={() => void trigger()} disabled={busy === 'trigger'} className="of-button of-button--primary" style={{ fontSize: 12 }}>
              {busy === 'trigger' ? 'Triggering…' : 'Trigger run'}
            </button>
          )}
        </div>
      </header>
      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}
      {runs.length === 0 && !loading ? (
        <p style={{ color: '#94a3b8', fontStyle: 'italic', margin: 0, fontSize: 12 }}>No runs yet.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr>
              {['Status', 'Trigger', 'Started', 'Finished', 'Attempt', 'Actions'].map((h) => (
                <th key={h} style={{ textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #1f2937', color: '#94a3b8', fontWeight: 500, textTransform: 'uppercase', fontSize: 11 }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.id} style={{ background: run.id === selectedRunId ? '#1e293b' : 'transparent' }}>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937' }}>
                  <span style={{ ...pill(run.status), padding: '2px 8px', borderRadius: 999, fontSize: 11 }}>{run.status}</span>
                </td>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937' }}>{run.trigger_type}</td>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937' }}>{fmt(run.started_at)}</td>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937' }}>{fmt(run.finished_at)}</td>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937' }}>#{run.attempt_number}</td>
                <td style={{ padding: '6px 8px', borderBottom: '1px solid #1f2937', display: 'flex', gap: 4 }}>
                  <button type="button" onClick={() => setSelectedRunId(run.id)} className="of-button" style={{ fontSize: 11 }}>Logs</button>
                  {!readOnly && run.status === 'running' && (
                    <button type="button" onClick={() => void abort(run.id)} disabled={busy === run.id} className="of-button" style={{ fontSize: 11 }}>Abort</button>
                  )}
                  {!readOnly && (run.status === 'failed' || run.status === 'aborted') && (
                    <button type="button" onClick={() => void retry(run.id)} disabled={busy === run.id} className="of-button" style={{ fontSize: 11 }}>Retry</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {selectedRun && (
        LIVE_STATUSES.has(selectedRun.status) ? (
          <LiveLogViewer jobRid={`ri.foundry.main.job.${selectedRun.id}`} mode="live" />
        ) : (
          <>
            <RunLogs run={selectedRun} onClose={() => setSelectedRunId(null)} />
            <LiveLogViewer jobRid={`ri.foundry.main.job.${selectedRun.id}`} mode="historical" />
          </>
        )
      )}
    </section>
  );
}
