import { useEffect, useState } from 'react';

import { datasetFileDownloadUrl, listDatasetFiles, type DatasetFilesResponse } from '@/lib/api/datasets';

export type RetentionPurgeMarker = {
  policyName: string;
  daysUntilPurge: number;
};

interface FilesTabProps {
  datasetRid: string;
  branch?: string;
  viewId?: string;
  initialPrefix?: string;
  retentionPurges?: Record<string, RetentionPurgeMarker>;
  onOpenRetention?: () => void;
}

function fmtBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function shortSha(value?: string | null) {
  if (!value) return '—';
  return value.length > 12 ? `${value.slice(0, 12)}…` : value;
}

export function FilesTab({
  datasetRid,
  branch = 'master',
  viewId,
  initialPrefix = '',
  retentionPurges = {},
  onOpenRetention,
}: FilesTabProps) {
  const [response, setResponse] = useState<DatasetFilesResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [prefix, setPrefix] = useState(initialPrefix);

  useEffect(() => {
    if (!datasetRid) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    listDatasetFiles(datasetRid, { branch, view_id: viewId, prefix: prefix || undefined })
      .then((r) => { if (!cancelled) setResponse(r); })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Files listing failed.');
          setResponse(null);
        }
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetRid, branch, viewId, prefix]);

  const files = response?.files ?? [];

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header className="of-panel" style={{ padding: 16, display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}>
        <div>
          <div style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.18em' }}>Files</div>
          <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Backing filesystem</h2>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>
            {files.length.toLocaleString()} file{files.length === 1 ? '' : 's'} on branch <code>{response?.branch ?? branch}</code>
          </p>
        </div>
        <input
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          placeholder="Filter by prefix…"
          className="of-input"
          style={{ fontSize: 12, fontFamily: 'var(--font-mono)', minWidth: 200 }}
        />
      </header>

      {loading && <div className="of-panel" style={{ padding: 16, fontSize: 13 }}>Loading files…</div>}
      {error && <div className="of-status-danger" style={{ padding: 16, borderRadius: 12, fontSize: 13 }}>{error}</div>}

      {response && (
        <div className="of-panel" style={{ overflow: 'auto' }}>
          <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Path', 'Size', 'SHA256', 'Tx', 'Modified', 'Status', ''].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-muted)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {files.map((f) => {
                const purge = retentionPurges[f.id];
                const downloadUrl = datasetFileDownloadUrl(datasetRid, f.id);
                return (
                  <tr key={f.id} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                    <td style={{ padding: '8px 12px', fontFamily: 'var(--font-mono)' }}>
                      {f.logical_path}
                      {purge && (
                        <button
                          type="button"
                          onClick={onOpenRetention}
                          style={{ marginLeft: 8, fontSize: 10, padding: '1px 6px', borderRadius: 999, background: '#7f1d1d', color: '#fecaca', border: 'none', cursor: 'pointer', fontWeight: 600 }}
                        >
                          Purges in {purge.daysUntilPurge}d
                        </button>
                      )}
                    </td>
                    <td style={{ padding: '8px 12px' }}>{fmtBytes(f.size_bytes)}</td>
                    <td style={{ padding: '8px 12px', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{shortSha(f.sha256)}</td>
                    <td style={{ padding: '8px 12px', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{f.transaction_id?.slice(0, 8) ?? '—'}…</td>
                    <td style={{ padding: '8px 12px', color: 'var(--text-muted)' }}>{new Date(f.modified_at).toLocaleString()}</td>
                    <td style={{ padding: '8px 12px' }}>
                      <span style={{ padding: '2px 8px', borderRadius: 999, fontSize: 10, textTransform: 'uppercase', fontWeight: 500, ...(f.status === 'active' ? { background: '#022c22', color: '#86efac' } : { background: '#7f1d1d', color: '#fecaca' }) }}>
                        {f.status}
                      </span>
                    </td>
                    <td style={{ padding: '8px 12px' }}>
                      <a href={downloadUrl} className="of-button" style={{ fontSize: 11, padding: '2px 8px', textDecoration: 'none' }}>Download</a>
                    </td>
                  </tr>
                );
              })}
              {files.length === 0 && !loading && (
                <tr>
                  <td colSpan={7} style={{ padding: 16, textAlign: 'center', color: 'var(--text-muted)' }}>No files match the current filter.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
