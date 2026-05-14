import { useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';

import { createSchedule, type ScheduleTarget, type Trigger } from '@/lib/api/schedules';
import { notifications } from '@/lib/stores/notifications';

type TriggerKind = 'time' | 'event';
type BuildStrategy = 'STALE_ONLY' | 'FORCE';

export function NewSchedulePage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const seededDataset = searchParams.get('event_target') ?? searchParams.get('dataset') ?? '';

  const [name, setName] = useState(seededDataset ? 'Build dataset on update' : 'New build schedule');
  const [description, setDescription] = useState('');
  const [projectRid, setProjectRid] = useState('ri.foundry.main.project.default');
  const [folderRid, setFolderRid] = useState('');
  const [datasetRid, setDatasetRid] = useState(seededDataset);
  const [branch, setBranch] = useState('master');
  const [buildStrategy, setBuildStrategy] = useState<BuildStrategy>('STALE_ONLY');
  const [triggerKind, setTriggerKind] = useState<TriggerKind>(seededDataset ? 'event' : 'time');
  const [cron, setCron] = useState('0 * * * *');
  const [timeZone, setTimeZone] = useState('UTC');
  const [eventType, setEventType] = useState<'DATA_UPDATED' | 'NEW_LOGIC' | 'JOB_SUCCEEDED'>('DATA_UPDATED');
  const [paused, setPaused] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const previewTrigger = useMemo(() => buildTrigger(), [triggerKind, cron, timeZone, eventType, datasetRid, branch]);
  const previewTarget = useMemo(() => buildTarget(), [datasetRid, branch, buildStrategy]);

  function buildTrigger(): Trigger {
    if (triggerKind === 'event') {
      return {
        kind: {
          event: {
            type: eventType,
            target_rid: datasetRid.trim(),
            branch_filter: branch.trim() ? [branch.trim()] : undefined,
          },
        },
      };
    }
    return {
      kind: {
        time: {
          cron: cron.trim() || '0 * * * *',
          time_zone: timeZone.trim() || 'UTC',
          flavor: 'UNIX_5',
        },
      },
    };
  }

  function buildTarget(): ScheduleTarget {
    return {
      kind: {
        dataset_build: {
          dataset_rid: datasetRid.trim(),
          build_branch: branch.trim() || 'master',
          force_build: buildStrategy === 'FORCE',
        },
      },
    };
  }

  async function submit() {
    setBusy(true);
    setError('');
    try {
      const created = await createSchedule({
        project_rid: projectRid.trim() || 'ri.foundry.main.project.default',
        folder_rid: folderRid.trim() || null,
        name: name.trim() || 'New build schedule',
        description,
        trigger: buildTrigger(),
        target: buildTarget(),
        paused,
        branch: branch.trim() || 'master',
        build_strategy: buildStrategy,
        scope_kind: 'USER',
      });
      notifications.success('Schedule created');
      navigate(`/schedules/${encodeURIComponent(created.rid)}`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Failed to create schedule';
      setError(message);
      notifications.error(message);
    } finally {
      setBusy(false);
    }
  }

  const canSubmit = name.trim() && datasetRid.trim() && projectRid.trim() && (triggerKind === 'time' || datasetRid.trim());

  return (
    <main className="of-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 1180, margin: '0 auto' }}>
      <nav style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 13 }}>
        <Link to="/build-schedules" style={{ color: 'var(--text-muted)' }}>Build schedules</Link>
        <span className="of-text-muted">/</span>
        <span className="of-text-muted">New schedule</span>
      </nav>

      <header className="of-panel" style={{ padding: 16, display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start' }}>
        <div>
          <h1 className="of-heading-xl" style={{ margin: 0 }}>New schedule</h1>
          <p className="of-text-muted" style={{ margin: '4px 0 0' }}>Create a time or event-driven build schedule for a dataset.</p>
        </div>
        <button type="button" className="of-button of-button--primary" onClick={() => void submit()} disabled={busy || !canSubmit}>
          {busy ? 'Creating...' : 'Create schedule'}
        </button>
      </header>

      {error && (
        <p role="alert" className="of-status-danger" style={{ padding: '10px 12px', borderRadius: 'var(--radius-md)', margin: 0 }}>
          {error}
        </p>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(360px, 1fr) minmax(320px, 420px)', gap: 16, alignItems: 'start' }}>
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 14 }}>
          <section style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Name</span>
              <input className="of-input" value={name} onChange={(event) => setName(event.target.value)} disabled={busy} />
            </label>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Project RID</span>
              <input className="of-input" value={projectRid} onChange={(event) => setProjectRid(event.target.value)} disabled={busy} />
            </label>
          </section>

          <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
            <span className="of-eyebrow">Description</span>
            <textarea className="of-textarea" value={description} onChange={(event) => setDescription(event.target.value)} disabled={busy} style={{ minHeight: 72 }} />
          </label>

          <section style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Folder RID</span>
              <input className="of-input" value={folderRid} onChange={(event) => setFolderRid(event.target.value)} disabled={busy} placeholder="Optional" />
            </label>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Target dataset RID</span>
              <input className="of-input" value={datasetRid} onChange={(event) => setDatasetRid(event.target.value)} disabled={busy} />
            </label>
          </section>

          <section style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12 }}>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Branch</span>
              <input className="of-input" value={branch} onChange={(event) => setBranch(event.target.value)} disabled={busy} />
            </label>
            <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
              <span className="of-eyebrow">Build strategy</span>
              <select className="of-select" value={buildStrategy} onChange={(event) => setBuildStrategy(event.target.value as BuildStrategy)} disabled={busy}>
                <option value="STALE_ONLY">Stale only</option>
                <option value="FORCE">Force build</option>
              </select>
            </label>
            <label style={{ display: 'flex', gap: 8, alignItems: 'end', fontSize: 12, paddingBottom: 8 }}>
              <input type="checkbox" checked={paused} onChange={(event) => setPaused(event.target.checked)} disabled={busy} />
              Create paused
            </label>
          </section>

          <section className="of-panel-muted" style={{ padding: 12, display: 'grid', gap: 12 }}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', justifyContent: 'space-between' }}>
              <h2 className="of-heading-sm" style={{ margin: 0 }}>Trigger</h2>
              <select className="of-select" value={triggerKind} onChange={(event) => setTriggerKind(event.target.value as TriggerKind)} disabled={busy} style={{ width: 160 }}>
                <option value="time">Time</option>
                <option value="event">Dataset event</option>
              </select>
            </div>

            {triggerKind === 'time' ? (
              <section style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                  <span className="of-eyebrow">Cron</span>
                  <input className="of-input" value={cron} onChange={(event) => setCron(event.target.value)} disabled={busy} />
                </label>
                <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                  <span className="of-eyebrow">Time zone</span>
                  <input className="of-input" value={timeZone} onChange={(event) => setTimeZone(event.target.value)} disabled={busy} />
                </label>
              </section>
            ) : (
              <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                <span className="of-eyebrow">Event</span>
                <select className="of-select" value={eventType} onChange={(event) => setEventType(event.target.value as typeof eventType)} disabled={busy}>
                  <option value="DATA_UPDATED">Data updated</option>
                  <option value="NEW_LOGIC">New logic</option>
                  <option value="JOB_SUCCEEDED">Job succeeded</option>
                </select>
              </label>
            )}
          </section>
        </section>

        <aside className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
          <div>
            <p className="of-eyebrow">Definition preview</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>{name || 'New schedule'}</h2>
          </div>
          <dl style={{ display: 'grid', gridTemplateColumns: '120px minmax(0, 1fr)', gap: '8px 12px', margin: 0, fontSize: 12 }}>
            <dt className="of-text-muted">Project</dt>
            <dd style={{ margin: 0, overflowWrap: 'anywhere' }}>{projectRid}</dd>
            <dt className="of-text-muted">Folder</dt>
            <dd style={{ margin: 0, overflowWrap: 'anywhere' }}>{folderRid || '-'}</dd>
            <dt className="of-text-muted">Branch</dt>
            <dd style={{ margin: 0 }}>{branch || 'master'}</dd>
            <dt className="of-text-muted">Strategy</dt>
            <dd style={{ margin: 0 }}>{buildStrategy}</dd>
            <dt className="of-text-muted">Pause state</dt>
            <dd style={{ margin: 0 }}>{paused ? 'Paused' : 'Active'}</dd>
          </dl>
          <pre style={{ margin: 0, padding: 12, background: 'var(--bg-subtle)', borderRadius: 'var(--radius-md)', overflow: 'auto', fontSize: 11 }}>
            {JSON.stringify({ trigger: previewTrigger, target: previewTarget }, null, 2)}
          </pre>
        </aside>
      </div>
    </main>
  );
}
