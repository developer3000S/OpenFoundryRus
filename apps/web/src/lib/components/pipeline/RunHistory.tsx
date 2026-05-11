import { useEffect, useMemo, useRef, useState } from 'react';

import { abortBuild, listRuns, pipelineNodeResultsFromRun, retryPipelineRun, triggerRun, type PipelineRun } from '@/lib/api/pipelines';
import { Glyph } from '@/lib/components/ui/Glyph';
import { RunLogs } from './RunLogs';
import { LiveLogViewer } from './LiveLogViewer';

interface RunHistoryProps {
  pipelineId: string;
  readOnly?: boolean;
}

const LIVE_STATUSES = new Set([
  'queued', 'running', 'pending', 'BUILD_RUNNING', 'BUILD_QUEUED',
  'BUILD_RESOLUTION', 'BUILD_ABORTING', 'RUN_PENDING', 'ABORT_PENDING',
]);

function statusTone(status: string) {
  const s = status.toLowerCase();
  if (s === 'completed' || s === 'succeeded' || s === 'success') return 'of-status-success';
  if (s === 'failed' || s === 'error' || s === 'aborted' || s === 'cancelled' || s === 'canceled') return 'of-status-danger';
  if (s === 'running' || s === 'pending' || s === 'queued' || s.includes('build_')) return 'of-status-info';
  return 'of-status-warning';
}

function fmt(ts: string | null) {
  return ts ? new Date(ts).toLocaleString() : '—';
}

function logRidForRun(run: PipelineRun) {
  return pipelineNodeResultsFromRun(run).find((result) => result.log_rid)?.log_rid ?? `ri.foundry.main.job.${run.id}`;
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
    const hasRunning = runs.some((r) => LIVE_STATUSES.has(r.status));
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
    <section className="of-panel" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
      <header
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '10px 12px',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'var(--bg-panel-muted)',
        }}
      >
        <div>
          <p className="of-eyebrow" style={{ margin: 0 }}>Build history</p>
          <p className="of-text-muted" style={{ margin: '2px 0 0', fontSize: 11 }}>
            {runs.length === 0 ? 'No runs yet' : `${runs.length} recent run${runs.length === 1 ? '' : 's'}`}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button
            type="button"
            onClick={() => void reload()}
            disabled={loading}
            className="of-button"
            style={{ fontSize: 11 }}
          >
            <Glyph name="history" size={12} />
            {loading ? 'Refreshing…' : 'Refresh'}
          </button>
          {!readOnly && (
            <button
              type="button"
              onClick={() => void trigger()}
              disabled={busy === 'trigger'}
              className="of-button of-button--primary"
              style={{ fontSize: 11 }}
            >
              <Glyph name="run" size={12} />
              {busy === 'trigger' ? 'Triggering…' : 'Trigger run'}
            </button>
          )}
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ margin: 10, padding: '6px 10px', borderRadius: 'var(--radius-sm)', fontSize: 12 }}>
          {error}
        </div>
      )}

      <div className="of-scrollbar" style={{ overflowX: 'auto' }}>
        <table className="of-table" style={{ minWidth: 600 }}>
          <thead>
            <tr>
              <th>Status</th>
              <th>Trigger</th>
              <th>Started</th>
              <th>Finished</th>
              <th>Attempt</th>
              <th style={{ textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {runs.length === 0 && !loading ? (
              <tr>
                <td colSpan={6} style={{ padding: 20, textAlign: 'center' }}>
                  <span className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 12 }}>
                    No runs yet. Trigger one to populate this history.
                  </span>
                </td>
              </tr>
            ) : (
              runs.map((run) => {
                const isSelected = run.id === selectedRunId;
                return (
                  <tr
                    key={run.id}
                    style={{
                      background: isSelected ? 'var(--bg-hover)' : undefined,
                      cursor: 'pointer',
                    }}
                    onClick={() => setSelectedRunId(run.id)}
                  >
                    <td>
                      <span
                        className={statusTone(run.status)}
                        style={{
                          display: 'inline-flex',
                          padding: '2px 8px',
                          borderRadius: 999,
                          fontSize: 11,
                          fontWeight: 700,
                        }}
                      >
                        {run.status}
                      </span>
                    </td>
                    <td className="of-text-muted">{run.trigger_type}</td>
                    <td className="of-text-muted">{fmt(run.started_at)}</td>
                    <td className="of-text-muted">{fmt(run.finished_at)}</td>
                    <td>#{run.attempt_number}</td>
                    <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'inline-flex', gap: 4 }}>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            setSelectedRunId(run.id);
                          }}
                          className="of-button"
                          style={{ fontSize: 11 }}
                        >
                          Logs
                        </button>
                        {!readOnly && (run.status === 'running' || run.status === 'queued') && (
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              void abort(run.id);
                            }}
                            disabled={busy === run.id}
                            className="of-button"
                            style={{ fontSize: 11, color: 'var(--status-danger)', borderColor: '#e0b4b1' }}
                          >
                            Abort
                          </button>
                        )}
                        {!readOnly && (run.status === 'failed' || run.status === 'aborted' || run.status === 'cancelled') && (
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              void retry(run.id);
                            }}
                            disabled={busy === run.id}
                            className="of-button"
                            style={{ fontSize: 11 }}
                          >
                            Retry
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {selectedRun && (
        <div style={{ borderTop: '1px solid var(--border-subtle)', padding: 10, display: 'grid', gap: 10 }}>
          {LIVE_STATUSES.has(selectedRun.status) ? (
            <LiveLogViewer jobRid={logRidForRun(selectedRun)} mode="live" />
          ) : (
            <>
              <RunLogs run={selectedRun} onClose={() => setSelectedRunId(null)} />
              <LiveLogViewer jobRid={logRidForRun(selectedRun)} mode="historical" />
            </>
          )}
        </div>
      )}
    </section>
  );
}
