import { useEffect, useState } from 'react';

import {
  publishDatasetProduct,
  type Dataset,
  type DatasetProduct,
  type PublishDatasetProductRequest,
} from '@/lib/api/datasets';

interface PublishToMarketplaceModalProps {
  dataset: Dataset;
  open: boolean;
  onClose: () => void;
  onPublished?: (product: DatasetProduct) => void;
}

export function PublishToMarketplaceModal({ dataset, open, onClose, onPublished }: PublishToMarketplaceModalProps) {
  const [name, setName] = useState('');
  const [version, setVersion] = useState('1.0.0');
  const [projectId, setProjectId] = useState('');
  const [bootstrapMode, setBootstrapMode] = useState<'schema-only' | 'with-snapshot'>('schema-only');
  const [includeSchema, setIncludeSchema] = useState(true);
  const [includeBranches, setIncludeBranches] = useState(false);
  const [includeRetention, setIncludeRetention] = useState(false);
  const [includeSchedules, setIncludeSchedules] = useState(false);
  const [exportIncludesData, setExportIncludesData] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [publishedUrl, setPublishedUrl] = useState<string | null>(null);

  useEffect(() => {
    if (open && !name) setName(dataset.name);
  }, [open, dataset.name, name]);

  async function submit() {
    if (!name.trim()) {
      setError('name is required');
      return;
    }
    setSaving(true);
    setError(null);
    setPublishedUrl(null);
    try {
      const payload: PublishDatasetProductRequest = {
        name: name.trim(),
        version: version.trim() || '1.0.0',
        bootstrap_mode: bootstrapMode,
        include_schema: includeSchema,
        include_branches: includeBranches,
        include_retention: includeRetention,
        include_schedules: includeSchedules,
        export_includes_data: exportIncludesData,
      };
      if (projectId.trim()) payload.project_id = projectId.trim();
      const product = await publishDatasetProduct(dataset.id, payload);
      setPublishedUrl(`/marketplace/products/${product.id}`);
      onPublished?.(product);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Publish failed.');
    } finally {
      setSaving(false);
    }
  }

  if (!open) return null;

  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100, padding: 16 }}>
      <div style={{ width: '100%', maxWidth: 520, padding: 20, borderRadius: 16, background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b' }}>
        <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>Publish dataset to Marketplace</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 12px', fontSize: 13 }}>
          Creates a marketplace product backed by this dataset.
        </p>

        <div style={{ display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Product name
            <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Version
            <input value={version} onChange={(e) => setVersion(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Project (optional)
            <input value={projectId} onChange={(e) => setProjectId(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Bootstrap mode
            <select value={bootstrapMode} onChange={(e) => setBootstrapMode(e.target.value as typeof bootstrapMode)} className="of-input" style={{ marginTop: 4 }}>
              <option value="schema-only">schema-only</option>
              <option value="with-snapshot">with-snapshot</option>
            </select>
          </label>
          <fieldset style={{ display: 'grid', gap: 4, padding: 8, border: '1px solid #1e293b', borderRadius: 6 }}>
            <legend style={{ fontSize: 11, color: 'var(--text-muted)', padding: '0 6px' }}>Include</legend>
            <label style={{ fontSize: 12, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={includeSchema} onChange={(e) => setIncludeSchema(e.target.checked)} /> schema
            </label>
            <label style={{ fontSize: 12, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={includeBranches} onChange={(e) => setIncludeBranches(e.target.checked)} /> branches
            </label>
            <label style={{ fontSize: 12, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={includeRetention} onChange={(e) => setIncludeRetention(e.target.checked)} /> retention policies
            </label>
            <label style={{ fontSize: 12, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={includeSchedules} onChange={(e) => setIncludeSchedules(e.target.checked)} /> schedules
            </label>
            <label style={{ fontSize: 12, display: 'flex', gap: 4, alignItems: 'center' }}>
              <input type="checkbox" checked={exportIncludesData} onChange={(e) => setExportIncludesData(e.target.checked)} /> export data snapshot
            </label>
          </fieldset>
        </div>

        {error && <p style={{ marginTop: 8, fontSize: 11, color: '#fca5a5' }}>{error}</p>}
        {publishedUrl && (
          <p style={{ marginTop: 8, fontSize: 12 }}>
            Published — <a href={publishedUrl} style={{ color: '#93c5fd' }}>open product</a>
          </p>
        )}

        <div style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end', gap: 6 }}>
          <button type="button" onClick={onClose} className="of-button">Close</button>
          <button type="button" onClick={() => void submit()} disabled={saving || Boolean(publishedUrl)} className="of-button of-button--primary">
            {saving ? 'Publishing…' : publishedUrl ? 'Done' : 'Publish'}
          </button>
        </div>
      </div>
    </div>
  );
}
