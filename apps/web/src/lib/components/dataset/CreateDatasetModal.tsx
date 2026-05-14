import { useEffect, useState, type FormEvent } from 'react';

import { createDataset, type Dataset } from '@/lib/api/datasets';
import { Glyph } from '@/lib/components/ui/Glyph';

const DATASET_FORMATS = ['parquet', 'csv', 'json', 'avro', 'text'] as const;
const VISIBILITIES = ['private', 'shared', 'organization', 'public'] as const;

interface CreateDatasetModalProps {
  open: boolean;
  initialTag?: string;
  onClose: () => void;
  onCreated: (dataset: Dataset) => void;
}

export function CreateDatasetModal({ open, initialTag = '', onClose, onCreated }: CreateDatasetModalProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [format, setFormat] = useState<(typeof DATASET_FORMATS)[number]>('parquet');
  const [tags, setTags] = useState('');
  const [folderPath, setFolderPath] = useState('/datasets');
  const [projectId, setProjectId] = useState('default');
  const [visibility, setVisibility] = useState<(typeof VISIBILITIES)[number]>('private');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setName('');
    setDescription('');
    setFormat('parquet');
    setTags(initialTag);
    setFolderPath('/datasets');
    setProjectId('default');
    setVisibility('private');
    setError('');
  }, [open, initialTag]);

  useEffect(() => {
    if (!open) return;
    function onKeydown(event: KeyboardEvent) {
      if (event.key === 'Escape' && !busy) {
        event.preventDefault();
        onClose();
      }
    }
    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  }, [open, busy, onClose]);

  if (!open) return null;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) return;
    setBusy(true);
    setError('');
    try {
      const dataset = await createDataset({
        name: trimmedName,
        description: description.trim() || undefined,
        format,
        tags: parseTags(tags),
        folder_path: folderPath.trim() || '/datasets',
        project_id: projectId.trim() || 'default',
        resource_visibility: visibility,
      });
      onCreated(dataset);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !busy) onClose();
      }}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
        background: 'rgba(31, 37, 45, 0.46)',
      }}
    >
      <form
        onSubmit={(event) => void submit(event)}
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-dataset-title"
        className="of-panel"
        style={{ width: 'min(560px, 100%)', background: '#fff', overflow: 'hidden' }}
      >
        <header
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            gap: 12,
            alignItems: 'center',
            padding: '12px 16px',
            borderBottom: '1px solid var(--border-default)',
          }}
        >
          <div>
            <p className="of-eyebrow" style={{ margin: 0 }}>
              DATASET-001
            </p>
            <h2 id="create-dataset-title" className="of-heading-sm" style={{ margin: 0 }}>
              New dataset
            </h2>
          </div>
          <button type="button" className="of-button of-button--ghost" onClick={onClose} disabled={busy} aria-label="Close">
            <Glyph name="x" size={15} />
          </button>
        </header>

        <div style={{ padding: 16, display: 'grid', gap: 10 }}>
          {error ? (
            <div className="of-status-danger" style={{ padding: '10px 12px', borderRadius: 'var(--radius-md)' }}>
              {error}
            </div>
          ) : null}

          <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
            <span style={{ fontWeight: 600 }}>Name</span>
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              className="of-input"
              placeholder="orders_clean"
              autoFocus
              required
            />
          </label>

          <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
            <span style={{ fontWeight: 600 }}>Description</span>
            <textarea
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              className="of-input"
              rows={3}
              placeholder="What this dataset contains and who should use it."
              style={{ resize: 'vertical' }}
            />
          </label>

          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 180px) minmax(0, 1fr)', gap: 10 }}>
            <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
              <span style={{ fontWeight: 600 }}>Format</span>
              <select
                value={format}
                onChange={(event) => setFormat(event.target.value as (typeof DATASET_FORMATS)[number])}
                className="of-input"
              >
                {DATASET_FORMATS.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </label>

            <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
              <span style={{ fontWeight: 600 }}>Tags</span>
              <input
                value={tags}
                onChange={(event) => setTags(event.target.value)}
                className="of-input"
                placeholder="finance, daily"
              />
            </label>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 140px)', gap: 10 }}>
            <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
              <span style={{ fontWeight: 600 }}>Folder</span>
              <input
                value={folderPath}
                onChange={(event) => setFolderPath(event.target.value)}
                className="of-input"
                placeholder="/datasets"
              />
            </label>

            <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
              <span style={{ fontWeight: 600 }}>Visibility</span>
              <select
                value={visibility}
                onChange={(event) => setVisibility(event.target.value as (typeof VISIBILITIES)[number])}
                className="of-input"
              >
                {VISIBILITIES.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <label style={{ display: 'grid', gap: 4, fontSize: 13 }}>
            <span style={{ fontWeight: 600 }}>Project</span>
            <input
              value={projectId}
              onChange={(event) => setProjectId(event.target.value)}
              className="of-input"
              placeholder="default"
            />
          </label>
        </div>

        <footer
          style={{
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 8,
            padding: '12px 16px',
            borderTop: '1px solid var(--border-default)',
            background: 'var(--bg-panel-muted)',
          }}
        >
          <button type="button" className="of-button of-button--ghost" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="of-button of-button--primary" disabled={busy || !name.trim()}>
            {busy ? 'Creating...' : 'Create dataset'}
          </button>
        </footer>
      </form>
    </div>
  );
}

function parseTags(value: string) {
  return Array.from(
    new Set(
      value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}
