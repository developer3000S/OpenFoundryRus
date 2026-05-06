import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { Pagination } from '@/lib/components/Pagination';
import { deleteDataset, getCatalogFacets, listDatasets, type Dataset } from '@/lib/api/datasets';

export function DatasetsListPage() {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [tags, setTags] = useState<{ value: string; count: number }[]>([]);
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState('');
  const [owner, setOwner] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function load() {
    setError('');
    try {
      const [res, facets] = await Promise.all([
        listDatasets({
          page,
          per_page: 20,
          search: search || undefined,
          tag: tag || undefined,
          owner_id: owner || undefined,
        }),
        getCatalogFacets().catch(() => ({ tags: [], owners: [] })),
      ]);
      setDatasets(res.data);
      setTotal(res.total);
      setTags(facets.tags ?? []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load datasets');
    }
  }

  useEffect(() => {
    void load();
  }, [page]);

  async function handleDelete(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete dataset?')) return;
    setBusy(true);
    try {
      await deleteDataset(id);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Datasets</h1>
          <p className="of-text-muted" style={{ marginTop: 4 }}>
            Browse + filter the catalog. {total} total.
          </p>
        </div>
        <Link to="/datasets/upload" className="of-button of-button--primary">Upload dataset</Link>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search…" className="of-input" style={{ width: 240 }} />
          <select value={tag} onChange={(e) => setTag(e.target.value)} className="of-input">
            <option value="">All tags</option>
            {tags.map((t) => (
              <option key={t.value} value={t.value}>{t.value} ({t.count})</option>
            ))}
          </select>
          <input value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="Owner id" className="of-input" style={{ width: 200 }} />
          <button type="button" onClick={() => { setPage(1); void load(); }} className="of-button">Apply</button>
        </div>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <ul style={{ paddingLeft: 0, listStyle: 'none' }}>
          {datasets.map((d) => (
            <li
              key={d.id}
              style={{
                padding: 12,
                borderBottom: '1px solid var(--border-default)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 8,
              }}
            >
              <div>
                <Link to={`/datasets/${d.id}`} style={{ fontWeight: 600 }}>{d.name}</Link>
                <p className="of-text-muted" style={{ fontSize: 11 }}>
                  {d.id} · {d.format ?? '?'} · tags: {d.tags.join(', ') || '—'} · {d.row_count ?? '?'} rows
                </p>
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                <Link to={`/datasets/${d.id}/branches`} className="of-button" style={{ fontSize: 11 }}>Branches</Link>
                <button type="button" onClick={() => void handleDelete(d.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                  Delete
                </button>
              </div>
            </li>
          ))}
          {datasets.length === 0 && <li className="of-text-muted">No datasets.</li>}
        </ul>
        <div style={{ marginTop: 8 }}>
          <Pagination page={page} perPage={20} total={total} onChange={setPage} />
        </div>
      </section>
    </section>
  );
}
