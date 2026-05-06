import { useEffect, useMemo, useState } from 'react';

import type { DatasetBranch } from '@/lib/api/datasets';

type SchemaField = { name: string; type?: string };

interface BranchPickerProps {
  branches: DatasetBranch[];
  currentBranch: string;
  onSwitch: (branchName: string) => Promise<void> | void;
  onCreate: (params: { name: string; from: string; description?: string }) => Promise<void> | void;
  onDelete?: (branchName: string) => Promise<void> | void;
  schemaCurrent?: SchemaField[];
  schemaMaster?: SchemaField[];
  busy?: boolean;
}

export function BranchPicker({
  branches,
  currentBranch,
  onSwitch,
  onCreate,
  onDelete,
  schemaCurrent,
  schemaMaster,
  busy = false,
}: BranchPickerProps) {
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [newName, setNewName] = useState('');
  const [newFrom, setNewFrom] = useState('');
  const [newDescription, setNewDescription] = useState('');
  const [error, setError] = useState('');

  const sortedBranches = useMemo(
    () =>
      [...branches].sort((a, b) => {
        if (a.is_default !== b.is_default) return a.is_default ? -1 : 1;
        return a.name.localeCompare(b.name);
      }),
    [branches],
  );

  useEffect(() => {
    if (!newFrom && branches.length > 0) {
      setNewFrom(currentBranch || branches.find((b) => b.is_default)?.name || branches[0].name);
    }
  }, [branches, currentBranch, newFrom]);

  const schemaDiff = useMemo(() => {
    if (!schemaCurrent || !schemaMaster) return null;
    const masterByName = new Map(schemaMaster.map((f) => [f.name, f]));
    const currentByName = new Map(schemaCurrent.map((f) => [f.name, f]));
    const added = schemaCurrent.filter((f) => !masterByName.has(f.name));
    const removed = schemaMaster.filter((f) => !currentByName.has(f.name));
    const changed = schemaCurrent.filter((f) => {
      const m = masterByName.get(f.name);
      return m && (m.type ?? '') !== (f.type ?? '');
    });
    return { added, removed, changed };
  }, [schemaCurrent, schemaMaster]);

  async function handleSwitch(name: string) {
    if (name === currentBranch || busy) return;
    setError('');
    try {
      await onSwitch(name);
      setOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Switch failed');
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) {
      setError('Name is required');
      return;
    }
    setError('');
    try {
      await onCreate({ name: newName.trim(), from: newFrom, description: newDescription.trim() || undefined });
      setNewName('');
      setNewDescription('');
      setCreating(false);
      setOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    }
  }

  async function handleDelete() {
    if (!onDelete) return;
    setError('');
    try {
      await onDelete(currentBranch);
      setConfirmingDelete(false);
      setOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    }
  }

  return (
    <div style={{ position: 'relative', display: 'inline-block' }}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="of-button"
        style={{ fontSize: 12, fontFamily: 'var(--font-mono)' }}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        {currentBranch || '— branch —'} ▾
      </button>
      {open && (
        <div
          role="dialog"
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 50,
            width: 360,
            background: 'var(--bg-default)',
            border: '1px solid var(--border-default)',
            borderRadius: 8,
            padding: 12,
            boxShadow: '0 8px 24px rgba(0,0,0,0.2)',
          }}
        >
          <p className="of-eyebrow" style={{ marginBottom: 6 }}>Switch branch</p>
          <ul style={{ paddingLeft: 0, listStyle: 'none', maxHeight: 180, overflow: 'auto', display: 'grid', gap: 2 }}>
            {sortedBranches.map((b) => (
              <li key={b.id}>
                <button
                  type="button"
                  onClick={() => void handleSwitch(b.name)}
                  disabled={busy}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: '6px 8px',
                    borderRadius: 4,
                    border: 'none',
                    background: b.name === currentBranch ? '#1d4ed8' : 'transparent',
                    color: b.name === currentBranch ? '#fff' : 'inherit',
                    cursor: 'pointer',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 12,
                  }}
                >
                  {b.name}{b.is_default ? ' ★' : ''}
                </button>
              </li>
            ))}
          </ul>
          {schemaDiff && (
            <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text-muted)' }}>
              vs master: +{schemaDiff.added.length} / −{schemaDiff.removed.length} / ~{schemaDiff.changed.length}
            </div>
          )}
          {!creating ? (
            <button type="button" onClick={() => setCreating(true)} className="of-button" style={{ marginTop: 8, fontSize: 11 }}>+ New branch</button>
          ) : (
            <form onSubmit={handleCreate} style={{ marginTop: 8, display: 'grid', gap: 6 }}>
              <input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="branch name" className="of-input" />
              <select value={newFrom} onChange={(e) => setNewFrom(e.target.value)} className="of-input">
                {sortedBranches.map((b) => <option key={b.id} value={b.name}>from {b.name}</option>)}
              </select>
              <input value={newDescription} onChange={(e) => setNewDescription(e.target.value)} placeholder="description (optional)" className="of-input" />
              <div style={{ display: 'flex', gap: 6 }}>
                <button type="submit" disabled={busy} className="of-button of-button--primary" style={{ fontSize: 11 }}>Create</button>
                <button type="button" onClick={() => setCreating(false)} className="of-button" style={{ fontSize: 11 }}>Cancel</button>
              </div>
            </form>
          )}
          {onDelete && (
            <div style={{ marginTop: 8, paddingTop: 8, borderTop: '1px solid var(--border-default)' }}>
              {confirmingDelete ? (
                <div style={{ display: 'flex', gap: 6 }}>
                  <button type="button" onClick={() => void handleDelete()} disabled={busy} className="of-button" style={{ fontSize: 11, background: '#dc2626', color: '#fff', borderColor: '#dc2626' }}>
                    Delete &ldquo;{currentBranch}&rdquo;
                  </button>
                  <button type="button" onClick={() => setConfirmingDelete(false)} className="of-button" style={{ fontSize: 11 }}>Cancel</button>
                </div>
              ) : (
                <button type="button" onClick={() => setConfirmingDelete(true)} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                  Delete current branch…
                </button>
              )}
            </div>
          )}
          {error && <p style={{ fontSize: 11, color: '#fca5a5', marginTop: 6 }}>{error}</p>}
        </div>
      )}
    </div>
  );
}
