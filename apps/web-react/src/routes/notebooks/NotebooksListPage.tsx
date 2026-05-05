import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { ConfirmDialog } from '@components/ConfirmDialog';
import {
  createNotebook,
  deleteNotebook,
  listNotebooks,
  type Notebook,
  type NotebookKernel,
} from '@/lib/api/notebooks';

export function NotebooksListPage() {
  const navigate = useNavigate();

  const [notebooks, setNotebooks] = useState<Notebook[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState('');
  const [newDescription, setNewDescription] = useState('');
  const [newKernel, setNewKernel] = useState<NotebookKernel>('python');
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const res = await listNotebooks({ page, per_page: 20, search });
      setNotebooks(res.data);
      setTotal(res.total);
    } catch {
      setNotebooks([]);
    } finally {
      setLoading(false);
    }
  }

  // Reload on page change.
  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page]);

  async function handleCreate() {
    if (!newName.trim()) return;
    const res = await createNotebook({
      name: newName,
      description: newDescription,
      default_kernel: newKernel,
    });
    navigate(`/notebooks/${res.id}`);
  }

  async function confirmDelete() {
    if (!confirmId) return;
    setConfirmBusy(true);
    try {
      await deleteNotebook(confirmId);
      await load();
    } finally {
      setConfirmId(null);
      setConfirmBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16, maxWidth: 1024, margin: '0 auto', width: '100%' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16 }}>
        <h1 className="of-heading-xl">Notebooks</h1>
        <button type="button" className="of-btn of-btn-primary" onClick={() => setShowCreate((v) => !v)}>
          + New notebook
        </button>
      </header>

      {showCreate && (
        <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 10 }}>
          <input
            className="of-input"
            placeholder="Name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
          <input
            className="of-input"
            placeholder="Description"
            value={newDescription}
            onChange={(e) => setNewDescription(e.target.value)}
          />
          <select
            className="of-select"
            value={newKernel}
            onChange={(e) => setNewKernel(e.target.value as NotebookKernel)}
            style={{ width: 'auto' }}
          >
            <option value="python">Python</option>
            <option value="sql">SQL</option>
            <option value="llm">LLM</option>
            <option value="r">R</option>
          </select>
          <button type="button" className="of-btn of-btn-primary" onClick={handleCreate} style={{ justifySelf: 'start' }}>
            Create
          </button>
        </div>
      )}

      <input
        className="of-input"
        placeholder="Search notebooks…"
        value={search}
        onChange={(e) => {
          setSearch(e.target.value);
          setPage(1);
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') void load();
        }}
      />

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : notebooks.length === 0 ? (
        <p className="of-text-muted">No notebooks found.</p>
      ) : (
        <div style={{ display: 'grid', gap: 8 }}>
          {notebooks.map((nb) => (
            <div
              key={nb.id}
              className="of-card"
              style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}
            >
              <Link to={`/notebooks/${nb.id}`} style={{ flex: 1, textDecoration: 'none' }}>
                <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{nb.name}</div>
                <div className="of-text-muted" style={{ fontSize: 13 }}>
                  {nb.description || 'No description'} · {nb.default_kernel} ·{' '}
                  {new Date(nb.updated_at).toLocaleDateString()}
                </div>
              </Link>
              <button
                type="button"
                className="of-btn of-btn-danger"
                onClick={() => setConfirmId(nb.id)}
                style={{ minHeight: 32, fontSize: 12 }}
              >
                Delete
              </button>
            </div>
          ))}
        </div>
      )}

      {total > 20 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            type="button"
            className="of-btn"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Prev
          </button>
          <span style={{ padding: '0 8px', fontSize: 13 }}>Page {page}</span>
          <button
            type="button"
            className="of-btn"
            disabled={page * 20 >= total}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </button>
        </div>
      )}

      <ConfirmDialog
        open={confirmId !== null}
        title="Delete notebook"
        message="This permanently removes the notebook and its history. Continue?"
        confirmLabel="Delete"
        danger
        busy={confirmBusy}
        onConfirm={confirmDelete}
        onCancel={() => setConfirmId(null)}
      />
    </section>
  );
}
