import { useEffect, useState } from 'react';

import { listProjectFolders, type OntologyProject, type OntologyProjectFolder } from '@/lib/api/ontology';
import { batchApply, moveResource, type BatchAction, type ResourceKind } from '@/lib/api/workspace';

interface BulkTarget {
  kind: ResourceKind;
  id: string;
  label: string;
}

interface MoveDialogProps {
  open: boolean;
  resourceKind: ResourceKind | null;
  resourceId: string | null;
  resourceLabel?: string;
  projects: OntologyProject[];
  initialProjectId?: string | null;
  targets?: BulkTarget[];
  onClose?: () => void;
  onMoved?: () => void;
}

export function MoveDialog({
  open,
  resourceKind,
  resourceId,
  resourceLabel,
  projects,
  initialProjectId,
  targets,
  onClose,
  onMoved,
}: MoveDialogProps) {
  const isBulk = Array.isArray(targets) && targets.length > 0;
  const [targetProjectId, setTargetProjectId] = useState('');
  const [targetFolderId, setTargetFolderId] = useState('');
  const [folders, setFolders] = useState<OntologyProjectFolder[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setTargetProjectId(initialProjectId ?? projects[0]?.id ?? '');
    setTargetFolderId('');
    setSubmitting(false);
    setError('');
  }, [open, initialProjectId, projects]);

  useEffect(() => {
    if (!open || !targetProjectId) {
      setFolders([]);
      return;
    }
    listProjectFolders(targetProjectId)
      .then(setFolders)
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Unable to load folders'));
  }, [open, targetProjectId]);

  async function submit() {
    if (!targetProjectId) { setError('Pick a destination project.'); return; }
    setSubmitting(true);
    setError('');
    try {
      if (isBulk && targets) {
        const actions: BatchAction[] = targets.map((t) => ({
          op: 'move',
          resource_kind: t.kind,
          resource_id: t.id,
          target_folder_id: targetFolderId || null,
        }));
        await batchApply(actions);
      } else {
        if (!resourceKind || !resourceId) { setError('Pick a destination project.'); setSubmitting(false); return; }
        await moveResource(resourceKind, resourceId, {
          target_project_id: targetProjectId,
          target_folder_id: targetFolderId || null,
        });
      }
      onMoved?.();
      onClose?.();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) return null;
  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16, zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 500, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', borderRadius: 12, boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #1e293b', padding: '12px 16px' }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>Move {isBulk ? `${targets!.length} items` : resourceLabel || 'resource'}</div>
          <button type="button" onClick={() => !submitting && onClose?.()} style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: 13 }}>✕</button>
        </div>
        <div style={{ display: 'grid', gap: 8, padding: 16 }}>
          <label style={{ fontSize: 12 }}>
            Project
            <select value={targetProjectId} onChange={(e) => setTargetProjectId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
              <option value="">— pick —</option>
              {projects.map((p) => <option key={p.id} value={p.id}>{p.display_name || p.slug}</option>)}
            </select>
          </label>
          <label style={{ fontSize: 12 }}>
            Folder (optional)
            <select value={targetFolderId} onChange={(e) => setTargetFolderId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
              <option value="">— root —</option>
              {folders.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}
            </select>
          </label>
          {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, borderTop: '1px solid #1e293b', padding: '12px 16px' }}>
          <button type="button" onClick={() => onClose?.()} disabled={submitting} className="of-button">Cancel</button>
          <button type="button" onClick={() => void submit()} disabled={submitting || !targetProjectId} className="of-button of-button--primary">
            {submitting ? 'Moving…' : 'Move'}
          </button>
        </div>
      </div>
    </div>
  );
}
