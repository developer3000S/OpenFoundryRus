import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import { VirtualizedPreviewTable } from '@/lib/components/dataset/VirtualizedPreviewTable';
import {
  deleteDataset,
  getDataset,
  getDatasetQuality,
  getDatasetSchema,
  getVersions,
  listDatasetFilesystem,
  listDatasetTransactions,
  previewDataset,
  refreshDatasetQualityProfile,
  updateDataset,
  type Dataset,
  type DatasetFilesystemEntry,
  type DatasetPreviewResponse,
  type DatasetQualityResponse,
  type DatasetSchema,
  type DatasetTransaction,
  type DatasetVersion,
} from '@/lib/api/datasets';

type Tab = 'preview' | 'schema' | 'files' | 'transactions' | 'versions' | 'quality' | 'metadata';

export function DatasetDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const [tab, setTab] = useState<Tab>('preview');
  const [dataset, setDataset] = useState<Dataset | null>(null);
  const [preview, setPreview] = useState<DatasetPreviewResponse | null>(null);
  const [schema, setSchema] = useState<DatasetSchema | null>(null);
  const [files, setFiles] = useState<DatasetFilesystemEntry[]>([]);
  const [transactions, setTransactions] = useState<DatasetTransaction[]>([]);
  const [versions, setVersions] = useState<DatasetVersion[]>([]);
  const [quality, setQuality] = useState<DatasetQualityResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // metadata edit
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tagsText, setTagsText] = useState('');

  async function load() {
    if (!id) return;
    setLoading(true);
    setError('');
    try {
      const d = await getDataset(id);
      setDataset(d);
      setName(d.name);
      setDescription(d.description);
      setTagsText(d.tags.join(', '));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load dataset');
    } finally {
      setLoading(false);
    }
  }

  async function loadTab(next: Tab) {
    setTab(next);
    if (!id) return;
    try {
      if (next === 'preview' && !preview) setPreview(await previewDataset(id, { limit: 50 }));
      if (next === 'schema' && !schema) setSchema(await getDatasetSchema(id));
      if (next === 'files' && files.length === 0) setFiles((await listDatasetFilesystem(id)).entries);
      if (next === 'transactions' && transactions.length === 0) setTransactions(await listDatasetTransactions(id));
      if (next === 'versions' && versions.length === 0) setVersions(await getVersions(id));
      if (next === 'quality' && !quality) setQuality(await getDatasetQuality(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load tab data');
    }
  }

  useEffect(() => {
    void load();
  }, [id]);

  useEffect(() => {
    if (dataset && tab === 'preview') void loadTab('preview');
  }, [dataset]);

  async function save() {
    if (!dataset) return;
    setBusy(true);
    try {
      const updated = await updateDataset(dataset.id, {
        name,
        description,
        tags: tagsText.split(',').map((t) => t.trim()).filter(Boolean),
      });
      setDataset(updated);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!dataset) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete dataset?')) return;
    setBusy(true);
    try {
      await deleteDataset(dataset.id);
      window.location.href = '/datasets';
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
      setBusy(false);
    }
  }

  async function refreshQuality() {
    if (!dataset) return;
    setBusy(true);
    try {
      await refreshDatasetQualityProfile(dataset.id);
      setQuality(await getDatasetQuality(dataset.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Quality refresh failed');
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading…</p>
      </section>
    );
  }

  if (!dataset) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Datasets</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Not found'}</p>
      </section>
    );
  }

  const previewRows = preview?.rows ?? [];
  const previewColumns = previewRows.length > 0 ? Object.keys(previewRows[0]) : [];

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <header className="of-panel" style={{ padding: 10, display: 'grid', gap: 8 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
          <div style={{ minWidth: 0 }}>
            <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 12 }}>← Datasets</Link>
            <h1 className="of-heading-lg" style={{ marginTop: 4 }}>{dataset.name}</h1>
            <p className="of-text-muted" style={{ marginTop: 2, fontSize: 11, fontFamily: 'var(--font-mono)' }}>
              {dataset.id}
            </p>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
            <Link to={`/datasets/${dataset.id}/branches`} className="of-button">Branches</Link>
            <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b42318', borderColor: '#e5b8b8' }}>
              Delete
            </button>
          </div>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <span className="of-chip">{dataset.format}</span>
          <span className="of-chip">{dataset.row_count.toLocaleString()} rows</span>
          <span className="of-chip">{dataset.size_bytes.toLocaleString()} bytes</span>
          <span className="of-chip of-chip-active">{dataset.active_branch}</span>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 460px), 1fr))', gap: 10, alignItems: 'start' }}>
        <section className="of-panel" style={{ minWidth: 0, overflow: 'hidden' }}>
          <Tabs
            tabs={['preview', 'schema', 'files', 'transactions', 'versions', 'quality', 'metadata'] as const}
            active={tab}
            onChange={(t) => void loadTab(t)}
          />

          <div style={{ padding: tab === 'preview' ? 0 : 10 }}>
            {tab === 'preview' && (
              preview ? (
                <VirtualizedPreviewTable
                  columns={preview.columns ?? previewColumns.map((name) => ({ name }))}
                  rows={previewRows}
                  transactions={transactions}
                  fileFormat={preview.format ?? null}
                />
              ) : (
                <p className="of-text-muted" style={{ padding: 12 }}>No preview yet.</p>
              )
            )}

            {tab === 'schema' && (schema ? <SchemaTable fields={schema.fields} /> : <p className="of-text-muted">Loading…</p>)}

            {tab === 'files' && (
              <table className="of-table">
                <thead><tr><th>Path</th><th>Type</th><th>Size</th></tr></thead>
                <tbody>
                  {files.map((f) => (
                    <tr key={f.path}><td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{f.path}</td><td>{f.entry_type}</td><td>{f.size_bytes ?? '—'} bytes</td></tr>
                  ))}
                  {files.length === 0 && <tr><td colSpan={3} className="of-text-muted">No files.</td></tr>}
                </tbody>
              </table>
            )}

            {tab === 'transactions' && (
              <table className="of-table">
                <thead><tr><th>ID</th><th>Operation</th><th>Status</th><th>Created</th></tr></thead>
                <tbody>
                  {transactions.map((t) => (
                    <tr key={t.id}>
                      <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{t.id}</td>
                      <td>{t.operation}</td>
                      <td>{t.status}</td>
                      <td>{new Date(t.created_at).toLocaleString()}</td>
                    </tr>
                  ))}
                  {transactions.length === 0 && <tr><td colSpan={4} className="of-text-muted">No transactions.</td></tr>}
                </tbody>
              </table>
            )}

            {tab === 'versions' && (
              <table className="of-table">
                <thead><tr><th>Version</th><th>Message</th><th>Rows</th><th>Created</th></tr></thead>
                <tbody>
                  {versions.map((v) => (
                    <tr key={v.id}><td>v{v.version}</td><td>{v.message || '—'}</td><td>{v.row_count}</td><td>{new Date(v.created_at).toLocaleString()}</td></tr>
                  ))}
                  {versions.length === 0 && <tr><td colSpan={4} className="of-text-muted">No versions.</td></tr>}
                </tbody>
              </table>
            )}

            {tab === 'quality' && (
              <div style={{ display: 'grid', gap: 8 }}>
                <button type="button" onClick={() => void refreshQuality()} disabled={busy} className="of-button" style={{ width: 'fit-content' }}>
                  Refresh quality profile
                </button>
                <pre style={{ padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 2, overflow: 'auto' }}>
                  {quality ? JSON.stringify(quality, null, 2) : 'Loading…'}
                </pre>
              </div>
            )}

            {tab === 'metadata' && (
              <div style={{ display: 'grid', gap: 8 }}>
                <label style={{ fontSize: 12 }}>
                  Name
                  <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
                </label>
                <label style={{ fontSize: 12 }}>
                  Description
                  <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
                </label>
                <label style={{ fontSize: 12 }}>
                  Tags
                  <input value={tagsText} onChange={(e) => setTagsText(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
                </label>
                <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary" style={{ width: 'fit-content' }}>
                  Save
                </button>
              </div>
            )}
          </div>
        </section>

        <aside className="of-panel" style={{ padding: 10, display: 'grid', gap: 10 }}>
          <div>
            <p className="of-heading-sm">Dataset details</p>
            <p className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>{dataset.description || 'No description.'}</p>
          </div>
          <dl style={{ display: 'grid', gridTemplateColumns: '96px minmax(0, 1fr)', gap: '7px 10px', fontSize: 12 }}>
            <dt className="of-text-muted">Owner</dt><dd>{dataset.owner_id}</dd>
            <dt className="of-text-muted">Version</dt><dd>v{dataset.current_version}</dd>
            <dt className="of-text-muted">Storage</dt><dd style={{ fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>{dataset.storage_path}</dd>
            <dt className="of-text-muted">Created</dt><dd>{new Date(dataset.created_at).toLocaleString()}</dd>
            <dt className="of-text-muted">Updated</dt><dd>{new Date(dataset.updated_at).toLocaleString()}</dd>
            <dt className="of-text-muted">Quality</dt><dd>{quality?.score == null ? '—' : `${Math.round(quality.score * 100)}%`}</dd>
          </dl>
          <div>
            <p className="of-eyebrow">Tags</p>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 6 }}>
              {dataset.tags.map((tag) => <span key={tag} className="of-chip">{tag}</span>)}
              {dataset.tags.length === 0 && <span className="of-text-muted">No tags</span>}
            </div>
          </div>
        </aside>
      </div>
    </section>
  );
}

interface SchemaField {
  name?: string;
  type?: string;
  nullable?: boolean;
  description?: string;
}

function SchemaTable({ fields }: { fields: unknown }) {
  const rows: SchemaField[] = Array.isArray(fields) ? (fields as SchemaField[]) : [];
  if (rows.length === 0) {
    return (
      <pre style={{ padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 2, overflow: 'auto' }}>
        {JSON.stringify(fields, null, 2)}
      </pre>
    );
  }
  return (
    <table className="of-table" style={{ fontSize: 12 }}>
      <thead>
        <tr>
          {['Name', 'Type', 'Nullable', 'Description'].map((h) => (
            <th key={h}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((f, i) => (
          <tr key={i}>
            <td style={{ fontFamily: 'var(--font-mono)' }}>{f.name ?? '—'}</td>
            <td>{f.type ?? '—'}</td>
            <td>{f.nullable === undefined ? '—' : f.nullable ? '✓' : '✗'}</td>
            <td className="of-text-muted">{f.description ?? '—'}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
