import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  deleteItem,
  getMediaSet,
  listItems,
  uploadItem,
  type MediaItem,
  type MediaSet,
} from '@/lib/api/mediaSets';

export function MediaSetDetailPage() {
  const { rid: ridParam = '' } = useParams();
  const rid = decodeURIComponent(ridParam);

  const [mediaSet, setMediaSet] = useState<MediaSet | null>(null);
  const [items, setItems] = useState<MediaItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [uploadFile, setUploadFile] = useState<File | null>(null);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [setRes, itemsRes] = await Promise.all([getMediaSet(rid), listItems(rid, {})]);
      setMediaSet(setRes);
      setItems(itemsRes);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load media set');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rid]);

  async function handleUpload() {
    if (!uploadFile || !mediaSet) return;
    setBusy(true);
    try {
      await uploadItem(rid, uploadFile);
      setUploadFile(null);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Upload failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteItem(itemRid: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete item?')) return;
    setBusy(true);
    try {
      await deleteItem(itemRid);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/media-sets" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Media sets
      </Link>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : mediaSet ? (
        <>
          <header>
            <h1 className="of-heading-xl">{mediaSet.name}</h1>
            <p className="of-text-muted" style={{ fontSize: 13 }}>
              {mediaSet.rid} · schema {mediaSet.schema} · {mediaSet.allowed_mime_types.join(', ')}
            </p>
          </header>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Upload item</p>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8 }}>
              <input type="file" onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)} />
              <button type="button" onClick={() => void handleUpload()} disabled={busy || !uploadFile} className="of-button of-button--primary">
                Upload
              </button>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Items ({items.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {items.map((item) => (
                <li key={item.rid} style={{ padding: 8, borderBottom: '1px solid var(--border-default)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <strong style={{ fontSize: 13 }}>{item.path ?? item.rid.slice(0, 12)}</strong>
                      <p className="of-text-muted" style={{ fontSize: 11 }}>
                        {item.mime_type} · {item.size_bytes}b · branch {item.branch}
                      </p>
                    </div>
                    <button type="button" onClick={() => void handleDeleteItem(item.rid)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                      Delete
                    </button>
                  </div>
                </li>
              ))}
              {items.length === 0 && <li className="of-text-muted">No items yet.</li>}
            </ul>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Media set JSON</p>
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(mediaSet, null, 2)}
            </pre>
          </section>
        </>
      ) : null}
    </section>
  );
}
