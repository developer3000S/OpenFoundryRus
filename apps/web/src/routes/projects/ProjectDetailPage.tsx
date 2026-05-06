import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import {
  bindProjectResource,
  createProjectFolder,
  deleteProject,
  getProject,
  listProjectFolders,
  listProjectMemberships,
  listProjectResources,
  unbindProjectResource,
  updateProject,
  type OntologyProject,
  type OntologyProjectFolder,
  type OntologyProjectMembership,
  type OntologyProjectResourceBinding,
} from '@/lib/api/ontology';

type Tab = 'overview' | 'folders' | 'resources' | 'memberships';

export function ProjectDetailPage() {
  const { projectId = '' } = useParams<{ projectId: string }>();
  const [tab, setTab] = useState<Tab>('overview');
  const [project, setProject] = useState<OntologyProject | null>(null);
  const [folders, setFolders] = useState<OntologyProjectFolder[]>([]);
  const [resources, setResources] = useState<OntologyProjectResourceBinding[]>([]);
  const [memberships, setMemberships] = useState<OntologyProjectMembership[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // edit
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [workspaceSlug, setWorkspaceSlug] = useState('');

  // create folder
  const [folderName, setFolderName] = useState('');
  const [folderParent, setFolderParent] = useState('');

  // bind resource
  const [bindKind, setBindKind] = useState('dataset');
  const [bindId, setBindId] = useState('');

  async function loadAll() {
    if (!projectId) return;
    setLoading(true);
    setError('');
    try {
      const [p, f, r, m] = await Promise.all([
        getProject(projectId),
        listProjectFolders(projectId).catch(() => [] as OntologyProjectFolder[]),
        listProjectResources(projectId).catch(() => [] as OntologyProjectResourceBinding[]),
        listProjectMemberships(projectId).catch(() => [] as OntologyProjectMembership[]),
      ]);
      setProject(p);
      setFolders(f);
      setResources(r);
      setMemberships(m);
      setDisplayName(p.display_name);
      setDescription(p.description);
      setWorkspaceSlug(p.workspace_slug ?? '');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load project');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadAll();
  }, [projectId]);

  async function save() {
    if (!project) return;
    setBusy(true);
    try {
      const updated = await updateProject(project.id, {
        display_name: displayName,
        description,
        workspace_slug: workspaceSlug || null,
      });
      setProject(updated);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!project) return;
    if (typeof window !== 'undefined' && !window.confirm('Move project to trash?')) return;
    setBusy(true);
    try {
      await deleteProject(project.id);
      window.location.href = '/projects';
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
      setBusy(false);
    }
  }

  async function addFolder() {
    if (!project || !folderName.trim()) return;
    setBusy(true);
    try {
      await createProjectFolder(project.id, {
        name: folderName.trim(),
        parent_folder_id: folderParent || null,
      });
      setFolderName('');
      setFolders(await listProjectFolders(project.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Folder create failed');
    } finally {
      setBusy(false);
    }
  }

  async function bind() {
    if (!project || !bindId.trim()) return;
    setBusy(true);
    try {
      await bindProjectResource(project.id, { resource_kind: bindKind, resource_id: bindId.trim() });
      setBindId('');
      setResources(await listProjectResources(project.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Bind failed');
    } finally {
      setBusy(false);
    }
  }

  async function unbind(b: OntologyProjectResourceBinding) {
    if (!project) return;
    setBusy(true);
    try {
      await unbindProjectResource(project.id, b.resource_kind, b.resource_id);
      setResources(await listProjectResources(project.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unbind failed');
    } finally {
      setBusy(false);
    }
  }

  const rootFolders = useMemo(() => folders.filter((f) => f.parent_folder_id === null), [folders]);

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading project…</p>
      </section>
    );
  }

  if (!project) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/projects" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Projects</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Project not found'}</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/projects" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Projects</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">{project.display_name || project.slug}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {project.id} · slug: {project.slug} · workspace: {project.workspace_slug ?? '—'}
          </p>
        </div>
        <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
          Delete
        </button>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs tabs={['overview', 'folders', 'resources', 'memberships'] as const} active={tab} onChange={setTab} />

      {tab === 'overview' && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Display name
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Description
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Workspace slug
            <input value={workspaceSlug} onChange={(e) => setWorkspaceSlug(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div>
            <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">Save</button>
          </div>
        </section>
      )}

      {tab === 'folders' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Add folder</p>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 }}>
              <input value={folderName} onChange={(e) => setFolderName(e.target.value)} placeholder="folder name" className="of-input" />
              <select value={folderParent} onChange={(e) => setFolderParent(e.target.value)} className="of-input">
                <option value="">— root —</option>
                {folders.map((f) => (
                  <option key={f.id} value={f.id}>{f.name}</option>
                ))}
              </select>
              <button type="button" onClick={() => void addFolder()} disabled={busy || !folderName.trim()} className="of-button of-button--primary">
                Create
              </button>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Folders ({folders.length} total · {rootFolders.length} at root)</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {folders.map((f) => (
                <li key={f.id}>
                  <Link to={`/projects/${project.id}/${f.id}`}><strong>{f.name}</strong></Link>
                  {f.description && ` · ${f.description}`}
                  · parent: {f.parent_folder_id ?? '—'}
                </li>
              ))}
              {folders.length === 0 && <li className="of-text-muted">No folders.</li>}
            </ul>
          </section>
        </>
      )}

      {tab === 'resources' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Bind resource</p>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 }}>
              <select value={bindKind} onChange={(e) => setBindKind(e.target.value)} className="of-input">
                {['dataset', 'pipeline', 'notebook', 'app', 'dashboard', 'report', 'model', 'workflow'].map((k) => (
                  <option key={k} value={k}>{k}</option>
                ))}
              </select>
              <input value={bindId} onChange={(e) => setBindId(e.target.value)} placeholder="resource id" className="of-input" />
              <button type="button" onClick={() => void bind()} disabled={busy || !bindId.trim()} className="of-button of-button--primary">
                Bind
              </button>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Bound resources ({resources.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {resources.map((r) => (
                <li key={`${r.resource_kind}-${r.resource_id}`} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <span>
                    {r.resource_kind} · <code>{r.resource_id}</code> · bound by {r.bound_by}
                  </span>
                  <button type="button" onClick={() => void unbind(r)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                    Unbind
                  </button>
                </li>
              ))}
              {resources.length === 0 && <li className="of-text-muted">No resources bound.</li>}
            </ul>
          </section>
        </>
      )}

      {tab === 'memberships' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Memberships ({memberships.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {memberships.map((m) => (
              <li key={`${m.project_id}-${m.user_id}`}>
                <code>{m.user_id}</code> · {m.role}
              </li>
            ))}
            {memberships.length === 0 && <li className="of-text-muted">No memberships.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
