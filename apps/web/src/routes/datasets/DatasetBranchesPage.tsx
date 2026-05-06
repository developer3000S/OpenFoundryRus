import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { BranchGraph } from '@/lib/components/dataset/BranchGraph';
import {
  createBranchV2,
  deleteDatasetBranch,
  getDataset,
  listBranches,
  listDatasetTransactions,
  type Dataset,
  type DatasetBranch,
  type DatasetTransaction,
} from '@/lib/api/datasets';

export function DatasetBranchesPage() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [dataset, setDataset] = useState<Dataset | null>(null);
  const [branches, setBranches] = useState<DatasetBranch[]>([]);
  const [transactions, setTransactions] = useState<DatasetTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [view, setView] = useState<'graph' | 'table'>('graph');

  // create form
  const [createName, setCreateName] = useState('feature/new-branch');
  const [createFromBranch, setCreateFromBranch] = useState('');
  const [createDescription, setCreateDescription] = useState('');

  async function load() {
    if (!id) return;
    setLoading(true);
    setError('');
    try {
      const [d, br, tx] = await Promise.all([
        getDataset(id),
        listBranches(id),
        listDatasetTransactions(id).catch(() => [] as DatasetTransaction[]),
      ]);
      setDataset(d);
      setBranches(br);
      setTransactions(tx);
      if (br.length > 0 && !createFromBranch) setCreateFromBranch(br[0].name);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load branches');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [id]);

  async function handleCreate() {
    if (!id) return;
    setBusy(true);
    setError('');
    try {
      await createBranchV2(id, {
        name: createName,
        source: createFromBranch ? { from_branch: createFromBranch } : { as_root: true },
        description: createDescription || undefined,
      });
      setCreateName('');
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(branchName: string) {
    if (!id) return;
    if (typeof window !== 'undefined' && !window.confirm(`Delete branch ${branchName}?`)) return;
    setBusy(true);
    try {
      await deleteDatasetBranch(id, branchName);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  function txCount(name: string) {
    return transactions.filter((t) => (t as DatasetTransaction & { branch_name?: string }).branch_name === name).length;
  }

  const txCountByName = useMemo(() => {
    const m: Record<string, { transactions: number }> = {};
    for (const b of branches) m[b.name] = { transactions: txCount(b.name) };
    return m;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [branches, transactions]);

  function parentName(b: DatasetBranch): string {
    if (!b.parent_branch_id) return '— (root)';
    const parent = branches.find((p) => p.id === b.parent_branch_id);
    return parent?.name ?? `${b.parent_branch_id.slice(0, 8)}…`;
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to={`/datasets/${id}`} style={{ color: 'var(--text-muted)', fontSize: 13 }}>← {dataset?.name ?? 'Dataset'}</Link>
      <header>
        <h1 className="of-heading-xl">Branches{dataset ? ` · ${dataset.name}` : ''}</h1>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Create branch</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
          <input value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="branch name" className="of-input" style={{ width: 240 }} />
          <select value={createFromBranch} onChange={(e) => setCreateFromBranch(e.target.value)} className="of-input">
            <option value="">as root</option>
            {branches.map((b) => (
              <option key={b.id} value={b.name}>from {b.name}</option>
            ))}
          </select>
          <input value={createDescription} onChange={(e) => setCreateDescription(e.target.value)} placeholder="description" className="of-input" style={{ width: 280 }} />
          <button type="button" onClick={() => void handleCreate()} disabled={busy || !createName} className="of-button of-button--primary">
            Create
          </button>
        </div>
      </section>

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : view === 'graph' ? (
        <section className="of-panel" style={{ padding: 16 }}>
          <div style={{ display: 'flex', gap: 6, marginBottom: 8 }}>
            <button type="button" onClick={() => setView('table')} className="of-button" style={{ fontSize: 11 }}>
              Switch to table view
            </button>
          </div>
          <BranchGraph
            branches={branches}
            extras={txCountByName}
            onSelect={() => { /* no-op for now */ }}
            onDoubleClick={(b) => navigate(`/datasets/${id}/branches/${encodeURIComponent(b.name)}`)}
          />
        </section>
      ) : (
        <section className="of-panel" style={{ padding: 16, overflow: 'auto' }}>
          <div style={{ display: 'flex', gap: 6, marginBottom: 8 }}>
            <button type="button" onClick={() => setView('graph')} className="of-button" style={{ fontSize: 11 }}>
              Switch to graph view
            </button>
          </div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr>
                {['Name', 'Parent', 'Head tx', 'Last activity', '# tx', 'Open?', 'Fallback', ''].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {branches.map((b) => (
                <tr key={b.id} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                  <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>
                    <Link to={`/datasets/${id}/branches/${encodeURIComponent(b.name)}`}>
                      {b.name}{b.is_default ? ' ★' : ''}
                    </Link>
                  </td>
                  <td style={{ padding: 6 }}>{parentName(b)}</td>
                  <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>
                    {b.head_transaction_id ? `${b.head_transaction_id.slice(0, 8)}…` : '—'}
                  </td>
                  <td style={{ padding: 6 }}>{b.last_activity_at?.slice(0, 16) ?? '—'}</td>
                  <td style={{ padding: 6 }}>{txCount(b.name)}</td>
                  <td style={{ padding: 6 }}>{b.has_open_transaction ? 'OPEN' : '—'}</td>
                  <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>{(b.fallback_chain ?? []).join(' → ') || '—'}</td>
                  <td style={{ padding: 6, textAlign: 'right' }}>
                    <button
                      type="button"
                      onClick={() => void handleDelete(b.name)}
                      disabled={busy}
                      className="of-button"
                      style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
              {branches.length === 0 && (
                <tr><td colSpan={8} className="of-text-muted" style={{ padding: 12 }}>No branches.</td></tr>
              )}
            </tbody>
          </table>
        </section>
      )}
    </section>
  );
}
