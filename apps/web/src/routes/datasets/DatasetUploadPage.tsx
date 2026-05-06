import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { createDataset, uploadData } from '@/lib/api/datasets';

export function DatasetUploadPage() {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [format, setFormat] = useState('parquet');
  const [tags, setTags] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0] ?? null;
    setFile(f);
    if (f && !name) {
      setName(f.name.replace(/\.[^.]+$/, ''));
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const tagList = tags.split(',').map((t) => t.trim()).filter(Boolean);
      const ds = await createDataset({ name, description: description || undefined, format, tags: tagList });
      if (file) await uploadData(ds.id, file);
      navigate(`/datasets/${ds.id}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Upload failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 720 }}>
      <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Datasets</Link>
      <header>
        <h1 className="of-heading-xl">Upload dataset</h1>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          Description
          <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          Format
          <select value={format} onChange={(e) => setFormat(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
            <option value="parquet">parquet</option>
            <option value="csv">csv</option>
            <option value="json">json</option>
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Tags (comma separated)
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="finance, monthly" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          File
          <input type="file" onChange={handleFile} accept=".parquet,.csv,.json,.jsonl,.tsv" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="submit" disabled={busy || !name} className="of-button of-button--primary">
            {busy ? 'Uploading…' : 'Create dataset'}
          </button>
          <Link to="/datasets" className="of-button">Cancel</Link>
        </div>
      </form>
    </section>
  );
}
