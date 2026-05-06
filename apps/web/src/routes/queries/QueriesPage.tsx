import { useEffect, useMemo, useState } from 'react';

import {
  createSavedQuery,
  deleteSavedQuery,
  executeQuery,
  explainQuery,
  listSavedQueries,
  type QueryResult,
  type SavedQuery,
} from '@/lib/api/queries';
import { listObjectTypes, listProperties, type ObjectType, type Property } from '@/lib/api/ontology';

const DEFAULT_SQL = 'SELECT *\nFROM `[Example]`';

export function QueriesPage() {
  const [sql, setSql] = useState(DEFAULT_SQL);
  const [result, setResult] = useState<QueryResult | null>(null);
  const [error, setError] = useState('');
  const [executing, setExecuting] = useState(false);
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  const [activeTab, setActiveTab] = useState<'results' | 'saved'>('results');
  const [showSaveDialog, setShowSaveDialog] = useState(false);
  const [saveName, setSaveName] = useState('');

  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [selectedTypeId, setSelectedTypeId] = useState('');
  const [selectedType, setSelectedType] = useState<ObjectType | null>(null);
  const [properties, setProperties] = useState<Property[]>([]);
  const [objectFilter, setObjectFilter] = useState('');
  const [propertyFilter, setPropertyFilter] = useState('');
  const [loadingCatalog, setLoadingCatalog] = useState(true);

  // Initial load: saved queries + ontology catalog.
  useEffect(() => {
    void (async () => {
      try {
        const res = await listSavedQueries();
        setSavedQueries(res.data);
      } catch {
        // ignore
      }
    })();

    void (async () => {
      setLoadingCatalog(true);
      try {
        const res = await listObjectTypes({ per_page: 100 });
        setObjectTypes(res.data);
        if (res.data.length > 0) setSelectedTypeId((current) => current || res.data[0].id);
      } finally {
        setLoadingCatalog(false);
      }
    })();
  }, []);

  // Load properties whenever the active type changes.
  useEffect(() => {
    if (!selectedTypeId) {
      setSelectedType(null);
      setProperties([]);
      return;
    }
    setSelectedType(objectTypes.find((entry) => entry.id === selectedTypeId) ?? null);
    let cancelled = false;
    (async () => {
      try {
        const next = await listProperties(selectedTypeId);
        if (!cancelled) setProperties(next);
      } catch {
        if (!cancelled) setProperties([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [selectedTypeId, objectTypes]);

  async function handleExecute() {
    setError('');
    setResult(null);
    setExecuting(true);
    try {
      const next = await executeQuery(sql, 1000);
      setResult(next);
      setActiveTab('results');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Query failed');
    } finally {
      setExecuting(false);
    }
  }

  async function handleExplain() {
    setError('');
    setExecuting(true);
    try {
      const plan = await explainQuery(sql);
      setResult({
        columns: [{ name: 'plan', data_type: 'Utf8' }],
        rows: [[plan.logical_plan], ['---'], [plan.physical_plan]],
        total_rows: 3,
        execution_time_ms: 0,
      });
      setActiveTab('results');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Explain failed');
    } finally {
      setExecuting(false);
    }
  }

  async function handleSave() {
    if (!saveName.trim()) return;
    try {
      await createSavedQuery({ name: saveName, sql });
      setShowSaveDialog(false);
      setSaveName('');
      const res = await listSavedQueries();
      setSavedQueries(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    }
  }

  async function handleDeleteSaved(id: string) {
    await deleteSavedQuery(id);
    const res = await listSavedQueries();
    setSavedQueries(res.data);
  }

  function loadQuery(q: SavedQuery) {
    setSql(q.sql);
    setActiveTab('results');
  }

  function handleKeydown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      void handleExecute();
    }
  }

  function insertText(text: string) {
    setSql((current) => `${current.trimEnd()}\n${text}`);
  }

  const filteredTypes = useMemo(() => {
    const query = objectFilter.trim().toLowerCase();
    if (!query) return objectTypes;
    return objectTypes.filter((item) =>
      `${item.display_name} ${item.name} ${item.description}`.toLowerCase().includes(query),
    );
  }, [objectFilter, objectTypes]);

  const filteredProperties = useMemo(() => {
    const query = propertyFilter.trim().toLowerCase();
    if (!query) return properties;
    return properties.filter((item) =>
      `${item.display_name} ${item.name} ${item.property_type}`.toLowerCase().includes(query),
    );
  }, [propertyFilter, properties]);

  return (
    <div style={{ display: 'grid', gap: 20 }}>
      <section className="of-panel" style={{ overflow: 'hidden' }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid var(--border-subtle)',
            padding: '16px 20px',
            gap: 12,
          }}
        >
          <div>
            <div className="of-heading-lg">Queries</div>
            <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
              Run SQL-style searches against the platform datasets and inspect structured results.
            </div>
          </div>

          <div style={{ display: 'flex', gap: 8 }}>
            <button
              type="button"
              className="of-btn"
              onClick={() => {
                setSql(DEFAULT_SQL);
                setResult(null);
                setError('');
              }}
            >
              Reset
            </button>
            <button type="button" className="of-btn" onClick={() => setShowSaveDialog(true)}>
              Save
            </button>
            <button
              type="button"
              className="of-btn"
              onClick={() => void handleExplain()}
              disabled={executing || !sql.trim()}
            >
              Explain
            </button>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={() => void handleExecute()}
              disabled={executing || !sql.trim()}
            >
              {executing ? 'Running…' : 'Run'}
            </button>
          </div>
        </div>

        <div style={{ display: 'grid', gap: 0, gridTemplateColumns: '420px minmax(0, 1fr)' }}>
          <aside style={{ borderRight: '1px solid var(--border-subtle)', background: '#fbfcfe' }}>
            <div style={{ borderBottom: '1px solid var(--border-subtle)', padding: '12px 16px' }}>
              <input
                type="text"
                className="of-input"
                placeholder="Filter object types…"
                value={objectFilter}
                onChange={(e) => setObjectFilter(e.target.value)}
                style={{ minHeight: 32, fontSize: 13 }}
              />
            </div>

            <div style={{ display: 'grid', minHeight: 640, gridTemplateColumns: '220px minmax(0, 1fr)' }}>
              <div style={{ borderRight: '1px solid var(--border-subtle)', padding: 12 }}>
                <div className="of-heading-sm" style={{ marginBottom: 12 }}>
                  Recently used
                </div>
                <div style={{ display: 'grid', gap: 4 }}>
                  {loadingCatalog ? (
                    <div style={{ padding: '32px 8px', fontSize: 13, color: 'var(--text-muted)' }}>
                      Loading types…
                    </div>
                  ) : (
                    filteredTypes.map((item) => (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => setSelectedTypeId(item.id)}
                        style={{
                          display: 'flex',
                          width: '100%',
                          alignItems: 'flex-start',
                          gap: 8,
                          borderRadius: 4,
                          padding: '8px',
                          textAlign: 'left',
                          background: selectedTypeId === item.id ? '#dce8fb' : 'transparent',
                          border: 0,
                          cursor: 'pointer',
                        }}
                      >
                        <span className="of-icon-box" style={{ width: 32, height: 32, flexShrink: 0 }}>
                          ◆
                        </span>
                        <span style={{ minWidth: 0, display: 'block' }}>
                          <span
                            style={{
                              display: 'block',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                              fontSize: 14,
                              color: 'var(--text-strong)',
                            }}
                          >
                            {item.display_name}
                          </span>
                          <span
                            style={{
                              display: 'block',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                              fontSize: 12,
                              color: 'var(--text-muted)',
                            }}
                          >
                            {item.name}
                          </span>
                        </span>
                      </button>
                    ))
                  )}
                </div>
              </div>

              <div style={{ padding: 16 }}>
                {selectedType ? (
                  <>
                    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16 }}>
                      <span
                        className="of-icon-box"
                        style={{ width: 44, height: 44, flexShrink: 0, fontSize: 20 }}
                      >
                        ◆
                      </span>
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-strong)' }}>
                          {selectedType.display_name}
                        </div>
                        <div className="of-text-muted" style={{ fontSize: 13 }}>
                          {selectedType.description || 'No description'}
                        </div>
                        <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          Ontology • {properties.length} properties
                        </div>
                      </div>
                    </div>

                    <div
                      style={{
                        marginTop: 16,
                        border: '1px solid var(--border-default)',
                        borderRadius: 'var(--radius-md)',
                      }}
                    >
                      <div
                        style={{
                          borderBottom: '1px solid var(--border-subtle)',
                          padding: '12px 16px',
                          display: 'flex',
                          alignItems: 'center',
                          gap: 12,
                        }}
                      >
                        <div className="of-heading-sm" style={{ flex: 1 }}>
                          Properties ({properties.length})
                        </div>
                        <input
                          type="text"
                          className="of-input"
                          placeholder="Filter…"
                          value={propertyFilter}
                          onChange={(e) => setPropertyFilter(e.target.value)}
                          style={{ minHeight: 28, fontSize: 12, width: 160 }}
                        />
                      </div>
                      <div className="of-scrollbar" style={{ maxHeight: 420, overflow: 'auto' }}>
                        {filteredProperties.map((property) => (
                          <button
                            key={property.id}
                            type="button"
                            onClick={() =>
                              insertText(
                                `-- ${property.display_name}\nSELECT ${property.name}\nFROM \`${selectedType?.display_name ?? 'ObjectType'}\`;`,
                              )
                            }
                            style={{
                              display: 'flex',
                              width: '100%',
                              alignItems: 'center',
                              justifyContent: 'space-between',
                              borderBottom: '1px solid var(--border-subtle)',
                              padding: '12px 16px',
                              textAlign: 'left',
                              background: 'transparent',
                              border: 0,
                              cursor: 'pointer',
                            }}
                          >
                            <span style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                              <span
                                style={{
                                  borderRadius: 4,
                                  border: '1px solid var(--border-default)',
                                  background: '#fff',
                                  padding: '2px 6px',
                                  fontSize: 10,
                                  fontWeight: 600,
                                  color: 'var(--text-muted)',
                                }}
                              >
                                {property.property_type}
                              </span>
                              <span>
                                <span style={{ display: 'block', fontSize: 14, color: 'var(--text-strong)' }}>
                                  {property.display_name}
                                </span>
                                <span style={{ display: 'block', fontSize: 12, color: 'var(--text-muted)' }}>
                                  {property.name}
                                </span>
                              </span>
                            </span>
                            <span style={{ color: 'var(--text-muted)', fontSize: 14 }}>+</span>
                          </button>
                        ))}
                      </div>
                    </div>
                  </>
                ) : (
                  <div className="of-text-muted" style={{ padding: '48px 8px', fontSize: 13 }}>
                    Select an object type to inspect its properties.
                  </div>
                )}
              </div>
            </div>
          </aside>

          <div style={{ minWidth: 0 }}>
            <div style={{ borderBottom: '1px solid var(--border-subtle)', padding: '12px 20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div className="of-tabbar" style={{ borderBottom: 0 }}>
                  <button
                    type="button"
                    className={`of-tab ${activeTab === 'results' ? 'of-tab-active' : ''}`}
                    onClick={() => setActiveTab('results')}
                  >
                    Code
                  </button>
                  <button
                    type="button"
                    className={`of-tab ${activeTab === 'saved' ? 'of-tab-active' : ''}`}
                    onClick={() => setActiveTab('saved')}
                  >
                    History <span className="of-badge" style={{ marginLeft: 8 }}>{savedQueries.length}</span>
                  </button>
                </div>
                <div className="of-text-muted" style={{ fontSize: 11 }}>
                  Run with ⌘/Ctrl + Enter
                </div>
              </div>
            </div>

            <div style={{ padding: 20 }}>
              <div
                style={{
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid var(--border-default)',
                  background: '#fff',
                }}
              >
                <textarea
                  value={sql}
                  onChange={(e) => setSql(e.target.value)}
                  onKeyDown={handleKeydown}
                  rows={10}
                  placeholder="Enter SQL query…"
                  spellCheck={false}
                  className="of-textarea"
                  style={{
                    minHeight: 220,
                    border: 0,
                    background: '#fbfcfe',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 16,
                    lineHeight: 1.6,
                  }}
                />
              </div>

              {showSaveDialog && (
                <div
                  style={{
                    marginTop: 16,
                    display: 'flex',
                    gap: 8,
                    borderRadius: 'var(--radius-md)',
                    border: '1px solid var(--border-default)',
                    background: '#fbfcfe',
                    padding: 12,
                  }}
                >
                  <input
                    type="text"
                    className="of-input"
                    placeholder="Query name…"
                    value={saveName}
                    onChange={(e) => setSaveName(e.target.value)}
                    style={{ flex: 1 }}
                  />
                  <button type="button" className="of-btn of-btn-primary" onClick={() => void handleSave()}>
                    Save
                  </button>
                  <button type="button" className="of-btn" onClick={() => setShowSaveDialog(false)}>
                    Cancel
                  </button>
                </div>
              )}

              {error && (
                <div
                  style={{
                    marginTop: 16,
                    borderRadius: 'var(--radius-md)',
                    border: '1px solid #efc1c1',
                    background: '#fff3f3',
                    padding: '12px 16px',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 13,
                    color: 'var(--status-danger)',
                  }}
                >
                  {error}
                </div>
              )}

              {activeTab === 'results' ? (
                result ? (
                  <>
                    <div
                      className="of-text-muted"
                      style={{
                        marginTop: 16,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        fontSize: 13,
                      }}
                    >
                      <span>
                        {result.total_rows} rows in {result.execution_time_ms}ms
                      </span>
                      <span>{result.columns.length} columns</span>
                    </div>
                    <div
                      className="of-scrollbar"
                      style={{
                        marginTop: 12,
                        overflow: 'auto',
                        borderRadius: 'var(--radius-md)',
                        border: '1px solid var(--border-default)',
                      }}
                    >
                      <table className="of-table">
                        <thead>
                          <tr>
                            {result.columns.map((col) => (
                              <th key={col.name}>
                                {col.name}
                                <span
                                  style={{
                                    marginLeft: 4,
                                    fontWeight: 400,
                                    fontSize: 10,
                                    textTransform: 'none',
                                    color: 'var(--text-soft)',
                                  }}
                                >
                                  {col.data_type}
                                </span>
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {result.rows.map((row, rowIndex) => (
                            <tr key={rowIndex}>
                              {row.map((cell, cellIndex) => (
                                <td
                                  key={cellIndex}
                                  style={{ fontFamily: 'var(--font-mono)', fontSize: 13 }}
                                >
                                  {cell}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : !executing ? (
                  <div
                    style={{
                      marginTop: 20,
                      borderRadius: 'var(--radius-md)',
                      border: '1px dashed var(--border-default)',
                      padding: '48px 16px',
                      textAlign: 'center',
                      fontSize: 13,
                      color: 'var(--text-muted)',
                    }}
                  >
                    Run a query to see results.
                  </div>
                ) : null
              ) : (
                <div style={{ marginTop: 16, display: 'grid', gap: 8 }}>
                  {savedQueries.map((q) => (
                    <div
                      key={q.id}
                      style={{
                        borderRadius: 'var(--radius-md)',
                        border: '1px solid var(--border-default)',
                        background: '#fff',
                        padding: 12,
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                        <button
                          type="button"
                          onClick={() => loadQuery(q)}
                          style={{
                            minWidth: 0,
                            flex: 1,
                            textAlign: 'left',
                            background: 'transparent',
                            border: 0,
                            cursor: 'pointer',
                            padding: 0,
                          }}
                        >
                          <div style={{ fontSize: 14, fontWeight: 500, color: 'var(--text-strong)' }}>
                            {q.name}
                          </div>
                          <pre
                            style={{
                              marginTop: 4,
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              whiteSpace: 'nowrap',
                              fontSize: 12,
                              color: 'var(--text-muted)',
                            }}
                          >
                            {q.sql}
                          </pre>
                        </button>
                        <button
                          type="button"
                          onClick={() => void handleDeleteSaved(q.id)}
                          style={{
                            background: 'transparent',
                            border: 0,
                            color: 'var(--status-danger)',
                            cursor: 'pointer',
                            fontSize: 13,
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  ))}
                  {savedQueries.length === 0 && (
                    <div
                      style={{
                        borderRadius: 'var(--radius-md)',
                        border: '1px dashed var(--border-default)',
                        padding: '48px 16px',
                        textAlign: 'center',
                        fontSize: 13,
                        color: 'var(--text-muted)',
                      }}
                    >
                      No saved queries yet.
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
