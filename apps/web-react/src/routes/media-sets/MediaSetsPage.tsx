import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { createMediaSet, deleteMediaSet, listMediaSets, uploadItem, type MediaSet } from '@/lib/api/mediaSets';

export function MediaSetsPage() {
  const [sets, setSets] = useState<MediaSet[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  // Create draft
  const [name, setName] = useState('Default media set');
  const [projectRid, setProjectRid] = useState('ri.project.default');
  const [contentTypes, setContentTypes] = useState('image/jpeg,image/png,application/pdf');

  // Upload state
  const [uploadTargetRid, setUploadTargetRid] = useState<string | null>(null);
  const [uploadFile, setUploadFile] = useState<File | null>(null);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const res = await listMediaSets({});
      setSets(res);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load media sets');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function handleCreate() {
    setBusy(true);
    setError('');
    try {
      await createMediaSet({
        name,
        project_rid: projectRid,
        schema: 'DOCUMENT',
        allowed_mime_types: contentTypes.split(',').map((s) => s.trim()).filter(Boolean),
      });
      setName('Default media set');
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create media set');
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(rid: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete this media set?')) return;
    setBusy(true);
    try {
      await deleteMediaSet(rid);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete');
    } finally {
      setBusy(false);
    }
  }

  async function handleUpload() {
    if (!uploadTargetRid || !uploadFile) return;
    setBusy(true);
    setError('');
    try {
      await uploadItem(uploadTargetRid, uploadFile);
      setUploadFile(null);
      setUploadTargetRid(null);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Upload failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Media sets</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Branch-aware media stores for images, PDFs, audio. Upload directly via signed PUTs.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Create media set</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" className="of-input" style={{ width: 240 }} />
          <input value={projectRid} onChange={(e) => setProjectRid(e.target.value)} placeholder="Project rid" className="of-input" style={{ width: 240 }} />
          <input
            value={contentTypes}
            onChange={(e) => setContentTypes(e.target.value)}
            placeholder="image/jpeg,image/png,…"
            className="of-input"
            style={{ width: 280 }}
          />
          <button type="button" onClick={() => void handleCreate()} disabled={busy} className="of-button of-button--primary">
            Create
          </button>
        </div>
      </section>

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Media sets ({sets.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {sets.map((s) => (
              <li key={s.rid} style={{ padding: 12, borderBottom: '1px solid var(--border-default)' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <Link to={`/media-sets/${encodeURIComponent(s.rid)}`} style={{ fontWeight: 600 }}>
                      {s.name}
                    </Link>
                    <p className="of-text-muted" style={{ fontSize: 11 }}>
                      {s.rid} · schema {s.schema} · {s.allowed_mime_types.length} mime types
                    </p>
                  </div>
                  <div style={{ display: 'flex', gap: 6 }}>
                    <button type="button" onClick={() => setUploadTargetRid(s.rid)} className="of-button" style={{ fontSize: 11 }}>
                      Upload
                    </button>
                    <button type="button" onClick={() => void handleDelete(s.rid)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                      Delete
                    </button>
                  </div>
                </div>
                {uploadTargetRid === s.rid && (
                  <div style={{ marginTop: 8, padding: 8, borderRadius: 8, background: 'var(--bg-subtle)' }}>
                    <input type="file" onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)} />
                    <button type="button" onClick={() => void handleUpload()} disabled={busy || !uploadFile} className="of-button of-button--primary" style={{ marginLeft: 8, fontSize: 11 }}>
                      Upload
                    </button>
                    <button type="button" onClick={() => setUploadTargetRid(null)} className="of-button" style={{ marginLeft: 4, fontSize: 11 }}>
                      Cancel
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
