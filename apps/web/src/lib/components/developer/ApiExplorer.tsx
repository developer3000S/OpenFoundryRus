import { useEffect, useMemo, useState } from 'react';

import type { OpenApiOperation, OpenApiSpec } from '@/lib/api/developer';

interface ApiExplorerProps {
  spec: OpenApiSpec | null;
  loading?: boolean;
  error?: string;
}

interface ExplorerOperation {
  key: string;
  path: string;
  method: string;
  operation: OpenApiOperation;
}

function methodTone(method: string) {
  if (method === 'get') return { background: '#dbeafe', color: '#0369a1' };
  if (method === 'post') return { background: '#d1fae5', color: '#047857' };
  if (method === 'patch') return { background: '#fef3c7', color: '#b45309' };
  return { background: '#fee2e2', color: '#b91c1c' };
}

export function ApiExplorer({ spec, loading = false, error = '' }: ApiExplorerProps) {
  const [search, setSearch] = useState('');
  const [selectedKey, setSelectedKey] = useState('');

  const operations = useMemo<ExplorerOperation[]>(() => {
    if (!spec) return [];
    return Object.entries(spec.paths).flatMap(([path, methods]) =>
      Object.entries(methods).map(([method, operation]) => ({
        key: `${method}:${path}`,
        path,
        method,
        operation,
      })),
    );
  }, [spec]);

  const filteredOperations = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return operations;
    return operations.filter(
      (entry) =>
        entry.path.toLowerCase().includes(query) ||
        entry.method.toLowerCase().includes(query) ||
        entry.operation.summary.toLowerCase().includes(query) ||
        entry.operation.operationId.toLowerCase().includes(query) ||
        entry.operation.tags.some((tag) => tag.toLowerCase().includes(query)),
    );
  }, [operations, search]);

  useEffect(() => {
    if (!filteredOperations.length) {
      setSelectedKey('');
      return;
    }
    if (!filteredOperations.some((entry) => entry.key === selectedKey)) {
      setSelectedKey(filteredOperations[0]?.key ?? '');
    }
  }, [filteredOperations, selectedKey]);

  const selectedOperation =
    filteredOperations.find((entry) => entry.key === selectedKey) ??
    operations.find((entry) => entry.key === selectedKey) ??
    null;

  return (
    <section className="of-panel" style={{ overflow: 'hidden' }}>
      <div style={{ borderBottom: '1px solid var(--border-default)', padding: '20px 24px' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <p className="of-eyebrow" style={{ color: '#059669' }}>
              REST API docs
            </p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Proto-derived explorer
            </h2>
            <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, maxWidth: 720 }}>
              Every operation in this panel is generated from the workspace proto services. Inspect
              request and response contracts before wiring SDK plugins, CLI scripts, or CI jobs.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', minWidth: 240 }}>
            <div className="of-panel-muted" style={{ padding: 12 }}>
              <p className="of-eyebrow">Paths</p>
              <p style={{ marginTop: 4, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                {spec ? Object.keys(spec.paths).length : 0}
              </p>
            </div>
            <div className="of-panel-muted" style={{ padding: 12 }}>
              <p className="of-eyebrow">Schemas</p>
              <p style={{ marginTop: 4, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                {spec ? Object.keys(spec.components.schemas).length : 0}
              </p>
            </div>
          </div>
        </div>
      </div>

      {loading ? (
        <div style={{ padding: 48, fontSize: 13, color: 'var(--text-muted)' }}>Loading OpenAPI document…</div>
      ) : error ? (
        <div style={{ padding: 48, fontSize: 13, color: 'var(--status-danger)' }}>{error}</div>
      ) : !spec ? (
        <div style={{ padding: 48, fontSize: 13, color: 'var(--text-muted)' }}>
          The generated OpenAPI document is not available yet.
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr' }}>
          <aside style={{ borderRight: '1px solid var(--border-default)', padding: '20px 24px' }}>
            <label style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
              Find operation
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="search by path, tag, summary"
                className="of-input"
                style={{ marginTop: 12, fontSize: 13 }}
              />
            </label>

            <div className="of-scrollbar" style={{ marginTop: 16, display: 'grid', gap: 8, maxHeight: 620, overflowY: 'auto' }}>
              {filteredOperations.map((entry) => {
                const tone = methodTone(entry.method);
                const active = selectedKey === entry.key;
                return (
                  <button
                    key={entry.key}
                    type="button"
                    onClick={() => setSelectedKey(entry.key)}
                    style={{
                      width: '100%',
                      textAlign: 'left',
                      padding: 12,
                      border: `1px solid ${active ? '#10b981' : 'var(--border-default)'}`,
                      background: active ? '#ecfdf5' : '#fff',
                      borderRadius: 'var(--radius-md)',
                      cursor: 'pointer',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
                      <span style={{ ...tone, padding: '2px 8px', borderRadius: 999 }}>{entry.method}</span>
                      <span>{entry.operation.tags[0] ?? 'open_foundry'}</span>
                    </div>
                    <div style={{ marginTop: 8, fontSize: 13, fontWeight: 500, color: 'var(--text-strong)' }}>
                      {entry.path}
                    </div>
                    <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                      {entry.operation.summary}
                    </div>
                  </button>
                );
              })}
              {!filteredOperations.length && (
                <div style={{ border: '1px dashed var(--border-default)', padding: 24, fontSize: 13, color: 'var(--text-muted)' }}>
                  No operations match the current filter.
                </div>
              )}
            </div>
          </aside>

          <section style={{ padding: '20px 24px' }}>
            {selectedOperation ? (
              <>
                <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12 }}>
                  <span
                    style={{ ...methodTone(selectedOperation.method), padding: '4px 12px', borderRadius: 999, fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em' }}
                  >
                    {selectedOperation.method}
                  </span>
                  <span className="of-chip" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.18em' }}>
                    {selectedOperation.operation.operationId}
                  </span>
                </div>
                <h3 className="of-heading-lg" style={{ marginTop: 16 }}>
                  {selectedOperation.path}
                </h3>
                <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                  {selectedOperation.operation.summary}
                </p>

                <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 24 }}>
                  <div className="of-panel-muted" style={{ padding: 16 }}>
                    <p className="of-eyebrow">Request body</p>
                    {selectedOperation.operation.requestBody ? (
                      <pre style={{ marginTop: 12, overflowX: 'auto', background: '#0c0a09', color: '#f5f5f4', padding: 12, borderRadius: 'var(--radius-md)', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                        {JSON.stringify(
                          selectedOperation.operation.requestBody.content['application/json']?.schema ?? {},
                          null,
                          2,
                        )}
                      </pre>
                    ) : (
                      <div style={{ marginTop: 12, border: '1px dashed var(--border-default)', padding: 16, fontSize: 13, color: 'var(--text-muted)' }}>
                        This operation does not declare a request body.
                      </div>
                    )}
                  </div>

                  <div className="of-panel-muted" style={{ padding: 16 }}>
                    <p className="of-eyebrow">Responses</p>
                    <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                      {Object.entries(selectedOperation.operation.responses).map(([status, response]) => (
                        <div key={status} className="of-panel" style={{ padding: 12 }}>
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                            <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>HTTP {status}</div>
                            <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.18em' }}>
                              {response.description}
                            </div>
                          </div>
                          <pre style={{ marginTop: 12, overflowX: 'auto', background: '#0c0a09', color: '#f5f5f4', padding: 12, borderRadius: 'var(--radius-md)', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                            {JSON.stringify(response.content['application/json']?.schema ?? {}, null, 2)}
                          </pre>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>

                <div className="of-panel-muted" style={{ padding: 16, marginTop: 16 }}>
                  <p className="of-eyebrow">Tags</p>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                    {selectedOperation.operation.tags.map((tag) => (
                      <span key={tag} className="of-chip">
                        {tag}
                      </span>
                    ))}
                  </div>
                </div>
              </>
            ) : (
              <div style={{ border: '1px dashed var(--border-default)', padding: 32, fontSize: 13, color: 'var(--text-muted)' }}>
                Select an operation to inspect its contract.
              </div>
            )}
          </section>
        </div>
      )}
    </section>
  );
}
