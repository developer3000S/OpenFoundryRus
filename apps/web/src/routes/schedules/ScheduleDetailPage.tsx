import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import {
  getSchedule,
  deleteSchedule,
  listScheduleRuns,
  listScheduleVersions,
  pauseSchedule,
  resumeSchedule,
  runScheduleNow,
  setAutoPauseExempt,
  type RunOutcome,
  type Schedule,
  type ScheduleRun,
  type ScheduleTarget,
  type ScheduleVersion,
  type Trigger,
} from '@/lib/api/schedules';
import { notifications } from '@/lib/stores/notifications';

type TabKey = 'overview' | 'runs' | 'versions' | 'raw';
type BusyAction = 'run-now' | 'pause' | 'resume' | 'auto-pause' | 'refresh' | 'delete' | null;
type NoticeTone = 'success' | 'error' | 'info';

interface Notice {
  tone: NoticeTone;
  text: string;
}

interface TargetSummary {
  label: string;
  primary: string;
  secondary: string;
}

const TABS: Array<{ key: TabKey; label: string }> = [
  { key: 'overview', label: 'Overview' },
  { key: 'runs', label: 'Run history' },
  { key: 'versions', label: 'Versions' },
  { key: 'raw', label: 'Raw JSON' },
];

const RUN_FILTERS: Array<'all' | RunOutcome> = ['all', 'SUCCEEDED', 'IGNORED', 'FAILED'];

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatDuration(start: string | null | undefined, end: string | null | undefined): string {
  if (!start || !end) return end ? '-' : 'In progress';
  const startTime = new Date(start).getTime();
  const endTime = new Date(end).getTime();
  if (Number.isNaN(startTime) || Number.isNaN(endTime) || endTime < startTime) return '-';
  const seconds = Math.round((endTime - startTime) / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainingSeconds}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}

function compactRid(value: string | null | undefined, length = 22): string {
  if (!value) return '-';
  if (value.length <= length) return value;
  return `${value.slice(0, length)}...`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function readString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  return typeof value === 'string' ? value : '';
}

function readBoolean(record: Record<string, unknown>, key: string): boolean | null {
  const value = record[key];
  return typeof value === 'boolean' ? value : null;
}

function humanizeKey(value: string): string {
  return value
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function summarizeTrigger(trigger: Trigger): string {
  const kind = trigger.kind;
  if ('time' in kind) {
    return `${kind.time.cron} in ${kind.time.time_zone || 'UTC'}`;
  }
  if ('event' in kind) {
    const branches = kind.event.branch_filter?.length ? ` on ${kind.event.branch_filter.join(', ')}` : '';
    return `${humanizeKey(kind.event.type)} for ${compactRid(kind.event.target_rid)}${branches}`;
  }
  if ('compound' in kind) {
    return `${kind.compound.op} trigger with ${kind.compound.components.length} condition(s)`;
  }
  return 'Unknown trigger';
}

function summarizeTarget(target: ScheduleTarget): TargetSummary {
  const kind = target.kind ?? {};
  const pipelineBuild = kind.pipeline_build ?? kind.pipelineBuild;
  const datasetBuild = kind.dataset_build ?? kind.datasetBuild;
  const syncRun = kind.sync_run ?? kind.syncRun;
  const healthCheck = kind.health_check ?? kind.healthCheck;

  if (isRecord(pipelineBuild)) {
    const branch = readString(pipelineBuild, 'build_branch') || 'default branch';
    const force = readBoolean(pipelineBuild, 'force_build') ? 'force build' : 'stale only';
    return {
      label: 'Pipeline build',
      primary: readString(pipelineBuild, 'pipeline_rid') || '-',
      secondary: `${branch} - ${force}`,
    };
  }

  if (isRecord(datasetBuild)) {
    const branch = readString(datasetBuild, 'build_branch') || 'default branch';
    const force = readBoolean(datasetBuild, 'force_build') ? 'force build' : 'stale only';
    return {
      label: 'Dataset build',
      primary: readString(datasetBuild, 'dataset_rid') || '-',
      secondary: `${branch} - ${force}`,
    };
  }

  if (isRecord(syncRun)) {
    return {
      label: 'Sync run',
      primary: readString(syncRun, 'sync_rid') || '-',
      secondary: readString(syncRun, 'source_rid') || 'No source RID',
    };
  }

  if (isRecord(healthCheck)) {
    return {
      label: 'Health check',
      primary: readString(healthCheck, 'check_rid') || '-',
      secondary: 'Health-check target',
    };
  }

  const [firstKey, firstValue] = Object.entries(kind)[0] ?? ['target', null];
  return {
    label: humanizeKey(firstKey),
    primary: isRecord(firstValue) ? compactRid(Object.values(firstValue).find((value) => typeof value === 'string') as string | undefined) : '-',
    secondary: 'Custom schedule target',
  };
}

function outcomeClass(outcome: RunOutcome): string {
  if (outcome === 'SUCCEEDED') return 'of-status-success';
  if (outcome === 'FAILED') return 'of-status-danger';
  return 'of-status-info';
}

function noticeClass(tone: NoticeTone): string {
  if (tone === 'success') return 'of-status-success';
  if (tone === 'error') return 'of-status-danger';
  return 'of-status-info';
}

function ScheduleStatusBadge({ schedule }: { schedule: Schedule }) {
  return (
    <span className={`of-chip ${schedule.paused ? 'of-status-warning' : 'of-status-success'}`}>
      {schedule.paused ? 'Paused' : 'Active'}
    </span>
  );
}

function OutcomeBadge({ outcome }: { outcome: RunOutcome }) {
  return <span className={`of-chip ${outcomeClass(outcome)}`}>{outcome}</span>;
}

function MetricCard({ label, value, sub }: { label: string; value: ReactNode; sub?: ReactNode }) {
  return (
    <section className="of-panel" style={{ padding: 14, minHeight: 96 }}>
      <p className="of-eyebrow">{label}</p>
      <div style={{ marginTop: 8, color: 'var(--text-strong)', fontSize: 18, fontWeight: 600 }}>{value}</div>
      {sub && <p className="of-text-muted" style={{ marginTop: 6, fontSize: 12 }}>{sub}</p>}
    </section>
  );
}

function KeyValueList({ items }: { items: Array<{ label: string; value: ReactNode }> }) {
  return (
    <dl style={{ display: 'grid', gridTemplateColumns: '150px minmax(0, 1fr)', gap: '8px 14px', margin: 0, fontSize: 12 }}>
      {items.map((item) => (
        <div key={item.label} style={{ display: 'contents' }}>
          <dt className="of-text-muted">{item.label}</dt>
          <dd style={{ margin: 0, minWidth: 0, overflowWrap: 'anywhere' }}>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function TriggerTree({ trigger, depth = 0 }: { trigger: Trigger; depth?: number }) {
  const kind = trigger.kind;
  const marginLeft = depth * 14;

  if ('time' in kind) {
    return (
      <div style={{ marginLeft, display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, padding: '6px 0' }}>
        <span className="of-chip of-status-info">Time</span>
        <code style={{ fontFamily: 'var(--font-mono)' }}>{kind.time.cron}</code>
        <span className="of-text-muted">{kind.time.time_zone || 'UTC'}</span>
        <span className="of-text-muted">{kind.time.flavor}</span>
      </div>
    );
  }

  if ('event' in kind) {
    return (
      <div style={{ marginLeft, display: 'grid', gap: 4, padding: '6px 0' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
          <span className="of-chip of-status-info">Event</span>
          <strong style={{ color: 'var(--text-strong)' }}>{humanizeKey(kind.event.type)}</strong>
          <code style={{ fontFamily: 'var(--font-mono)' }}>{compactRid(kind.event.target_rid, 34)}</code>
        </div>
        {kind.event.branch_filter?.length ? (
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>
            Branches: {kind.event.branch_filter.join(', ')}
          </p>
        ) : null}
      </div>
    );
  }

  return (
    <div style={{ marginLeft, padding: '6px 0' }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
        <span className="of-chip of-status-warning">{kind.compound.op}</span>
        <span className="of-text-muted">{kind.compound.components.length} condition(s)</span>
      </div>
      <div style={{ marginTop: 4 }}>
        {kind.compound.components.map((component, index) => (
          <TriggerTree key={`${depth}-${index}`} trigger={component} depth={depth + 1} />
        ))}
      </div>
    </div>
  );
}

export function ScheduleDetailPage() {
  const navigate = useNavigate();
  const { rid: routeRid = '' } = useParams<{ rid: string }>();
  const rid = useMemo(() => safeDecode(routeRid), [routeRid]);

  const [schedule, setSchedule] = useState<Schedule | null>(null);
  const [runs, setRuns] = useState<ScheduleRun[]>([]);
  const [versions, setVersions] = useState<ScheduleVersion[]>([]);
  const [activeTab, setActiveTab] = useState<TabKey>('overview');
  const [runFilter, setRunFilter] = useState<'all' | RunOutcome>('all');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState<Notice | null>(null);
  const [pauseReason, setPauseReason] = useState('Manually paused');

  const refresh = useCallback(async (options: { showLoading?: boolean } = {}) => {
    if (!rid) return;
    const showLoading = options.showLoading ?? false;
    if (showLoading) setLoading(true);
    else setRefreshing(true);
    setError('');

    try {
      const [nextSchedule, runRes, versionRes] = await Promise.all([
        getSchedule(rid),
        listScheduleRuns(rid, { limit: 50 }).catch(() => null),
        listScheduleVersions(rid, { limit: 20 }).catch(() => null),
      ]);
      setSchedule(nextSchedule);
      setRuns(runRes?.data ?? []);
      setVersions(versionRes?.data ?? []);
    } catch (cause) {
      setSchedule(null);
      setRuns([]);
      setVersions([]);
      setError(cause instanceof Error ? cause.message : 'Failed to load schedule');
    } finally {
      if (showLoading) setLoading(false);
      else setRefreshing(false);
    }
  }, [rid]);

  useEffect(() => {
    void refresh({ showLoading: true });
  }, [refresh]);

  async function performAction<T>(action: Exclude<BusyAction, null>, task: () => Promise<T>, success: (result: T) => string) {
    setBusyAction(action);
    setError('');
    setNotice(null);
    try {
      const result = await task();
      const text = success(result);
      setNotice({ tone: 'success', text });
      notifications.success(text);
      if (action === 'delete' && result === true) return;
      await refresh();
    } catch (cause) {
      const text = cause instanceof Error ? cause.message : 'Schedule action failed';
      setNotice({ tone: 'error', text });
      notifications.error(text);
    } finally {
      setBusyAction(null);
    }
  }

  const targetSummary = useMemo(() => (schedule ? summarizeTarget(schedule.target) : null), [schedule]);

  const runStats = useMemo(() => {
    const total = runs.length;
    const succeeded = runs.filter((run) => run.outcome === 'SUCCEEDED').length;
    const failed = runs.filter((run) => run.outcome === 'FAILED').length;
    const ignored = runs.filter((run) => run.outcome === 'IGNORED').length;
    const latest = runs[0] ?? null;
    return { total, succeeded, failed, ignored, latest };
  }, [runs]);

  const visibleRuns = useMemo(
    () => (runFilter === 'all' ? runs : runs.filter((run) => run.outcome === runFilter)),
    [runFilter, runs],
  );

  const isBusy = busyAction !== null;

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading schedule...</p>
      </section>
    );
  }

  if (!schedule || !targetSummary) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/build-schedules" style={{ color: 'var(--text-muted)', fontSize: 13 }}>Back to schedules</Link>
        <p className="of-status-danger" style={{ marginTop: 12, padding: '10px 14px', borderRadius: 'var(--radius-md)' }}>
          {error || 'Schedule not found'}
        </p>
      </section>
    );
  }

  return (
    <section className="of-page" data-testid="schedule-detail-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 1320, margin: '0 auto' }}>
      <nav style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 13 }}>
        <Link to="/build-schedules" style={{ color: 'var(--text-muted)' }}>Build schedules</Link>
        <span className="of-text-muted">/</span>
        <code style={{ color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{compactRid(schedule.rid, 42)}</code>
      </nav>

      <header style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <div style={{ minWidth: 280 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
            <h1 className="of-heading-xl">{schedule.name || 'Untitled schedule'}</h1>
            <ScheduleStatusBadge schedule={schedule} />
            {schedule.auto_pause_exempt && <span className="of-chip of-status-info">Auto-pause exempt</span>}
          </div>
          <p className="of-text-muted" style={{ marginTop: 4, maxWidth: 760 }}>
            {schedule.description || 'No description provided.'}
          </p>
        </div>

        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          <button
            type="button"
            onClick={() => void performAction('refresh', () => refresh(), () => 'Schedule refreshed')}
            disabled={isBusy || refreshing}
            className="of-button"
          >
            {refreshing || busyAction === 'refresh' ? 'Refreshing...' : 'Refresh'}
          </button>
          <button
            type="button"
            data-testid="schedule-run-now"
            onClick={() => void performAction('run-now', () => runScheduleNow(rid), (result) => `Run queued: ${compactRid(result.run_id || result.schedule_rid)}`)}
            disabled={isBusy}
            className="of-button of-button--primary"
            title="Requires schedules.run"
          >
            {busyAction === 'run-now' ? 'Running...' : 'Run now'}
          </button>
          {schedule.paused ? (
            <button
              type="button"
              data-testid="schedule-resume"
              onClick={() => void performAction('resume', () => resumeSchedule(rid), () => 'Schedule resumed')}
              disabled={isBusy}
              className="of-button"
              title="Requires schedules.manage"
            >
              {busyAction === 'resume' ? 'Resuming...' : 'Resume'}
            </button>
          ) : (
            <button
              type="button"
              data-testid="schedule-pause"
              onClick={() => void performAction('pause', () => pauseSchedule(rid, pauseReason.trim() || 'Manually paused'), () => 'Schedule paused')}
              disabled={isBusy}
              className="of-button"
              title="Requires schedules.manage"
            >
              {busyAction === 'pause' ? 'Pausing...' : 'Pause'}
            </button>
          )}
          <button
            type="button"
            onClick={() => void performAction(
              'delete',
              async () => {
                if (!window.confirm(`Delete schedule "${schedule.name}"?`)) return false;
                await deleteSchedule(rid);
                return true;
              },
              (deleted) => {
                if (deleted) navigate('/build-schedules');
                return deleted ? 'Schedule deleted' : 'Delete cancelled';
              },
            )}
            disabled={isBusy}
            className="of-button"
            title="Requires schedules.manage"
          >
            {busyAction === 'delete' ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </header>

      {error && (
        <div role="alert" className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {notice && (
        <div className={noticeClass(notice.tone)} style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {notice.text}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
        <MetricCard
          label="State"
          value={<ScheduleStatusBadge schedule={schedule} />}
          sub={schedule.paused ? schedule.paused_reason || 'Paused without a reason' : 'Eligible for scheduled dispatch'}
        />
        <MetricCard
          label="Last run"
          value={formatTimestamp(schedule.last_run_at)}
          sub={runStats.latest ? <OutcomeBadge outcome={runStats.latest.outcome} /> : 'No recent run history'}
        />
        <MetricCard
          label="Active run"
          value={<code style={{ fontFamily: 'var(--font-mono)' }}>{compactRid(schedule.active_run_id, 24)}</code>}
          sub={schedule.pending_re_run ? 'Pending re-run requested' : 'No pending re-run'}
        />
        <MetricCard
          label="Recent outcomes"
          value={`${runStats.failed}/${runStats.total} failed`}
          sub={`${runStats.succeeded} succeeded - ${runStats.ignored} ignored`}
        />
      </div>

      <div className="of-tabbar" role="tablist" aria-label="Schedule detail tabs">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            type="button"
            role="tab"
            aria-selected={activeTab === tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`of-tab ${activeTab === tab.key ? 'of-tab-active' : ''}`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'overview' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 380px), 1fr))', gap: 16 }}>
          <div style={{ display: 'grid', gap: 16 }}>
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">When to build</p>
              <h2 className="of-heading-md" style={{ marginTop: 8 }}>{summarizeTrigger(schedule.trigger)}</h2>
              <div style={{ marginTop: 12, borderTop: '1px solid var(--border-subtle)', paddingTop: 8 }}>
                <TriggerTree trigger={schedule.trigger} />
              </div>
            </section>

            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Target</p>
              <h2 className="of-heading-md" style={{ marginTop: 8 }}>{targetSummary.label}</h2>
              <KeyValueList
                items={[
                  { label: 'Primary RID', value: <code style={{ fontFamily: 'var(--font-mono)' }}>{targetSummary.primary}</code> },
                  { label: 'Configuration', value: targetSummary.secondary },
                ]}
              />
            </section>

            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Build scope</p>
              <KeyValueList
                items={[
                  { label: 'Project', value: <code style={{ fontFamily: 'var(--font-mono)' }}>{schedule.project_rid}</code> },
                  { label: 'Scope kind', value: <span className="of-chip">{schedule.scope_kind}</span> },
                  {
                    label: 'Project scope RIDs',
                    value: schedule.project_scope_rids.length ? schedule.project_scope_rids.map((scopeRid) => (
                      <span key={scopeRid} className="of-chip" style={{ marginRight: 4, marginBottom: 4 }}>
                        {compactRid(scopeRid, 28)}
                      </span>
                    )) : '-',
                  },
                  { label: 'Run as user', value: schedule.run_as_user_id ? <code style={{ fontFamily: 'var(--font-mono)' }}>{schedule.run_as_user_id}</code> : '-' },
                  { label: 'Service principal', value: schedule.service_principal_id ? <code style={{ fontFamily: 'var(--font-mono)' }}>{schedule.service_principal_id}</code> : '-' },
                ]}
              />
            </section>
          </div>

          <aside style={{ display: 'grid', gap: 16, alignContent: 'start' }}>
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Controls</p>
              {!schedule.paused && (
                <label style={{ display: 'grid', gap: 4, marginTop: 12, fontSize: 12 }}>
                  Pause reason
                  <input
                    value={pauseReason}
                    onChange={(event) => setPauseReason(event.target.value)}
                    className="of-input"
                    placeholder="Reason recorded on pause"
                  />
                </label>
              )}
              <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                <button
                  type="button"
                  onClick={() => void performAction('run-now', () => runScheduleNow(rid), (result) => `Run queued: ${compactRid(result.run_id || result.schedule_rid)}`)}
                  disabled={isBusy}
                  className="of-button of-button--primary"
                  title="Requires schedules.run"
                >
                  {busyAction === 'run-now' ? 'Running...' : 'Run now'}
                </button>
                {schedule.paused ? (
                  <button
                    type="button"
                    onClick={() => void performAction('resume', () => resumeSchedule(rid), () => 'Schedule resumed')}
                    disabled={isBusy}
                    className="of-button"
                    title="Requires schedules.manage"
                  >
                    {busyAction === 'resume' ? 'Resuming...' : 'Resume schedule'}
                  </button>
                ) : (
                  <button
                    type="button"
                    onClick={() => void performAction('pause', () => pauseSchedule(rid, pauseReason.trim() || 'Manually paused'), () => 'Schedule paused')}
                    disabled={isBusy}
                    className="of-button"
                    title="Requires schedules.manage"
                  >
                    {busyAction === 'pause' ? 'Pausing...' : 'Pause schedule'}
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => void performAction(
                    'auto-pause',
                    () => setAutoPauseExempt(rid, !schedule.auto_pause_exempt),
                    () => (schedule.auto_pause_exempt ? 'Auto-pause exemption disabled' : 'Auto-pause exemption enabled'),
                  )}
                  disabled={isBusy}
                  className="of-button"
                  title="Requires schedules.manage"
                >
                  {busyAction === 'auto-pause'
                    ? 'Updating...'
                    : schedule.auto_pause_exempt
                      ? 'Disable auto-pause exemption'
                      : 'Enable auto-pause exemption'}
                </button>
              </div>
              <div style={{ marginTop: 14, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                <span className="of-chip">schedules.run</span>
                <span className="of-chip">schedules.manage</span>
              </div>
            </section>

            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Latest activity</p>
              <KeyValueList
                items={[
                  { label: 'Latest run', value: runStats.latest ? <OutcomeBadge outcome={runStats.latest.outcome} /> : '-' },
                  { label: 'Triggered at', value: formatTimestamp(runStats.latest?.triggered_at) },
                  { label: 'Finished at', value: formatTimestamp(runStats.latest?.finished_at) },
                  { label: 'Failure reason', value: runStats.latest?.failure_reason || '-' },
                ]}
              />
            </section>

            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Audit</p>
              <KeyValueList
                items={[
                  { label: 'Version', value: `v${schedule.version}` },
                  { label: 'Created by', value: <code style={{ fontFamily: 'var(--font-mono)' }}>{schedule.created_by}</code> },
                  { label: 'Created', value: formatTimestamp(schedule.created_at) },
                  { label: 'Updated', value: formatTimestamp(schedule.updated_at) },
                ]}
              />
            </section>
          </aside>
        </div>
      )}

      {activeTab === 'runs' && (
        <section className="of-panel" style={{ padding: 16, overflowX: 'auto' }}>
          <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
            <div>
              <p className="of-eyebrow">Run history</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>{visibleRuns.length} run(s)</h2>
            </div>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
              Outcome
              <select value={runFilter} onChange={(event) => setRunFilter(event.target.value as 'all' | RunOutcome)} className="of-input" style={{ width: 150 }}>
                {RUN_FILTERS.map((filter) => (
                  <option key={filter} value={filter}>{filter === 'all' ? 'All' : filter}</option>
                ))}
              </select>
            </label>
          </header>

          {visibleRuns.length === 0 ? (
            <p className="of-text-muted" style={{ marginTop: 12 }}>No runs match this filter.</p>
          ) : (
            <table className="of-table" style={{ marginTop: 12 }}>
              <thead>
                <tr>
                  {['Triggered', 'Outcome', 'Duration', 'Build', 'Version', 'Reason', 'Snapshot', 'Diagnostics'].map((heading) => (
                    <th key={heading}>{heading}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {visibleRuns.map((run) => (
                  <tr key={run.id || run.rid}>
                    <td>{formatTimestamp(run.triggered_at)}</td>
                    <td><OutcomeBadge outcome={run.outcome} /></td>
                    <td>{formatDuration(run.triggered_at, run.finished_at)}</td>
                    <td>
                      {run.build_rid ? (
                        <Link to={`/builds/${encodeURIComponent(run.build_rid)}`}>{compactRid(run.build_rid)}</Link>
                      ) : '-'}
                    </td>
                    <td>v{run.schedule_version}</td>
                    <td>{run.failure_reason || '-'}</td>
                    <td>
                      {Object.keys(run.trigger_snapshot).length ? (
                        <details>
                          <summary>{Object.keys(run.trigger_snapshot).length} field(s)</summary>
                          <pre style={{ margin: '8px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11, whiteSpace: 'pre-wrap' }}>
                            {JSON.stringify(run.trigger_snapshot, null, 2)}
                          </pre>
                        </details>
                      ) : '-'}
                    </td>
                    <td>
                      {Object.keys(run.diagnostics ?? {}).length ? (
                        <details>
                          <summary>{Object.keys(run.diagnostics ?? {}).length} field(s)</summary>
                          <pre style={{ margin: '8px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11, whiteSpace: 'pre-wrap' }}>
                            {JSON.stringify(run.diagnostics, null, 2)}
                          </pre>
                        </details>
                      ) : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      {activeTab === 'versions' && (
        <section className="of-panel" style={{ padding: 16, overflowX: 'auto' }}>
          <p className="of-eyebrow">Versions</p>
          <h2 className="of-heading-md" style={{ marginTop: 4 }}>{versions.length} version(s)</h2>
          {versions.length === 0 ? (
            <p className="of-text-muted" style={{ marginTop: 12 }}>No version history.</p>
          ) : (
            <table className="of-table" style={{ marginTop: 12 }}>
              <thead>
                <tr>
                  {['Version', 'Edited', 'Edited by', 'Name', 'Comment', 'Definition'].map((heading) => (
                    <th key={heading}>{heading}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {versions.map((version) => (
                  <tr key={version.id || version.version}>
                    <td>v{version.version}</td>
                    <td>{formatTimestamp(version.edited_at)}</td>
                    <td><code style={{ fontFamily: 'var(--font-mono)' }}>{version.edited_by}</code></td>
                    <td>{version.name}</td>
                    <td>{version.comment || '-'}</td>
                    <td>
                      <details>
                        <summary>Trigger and target</summary>
                        <pre style={{ margin: '8px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11, whiteSpace: 'pre-wrap' }}>
                          {JSON.stringify({ trigger: version.trigger_json, target: version.target_json }, null, 2)}
                        </pre>
                      </details>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      {activeTab === 'raw' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Raw JSON</p>
          <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 'var(--radius-md)', overflow: 'auto', maxHeight: 520 }}>
            {JSON.stringify({ schedule, runs, versions }, null, 2)}
          </pre>
        </section>
      )}
    </section>
  );
}
