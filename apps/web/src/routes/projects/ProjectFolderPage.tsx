import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  createProjectFolder,
  getProject,
  listProjectFolders,
  type OntologyProject,
  type OntologyProjectFolder,
} from '@/lib/api/ontology';
import { recordAccess } from '@/lib/api/workspace';

export function ProjectFolderPage() {
  const { projectId = '', folderId = '' } = useParams<{ projectId: string; folderId: string }>();
  const [project, setProject] = useState<OntologyProject | null>(null);
  const [folders, setFolders] = useState<OntologyProjectFolder[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [childName, setChildName] = useState('');

  async function load() {
    if (!projectId) return;
    setError('');
    try {
      const [p, f] = await Promise.all([getProject(projectId), listProjectFolders(projectId)]);
      setProject(p);
      setFolders(f);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void load();
  }, [projectId]);

  useEffect(() => {
    if (!folderId) return;
    recordAccess({ resource_kind: 'ontology_folder', resource_id: folderId }).catch(() => {});
  }, [folderId]);

  const folder = useMemo(() => folders.find((f) => f.id === folderId) ?? null, [folders, folderId]);
  const childFolders = useMemo(() => folders.filter((f) => f.parent_folder_id === folderId), [folders, folderId]);

  async function addChild() {
    if (!project || !folder || !childName.trim()) return;
    setBusy(true);
    try {
      await createProjectFolder(project.id, { name: childName.trim(), parent_folder_id: folder.id });
      setChildName('');
      setFolders(await listProjectFolders(project.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Folder create failed');
    } finally {
      setBusy(false);
    }
  }

  if (!project) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/projects" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Projects</Link>
        <p className="of-text-muted" style={{ marginTop: 12 }}>Loading…</p>
      </section>
    );
  }

  if (!folder) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to={`/projects/${project.id}`} style={{ color: 'var(--text-muted)', fontSize: 13 }}>← {project.display_name || project.slug}</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>Folder {folderId} not found in this project.</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to={`/projects/${project.id}`} style={{ color: 'var(--text-muted)', fontSize: 13 }}>← {project.display_name || project.slug}</Link>
      <header>
        <h1 className="of-heading-xl">{folder.name}</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
          {folder.id} · slug: {folder.slug} · parent: {folder.parent_folder_id ?? '— root'}
          {folder.description && ` · ${folder.description}`}
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Add subfolder</p>
        <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
          <input value={childName} onChange={(e) => setChildName(e.target.value)} placeholder="subfolder name" className="of-input" />
          <button type="button" onClick={() => void addChild()} disabled={busy || !childName.trim()} className="of-button of-button--primary">
            Create
          </button>
        </div>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Subfolders ({childFolders.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {childFolders.map((f) => (
            <li key={f.id}>
              <Link to={`/projects/${project.id}/${f.id}`}><strong>{f.name}</strong></Link>
              {f.description && ` · ${f.description}`}
            </li>
          ))}
          {childFolders.length === 0 && <li className="of-text-muted">No subfolders.</li>}
        </ul>
      </section>
    </section>
  );
}
