import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';

import {
  deleteSchedule,
  getScheduleVersionDiff,
  listScheduleRuns,
  listScheduleVersions,
  listSchedules,
  pauseSchedule,
  resumeSchedule,
  runScheduleNow,
  type ListSchedulesQuery,
  type RunOutcome,
  type Schedule,
  type ScheduleRun,
  type ScheduleTarget,
  type ScheduleVersion,
  type Trigger,
  type VersionDiff,
} from '@/lib/api/schedules';
import { ScheduleDiff } from '@/lib/components/pipeline/ScheduleDiff';
import { EditScheduleDialog } from '@/lib/components/schedules/EditScheduleDialog';

type PauseFilter = 'all' | 'paused' | 'active';
type SortKey = 'name' | 'created_at' | 'last_run_at' | 'updated_at';

interface DiffRange {
  from: number;
  to: number;
}

const SORT_LABELS: Record<SortKey, string> = {
  name: 'name',
  created_at: 'creation date',
  last_run_at: 'last run',
  updated_at: 'last update',
};

const OUTCOME_TONE: Record<RunOutcome, { background: string; color: string; borderColor: string }> = {
  SUCCEEDED: {
    background: 'var(--status-success-bg)',
    color: 'var(--status-success)',
    borderColor: 'var(--status-success)',
  },
  FAILED: {
    background: 'var(--status-danger-bg)',
    color: 'var(--status-danger)',
    borderColor: 'var(--status-danger)',
  },
  IGNORED: {
    background: 'var(--bg-panel-muted)',
    color: 'var(--text-muted)',
    borderColor: 'var(--border-strong)',
  },
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function formatDate(value: string | null) {
  if (!value) return 'Never';
  return new Date(value).toLocaleString();
}

function getTimeTrigger(trigger: Trigger) {
  if ('time' in trigger.kind) return trigger.kind.time;
  return null;
}

function summarizeTrigger(s: Schedule): string {
  const kind = s.trigger.kind;
  if ('time' in kind) return `${kind.time.cron} (${kind.time.time_zone})`;
  if ('event' in kind) return `On ${kind.event.type} -> ${kind.event.target_rid}`;
  if ('compound' in kind) return `${kind.compound.op} of ${kind.compound.components.length} components`;
  return 'Unknown trigger';
}

function titleize(value: string) {
  return value
    .split('_')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function summarizeTarget(target: ScheduleTarget): string {
  const [kind, value] = Object.entries(target.kind)[0] ?? [];
  if (!kind) return 'Unknown target';
  if (!isRecord(value)) return titleize(kind);

  const rid =
    value.pipeline_rid ??
    value.dataset_rid ??
    value.sync_rid ??
    value.check_rid ??
    value.source_rid;
  return rid ? `${titleize(kind)}: ${String(rid)}` : titleize(kind);
}

function getRunCounts(runs: ScheduleRun[]) {
  return runs.reduce(
    (acc, run) => {
      acc.total += 1;
      if (run.outcome === 'SUCCEEDED') acc.succeeded += 1;
      if (run.outcome === 'FAILED') acc.failed += 1;
      if (run.outcome === 'IGNORED') acc.ignored += 1;
      return acc;
    },
    { total: 0, succeeded: 0, failed: 0, ignored: 0 },
  );
}

function FilterTokens({
  values,
  onRemove,
}: {
  values: string[];
  onRemove: (value: string) => void;
}) {
  if (values.length === 0) return null;
  return (
    <ul style={{ listStyle: 'none', margin: '6px 0 0', padding: 0, display: 'flex', flexWrap: 'wrap', gap: 4 }}>
      {values.map((value) => (
        <li
          key={value}
          className="of-chip"
          style={{ display: 'flex', alignItems: 'center', gap: 4, maxWidth: '100%' }}
        >
          <code style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{value}</code>
          <button
            type="button"
            onClick={() => onRemove(value)}
            aria-label={`Remove ${value}`}
            className="of-button of-button--ghost"
            style={{ minHeight: 18, padding: '0 2px', fontSize: 12 }}
          >
            x
          </button>
        </li>
      ))}
    </ul>
  );
}

function ScheduleCard({
  schedule,
  selected,
  busy,
  onSelect,
  onEdit,
  onPause,
  onResume,
  onRunNow,
  onDelete,
}: {
  schedule: Schedule;
  selected: boolean;
  busy: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onPause: () => void;
  onResume: () => void;
  onRunNow: () => void;
  onDelete: () => void;
}) {
  const timeTrigger = getTimeTrigger(schedule.trigger);
  return (
    <article
      className="of-panel"
      data-testid="schedule-card"
      style={{
        borderColor: selected ? 'var(--border-focus)' : 'var(--border-default)',
        background: selected ? 'var(--status-info-bg)' : 'var(--bg-panel)',
      }}
    >
      <button
        type="button"
        onClick={onSelect}
        style={{
          width: '100%',
          border: 0,
          background: 'transparent',
          color: 'inherit',
          textAlign: 'left',
          padding: 0,
        }}
      >
        <div style={{ display: 'grid', gap: 10, padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
            <div style={{ minWidth: 0 }}>
              <h2 className="of-heading-sm" style={{ margin: 0, overflowWrap: 'anywhere' }}>
                {schedule.name}
              </h2>
              <p className="of-text-muted" style={{ margin: '3px 0 0', fontSize: 11 }}>
                <code>{schedule.rid}</code>
              </p>
            </div>
            <span
              className={schedule.paused ? 'of-chip of-status-warning' : 'of-chip of-status-success'}
              style={{ flexShrink: 0 }}
            >
              {schedule.paused ? 'Paused' : 'Active'}
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, fontSize: 12 }}>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>
                When to build
              </p>
              <p style={{ margin: '3px 0 0', fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>
                {summarizeTrigger(schedule)}
              </p>
            </div>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>
                Target
              </p>
              <p style={{ margin: '3px 0 0', overflowWrap: 'anywhere' }}>{summarizeTarget(schedule.target)}</p>
            </div>
          </div>

          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            <span className="of-chip">{schedule.scope_kind}</span>
            {timeTrigger && <span className="of-chip">{timeTrigger.flavor}</span>}
            {schedule.pending_re_run && <span className="of-chip of-status-info">Pending rerun</span>}
            {schedule.active_run_id && <span className="of-chip of-status-info">Run active</span>}
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8, fontSize: 11 }}>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>
                Last run
              </p>
              <p style={{ margin: '3px 0 0' }}>{formatDate(schedule.last_run_at)}</p>
            </div>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>
                Updated
              </p>
              <p style={{ margin: '3px 0 0' }}>{formatDate(schedule.updated_at)}</p>
            </div>
            <div>
              <p className="of-eyebrow" style={{ margin: 0 }}>
                Owner
              </p>
              <p style={{ margin: '3px 0 0', overflow: 'hidden', textOverflow: 'ellipsis' }}>{schedule.created_by}</p>
            </div>
          </div>
        </div>
      </button>

      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          justifyContent: 'flex-end',
          gap: 6,
          borderTop: '1px solid var(--border-subtle)',
          padding: '8px 12px',
          background: 'var(--bg-panel-muted)',
        }}
      >
        <button type="button" className="of-button" onClick={onEdit} disabled={busy}>
          Edit schedule
        </button>
        <button type="button" className="of-button" onClick={schedule.paused ? onResume : onPause} disabled={busy}>
          {schedule.paused ? 'Resume' : 'Pause'}
        </button>
        <button type="button" className="of-button" onClick={onRunNow} disabled={busy}>
          Run now
        </button>
        <button type="button" className="of-button" onClick={onDelete} disabled={busy}>
          Delete
        </button>
        <Link to={`/schedules/${schedule.rid}`} className="of-button">
          Metrics
        </Link>
      </div>
    </article>
  );
}

export function BuildSchedulesPage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const [filterFiles, setFilterFiles] = useState<string[]>(() => searchParams.getAll('files'));
  const [filterUsers, setFilterUsers] = useState<string[]>(() => searchParams.getAll('users'));
  const [filterProjects, setFilterProjects] = useState<string[]>(() => searchParams.getAll('projects'));
  const [filterName, setFilterName] = useState(() => searchParams.get('q') ?? '');
  const [filterPaused, setFilterPaused] = useState<PauseFilter>(
    () => (searchParams.get('paused_filter') ?? 'all') as PauseFilter,
  );
  const [sortBy, setSortBy] = useState<SortKey>(() => (searchParams.get('sort') ?? 'updated_at') as SortKey);
  const [selectedRid, setSelectedRid] = useState(() => searchParams.get('selected') ?? '');

  const [filterInputFiles, setFilterInputFiles] = useState('');
  const [filterInputUsers, setFilterInputUsers] = useState('');
  const [filterInputProjects, setFilterInputProjects] = useState('');

  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [actionBusyRid, setActionBusyRid] = useState<string | null>(null);
  const [editingSchedule, setEditingSchedule] = useState<Schedule | null>(null);

  const [runs, setRuns] = useState<ScheduleRun[]>([]);
  const [versions, setVersions] = useState<ScheduleVersion[]>([]);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [diff, setDiff] = useState<VersionDiff | null>(null);
  const [diffRange, setDiffRange] = useState<DiffRange | null>(null);
  const [diffError, setDiffError] = useState<string | null>(null);

  const selectedSchedule = useMemo(
    () => schedules.find((schedule) => schedule.rid === selectedRid) ?? schedules[0] ?? null,
    [schedules, selectedRid],
  );
  const selectedScheduleRid = selectedSchedule?.rid ?? '';
  const selectedScheduleVersion = selectedSchedule?.version ?? 0;
  const runCounts = getRunCounts(runs);

  const showOwnerOnlyBanner =
    filterFiles.length === 0 && filterProjects.length === 0 && filterName.trim() === '' && filterUsers.length === 0;

  function buildQuery(): ListSchedulesQuery {
    const query: ListSchedulesQuery = {
      files: filterFiles,
      users: filterUsers,
      projects: filterProjects,
      q: filterName.trim() || undefined,
      sort: sortBy,
    };
    if (filterPaused === 'paused') query.paused = true;
    else if (filterPaused === 'active') query.paused = false;
    return query;
  }

  async function refreshSchedules() {
    setLoading(true);
    setErrorMsg(null);
    try {
      const res = await listSchedules(buildQuery());
      setSchedules(res.data);
      setTotal(res.total);
      setSelectedRid((current) => {
        if (current && res.data.some((schedule) => schedule.rid === current)) return current;
        return res.data[0]?.rid ?? '';
      });
    } catch (cause) {
      setErrorMsg(cause instanceof Error ? cause.message : String(cause));
      setSchedules([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const next = new URLSearchParams();
    for (const file of filterFiles) next.append('files', file);
    for (const user of filterUsers) next.append('users', user);
    for (const project of filterProjects) next.append('projects', project);
    if (filterName.trim()) next.set('q', filterName.trim());
    if (filterPaused !== 'all') next.set('paused_filter', filterPaused);
    if (sortBy !== 'updated_at') next.set('sort', sortBy);
    if (selectedRid) next.set('selected', selectedRid);
    setSearchParams(next, { replace: true });

    let cancelled = false;
    async function refresh() {
      setLoading(true);
      setErrorMsg(null);
      try {
        const res = await listSchedules(buildQuery());
        if (cancelled) return;
        setSchedules(res.data);
        setTotal(res.total);
        setSelectedRid((current) => {
          if (current && res.data.some((schedule) => schedule.rid === current)) return current;
          return res.data[0]?.rid ?? '';
        });
      } catch (cause) {
        if (cancelled) return;
        setErrorMsg(cause instanceof Error ? cause.message : String(cause));
        setSchedules([]);
        setTotal(0);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void refresh();
    return () => {
      cancelled = true;
    };
  }, [filterFiles, filterUsers, filterProjects, filterName, filterPaused, sortBy, selectedRid]);

  useEffect(() => {
    if (!selectedScheduleRid) {
      setRuns([]);
      setVersions([]);
      setDiff(null);
      setDiffRange(null);
      return;
    }

    let cancelled = false;
    async function refreshDetails() {
      setDetailLoading(true);
      setDetailError(null);
      setDiff(null);
      setDiffRange(null);
      setDiffError(null);
      try {
        const [runRes, versionRes] = await Promise.all([
          listScheduleRuns(selectedScheduleRid, { limit: 10 }),
          listScheduleVersions(selectedScheduleRid, { limit: 12 }),
        ]);
        if (cancelled) return;
        setRuns(runRes.data ?? []);
        setVersions(versionRes.data ?? []);
        if (selectedScheduleVersion > 1) {
          const range = { from: selectedScheduleVersion - 1, to: selectedScheduleVersion };
          setDiffRange(range);
          getScheduleVersionDiff(selectedScheduleRid, range.from, range.to)
            .then((nextDiff) => {
              if (!cancelled) setDiff(nextDiff);
            })
            .catch((cause: unknown) => {
              if (!cancelled) setDiffError(cause instanceof Error ? cause.message : String(cause));
            });
        }
      } catch (cause) {
        if (!cancelled) setDetailError(cause instanceof Error ? cause.message : String(cause));
        if (!cancelled) {
          setRuns([]);
          setVersions([]);
        }
      } finally {
        if (!cancelled) setDetailLoading(false);
      }
    }
    void refreshDetails();
    return () => {
      cancelled = true;
    };
  }, [selectedScheduleRid, selectedScheduleVersion]);

  function addFilter(arr: string[], value: string): string[] {
    const v = value.trim();
    if (!v || arr.includes(v)) return arr;
    return [...arr, v];
  }

  function removeFilter(arr: string[], value: string): string[] {
    return arr.filter((x) => x !== value);
  }

  function replaceSchedule(updated: Schedule) {
    setSchedules((current) => current.map((schedule) => (schedule.rid === updated.rid ? updated : schedule)));
    setSelectedRid(updated.rid);
  }

  async function withScheduleAction(schedule: Schedule, action: () => Promise<unknown>, success?: (updated: unknown) => void) {
    setActionBusyRid(schedule.rid);
    setErrorMsg(null);
    try {
      const updated = await action();
      success?.(updated);
    } catch (cause) {
      setErrorMsg(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setActionBusyRid(null);
    }
  }

  async function refreshRuns(rid: string) {
    const runRes = await listScheduleRuns(rid, { limit: 10 });
    setRuns(runRes.data ?? []);
  }

  return (
    <main className="of-page" data-testid="build-schedules-page" style={{ padding: 16, display: 'grid', gap: 12 }}>
      <header
        className="of-panel"
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          padding: '14px 16px',
        }}
      >
        <div>
          <h1 className="of-heading-xl" style={{ margin: 0 }}>
            Build schedules
          </h1>
          <p className="of-text-muted" style={{ margin: '4px 0 0' }}>
            {total} schedules · sorted by {SORT_LABELS[sortBy]}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <Link to="/build-schedules/sweep" className="of-button">
            Sweep schedules
          </Link>
          <Link to="/schedules/new" className="of-button">
            New schedule
          </Link>
          {selectedSchedule && (
            <button type="button" className="of-button of-button--primary" onClick={() => setEditingSchedule(selectedSchedule)}>
              Edit selected
            </button>
          )}
        </div>
      </header>

      {showOwnerOnlyBanner && (
        <p
          data-testid="owner-only-banner"
          className="of-status-info"
          style={{ padding: '8px 12px', borderRadius: 'var(--radius-md)', fontSize: 12, margin: 0 }}
        >
          Showing schedules you created. Add files, users, or projects to broaden the search.
        </p>
      )}

      {errorMsg && (
        <p role="alert" className="of-status-danger" style={{ padding: '10px 12px', borderRadius: 'var(--radius-md)', fontSize: 13, margin: 0 }}>
          {errorMsg}
        </p>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '260px minmax(360px, 1fr) minmax(320px, 420px)', gap: 12, alignItems: 'start' }}>
        <aside data-testid="filters-sidebar" className="of-panel" style={{ padding: 14, display: 'grid', gap: 14 }}>
          <div>
            <h2 className="of-heading-md" style={{ margin: 0 }}>
              Search criteria
            </h2>
            <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
              Criteria are OR matched, then refined below.
            </p>
          </div>

          <section data-testid="filter-files">
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Files
            </h3>
            <input
              type="text"
              placeholder="Add dataset RID + Enter"
              data-testid="filter-files-input"
              value={filterInputFiles}
              onChange={(e) => setFilterInputFiles(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  setFilterFiles((current) => addFilter(current, filterInputFiles));
                  setFilterInputFiles('');
                }
              }}
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            />
            <FilterTokens values={filterFiles} onRemove={(value) => setFilterFiles((current) => removeFilter(current, value))} />
          </section>

          <section data-testid="filter-users">
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Users
            </h3>
            <input
              type="text"
              placeholder="Add user id + Enter"
              data-testid="filter-users-input"
              value={filterInputUsers}
              onChange={(e) => setFilterInputUsers(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  setFilterUsers((current) => addFilter(current, filterInputUsers));
                  setFilterInputUsers('');
                }
              }}
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            />
            <FilterTokens values={filterUsers} onRemove={(value) => setFilterUsers((current) => removeFilter(current, value))} />
          </section>

          <section data-testid="filter-projects">
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Projects
            </h3>
            <input
              type="text"
              placeholder="Add project RID + Enter"
              data-testid="filter-projects-input"
              value={filterInputProjects}
              onChange={(e) => setFilterInputProjects(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  setFilterProjects((current) => addFilter(current, filterInputProjects));
                  setFilterInputProjects('');
                }
              }}
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            />
            <FilterTokens values={filterProjects} onRemove={(value) => setFilterProjects((current) => removeFilter(current, value))} />
          </section>

          <section>
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Name
            </h3>
            <input
              type="text"
              placeholder="Filter by name"
              data-testid="filter-name-input"
              value={filterName}
              onChange={(e) => setFilterName(e.target.value)}
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            />
          </section>

          <section>
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Pause status
            </h3>
            <select
              value={filterPaused}
              onChange={(e) => setFilterPaused(e.target.value as PauseFilter)}
              data-testid="filter-paused-select"
              className="of-select"
              style={{ marginTop: 4, fontSize: 12 }}
            >
              <option value="all">All</option>
              <option value="paused">Paused</option>
              <option value="active">Active</option>
            </select>
          </section>

          <section>
            <h3 className="of-eyebrow" style={{ margin: 0 }}>
              Sort
            </h3>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as SortKey)}
              data-testid="sort-select"
              className="of-select"
              style={{ marginTop: 4, fontSize: 12 }}
            >
              <option value="name">Name</option>
              <option value="created_at">Creation date</option>
              <option value="last_run_at">Last run</option>
              <option value="updated_at">Last update</option>
            </select>
          </section>
        </aside>

        <section style={{ display: 'grid', gap: 10 }}>
          <div className="of-toolbar" style={{ justifyContent: 'space-between' }}>
            <strong>{schedules.length} visible</strong>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              {filterName.trim() && <span className="of-chip">Name: {filterName.trim()}</span>}
              {filterPaused !== 'all' && <span className="of-chip">Status: {filterPaused}</span>}
              <span className="of-chip">Sort: {SORT_LABELS[sortBy]}</span>
            </div>
          </div>

          {loading ? (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-text-muted" style={{ margin: 0, fontStyle: 'italic' }}>
                Loading schedules...
              </p>
            </section>
          ) : schedules.length === 0 ? (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-text-muted" style={{ margin: 0, fontStyle: 'italic' }}>
                No schedules match the current filters.
              </p>
            </section>
          ) : (
            schedules.map((schedule) => (
              <ScheduleCard
                key={schedule.rid}
                schedule={schedule}
                selected={selectedScheduleRid === schedule.rid}
                busy={actionBusyRid === schedule.rid}
                onSelect={() => setSelectedRid(schedule.rid)}
                onEdit={() => setEditingSchedule(schedule)}
                onPause={() =>
                  void withScheduleAction(schedule, () => pauseSchedule(schedule.rid, 'Manual pause'), (updated) => {
                    replaceSchedule(updated as Schedule);
                  })
                }
                onResume={() =>
                  void withScheduleAction(schedule, () => resumeSchedule(schedule.rid), (updated) => {
                    replaceSchedule(updated as Schedule);
                  })
                }
                onRunNow={() =>
                  void withScheduleAction(schedule, () => runScheduleNow(schedule.rid), () => {
                    if (selectedScheduleRid === schedule.rid) void refreshRuns(schedule.rid);
                  })
                }
                onDelete={() =>
                  void withScheduleAction(schedule, async () => {
                    if (!window.confirm(`Delete schedule "${schedule.name}"?`)) return null;
                    await deleteSchedule(schedule.rid);
                    await refreshSchedules();
                    return null;
                  })
                }
              />
            ))
          )}
        </section>

        <aside className="of-panel" style={{ overflow: 'hidden' }}>
          {selectedSchedule ? (
            <div style={{ display: 'grid', gap: 0 }}>
              <header style={{ padding: 14, borderBottom: '1px solid var(--border-default)' }}>
                <p className="of-eyebrow" style={{ margin: 0 }}>
                  Schedule detail
                </p>
                <h2 className="of-heading-md" style={{ margin: '4px 0 0', overflowWrap: 'anywhere' }}>
                  {selectedSchedule.name}
                </h2>
                <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
                  v{selectedSchedule.version} · {selectedSchedule.project_rid}
                </p>
              </header>

              {detailError && (
                <div className="of-status-danger" style={{ margin: 12, padding: '8px 10px', borderRadius: 'var(--radius-md)' }}>
                  {detailError}
                </div>
              )}

              <section style={{ padding: 14, display: 'grid', gap: 10, borderBottom: '1px solid var(--border-subtle)' }}>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 8 }}>
                  {[
                    ['Total', runCounts.total],
                    ['Succeeded', runCounts.succeeded],
                    ['Failed', runCounts.failed],
                    ['Ignored', runCounts.ignored],
                  ].map(([label, value]) => (
                    <div key={label} className="of-panel-muted" style={{ padding: 8, textAlign: 'center' }}>
                      <strong style={{ display: 'block', color: 'var(--text-strong)', fontSize: 18 }}>{value}</strong>
                      <span className="of-text-muted" style={{ fontSize: 11 }}>
                        {label}
                      </span>
                    </div>
                  ))}
                </div>

                <div>
                  <p className="of-eyebrow" style={{ margin: 0 }}>
                    Recent runs
                  </p>
                  {detailLoading ? (
                    <p className="of-text-muted" style={{ margin: '6px 0 0', fontStyle: 'italic' }}>
                      Loading run history...
                    </p>
                  ) : runs.length === 0 ? (
                    <p className="of-text-muted" style={{ margin: '6px 0 0', fontStyle: 'italic' }}>
                      No recent runs.
                    </p>
                  ) : (
                    <div style={{ display: 'flex', gap: 5, marginTop: 8, flexWrap: 'wrap' }}>
                      {runs
                        .slice()
                        .reverse()
                        .map((run) => {
                          const dot = (
                            <span
                              title={`${run.outcome} · ${formatDate(run.triggered_at)}`}
                              style={{
                                display: 'inline-block',
                                width: 12,
                                height: 12,
                                borderRadius: '50%',
                                border: `1px solid ${OUTCOME_TONE[run.outcome].borderColor}`,
                                background: OUTCOME_TONE[run.outcome].background,
                              }}
                            />
                          );
                          return run.build_rid ? (
                            <Link key={run.id} to={`/builds/${run.build_rid}`} aria-label={`Open build ${run.build_rid}`}>
                              {dot}
                            </Link>
                          ) : (
                            <span key={run.id}>{dot}</span>
                          );
                        })}
                    </div>
                  )}
                </div>
              </section>

              <section style={{ padding: 14, display: 'grid', gap: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <p className="of-eyebrow" style={{ margin: 0 }}>
                  Configuration
                </p>
                <div className="of-panel-muted" style={{ padding: 10, display: 'grid', gap: 8 }}>
                  <div>
                    <strong>Trigger</strong>
                    <p style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 12, overflowWrap: 'anywhere' }}>
                      {summarizeTrigger(selectedSchedule)}
                    </p>
                  </div>
                  <div>
                    <strong>Target</strong>
                    <p style={{ margin: '2px 0 0', fontSize: 12, overflowWrap: 'anywhere' }}>
                      {summarizeTarget(selectedSchedule.target)}
                    </p>
                  </div>
                </div>
              </section>

              <section style={{ padding: 14, display: 'grid', gap: 8, borderBottom: '1px solid var(--border-subtle)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
                  <p className="of-eyebrow" style={{ margin: 0 }}>
                    Versions
                  </p>
                  <span className="of-text-muted" style={{ fontSize: 11 }}>
                    {versions.length} loaded
                  </span>
                </div>
                <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: 6, maxHeight: 156, overflow: 'auto' }}>
                  {versions.map((version) => (
                    <li key={version.id} className="of-panel-muted" style={{ padding: '7px 8px', fontSize: 12 }}>
                      <strong>v{version.version}</strong>
                      <span className="of-text-muted"> · {formatDate(version.edited_at)} · {version.edited_by}</span>
                      {version.comment && <p style={{ margin: '3px 0 0' }}>{version.comment}</p>}
                    </li>
                  ))}
                  {!detailLoading && versions.length === 0 && (
                    <li className="of-text-muted" style={{ fontStyle: 'italic' }}>
                      No version history.
                    </li>
                  )}
                </ul>
              </section>

              <section style={{ padding: 14, display: 'grid', gap: 8 }}>
                <p className="of-eyebrow" style={{ margin: 0 }}>
                  Latest version diff
                </p>
                {diffError ? (
                  <p className="of-status-warning" style={{ margin: 0, padding: '8px 10px', borderRadius: 'var(--radius-md)' }}>
                    {diffError}
                  </p>
                ) : diffRange ? (
                  <ScheduleDiff diff={diff} fromVersion={diffRange.from} toVersion={diffRange.to} />
                ) : (
                  <p className="of-text-muted" style={{ margin: 0, fontStyle: 'italic' }}>
                    No previous version to compare.
                  </p>
                )}
              </section>
            </div>
          ) : (
            <p className="of-text-muted" style={{ margin: 0, padding: 14, fontStyle: 'italic' }}>
              Select a schedule to view runs, versions, and diff.
            </p>
          )}
        </aside>
      </div>

      <EditScheduleDialog
        open={editingSchedule !== null}
        schedule={editingSchedule}
        onClose={() => setEditingSchedule(null)}
        onSaved={(updated) => {
          replaceSchedule(updated);
          void refreshSchedules();
        }}
      />
    </main>
  );
}
