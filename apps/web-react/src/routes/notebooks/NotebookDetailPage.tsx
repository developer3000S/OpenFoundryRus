import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { MonacoEditor } from '@components/MonacoEditor';
import { CellOutput } from '@/lib/components/notebook/CellOutput';
import { KernelSelector } from '@/lib/components/notebook/KernelSelector';
import {
  addCell,
  createSession,
  deleteCell,
  deleteWorkspaceFile,
  executeCell,
  getNotebook,
  listSessions,
  listWorkspaceFiles,
  stopSession,
  updateCell,
  updateNotebook,
  upsertWorkspaceFile,
  type Cell,
  type CellOutput as NotebookCellOutput,
  type Notebook,
  type NotebookKernel,
  type NotebookWorkspaceFile,
  type Session,
} from '@/lib/api/notebooks';

function kernelKey(kernel: string): NotebookKernel {
  if (kernel === 'sql' || kernel === 'llm' || kernel === 'r') return kernel;
  return 'python';
}

function workspaceEditorLanguage(file: NotebookWorkspaceFile | null) {
  if (!file) return 'text';
  const supported = ['markdown', 'typescript', 'javascript', 'json', 'python', 'sql', 'r', 'toml'];
  return supported.includes(file.language) ? file.language : 'text';
}

export function NotebookDetailPage() {
  const { id } = useParams<{ id: string }>();
  const notebookId = id ?? '';

  const [notebook, setNotebook] = useState<Notebook | null>(null);
  const [cells, setCells] = useState<Cell[]>([]);
  const [outputs, setOutputs] = useState<Record<string, NotebookCellOutput>>({});
  const [executing, setExecuting] = useState<Record<string, boolean>>({});
  const [sessionsByKernel, setSessionsByKernel] = useState<Record<NotebookKernel, Session | null>>({
    python: null,
    sql: null,
    llm: null,
    r: null,
  });
  const [activeKernel, setActiveKernel] = useState<NotebookKernel>('python');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [workspaceFiles, setWorkspaceFiles] = useState<NotebookWorkspaceFile[]>([]);
  const [loadingWorkspace, setLoadingWorkspace] = useState(true);
  const [selectedWorkspaceFilePath, setSelectedWorkspaceFilePath] = useState('');
  const [newWorkspaceFilePath, setNewWorkspaceFilePath] = useState('');
  const [savingWorkspaceFile, setSavingWorkspaceFile] = useState<Record<string, boolean>>({});

  function upsertCell(nextCell: Cell) {
    setCells((prev) =>
      prev
        .map((cell) => (cell.id === nextCell.id ? nextCell : cell))
        .sort((a, b) => a.position - b.position),
    );
  }

  function updateSession(kernel: NotebookKernel, session: Session | null) {
    setSessionsByKernel((prev) => ({ ...prev, [kernel]: session }));
  }

  function syncWorkspaceSelection(files: NotebookWorkspaceFile[], current: string) {
    if (files.length === 0) return '';
    if (!files.some((file) => file.path === current)) return files[0].path;
    return current;
  }

  async function loadSessionsForNotebook() {
    const res = await listSessions(notebookId);
    const next: Record<NotebookKernel, Session | null> = { python: null, sql: null, llm: null, r: null };
    for (const session of res.data) {
      const key = kernelKey(session.kernel);
      if (!next[key]) next[key] = session;
    }
    setSessionsByKernel(next);
  }

  async function loadWorkspace() {
    setLoadingWorkspace(true);
    try {
      const res = await listWorkspaceFiles(notebookId);
      setWorkspaceFiles(res.data);
      setSelectedWorkspaceFilePath((current) => syncWorkspaceSelection(res.data, current));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load notebook workspace');
      setWorkspaceFiles([]);
    } finally {
      setLoadingWorkspace(false);
    }
  }

  async function load() {
    setLoading(true);
    setError('');
    try {
      const [res] = await Promise.all([getNotebook(notebookId), loadWorkspace()]);
      setNotebook(res.notebook);
      const sortedCells = [...res.cells].sort((a, b) => a.position - b.position);
      setCells(sortedCells);
      const initialOutputs: Record<string, NotebookCellOutput> = {};
      for (const cell of sortedCells) {
        if (cell.last_output) initialOutputs[cell.id] = cell.last_output;
      }
      setOutputs(initialOutputs);
      setActiveKernel(kernelKey(res.notebook.default_kernel));
      await loadSessionsForNotebook();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load notebook');
      setNotebook(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (notebookId) void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [notebookId]);

  async function ensureSession(kernel: NotebookKernel): Promise<Session> {
    const existing = sessionsByKernel[kernel];
    if (existing && existing.status !== 'dead') return existing;
    const session = await createSession(notebookId, kernel);
    updateSession(kernel, session);
    return session;
  }

  async function handleKernelChange(kernel: NotebookKernel) {
    setActiveKernel(kernel);
    if (!notebook || notebook.default_kernel === kernel) return;
    const next = await updateNotebook(notebookId, { default_kernel: kernel });
    setNotebook(next);
  }

  async function handleStartSession() {
    await ensureSession(activeKernel);
  }

  async function handleStopSession() {
    const current = sessionsByKernel[activeKernel];
    if (!current) return;
    const stopped = await stopSession(notebookId, current.id);
    updateSession(activeKernel, stopped);
  }

  async function handleAddCell(type: 'code' | 'markdown') {
    const cell = await addCell(notebookId, {
      cell_type: type,
      kernel: type === 'code' ? activeKernel : undefined,
      source: '',
    });
    setCells((prev) => [...prev, cell].sort((a, b) => a.position - b.position));
  }

  function handleSourceChange(cellId: string, source: string) {
    setCells((prev) => prev.map((cell) => (cell.id === cellId ? { ...cell, source } : cell)));
  }

  async function handlePersistSource(cellId: string, source: string) {
    const updated = await updateCell(notebookId, cellId, { source });
    upsertCell(updated);
  }

  async function handleCellKernelChange(cellId: string, kernel: NotebookKernel) {
    const updated = await updateCell(notebookId, cellId, { kernel });
    upsertCell(updated);
  }

  async function handleDeleteCell(cellId: string) {
    await deleteCell(notebookId, cellId);
    setCells((prev) => prev.filter((cell) => cell.id !== cellId));
  }

  async function handleExecute(cellId: string) {
    const cell = cells.find((entry) => entry.id === cellId);
    if (!cell || cell.cell_type !== 'code') return;

    const key = kernelKey(cell.kernel);
    setExecuting((prev) => ({ ...prev, [cellId]: true }));

    try {
      const session = await ensureSession(key);
      updateSession(key, { ...session, status: 'busy' });

      const output = await executeCell(notebookId, cellId, session.id);
      setOutputs((prev) => ({ ...prev, [cellId]: output }));
      setCells((prev) =>
        prev.map((entry) =>
          entry.id === cellId
            ? { ...entry, execution_count: output.execution_count, last_output: output }
            : entry,
        ),
      );
      updateSession(key, {
        ...(sessionsByKernel[key] ?? session),
        status: 'idle',
        last_activity: new Date().toISOString(),
      });
    } catch (cause) {
      setOutputs((prev) => ({
        ...prev,
        [cellId]: {
          output_type: 'error',
          content: cause instanceof Error ? cause.message : 'Execution failed',
          execution_count: (cell.execution_count ?? 0) + 1,
        },
      }));
      const session = sessionsByKernel[key];
      if (session) updateSession(key, { ...session, status: 'idle' });
    } finally {
      setExecuting((prev) => ({ ...prev, [cellId]: false }));
    }
  }

  async function handleRunAll() {
    for (const cell of cells) {
      if (cell.cell_type === 'code') {
        // Sequential to mirror the Svelte implementation; parallel would
        // race the shared kernel session.
        await handleExecute(cell.id);
      }
    }
  }

  async function addWorkspaceFile() {
    const path = newWorkspaceFilePath.trim();
    if (!path) return;
    if (workspaceFiles.some((file) => file.path === path)) {
      setError('That workspace file already exists.');
      return;
    }
    const file = await upsertWorkspaceFile(notebookId, { path, content: '' });
    const next = [...workspaceFiles, file].sort((a, b) => a.path.localeCompare(b.path));
    setWorkspaceFiles(next);
    setSelectedWorkspaceFilePath(file.path);
    setNewWorkspaceFilePath('');
    setError('');
  }

  function handleWorkspaceContentChange(path: string, content: string) {
    setWorkspaceFiles((prev) => prev.map((file) => (file.path === path ? { ...file, content } : file)));
  }

  async function persistWorkspaceFile(path: string, content: string) {
    setSavingWorkspaceFile((prev) => ({ ...prev, [path]: true }));
    try {
      const file = await upsertWorkspaceFile(notebookId, { path, content });
      setWorkspaceFiles((prev) =>
        prev.map((entry) => (entry.path === path ? file : entry)).sort((a, b) => a.path.localeCompare(b.path)),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to persist workspace file');
    } finally {
      setSavingWorkspaceFile((prev) => ({ ...prev, [path]: false }));
    }
  }

  async function removeWorkspaceFile(path: string) {
    await deleteWorkspaceFile(notebookId, path);
    const next = workspaceFiles.filter((file) => file.path !== path);
    setWorkspaceFiles(next);
    setSelectedWorkspaceFilePath((current) => syncWorkspaceSelection(next, current));
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 80, textAlign: 'center', color: 'var(--text-muted)' }}>
        Loading…
      </section>
    );
  }

  if (!notebook) {
    return (
      <section className="of-page" style={{ padding: 80, textAlign: 'center', color: 'var(--status-danger)' }}>
        Notebook not found.
      </section>
    );
  }

  const selectedWorkspaceFile = workspaceFiles.find((f) => f.path === selectedWorkspaceFilePath) ?? null;

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div style={{ maxWidth: 720 }}>
          <p className="of-eyebrow">Code workbook</p>
          <h1 className="of-heading-xl" style={{ marginTop: 4 }}>
            {notebook.name}
          </h1>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
            {notebook.description || 'No description'}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="of-btn of-btn-primary" onClick={() => void handleRunAll()}>
            ▶ Run all
          </button>
          <Link to="/notebooks" className="of-btn">
            Back
          </Link>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div className="of-panel-muted" style={{ padding: 16 }}>
        <KernelSelector
          value={activeKernel}
          status={sessionsByKernel[activeKernel]?.status ?? null}
          onChange={(k) => void handleKernelChange(k)}
          onStart={() => void handleStartSession()}
          onStop={() => void handleStopSession()}
        />
        <div style={{ marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 6, fontSize: 11 }}>
          <span className="of-chip">Default kernel: {activeKernel}</span>
          <span className="of-chip">Available: Python, SQL, LLM, R</span>
          <span className="of-chip">{workspaceFiles.length} workspace file(s)</span>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 340px' }}>
        <div style={{ display: 'grid', gap: 12 }}>
          {cells.map((cell) => (
            <div
              key={cell.id}
              style={{
                overflow: 'hidden',
                background: '#fff',
                border: '1px solid var(--border-default)',
                borderRadius: 'var(--radius-md)',
                boxShadow: 'var(--shadow-panel)',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  borderBottom: '1px solid var(--border-default)',
                  background: 'var(--bg-panel-muted)',
                  padding: '8px 12px',
                  fontSize: 12,
                }}
              >
                <span style={{ color: 'var(--text-soft)' }}>In [{cell.execution_count ?? ' '}]</span>
                <span style={{ fontSize: 11, color: 'var(--text-soft)' }}>{cell.cell_type}</span>

                {cell.cell_type === 'code' ? (
                  <select
                    className="of-select"
                    value={kernelKey(cell.kernel)}
                    onChange={(e) =>
                      void handleCellKernelChange(cell.id, e.target.value as NotebookKernel)
                    }
                    style={{ width: 'auto', minHeight: 26, padding: '0 6px', fontSize: 11, fontFamily: 'var(--font-mono)' }}
                  >
                    <option value="python">python</option>
                    <option value="sql">sql</option>
                    <option value="llm">llm</option>
                    <option value="r">r</option>
                  </select>
                ) : (
                  <span
                    style={{
                      padding: '2px 6px',
                      background: '#e2e8f0',
                      borderRadius: 'var(--radius-sm)',
                      fontSize: 11,
                      fontFamily: 'var(--font-mono)',
                      color: 'var(--text-default)',
                    }}
                  >
                    markdown
                  </span>
                )}

                <div style={{ flex: 1 }} />

                {cell.cell_type === 'code' && (
                  <button
                    type="button"
                    onClick={() => void handleExecute(cell.id)}
                    disabled={executing[cell.id]}
                    style={{
                      background: 'transparent',
                      border: 0,
                      color: 'var(--status-success)',
                      cursor: 'pointer',
                      fontSize: 14,
                    }}
                  >
                    {executing[cell.id] ? '⏳' : '▶'}
                  </button>
                )}

                <button
                  type="button"
                  onClick={() => void handleDeleteCell(cell.id)}
                  style={{
                    background: 'transparent',
                    border: 0,
                    color: 'var(--status-danger)',
                    cursor: 'pointer',
                    fontSize: 14,
                  }}
                >
                  ✕
                </button>
              </div>

              <MonacoEditor
                value={cell.source}
                language={
                  cell.cell_type === 'markdown'
                    ? 'markdown'
                    : kernelKey(cell.kernel) === 'llm'
                      ? 'markdown'
                      : kernelKey(cell.kernel)
                }
                minHeight={cell.cell_type === 'markdown' ? 120 : 180}
                onChange={(source) => handleSourceChange(cell.id, source)}
                onBlur={(source) => void handlePersistSource(cell.id, source)}
              />

              <CellOutput output={outputs[cell.id] ?? cell.last_output} />
            </div>
          ))}

          <div style={{ display: 'flex', gap: 8, marginTop: 4 }}>
            <button type="button" className="of-btn" onClick={() => void handleAddCell('code')}>
              + Code cell
            </button>
            <button type="button" className="of-btn" onClick={() => void handleAddCell('markdown')}>
              + Markdown cell
            </button>
          </div>
        </div>

        <aside style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Workspace</p>
            <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
              Persist helper files, prompts, scripts, and notes next to the notebook.
            </p>

            <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
              <input
                className="of-input"
                value={newWorkspaceFilePath}
                onChange={(e) => setNewWorkspaceFilePath(e.target.value)}
                placeholder="prompts/system.md"
                style={{ minWidth: 0, flex: 1 }}
              />
              <button type="button" className="of-btn" onClick={() => void addWorkspaceFile()}>
                Add
              </button>
            </div>

            {loadingWorkspace ? (
              <div style={{ marginTop: 12, fontSize: 13, color: 'var(--text-muted)' }}>
                Loading workspace…
              </div>
            ) : workspaceFiles.length === 0 ? (
              <div
                style={{
                  marginTop: 12,
                  padding: 24,
                  border: '1px dashed var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  textAlign: 'center',
                  fontSize: 13,
                  color: 'var(--text-muted)',
                }}
              >
                No workspace files yet.
              </div>
            ) : (
              <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                <div style={{ display: 'grid', gap: 6 }}>
                  {workspaceFiles.map((file) => (
                    <button
                      key={file.path}
                      type="button"
                      onClick={() => setSelectedWorkspaceFilePath(file.path)}
                      style={{
                        textAlign: 'left',
                        padding: 12,
                        background: selectedWorkspaceFilePath === file.path ? '#ecfeff' : '#fff',
                        border: `1px solid ${
                          selectedWorkspaceFilePath === file.path ? '#06b6d4' : 'var(--border-default)'
                        }`,
                        borderRadius: 'var(--radius-md)',
                        fontSize: 13,
                        cursor: 'pointer',
                      }}
                    >
                      <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{file.path}</div>
                      <div style={{ marginTop: 4, fontSize: 11, color: 'var(--text-muted)' }}>
                        {file.language} · {file.size_bytes} bytes
                      </div>
                    </button>
                  ))}
                </div>

                {selectedWorkspaceFile && (
                  <div style={{ display: 'grid', gap: 8 }}>
                    <div
                      style={{
                        display: 'flex',
                        flexWrap: 'wrap',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        gap: 8,
                      }}
                    >
                      <div className="of-chip">{selectedWorkspaceFile.path}</div>
                      <button
                        type="button"
                        className="of-btn of-btn-danger"
                        onClick={() => void removeWorkspaceFile(selectedWorkspaceFile.path)}
                        style={{ minHeight: 28, fontSize: 11 }}
                      >
                        Remove file
                      </button>
                    </div>

                    <MonacoEditor
                      value={selectedWorkspaceFile.content ?? ''}
                      language={workspaceEditorLanguage(selectedWorkspaceFile)}
                      minHeight={360}
                      onChange={(content) =>
                        handleWorkspaceContentChange(selectedWorkspaceFile.path, content)
                      }
                      onBlur={(content) =>
                        void persistWorkspaceFile(selectedWorkspaceFile.path, content)
                      }
                    />

                    <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                      {savingWorkspaceFile[selectedWorkspaceFile.path]
                        ? 'Saving…'
                        : `Updated ${new Date(selectedWorkspaceFile.updated_at).toLocaleString()}`}
                    </div>
                  </div>
                )}
              </div>
            )}
          </section>
        </aside>
      </div>
    </section>
  );
}
