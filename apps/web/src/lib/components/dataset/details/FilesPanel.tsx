import type { DatasetFilesystemEntry } from '@/lib/api/datasets';

interface FilesPanelProps {
  entries: DatasetFilesystemEntry[];
  currentVersion: number;
  activeBranch: string;
  loading?: boolean;
  error?: string;
}

function formatBytes(bytes?: number) {
  if (bytes === undefined || bytes === null) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatGuess(entry: DatasetFilesystemEntry) {
  const ext = entry.name.split('.').pop()?.toLowerCase() ?? '';
  if (ext === 'parquet') return 'Parquet';
  if (ext === 'avro') return 'Avro';
  if (ext === 'csv' || ext === 'tsv' || ext === 'txt') return 'Text';
  if (ext === 'json' || ext === 'ndjson') return 'JSON';
  if (entry.content_type) return entry.content_type;
  return ext ? ext.toUpperCase() : '—';
}

function transactionOf(entry: DatasetFilesystemEntry): string {
  const tx = entry.metadata?.['transaction_id'];
  return typeof tx === 'string' ? tx : '—';
}

export function FilesPanel({ entries, currentVersion, activeBranch, loading = false, error = '' }: FilesPanelProps) {
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Files</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Backing artifacts</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>
          Branch <code>{activeBranch}</code>, version <code>v{currentVersion}</code>.
        </p>
      </header>
      {error && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 12, fontSize: 13 }}>{error}</div>}
      {loading ? (
        <div className="of-panel" style={{ padding: 24, textAlign: 'center', fontSize: 13 }}>Loading files…</div>
      ) : entries.length === 0 ? (
        <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', borderRadius: 12, border: '1px dashed var(--border-default)', fontSize: 13 }}>
          No files in this view yet. Upload data or run a pipeline that writes here.
        </div>
      ) : (
        <div className="of-panel" style={{ overflow: 'auto' }}>
          <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Path', 'Format', 'Size', 'Last modified', 'Introduced by'].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-muted)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => {
                const tx = transactionOf(e);
                return (
                  <tr key={e.path} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                    <td style={{ padding: '8px 12px', fontFamily: 'var(--font-mono)' }}>
                      {e.entry_type === 'directory' ? '📁 ' : ''}{e.path}
                    </td>
                    <td style={{ padding: '8px 12px' }}>{formatGuess(e)}</td>
                    <td style={{ padding: '8px 12px', textAlign: 'right' }}>{formatBytes(e.size_bytes)}</td>
                    <td style={{ padding: '8px 12px', color: 'var(--text-muted)' }}>{e.last_modified ? new Date(e.last_modified).toLocaleString() : '—'}</td>
                    <td style={{ padding: '8px 12px', fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>
                      {tx.slice(0, 12)}{tx.length > 12 ? '…' : ''}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
