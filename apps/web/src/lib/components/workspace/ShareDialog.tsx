import { useEffect, useState } from 'react';

import {
  createShare,
  listResourceShares,
  revokeShare,
  type AccessLevel,
  type ResourceKind,
  type ResourceShare,
} from '@/lib/api/workspace';
import { PrincipalPicker } from './PrincipalPicker';

interface ShareDialogProps {
  open: boolean;
  resourceKind: ResourceKind | null;
  resourceId: string | null;
  resourceLabel?: string;
  onClose?: () => void;
  onShared?: () => void;
}

export function ShareDialog({ open, resourceKind, resourceId, resourceLabel, onClose, onShared }: ShareDialogProps) {
  const [shares, setShares] = useState<ResourceShare[]>([]);
  const [principal, setPrincipal] = useState<'user' | 'group'>('user');
  const [principalId, setPrincipalId] = useState('');
  const [accessLevel, setAccessLevel] = useState<AccessLevel>('viewer');
  const [note, setNote] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  async function refresh() {
    if (!resourceKind || !resourceId) return;
    try {
      setShares(await listResourceShares(resourceKind, resourceId));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to load shares');
    }
  }

  useEffect(() => {
    if (!open || !resourceKind || !resourceId) {
      setShares([]);
      return;
    }
    setPrincipalId('');
    setNote('');
    setExpiresAt('');
    setAccessLevel('viewer');
    setError('');
    void refresh();
  }, [open, resourceKind, resourceId]);

  async function submit() {
    if (!resourceKind || !resourceId) return;
    const id = principalId.trim();
    if (!id) { setError('Provide a user or group id.'); return; }
    setSubmitting(true);
    setError('');
    try {
      await createShare(resourceKind, resourceId, {
        shared_with_user_id: principal === 'user' ? id : undefined,
        shared_with_group_id: principal === 'group' ? id : undefined,
        access_level: accessLevel,
        note: note.trim() || undefined,
        expires_at: expiresAt ? new Date(expiresAt).toISOString() : null,
      });
      setPrincipalId('');
      setNote('');
      setExpiresAt('');
      onShared?.();
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSubmitting(false);
    }
  }

  async function revoke(id: string) {
    try {
      await revokeShare(id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  if (!open) return null;
  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16, zIndex: 100 }}>
      <div style={{ width: '100%', maxWidth: 560, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', borderRadius: 12, boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #1e293b', padding: '12px 16px' }}>
          <div style={{ fontSize: 13, fontWeight: 600 }}>Share {resourceLabel || 'resource'}</div>
          <button type="button" onClick={onClose} style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: 13 }}>✕</button>
        </div>
        <div style={{ display: 'grid', gap: 10, padding: 16 }}>
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => setPrincipal('user')} className="of-button" style={{ fontSize: 11, ...(principal === 'user' ? { background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' } : {}) }}>User</button>
            <button type="button" onClick={() => setPrincipal('group')} className="of-button" style={{ fontSize: 11, ...(principal === 'group' ? { background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' } : {}) }}>Group</button>
          </div>
          <PrincipalPicker
            kind={principal}
            value={principalId}
            onChange={(p) => setPrincipalId(p.id)}
          />
          <div style={{ display: 'flex', gap: 8 }}>
            <label style={{ fontSize: 12, flex: 1 }}>
              Access level
              <select value={accessLevel} onChange={(e) => setAccessLevel(e.target.value as AccessLevel)} className="of-input" style={{ marginTop: 4 }}>
                <option value="viewer">viewer</option>
                <option value="editor">editor</option>
                <option value="owner">owner</option>
              </select>
            </label>
            <label style={{ fontSize: 12, flex: 1 }}>
              Expires (optional)
              <input type="datetime-local" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
            </label>
          </div>
          <label style={{ fontSize: 12 }}>
            Note (optional)
            <input value={note} onChange={(e) => setNote(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          {error && <p style={{ color: '#fca5a5', fontSize: 11, margin: 0 }}>{error}</p>}
          <button type="button" onClick={() => void submit()} disabled={submitting} className="of-button of-button--primary" style={{ alignSelf: 'flex-start' }}>
            {submitting ? 'Sharing…' : 'Share'}
          </button>
        </div>
        <div style={{ borderTop: '1px solid #1e293b', padding: '12px 16px' }}>
          <p className="of-eyebrow" style={{ fontSize: 10 }}>Existing shares ({shares.length})</p>
          <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 8, maxHeight: 200, overflow: 'auto' }}>
            {shares.map((s) => (
              <li key={s.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '4px 8px', background: '#1e293b', borderRadius: 4, fontSize: 11 }}>
                <span>{s.shared_with_user_id || s.shared_with_group_id} · {s.access_level}</span>
                <button type="button" onClick={() => void revoke(s.id)} className="of-button" style={{ fontSize: 10, color: '#fca5a5', borderColor: '#7f1d1d' }}>Revoke</button>
              </li>
            ))}
            {shares.length === 0 && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>Not shared.</li>}
          </ul>
        </div>
      </div>
    </div>
  );
}
