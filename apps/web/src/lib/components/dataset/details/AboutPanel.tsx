import { useEffect, useState } from 'react';

import type { Dataset } from '@/lib/api/datasets';
import type { UserProfile } from '@/lib/api/auth';

interface AboutPanelProps {
  dataset: Dataset;
  users: UserProfile[];
  saving?: boolean;
  error?: string;
  onSave: (patch: { description: string; tags: string[]; owner_id: string }) => void | Promise<void>;
}

export function AboutPanel({ dataset, users, saving = false, error = '', onSave }: AboutPanelProps) {
  const [description, setDescription] = useState(dataset.description);
  const [tagsInput, setTagsInput] = useState(dataset.tags.join(', '));
  const [ownerId, setOwnerId] = useState(dataset.owner_id);

  useEffect(() => {
    setDescription(dataset.description);
    setTagsInput(dataset.tags.join(', '));
    setOwnerId(dataset.owner_id);
  }, [dataset.id]);

  const ownerName = (uid: string) => users.find((u) => u.id === uid)?.name ?? uid.slice(0, 8);

  const projectPath = (() => {
    const parts = (dataset.storage_path || '').split('/').filter(Boolean);
    return parts.length === 0 ? dataset.name : parts.join(' / ');
  })();

  async function submit() {
    const tags = tagsInput.split(',').map((t) => t.trim()).filter(Boolean);
    await onSave({ description, tags, owner_id: ownerId });
  }

  return (
    <section style={{ display: 'grid', gap: 20 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>About</div>
          <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>{dataset.name}</h2>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>Identity, ownership and human-readable metadata.</p>
        </div>
        <button type="button" onClick={() => void submit()} disabled={saving} className="of-button of-button--primary">
          {saving ? 'Saving…' : 'Save'}
        </button>
      </header>

      <dl style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', fontSize: 13 }}>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>RID</dt>
          <dd style={{ margin: '2px 0 0', fontFamily: 'var(--font-mono)', fontSize: 11, wordBreak: 'break-all' }}>{dataset.id}</dd>
        </div>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>Project path</dt>
          <dd style={{ margin: '2px 0 0' }}>{projectPath}</dd>
        </div>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>Format</dt>
          <dd style={{ margin: '2px 0 0' }}>{dataset.format}</dd>
        </div>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>Active branch</dt>
          <dd style={{ margin: '2px 0 0' }}>{dataset.active_branch} (v{dataset.current_version})</dd>
        </div>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>Created</dt>
          <dd style={{ margin: '2px 0 0' }}>{new Date(dataset.created_at).toLocaleString()}</dd>
        </div>
        <div>
          <dt style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>Updated</dt>
          <dd style={{ margin: '2px 0 0' }}>{new Date(dataset.updated_at).toLocaleString()}</dd>
        </div>
      </dl>

      <div style={{ display: 'grid', gap: 12 }}>
        <label style={{ fontSize: 13 }}>
          Owner
          <select value={ownerId} onChange={(e) => setOwnerId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
            {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          </select>
          <span className="of-text-muted" style={{ display: 'block', marginTop: 4, fontSize: 11 }}>Currently: {ownerName(dataset.owner_id)}</span>
        </label>

        <label style={{ fontSize: 13 }}>
          Description (Markdown)
          <textarea rows={5} value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
        </label>

        <label style={{ fontSize: 13 }}>
          Tags
          <input value={tagsInput} onChange={(e) => setTagsInput(e.target.value)} placeholder="finance, monthly, curated" className="of-input" style={{ marginTop: 4 }} />
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
            {dataset.tags.map((tag) => (
              <span key={tag} style={{ background: '#1e3a8a', color: '#bfdbfe', padding: '3px 10px', borderRadius: 999, fontSize: 11, fontWeight: 500 }}>
                {tag}
              </span>
            ))}
          </div>
        </label>
      </div>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 12, fontSize: 13 }}>
          {error}
        </div>
      )}
    </section>
  );
}
