import { useState } from 'react';

import { virtualTables, type DiscoveredEntry, type RegisterVirtualTableRequest, type TableType, type VirtualTableProvider } from '@/lib/api/virtual-tables';

import { RemoteCatalogBrowser } from './RemoteCatalogBrowser';

interface StagedEntry {
  entry: DiscoveredEntry;
  name: string;
  parentFolderRid: string;
  tableType: TableType;
}

interface Props {
  open: boolean;
  sourceRid: string;
  provider: VirtualTableProvider;
  onClose: () => void;
  onCompleted?: (registered: number, failed: number) => void;
}

const TABLE_TYPES: TableType[] = ['TABLE', 'VIEW', 'MATERIALIZED_VIEW', 'EXTERNAL_DELTA', 'MANAGED_DELTA', 'MANAGED_ICEBERG', 'PARQUET_FILES', 'AVRO_FILES', 'CSV_FILES', 'OTHER'];

export function BulkRegisterDialog({ open, sourceRid, provider, onClose, onCompleted }: Props) {
  const [projectRid, setProjectRid] = useState('');
  const [staged, setStaged] = useState<StagedEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [error, setError] = useState('');
  const [errors, setErrors] = useState<Array<{ name: string; error: string }>>([]);

  if (!open) return null;

  function stageEntry(entry: DiscoveredEntry) {
    if (!entry.registrable) return;
    setStaged((prev) => {
      if (prev.some((s) => s.entry.path === entry.path)) return prev;
      return [...prev, { entry, name: entry.display_name, parentFolderRid: '', tableType: entry.inferred_table_type ?? 'OTHER' }];
    });
  }

  function unstage(index: number) {
    setStaged((prev) => prev.filter((_, i) => i !== index));
  }

  function patchRow(index: number, patch: Partial<StagedEntry>) {
    setStaged((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  }

  function deriveLocator(entry: DiscoveredEntry): RegisterVirtualTableRequest['locator'] {
    const parts = entry.path.split('/').filter(Boolean);
    if (provider === 'AMAZON_S3' || provider === 'AZURE_ABFS' || provider === 'GCS') {
      return { kind: 'file', bucket: parts[0] ?? entry.path, prefix: parts.slice(1).join('/'), format: 'parquet' };
    }
    return { kind: 'tabular', database: parts[0] ?? '', schema: parts[1] ?? '', table: parts[parts.length - 1] ?? entry.display_name };
  }

  async function submit() {
    if (!projectRid.trim()) { setError('Project rid is required'); return; }
    if (staged.length === 0) { setError('Pick at least one entry from the catalog tree'); return; }
    setBusy(true);
    setError('');
    setErrors([]);
    setProgress({ done: 0, total: staged.length });
    try {
      const response = await virtualTables.bulkRegister(sourceRid, {
        project_rid: projectRid.trim(),
        entries: staged.map((s) => ({
          project_rid: projectRid.trim(),
          parent_folder_rid: s.parentFolderRid.trim() || undefined,
          name: s.name.trim() || undefined,
          locator: deriveLocator(s.entry),
          table_type: s.tableType,
        })),
      });
      setProgress({ done: staged.length, total: staged.length });
      setErrors(response.errors);
      onCompleted?.(response.registered.length, response.errors.length);
      if (response.errors.length === 0) onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bulk register failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div role="dialog" aria-modal="true" style={backdropStyle}>
      <div style={modalStyle}>
        <header>
          <h3 style={{ margin: 0 }}>Bulk register virtual tables</h3>
          <p style={mutedStyle}>Pick entries from the source catalog on the left, edit names / folders on the right, then register in one batch.</p>
        </header>

        <label style={{ display: 'flex', flexDirection: 'column', gap: 4, fontSize: 12 }}>
          <span>Target project rid</span>
          <input type="text" value={projectRid} onChange={(e) => setProjectRid(e.target.value)} placeholder="ri.foundry.main.project..." style={inputStyle} />
        </label>

        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(260px, 1fr) minmax(360px, 2fr)', gap: 12 }}>
          <RemoteCatalogBrowser sourceRid={sourceRid} onSelect={stageEntry} />
          <div style={{ border: '1px solid #e5e7eb', borderRadius: 8, padding: 8, background: '#fff', overflow: 'auto', maxHeight: 520 }}>
            {staged.length === 0 ? (
              <div style={mutedStyle}>No entries staged yet. Click an entry on the left.</div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={thStyle}>Name</th>
                    <th style={thStyle}>Parent folder</th>
                    <th style={thStyle}>Type</th>
                    <th style={thStyle}></th>
                  </tr>
                </thead>
                <tbody>
                  {staged.map((row, i) => (
                    <tr key={row.entry.path}>
                      <td style={tdStyle}>
                        <input type="text" value={row.name} onChange={(e) => patchRow(i, { name: e.target.value })} style={smallInputStyle} />
                        <div style={{ color: '#6b7280', fontSize: 10, fontFamily: 'ui-monospace, SFMono-Regular, monospace' }}>{row.entry.path}</div>
                      </td>
                      <td style={tdStyle}>
                        <input type="text" value={row.parentFolderRid} onChange={(e) => patchRow(i, { parentFolderRid: e.target.value })} placeholder="(default)" style={smallInputStyle} />
                      </td>
                      <td style={tdStyle}>
                        <select value={row.tableType} onChange={(e) => patchRow(i, { tableType: e.target.value as TableType })} style={smallInputStyle}>
                          {TABLE_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
                        </select>
                      </td>
                      <td style={tdStyle}>
                        <button type="button" onClick={() => unstage(i)} aria-label="Remove" style={{ border: 'none', background: 'transparent', cursor: 'pointer', color: '#b91c1c' }}>✕</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

        {progress && busy && (
          <div style={{ border: '1px solid #e5e7eb', borderRadius: 4, padding: '8px 12px', fontSize: 14 }}>Registering {progress.done} / {progress.total}…</div>
        )}

        {errors.length > 0 ? (
          <div role="alert" style={{ background: '#fef2f2', border: '1px solid #fecaca', color: '#b91c1c', borderRadius: 4, padding: '8px 12px', fontSize: 14 }}>
            <p>{errors.length} entries failed:</p>
            <ul style={{ margin: '4px 0 0', paddingLeft: 20, fontSize: 12 }}>
              {errors.map((e, i) => <li key={`${e.name}-${i}`}><strong>{e.name}</strong>: {e.error}</li>)}
            </ul>
          </div>
        ) : error ? (
          <div role="alert" style={{ background: '#fef2f2', border: '1px solid #fecaca', color: '#b91c1c', borderRadius: 4, padding: '8px 12px', fontSize: 14 }}>{error}</div>
        ) : null}

        <footer style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" onClick={onClose} disabled={busy} style={btnStyle}>Close</button>
          <button type="button" onClick={() => void submit()} disabled={busy || staged.length === 0} style={primaryBtnStyle}>{busy ? 'Registering…' : `Register ${staged.length}`}</button>
        </footer>
      </div>
    </div>
  );
}

const backdropStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 };
const modalStyle: React.CSSProperties = { background: '#fff', borderRadius: 8, padding: 20, width: 'min(960px, 96vw)', maxHeight: '92vh', overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 12 };
const mutedStyle: React.CSSProperties = { color: '#6b7280', fontSize: 12 };
const inputStyle: React.CSSProperties = { padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 4, fontSize: 14 };
const thStyle: React.CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #e5e7eb', fontSize: 14, verticalAlign: 'top' };
const tdStyle: React.CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #e5e7eb', fontSize: 14, verticalAlign: 'top' };
const smallInputStyle: React.CSSProperties = { width: '100%', padding: '4px 6px', border: '1px solid #d1d5db', borderRadius: 4, fontSize: 12 };
const btnStyle: React.CSSProperties = { padding: '6px 12px', border: '1px solid #d1d5db', borderRadius: 4, background: '#fff', cursor: 'pointer', fontSize: 14 };
const primaryBtnStyle: React.CSSProperties = { ...btnStyle, background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' };
