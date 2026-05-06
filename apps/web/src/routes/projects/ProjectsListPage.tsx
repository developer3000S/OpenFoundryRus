import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import { createProject, deleteProject, listProjects, type OntologyProject } from '@/lib/api/ontology';
import {
  listSharedWithMe,
  listTrash,
  purgeResource,
  restoreResource,
  type ResourceShare,
  type TrashEntry,
} from '@/lib/api/workspace';

type Tab = 'projects' | 'shared' | 'trash';

export function ProjectsListPage() {
  const [tab, setTab] = useState<Tab>('projects');
  const [projects, setProjects] = useState<OntologyProject[]>([]);
  const [shared, setShared] = useState<ResourceShare[]>([]);
  const [trash, setTrash] = useState<TrashEntry[]>([]);
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // create
  const [open, setOpen] = useState(false);
  const [slug, setSlug] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [workspaceSlug, setWorkspaceSlug] = useState('');

  async function refresh() {
    setError('');
    try {
      if (tab === 'projects') {
        const res = await listProjects({ search: search || undefined, per_page: 200 });
        setProjects(res.data);
      } else if (tab === 'shared') {
        setShared(await listSharedWithMe({ kind: 'ontology_project', limit: 100 }));
      } else if (tab === 'trash') {
        setTrash(await listTrash({ kind: 'ontology_project', limit: 100 }));
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void refresh();
  }, [tab]);

  async function handleCreate() {
    setBusy(true);
    setError('');
    try {
      const created = await createProject({
        slug,
        display_name: displayName || undefined,
        description: description || undefined,
        workspace_slug: workspaceSlug || undefined,
      });
      setSlug('');
      setDisplayName('');
      setDescription('');
      setWorkspaceSlug('');
      setOpen(false);
      window.location.href = `/projects/${created.id}`;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Move project to trash?')) return;
    setBusy(true);
    try {
      await deleteProject(id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleRestore(kind: TrashEntry['resource_kind'], id: string) {
    setBusy(true);
    try {
      await restoreResource(kind, id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Restore failed');
    } finally {
      setBusy(false);
    }
  }

  async function handlePurge(kind: TrashEntry['resource_kind'], id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Permanently delete?')) return;
    setBusy(true);
    try {
      await purgeResource(kind, id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Purge failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Projects</h1>
          <p className="of-text-muted" style={{ marginTop: 4 }}>
            Workspace projects: ontology projects + shared resources + trash.
          </p>
        </div>
        <button type="button" onClick={() => setOpen(!open)} className="of-button of-button--primary">
          {open ? 'Cancel' : '+ New project'}
        </button>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {open && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Slug *
            <input value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="my-project" className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Display name
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Description
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Workspace slug (optional)
            <input value={workspaceSlug} onChange={(e) => setWorkspaceSlug(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <button type="button" onClick={() => void handleCreate()} disabled={busy || !slug.trim()} className="of-button of-button--primary">
            Create project
          </button>
        </section>
      )}

      <Tabs
        tabs={[
          { id: 'projects' as const, label: 'Projects' },
          { id: 'shared' as const, label: 'Shared with me' },
          { id: 'trash' as const, label: 'Trash' },
        ]}
        active={tab}
        onChange={setTab}
      />

      {tab === 'projects' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <div style={{ display: 'flex', gap: 6 }}>
              <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search…" className="of-input" />
              <button type="button" onClick={() => void refresh()} className="of-button">Apply</button>
            </div>
          </section>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Projects ({projects.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {projects.map((p) => (
                <li key={p.id} style={{ padding: 12, borderBottom: '1px solid var(--border-default)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <Link to={`/projects/${p.id}`} style={{ fontWeight: 600 }}>{p.display_name || p.slug}</Link>
                    <p className="of-text-muted" style={{ fontSize: 11 }}>
                      {p.id} · slug: {p.slug} · workspace: {p.workspace_slug ?? '—'}
                    </p>
                  </div>
                  <button type="button" onClick={() => void handleDelete(p.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                    Delete
                  </button>
                </li>
              ))}
              {projects.length === 0 && <li className="of-text-muted">No projects.</li>}
            </ul>
          </section>
        </>
      )}

      {tab === 'shared' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Shared with me ({shared.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {shared.map((s) => (
              <li key={s.id}>
                {s.resource_kind} <code>{s.resource_id}</code> · {s.access_level} · from {s.sharer_id}
                {s.note && ` · ${s.note}`}
              </li>
            ))}
            {shared.length === 0 && <li className="of-text-muted">Nothing shared with you.</li>}
          </ul>
        </section>
      )}

      {tab === 'trash' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Trash ({trash.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {trash.map((t) => (
              <li key={`${t.resource_kind}-${t.resource_id}`} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span>
                  {t.resource_kind} · <code>{t.resource_id}</code> · {t.display_name} · deleted {new Date(t.deleted_at).toLocaleString()}
                </span>
                <span style={{ display: 'flex', gap: 6 }}>
                  <button type="button" onClick={() => void handleRestore(t.resource_kind, t.resource_id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                    Restore
                  </button>
                  <button type="button" onClick={() => void handlePurge(t.resource_kind, t.resource_id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                    Purge
                  </button>
                </span>
              </li>
            ))}
            {trash.length === 0 && <li className="of-text-muted">Trash is empty.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
