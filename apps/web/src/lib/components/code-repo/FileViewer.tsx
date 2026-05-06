import type { RepositoryFile, SearchResult } from '@/lib/api/code-repos';

interface Props {
  files: RepositoryFile[];
  selectedFilePath: string;
  searchQuery: string;
  searchResults: SearchResult[];
  busy?: boolean;
  onSelectFile: (path: string) => void;
  onSearchQueryChange: (query: string) => void;
  onRunSearch: () => void;
}

export function FileViewer({
  files,
  selectedFilePath,
  searchQuery,
  searchResults,
  busy = false,
  onSelectFile,
  onSearchQueryChange,
  onRunSearch,
}: Props) {
  const selectedFile = files.find((file) => file.path === selectedFilePath) ?? files[0] ?? null;
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#b45309' }}>
            File Browser
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Repository tree and Tantivy-style search results
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Switch across tracked files, inspect content, and search through indexed snippets.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6, maxWidth: 360, width: '100%' }}>
          <input
            value={searchQuery}
            onChange={(e) => onSearchQueryChange(e.target.value)}
            placeholder="Search package, widget, connector…"
            className="of-input"
            style={{ borderRadius: 999 }}
          />
          <button type="button" onClick={onRunSearch} disabled={busy} className="of-button of-button--primary" style={{ background: '#f59e0b', color: '#0c0a09', borderRadius: 999 }}>
            Search
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.8fr) minmax(0, 1.2fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 8 }}>
          {files.map((file) => {
            const active = selectedFilePath === file.path;
            return (
              <button
                key={file.path}
                type="button"
                onClick={() => onSelectFile(file.path)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 12,
                  border: `1px solid ${active ? '#f59e0b' : 'var(--border-default)'}`,
                  background: active ? '#fffbeb' : 'var(--bg-elevated)',
                  borderRadius: 16,
                  cursor: 'pointer',
                }}
              >
                <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{file.path}</p>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                  {file.language} · {file.branch_name} · {file.size_bytes} bytes
                </p>
              </button>
            );
          })}
        </div>

        <div style={{ display: 'grid', gap: 12 }}>
          <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <div>
                <p style={{ fontWeight: 600 }}>{selectedFile?.path ?? 'No file selected'}</p>
                <p style={{ marginTop: 4, fontSize: 11, color: '#a8a29e' }}>
                  {selectedFile?.language ?? 'n/a'} · commit {selectedFile?.last_commit_sha ?? 'n/a'}
                </p>
              </div>
            </div>
            <pre
              style={{
                marginTop: 14,
                overflowX: 'auto',
                borderRadius: 16,
                border: '1px solid #44403c',
                background: '#1c1917',
                padding: 14,
                fontSize: 11,
                color: '#fde68a',
                fontFamily: 'var(--font-mono)',
              }}
            >
              {selectedFile?.content ?? 'Select a file to inspect its content.'}
            </pre>
          </div>

          <div className="of-panel" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Search results</p>
              <p className="of-eyebrow">{searchResults.length} matches</p>
            </div>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {searchResults.map((result, index) => (
                <div key={`${result.path}-${index}`} className="of-panel-muted" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{result.path}</p>
                    <span className="of-chip">score {result.score.toFixed(2)}</span>
                  </div>
                  <p className="of-text-muted" style={{ marginTop: 6, fontSize: 11 }}>
                    {result.branch_name}
                  </p>
                  <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                    {result.snippet}
                  </p>
                </div>
              ))}
              {searchResults.length === 0 && (
                <p
                  className="of-text-muted"
                  style={{ border: '1px dashed var(--border-default)', borderRadius: 16, padding: 18, fontSize: 13, textAlign: 'center' }}
                >
                  Run a query to surface indexed snippets across repository files.
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
