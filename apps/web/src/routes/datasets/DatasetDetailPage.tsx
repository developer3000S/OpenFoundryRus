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
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Datasets</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">{dataset.name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {dataset.id} · {dataset.format} · {dataset.row_count} rows · {dataset.size_bytes} bytes · branch: {dataset.active_branch}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Link to={`/datasets/${dataset.id}/branches`} className="of-button" style={{ fontSize: 12 }}>Branches</Link>
          <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
            Delete
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs
        tabs={['preview', 'schema', 'files', 'transactions', 'versions', 'quality', 'metadata'] as const}
        active={tab}
        onChange={(t) => void loadTab(t)}
      />

      {tab === 'preview' && (
        <section className="of-panel" style={{ padding: 16 }}>
          {preview ? (
            <VirtualizedPreviewTable
              columns={preview.columns ?? previewColumns.map((name) => ({ name }))}
              rows={previewRows}
              transactions={transactions}
              fileFormat={preview.format ?? null}
            />
          ) : (
            <p className="of-text-muted">No preview yet.</p>
          )}
        </section>
      )}

      {tab === 'schema' && (
        <section className="of-panel" style={{ padding: 16 }}>
          {schema ? <SchemaTable fields={schema.fields} /> : <p className="of-text-muted">Loading…</p>}
        </section>
      )}

      {tab === 'files' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <ul style={{ paddingLeft: 18, fontSize: 12 }}>
            {files.map((f) => (
              <li key={f.path}>{f.path} · {f.entry_type} · {f.size_bytes ?? '—'} bytes</li>
            ))}
            {files.length === 0 && <li className="of-text-muted">No files.</li>}
          </ul>
        </section>
      )}

      {tab === 'transactions' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <ul style={{ paddingLeft: 18, fontSize: 12 }}>
            {transactions.map((t) => (
              <li key={t.id}>
                {t.id} · {t.operation} · {t.status} · {new Date(t.created_at).toLocaleString()}
              </li>
            ))}
            {transactions.length === 0 && <li className="of-text-muted">No transactions.</li>}
          </ul>
        </section>
      )}

      {tab === 'versions' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <ul style={{ paddingLeft: 18, fontSize: 12 }}>
            {versions.map((v) => (
              <li key={v.id}>v{v.version} · {v.message || '—'} · {v.row_count} rows · {new Date(v.created_at).toLocaleString()}</li>
            ))}
            {versions.length === 0 && <li className="of-text-muted">No versions.</li>}
          </ul>
        </section>
      )}

      {tab === 'quality' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <button type="button" onClick={() => void refreshQuality()} disabled={busy} className="of-button" style={{ fontSize: 12 }}>
            Refresh quality profile
          </button>
          <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {quality ? JSON.stringify(quality, null, 2) : 'Loading…'}
          </pre>
        </section>
      )}

      {tab === 'metadata' && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Description
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Tags (comma separated)
            <input value={tagsText} onChange={(e) => setTagsText(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div>
            <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">
              Save
            </button>
          </div>
        </section>
      )}
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
      <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
        {JSON.stringify(fields, null, 2)}
      </pre>
    );
  }
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
      <thead>
        <tr>
          {['Name', 'Type', 'Nullable', 'Description'].map((h) => (
            <th key={h} style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((f, i) => (
          <tr key={i} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
            <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>{f.name ?? '—'}</td>
            <td style={{ padding: 6 }}>{f.type ?? '—'}</td>
            <td style={{ padding: 6 }}>{f.nullable === undefined ? '—' : f.nullable ? '✓' : '✗'}</td>
            <td style={{ padding: 6 }} className="of-text-muted">{f.description ?? '—'}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
