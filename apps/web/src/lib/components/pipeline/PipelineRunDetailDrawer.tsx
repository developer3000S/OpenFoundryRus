import { useEffect, useMemo, useState } from 'react';

import { Tabs } from '@/lib/components/Tabs';
import { getRun, pipelineNodeResultsFromRun, pipelineNodesFromDAG, type Pipeline, type PipelineRun } from '@/lib/api/pipelines';
import { LineageView } from './LineageView';
import { LiveLogViewer } from './LiveLogViewer';
import { RunLogs } from './RunLogs';

type RunDetailTab = 'summary' | 'logs' | 'lineage';

interface PipelineRunDetailDrawerProps {
  pipeline: Pipeline;
  runId: string;
  initialRun?: PipelineRun | null;
  busy?: boolean;
  onClose: () => void;
  onRetry: (runId: string) => Promise<void> | void;
  onAbort: (runId: string) => Promise<void> | void;
}

interface DatasetOption {
  id: string;
  label: string;
  role: 'Input' | 'Output';
  nodeId: string;
  nodeLabel: string;
}

const STATUS_STYLE: Record<string, { background: string; color: string }> = {
  queued: { background: '#334155', color: '#cbd5e1' },
  running: { background: '#1d4ed8', color: '#dbeafe' },
  pending: { background: '#1d4ed8', color: '#dbeafe' },
  succeeded: { background: '#166534', color: '#d1fae5' },
  completed: { background: '#166534', color: '#d1fae5' },
  failed: { background: '#991b1b', color: '#fee2e2' },
  cancelled: { background: '#92400e', color: '#fde68a' },
  aborted: { background: '#92400e', color: '#fde68a' },
};

function normalizedStatus(status: string) {
  const value = status.toLowerCase().replace(/^build_/, '');
  if (value === 'completed' || value === 'success') return 'succeeded';
  if (value === 'aborted' || value === 'canceled') return 'cancelled';
  if (value === 'pending' || value === 'resolution') return 'queued';
  return value;
}

function statusStyle(status: string) {
  return STATUS_STYLE[normalizedStatus(status)] ?? { background: '#334155', color: '#cbd5e1' };
}

function isLiveStatus(status: string) {
  const value = normalizedStatus(status);
  return (
    value.includes('running') ||
    value.includes('pending') ||
    value.includes('queued') ||
    value.includes('resolution') ||
    value.includes('aborting')
  );
}

function isRetryableStatus(status: string) {
  const value = normalizedStatus(status);
  return value.includes('failed') || value.includes('cancelled');
}

function isFailureStatus(status: string) {
  return normalizedStatus(status).includes('failed');
}

function shortId(id: string) {
  return id.length > 12 ? id.slice(0, 12) : id;
}

function formatTimestamp(value: string | null) {
  return value ? new Date(value).toLocaleString() : '-';
}

function formatDuration(run: PipelineRun) {
  const started = new Date(run.started_at).getTime();
  const finished = run.finished_at ? new Date(run.finished_at).getTime() : Date.now();
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) return '-';
  const seconds = Math.max(0, Math.round((finished - started) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return `${minutes}m ${remainder}s`;
}

function nodeStatusCounts(run: PipelineRun) {
  return pipelineNodeResultsFromRun(run).reduce<Record<string, number>>((counts, result) => {
    counts[result.status] = (counts[result.status] ?? 0) + 1;
    return counts;
  }, {});
}

function addDatasetOption(options: Map<string, DatasetOption>, option: DatasetOption) {
  if (!option.id || options.has(option.id)) return;
  options.set(option.id, option);
}

function buildDatasetOptions(pipeline: Pipeline, run: PipelineRun | null) {
  const options = new Map<string, DatasetOption>();
  const nodes = pipelineNodesFromDAG(pipeline.dag);
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  const orderedNodeIds = new Set([
    ...pipelineNodeResultsFromRun(run).map((result) => result.node_id),
    ...nodes.map((node) => node.id),
  ]);

  orderedNodeIds.forEach((nodeId) => {
    const node = nodesById.get(nodeId);
    if (!node) return;
    node.input_dataset_ids.forEach((datasetId) => {
      addDatasetOption(options, {
        id: datasetId,
        label: `${datasetId} - input to ${node.label}`,
        role: 'Input',
        nodeId: node.id,
        nodeLabel: node.label,
      });
    });
    if (node.output_dataset_id) {
      addDatasetOption(options, {
        id: node.output_dataset_id,
        label: `${node.output_dataset_id} - output from ${node.label}`,
        role: 'Output',
        nodeId: node.id,
        nodeLabel: node.label,
      });
    }
  });

  return Array.from(options.values());
}

function preferredDatasetId(options: DatasetOption[], run: PipelineRun | null) {
  if (options.length === 0) return '';
  const failedNodeId = pipelineNodeResultsFromRun(run).find((result) => isFailureStatus(result.status))?.node_id;
  if (failedNodeId) {
    return (
      options.find((option) => option.nodeId === failedNodeId && option.role === 'Output')?.id ??
      options.find((option) => option.nodeId === failedNodeId)?.id ??
      options[0].id
    );
  }
  return options.find((option) => option.role === 'Output')?.id ?? options[0].id;
}

export function PipelineRunDetailDrawer({
  pipeline,
  runId,
  initialRun = null,
  busy = false,
  onClose,
  onRetry,
  onAbort,
}: PipelineRunDetailDrawerProps) {
  const [run, setRun] = useState<PipelineRun | null>(initialRun);
  const [tab, setTab] = useState<RunDetailTab>('summary');
  const [selectedDatasetId, setSelectedDatasetId] = useState('');
  const [loading, setLoading] = useState(!initialRun);
  const [error, setError] = useState('');
  const [action, setAction] = useState<'retry' | 'abort' | null>(null);

  useEffect(() => {
    let cancelled = false;
    setRun(initialRun);
    setLoading(!initialRun);
    setError('');

    async function loadRun() {
      try {
        const fresh = await getRun(pipeline.id, runId);
        if (!cancelled) setRun(fresh);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load run detail');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void loadRun();
    return () => {
      cancelled = true;
    };
  }, [pipeline.id, runId, initialRun]);

  useEffect(() => {
    function handleKeydown(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose();
    }
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  }, [onClose]);

  const datasetOptions = useMemo(() => buildDatasetOptions(pipeline, run), [pipeline, run]);
  const nodeResults = useMemo(() => pipelineNodeResultsFromRun(run), [run]);
  const statusCounts = useMemo(() => (run ? nodeStatusCounts(run) : {}), [run]);
  const logMode = run && isLiveStatus(run.status) ? 'live' : 'historical';
  const jobRid =
    nodeResults.find((result) => isFailureStatus(result.status) && result.log_rid)?.log_rid ??
    nodeResults.find((result) => result.log_rid)?.log_rid ??
    `ri.foundry.main.job.${run?.id ?? runId}`;

  useEffect(() => {
    setSelectedDatasetId((current) => {
      if (current && datasetOptions.some((option) => option.id === current)) return current;
      return preferredDatasetId(datasetOptions, run);
    });
  }, [datasetOptions, run]);

  async function retry() {
    if (!run || busy || action) return;
    setAction('retry');
    try {
      await onRetry(run.id);
    } finally {
      setAction(null);
    }
  }

  async function abort() {
    if (!run || busy || action) return;
    if (typeof window !== 'undefined' && !window.confirm('Abort this run?')) return;
    setAction('abort');
    try {
      await onAbort(run.id);
    } finally {
      setAction(null);
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="pipeline-run-detail-title"
      style={{ position: 'fixed', inset: 0, zIndex: 60, display: 'flex', justifyContent: 'flex-end' }}
    >
      <div onClick={onClose} style={{ position: 'absolute', inset: 0, background: 'rgba(15, 23, 42, 0.32)' }} />
      <aside
        className="of-panel"
        style={{
          position: 'relative',
          zIndex: 1,
          display: 'flex',
          width: 'min(920px, calc(100vw - 32px))',
          height: '100vh',
          flexDirection: 'column',
          borderRadius: 0,
          borderTop: 0,
          borderRight: 0,
          borderBottom: 0,
          boxShadow: 'var(--shadow-popover)',
        }}
      >
        <header style={{ display: 'grid', gap: 10, padding: '12px 14px', borderBottom: '1px solid var(--border-default)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
            <div style={{ minWidth: 0 }}>
              <p className="of-eyebrow" style={{ margin: 0 }}>Pipeline run</p>
              <h2 id="pipeline-run-detail-title" className="of-heading-md" style={{ margin: '2px 0 0' }}>
                {pipeline.name}
              </h2>
              <p className="of-text-muted" style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                {shortId(runId)}
              </p>
            </div>
            <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              {run && (
                <span style={{ ...statusStyle(run.status), padding: '3px 8px', borderRadius: 3, fontSize: 11, fontWeight: 700 }}>
                  {run.status}
                </span>
              )}
              {run && isRetryableStatus(run.status) && (
                <button type="button" onClick={() => void retry()} disabled={busy || action !== null} className="of-button" style={{ fontSize: 11 }}>
                  {action === 'retry' ? 'Retrying...' : 'Retry'}
                </button>
              )}
              {run && isLiveStatus(run.status) && (
                <button type="button" onClick={() => void abort()} disabled={busy || action !== null} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                  {action === 'abort' ? 'Aborting...' : 'Abort'}
                </button>
              )}
              <button type="button" onClick={onClose} className="of-button" style={{ fontSize: 11 }}>
                Close
              </button>
            </div>
          </div>

          {run && (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(120px, 1fr))', gap: 8 }}>
              {[
                ['Started', formatTimestamp(run.started_at)],
                ['Finished', formatTimestamp(run.finished_at)],
                ['Duration', formatDuration(run)],
                ['Attempt', `#${run.attempt_number}`],
              ].map(([label, value]) => (
                <div key={label} className="of-panel-muted" style={{ padding: '8px 10px', borderRadius: 3 }}>
                  <p className="of-eyebrow" style={{ margin: 0, fontSize: 10 }}>{label}</p>
                  <p style={{ margin: '2px 0 0', fontSize: 12, color: 'var(--text-strong)' }}>{value}</p>
                </div>
              ))}
            </div>
          )}
        </header>

        <Tabs
          tabs={[
            { id: 'summary', label: 'Summary' },
            { id: 'logs', label: 'Logs' },
            { id: 'lineage', label: 'Lineage' },
          ] as const}
          active={tab}
          onChange={setTab}
        />

        <div className="of-scrollbar" style={{ flex: 1, overflow: 'auto', padding: 14 }}>
          {loading && <p className="of-text-muted" style={{ margin: 0 }}>Loading run detail...</p>}
          {error && (
            <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
              {error}
            </div>
          )}

          {!loading && run && tab === 'summary' && (
            <section style={{ display: 'grid', gap: 12 }}>
              <div className="of-panel-muted" style={{ display: 'flex', gap: 8, flexWrap: 'wrap', padding: 10 }}>
                <span className="of-chip">Trigger {run.trigger_type}</span>
                <span className="of-chip">Nodes {nodeResults.length}</span>
                {Object.entries(statusCounts).map(([status, count]) => (
                  <span key={status} className="of-chip">{status} {count}</span>
                ))}
                {run.started_from_node_id && <span className="of-chip">From {run.started_from_node_id}</span>}
                {run.retry_of_run_id && <span className="of-chip">Retry of {shortId(run.retry_of_run_id)}</span>}
              </div>
              <RunLogs run={run} />
            </section>
          )}

          {!loading && run && tab === 'logs' && (
            <LiveLogViewer jobRid={jobRid} mode={logMode} />
          )}

          {!loading && run && tab === 'lineage' && (
            <section style={{ display: 'grid', gap: 10 }}>
              {datasetOptions.length > 0 ? (
                <>
                  <div className="of-toolbar" style={{ justifyContent: 'space-between', flexWrap: 'wrap' }}>
                    <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 12, minWidth: 280 }}>
                      Dataset
                      <select
                        value={selectedDatasetId}
                        onChange={(event) => setSelectedDatasetId(event.target.value)}
                        className="of-select"
                        style={{ minWidth: 260 }}
                      >
                        {datasetOptions.map((option) => (
                          <option key={option.id} value={option.id}>
                            {option.role}: {option.label}
                          </option>
                        ))}
                      </select>
                    </label>
                    {selectedDatasetId && (
                      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                        <a className="of-button" href={`/datasets/${encodeURIComponent(selectedDatasetId)}`} style={{ fontSize: 11 }}>
                          Open dataset
                        </a>
                        <a className="of-button" href={`/lineage?dataset=${encodeURIComponent(selectedDatasetId)}`} style={{ fontSize: 11 }}>
                          Open lineage
                        </a>
                      </div>
                    )}
                  </div>
                  {selectedDatasetId ? (
                    <LineageView
                      datasetId={selectedDatasetId}
                      onSelect={(nodeId, kind) => {
                        if (kind === 'dataset') setSelectedDatasetId(nodeId);
                      }}
                    />
                  ) : (
                    <p className="of-text-muted" style={{ margin: 0 }}>Select a dataset to inspect lineage.</p>
                  )}
                </>
              ) : (
                <div className="of-panel-muted" style={{ padding: 12 }}>
                  <p className="of-text-muted" style={{ margin: 0 }}>
                    This pipeline DAG does not expose input or output dataset IDs for lineage lookup.
                  </p>
                </div>
              )}
            </section>
          )}
        </div>
      </aside>
    </div>
  );
}
