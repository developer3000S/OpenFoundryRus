import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';

import { listSchedules, type ListSchedulesQuery, type Schedule } from '@/lib/api/schedules';

type PauseFilter = 'all' | 'paused' | 'active';
type SortKey = 'name' | 'created_at' | 'last_run_at' | 'updated_at';

function summarizeTrigger(s: Schedule): string {
  const kind = s.trigger.kind;
  if ('time' in kind) return `${kind.time.cron} (${kind.time.time_zone})`;
  if ('event' in kind) return `On ${kind.event.type} → ${kind.event.target_rid}`;
  if ('compound' in kind) return `${kind.compound.op} of ${kind.compound.components.length} components`;
  return 'unknown trigger';
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
  const [sortBy, setSortBy] = useState<SortKey>(
    () => (searchParams.get('sort') ?? 'updated_at') as SortKey,
  );

  const [filterInputFiles, setFilterInputFiles] = useState('');
  const [filterInputUsers, setFilterInputUsers] = useState('');
  const [filterInputProjects, setFilterInputProjects] = useState('');

  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const showOwnerOnlyBanner =
    filterFiles.length === 0 && filterProjects.length === 0 && filterName.trim() === '' && filterUsers.length === 0;

  useEffect(() => {
    const next = new URLSearchParams();
    for (const f of filterFiles) next.append('files', f);
    for (const u of filterUsers) next.append('users', u);
    for (const p of filterProjects) next.append('projects', p);
    if (filterName.trim()) next.set('q', filterName.trim());
    if (filterPaused !== 'all') next.set('paused_filter', filterPaused);
    if (sortBy !== 'updated_at') next.set('sort', sortBy);
    setSearchParams(next, { replace: true });

    let cancelled = false;
    async function refresh() {
      setLoading(true);
      setErrorMsg(null);
      try {
        const query: ListSchedulesQuery = {
          files: filterFiles,
          users: filterUsers,
          projects: filterProjects,
          q: filterName.trim() || undefined,
          sort: sortBy,
        };
        if (filterPaused === 'paused') query.paused = true;
        else if (filterPaused === 'active') query.paused = false;
        const res = await listSchedules(query);
        if (cancelled) return;
        setSchedules(res.data);
      } catch (err) {
        if (cancelled) return;
        setErrorMsg(err instanceof Error ? err.message : String(err));
        setSchedules([]);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void refresh();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterFiles, filterUsers, filterProjects, filterName, filterPaused, sortBy]);

  function addFilter(arr: string[], value: string): string[] {
    const v = value.trim();
    if (!v || arr.includes(v)) return arr;
    return [...arr, v];
  }

  function removeFilter(arr: string[], value: string): string[] {
    return arr.filter((x) => x !== value);
  }

  return (
    <main className="of-page" data-testid="build-schedules-page" style={{ padding: 24, maxWidth: 1280, margin: '0 auto' }}>
      <header style={{ marginBottom: 16 }}>
        <h1 className="of-heading-xl">Build Schedules</h1>
      </header>

      {showOwnerOnlyBanner && (
        <p
          data-testid="owner-only-banner"
          style={{
            background: 'var(--bg-subtle)',
            border: '1px solid var(--border-default)',
            color: 'var(--text-muted)',
            padding: '8px 12px',
            borderRadius: 'var(--radius-md)',
            fontSize: 12,
            marginBottom: 12,
          }}
        >
          Showing your schedules. Add filters to broaden.
        </p>
      )}

      {errorMsg && (
        <p role="alert" className="of-status-danger" style={{ padding: '10px 12px', borderRadius: 'var(--radius-md)', fontSize: 13, marginBottom: 12 }}>
          {errorMsg}
        </p>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '240px 1fr', gap: 16 }}>
        <aside data-testid="filters-sidebar" className="of-panel" style={{ padding: 14 }}>
          <h2 className="of-heading-md" style={{ marginBottom: 10 }}>
            Filters
          </h2>

          <section data-testid="filter-files" style={{ marginBottom: 14 }}>
            <h3 className="of-eyebrow">Files</h3>
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
            <ul style={{ listStyle: 'none', margin: '6px 0 0', padding: 0, display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              {filterFiles.map((f) => (
                <li
                  key={f}
                  className="of-chip"
                  style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-mono)' }}
                >
                  <span>{f}</span>
                  <button
                    type="button"
                    onClick={() => setFilterFiles((current) => removeFilter(current, f))}
                    style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <section data-testid="filter-users" style={{ marginBottom: 14 }}>
            <h3 className="of-eyebrow">User</h3>
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
            <ul style={{ listStyle: 'none', margin: '6px 0 0', padding: 0, display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              {filterUsers.map((u) => (
                <li key={u} className="of-chip" style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-mono)' }}>
                  <span>{u}</span>
                  <button
                    type="button"
                    onClick={() => setFilterUsers((current) => removeFilter(current, u))}
                    style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <section data-testid="filter-projects" style={{ marginBottom: 14 }}>
            <h3 className="of-eyebrow">Projects</h3>
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
            <ul style={{ listStyle: 'none', margin: '6px 0 0', padding: 0, display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              {filterProjects.map((p) => (
                <li key={p} className="of-chip" style={{ display: 'flex', alignItems: 'center', gap: 4, fontFamily: 'var(--font-mono)' }}>
                  <span>{p}</span>
                  <button
                    type="button"
                    onClick={() => setFilterProjects((current) => removeFilter(current, p))}
                    style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <section style={{ marginBottom: 14 }}>
            <h3 className="of-eyebrow">Name</h3>
            <input
              type="text"
              placeholder="Substring search"
              data-testid="filter-name-input"
              value={filterName}
              onChange={(e) => setFilterName(e.target.value)}
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            />
          </section>

          <section style={{ marginBottom: 14 }}>
            <h3 className="of-eyebrow">Pause status</h3>
            <select
              value={filterPaused}
              onChange={(e) => setFilterPaused(e.target.value as PauseFilter)}
              data-testid="filter-paused-select"
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            >
              <option value="all">All</option>
              <option value="paused">Paused</option>
              <option value="active">Active</option>
            </select>
          </section>

          <section>
            <h3 className="of-eyebrow">Sort</h3>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as SortKey)}
              data-testid="sort-select"
              className="of-input"
              style={{ marginTop: 4, fontSize: 12 }}
            >
              <option value="name">Name</option>
              <option value="created_at">Creation date</option>
              <option value="last_run_at">Last run</option>
              <option value="updated_at">Last update</option>
            </select>
          </section>
        </aside>

        <section className="of-panel" style={{ padding: 14, overflowX: 'auto' }}>
          {loading ? (
            <p style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>Loading…</p>
          ) : schedules.length === 0 ? (
            <p style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>No schedules match the current filters.</p>
          ) : (
            <table data-testid="schedules-table" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr>
                  {['Name', 'Project', 'Trigger', 'Scope', 'Paused', 'Last run', 'Last update', 'Owner'].map((header) => (
                    <th
                      key={header}
                      style={{
                        textAlign: 'left',
                        padding: '8px 10px',
                        color: 'var(--text-muted)',
                        fontWeight: 500,
                        borderBottom: '1px solid var(--border-default)',
                      }}
                    >
                      {header}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {schedules.map((s) => (
                  <tr key={s.rid} data-testid="schedule-row">
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      <Link to={`/schedules/${s.rid}`} style={{ color: '#2563eb' }}>
                        {s.name}
                      </Link>
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      <code style={{ fontFamily: 'var(--font-mono)', color: '#2563eb' }}>{s.project_rid}</code>
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)', fontFamily: 'var(--font-mono)' }}>
                      {summarizeTrigger(s)}
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      <span
                        style={{
                          padding: '2px 6px',
                          borderRadius: 3,
                          fontSize: 10,
                          fontWeight: 600,
                          ...(s.scope_kind === 'PROJECT_SCOPED'
                            ? { background: '#ecfdf5', color: '#047857' }
                            : { background: 'var(--bg-subtle)', color: 'var(--text-muted)' }),
                        }}
                      >
                        {s.scope_kind}
                      </span>
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>{s.paused ? '⏸︎' : '▶︎'}</td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      {s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '—'}
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      {new Date(s.updated_at).toLocaleString()}
                    </td>
                    <td style={{ padding: '8px 10px', borderBottom: '1px solid var(--border-default)' }}>
                      <code style={{ fontFamily: 'var(--font-mono)', color: '#2563eb' }}>{s.created_by}</code>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      </div>
    </main>
  );
}
