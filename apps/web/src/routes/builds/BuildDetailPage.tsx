import { useCallback, useEffect, useMemo, useState, type CSSProperties } from 'react';
import { Link, useParams } from 'react-router-dom';

import { ConfirmDialog } from '@/lib/components/ConfirmDialog';
import { ErrorBanner } from '@/lib/components/ErrorBanner';
import { LoadingState } from '@/lib/components/LoadingState';
import { Tabs } from '@/lib/components/Tabs';
import { ArtifactsPanel } from '@/lib/components/builds/ArtifactsPanel';
import { BuildRunLogs } from '@/lib/components/builds/BuildRunLogs';
import { StateBadge } from '@/lib/components/builds/StateBadge';
import {
  abortBuildV1,
  getBuildV1,
  getJobInputResolutionsV1,
  getJobOutputsV1,
  type BuildEnvelope,
  type BuildState,
  type InputResolutionRow,
  type Job,
  type JobInputResolutionsResponse,
  type JobOutputsResponse,
} from '@/lib/api/buildsV1';

type BuildDetailTab = 'overview' | 'logs' | 'artifacts' | 'raw';

const ABORTABLE_STATES: BuildState[] = ['BUILD_RESOLUTION', 'BUILD_QUEUED', 'BUILD_RUNNING'];

const TABS: Array<{ id: BuildDetailTab; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'logs', label: 'Logs' },
  { id: 'artifacts', label: 'Artifacts' },
  { id: 'raw', label: 'Raw' },
];

function safeDecode(value: string) {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function shortRid(value: string | null | undefined, chars = 12) {
  return value ? value.slice(0, chars) : '—';
}

function DatasetRidLink({ rid, chars = 18 }: { rid: string | null | undefined; chars?: number }) {
  if (!rid) return <>—</>;
  return (
    <Link to={`/datasets/${encodeURIComponent(rid)}`} className="of-link" style={{ fontFamily: 'var(--font-mono)' }}>
      {shortRid(rid, chars)}
    </Link>
  );
}

function formatDate(value: string | null | undefined) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function duration(start: string | null | undefined, end: string | null | undefined) {
  if (!start) return '—';
  const startDate = new Date(start);
  const endDate = end ? new Date(end) : new Date();
  if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) return '—';
  const ms = Math.max(0, endDate.getTime() - startDate.getTime());
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ${Math.floor((ms % 60_000) / 1000)}s`;
  return `${Math.floor(ms / 3_600_000)}h ${Math.floor((ms % 3_600_000) / 60_000)}m`;
}

function filterLabel(filter: InputResolutionRow['filter']) {
  switch (filter.kind) {
    case 'AT_TIMESTAMP':
      return `at ${filter.value}`;
    case 'AT_TRANSACTION':
      return `transaction ${shortRid(filter.transaction_rid, 18)}`;
    case 'RANGE':
      return `${shortRid(filter.from_transaction, 12)} → ${shortRid(filter.to_transaction, 12)}`;
    case 'INCREMENTAL_SINCE_LAST_BUILD':
      return 'incremental since last build';
    default:
      return (filter as { kind: string }).kind;
  }
}

export function BuildDetailPage() {
  const { rid: ridParam = '' } = useParams();
  const rid = useMemo(() => safeDecode(ridParam), [ridParam]);

  const [build, setBuild] = useState<BuildEnvelope | null>(null);
  const [selectedJobRid, setSelectedJobRid] = useState('');
  const [tab, setTab] = useState<BuildDetailTab>('overview');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [abortOpen, setAbortOpen] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const next = await getBuildV1(rid);
      setBuild(next);
      setSelectedJobRid((current) => {
        if (current && (next.jobs ?? []).some((job) => job.rid === current)) return current;
        return next.jobs?.[0]?.rid ?? '';
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load build');
    } finally {
      setLoading(false);
    }
  }, [rid]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectedJob = useMemo(
    () => build?.jobs?.find((job) => job.rid === selectedJobRid) ?? build?.jobs?.[0] ?? null,
    [build, selectedJobRid],
  );

  const canAbort = build ? ABORTABLE_STATES.includes(build.state) : false;

  async function handleAbort() {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await abortBuildV1(rid);
      setAbortOpen(false);
      setNotice('Abort requested.');
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Abort failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/builds" className="of-link" style={{ justifySelf: 'start', fontSize: 13 }}>
        ← Builds
      </Link>

      <ErrorBanner error={error} />
      {notice && (
        <div className="of-status-success" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {notice}
        </div>
      )}

      {loading && !build ? (
        <section className="of-panel" style={{ padding: 16 }}>
          <LoadingState label="Loading build…" />
        </section>
      ) : build ? (
        <>
          <header className="of-panel" style={{ padding: 16, display: 'grid', gap: 16 }}>
            <div style={{ display: 'flex', alignItems: 'start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
              <div style={{ minWidth: 0 }}>
                <p className="of-eyebrow" style={{ margin: 0 }}>Build detail</p>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 4, flexWrap: 'wrap' }}>
                  <h1 className="of-heading-xl" style={{ margin: 0, fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>
                    {shortRid(build.rid, 18)}
                  </h1>
                  <StateBadge kind="build" state={build.state} />
                </div>
                <p className="of-text-muted" style={{ margin: '6px 0 0', fontSize: 12, fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>
                  {build.rid}
                </p>
              </div>

              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                <button type="button" onClick={() => void refresh()} disabled={loading || busy} className="of-button">
                  {loading ? 'Refreshing…' : 'Refresh'}
                </button>
                {canAbort && (
                  <button
                    type="button"
                    onClick={() => setAbortOpen(true)}
                    disabled={busy}
                    className="of-button"
                    style={{ color: '#b91c1c', borderColor: '#fecaca' }}
                  >
                    Abort
                  </button>
                )}
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 10 }}>
              <Stat label="Pipeline" value={shortRid(build.pipeline_rid, 18)} mono />
              <Stat label="Branch" value={build.build_branch} />
              <Stat label="Trigger" value={build.trigger_kind} />
              <Stat label="Jobs" value={String(build.jobs?.length ?? 0)} />
              <Stat label="Duration" value={duration(build.started_at, build.finished_at)} />
              <Stat label="Requested by" value={build.requested_by || '—'} />
            </div>
          </header>

          <Tabs tabs={TABS} active={tab} onChange={setTab} />

          {tab === 'overview' && (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16, alignItems: 'start' }}>
              <section style={{ display: 'grid', gap: 16 }}>
                <JobsPanel build={build} selectedJobRid={selectedJob?.rid ?? ''} onSelectJob={setSelectedJobRid} />
                <BuildTimeline build={build} />
              </section>
              <JobDataPanel job={selectedJob} />
            </div>
          )}

          {tab === 'logs' && (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16, alignItems: 'start' }}>
              <JobsPanel build={build} selectedJobRid={selectedJob?.rid ?? ''} onSelectJob={setSelectedJobRid} compact />
              <BuildRunLogs job={selectedJob} />
            </div>
          )}

          {tab === 'artifacts' && (
            <ArtifactsPanel build={build} selectedJobRid={selectedJob?.rid ?? ''} onSelectJob={setSelectedJobRid} />
          )}

          {tab === 'raw' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow" style={{ margin: 0 }}>Build envelope</p>
              <pre style={preStyle}>{JSON.stringify(build, null, 2)}</pre>
            </section>
          )}

          <ConfirmDialog
            open={abortOpen}
            title="Abort build"
            message={`Abort build ${shortRid(build.rid, 18)}? Running jobs will move to abort pending.`}
            confirmLabel="Abort"
            danger
            busy={busy}
            onCancel={() => setAbortOpen(false)}
            onConfirm={() => void handleAbort()}
          />
        </>
      ) : (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-text-muted" style={{ margin: 0 }}>Build not found.</p>
        </section>
      )}
    </section>
  );
}

function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="of-panel-muted" style={{ padding: 10, minWidth: 0 }}>
      <dt className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', fontWeight: 700 }}>{label}</dt>
      <dd style={{ margin: '4px 0 0', fontWeight: 600, fontFamily: mono ? 'var(--font-mono)' : undefined, overflowWrap: 'anywhere' }}>{value}</dd>
    </div>
  );
}

function JobsPanel({
  build,
  selectedJobRid,
  onSelectJob,
  compact = false,
}: {
  build: BuildEnvelope;
  selectedJobRid: string;
  onSelectJob: (rid: string) => void;
  compact?: boolean;
}) {
  const jobs = build.jobs ?? [];
  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 10 }}>
      <header>
        <p className="of-eyebrow" style={{ margin: 0 }}>Jobs</p>
        <h2 className="of-heading-md" style={{ margin: '4px 0 0' }}>{jobs.length} job{jobs.length === 1 ? '' : 's'}</h2>
      </header>

      {jobs.length === 0 ? (
        <p className="of-text-muted" style={{ margin: 0 }}>No jobs recorded for this build.</p>
      ) : (
        <div style={{ display: 'grid', gap: 6 }}>
          {jobs.map((job) => {
            const selected = selectedJobRid === job.rid;
            return (
              <button
                key={job.rid}
                type="button"
                onClick={() => onSelectJob(job.rid)}
                className={selected ? 'of-button of-button--primary' : 'of-button'}
                style={{
                  justifyContent: 'space-between',
                  minHeight: compact ? 36 : 44,
                  padding: compact ? '6px 8px' : '8px 10px',
                  textAlign: 'left',
                  width: '100%',
                }}
              >
                <span style={{ minWidth: 0 }}>
                  <span style={{ display: 'block', fontFamily: 'var(--font-mono)', overflow: 'hidden', textOverflow: 'ellipsis' }}>{shortRid(job.rid, 18)}</span>
                  {!compact && <span style={{ display: 'block', marginTop: 2, fontSize: 11, opacity: 0.82 }}>attempt {job.attempt} · spec {shortRid(job.job_spec_rid, 12)}</span>}
                </span>
                <StateBadge kind="job" state={job.state} size="sm" />
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
}

function BuildTimeline({ build }: { build: BuildEnvelope }) {
  const events = [
    ['Created', build.created_at],
    ['Queued', build.queued_at],
    ['Started', build.started_at],
    ['Finished', build.finished_at],
  ] as const;

  return (
    <section className="of-panel" style={{ padding: 16 }}>
      <p className="of-eyebrow" style={{ margin: 0 }}>Timeline</p>
      <dl style={{ display: 'grid', gridTemplateColumns: 'max-content minmax(0, 1fr)', gap: '8px 14px', margin: '10px 0 0' }}>
        {events.map(([label, value]) => (
          <div key={label} style={{ display: 'contents' }}>
            <dt className="of-text-muted" style={{ fontSize: 12 }}>{label}</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>{formatDate(value)}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function JobDataPanel({ job }: { job: Job | null }) {
  const [outputs, setOutputs] = useState<JobOutputsResponse | null>(null);
  const [resolutions, setResolutions] = useState<JobInputResolutionsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    if (!job) {
      setOutputs(null);
      setResolutions(null);
      return;
    }

    const jobRid = job.rid;
    async function load() {
      setLoading(true);
      setError('');
      setOutputs(null);
      setResolutions(null);
      try {
        const [oRes, rRes] = await Promise.all([getJobOutputsV1(jobRid), getJobInputResolutionsV1(jobRid)]);
        if (cancelled) return;
        setOutputs(oRes);
        setResolutions(rRes);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load job details');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [job]);

  if (!job) {
    return (
      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-text-muted" style={{ margin: 0 }}>No job selected.</p>
      </section>
    );
  }

  const outputRows = outputs?.outputs ?? [];
  const resolutionRows = Array.isArray(resolutions?.input_view_resolutions) ? resolutions.input_view_resolutions : [];

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 14, minWidth: 0 }}>
      <header style={{ display: 'flex', alignItems: 'start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0 }}>
          <p className="of-eyebrow" style={{ margin: 0 }}>Selected job</p>
          <h2 className="of-heading-md" style={{ margin: '4px 0 0', fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>{job.rid}</h2>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
            attempt {job.attempt} · changed {formatDate(job.state_changed_at)}
          </p>
        </div>
        <StateBadge kind="job" state={job.state} />
      </header>

      {error && (
        <div className="of-status-warning" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}
      {loading && <LoadingState label="Loading job details…" inline />}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 12 }}>
        <section className="of-panel-muted" style={{ padding: 12, minWidth: 0 }}>
          <p className="of-eyebrow" style={{ margin: 0 }}>Outputs</p>
          {outputRows.length === 0 ? (
            <p className="of-text-muted" style={{ marginBottom: 0 }}>No output rows.</p>
          ) : (
            <div style={{ marginTop: 8, overflow: 'auto' }}>
              <table className="of-table" style={{ minWidth: 520 }}>
                <thead>
                  <tr>
                    <th>Dataset</th>
                    <th>Transaction</th>
                    <th>Commit</th>
                  </tr>
                </thead>
                <tbody>
                  {outputRows.map((row) => (
                    <tr key={`${row.output_dataset_rid}:${row.transaction_rid}`}>
                      <td><DatasetRidLink rid={row.output_dataset_rid} /></td>
                      <td style={{ fontFamily: 'var(--font-mono)' }}>{shortRid(row.transaction_rid, 18)}</td>
                      <td>{row.aborted ? 'aborted' : row.committed ? 'committed' : 'open'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="of-panel-muted" style={{ padding: 12, minWidth: 0 }}>
          <p className="of-eyebrow" style={{ margin: 0 }}>Input resolutions</p>
          {resolutionRows.length === 0 ? (
            <p className="of-text-muted" style={{ marginBottom: 0 }}>No input resolutions.</p>
          ) : (
            <div style={{ marginTop: 8, overflow: 'auto' }}>
              <table className="of-table" style={{ minWidth: 620 }}>
                <thead>
                  <tr>
                    <th>Dataset</th>
                    <th>Branch</th>
                    <th>Filter</th>
                    <th>Resolved</th>
                  </tr>
                </thead>
                <tbody>
                  {resolutionRows.map((row) => (
                    <tr key={`${row.dataset_rid}:${row.branch}:${filterLabel(row.filter)}`}>
                      <td><DatasetRidLink rid={row.dataset_rid} /></td>
                      <td>{row.branch}</td>
                      <td>{filterLabel(row.filter)}</td>
                      <td style={{ fontFamily: 'var(--font-mono)' }}>
                        {shortRid(row.resolved_transaction_rid ?? row.resolved_view_id ?? row.range_to_transaction_rid, 18)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>

      {(job.failure_reason || job.stale_skipped) && (
        <div className={job.failure_reason ? 'of-status-danger' : 'of-status-info'} style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {job.failure_reason || 'Job skipped because outputs were not stale.'}
        </div>
      )}
    </section>
  );
}

const preStyle: CSSProperties = {
  margin: '8px 0 0',
  padding: 12,
  background: 'var(--bg-subtle)',
  fontSize: 11,
  fontFamily: 'var(--font-mono)',
  borderRadius: 'var(--radius-md)',
  overflow: 'auto',
  maxHeight: 640,
};
