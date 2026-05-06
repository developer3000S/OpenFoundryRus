import { useEffect, useState } from 'react';

import {
  addGlobalBranchLink,
  createGlobalBranch,
  getGlobalBranch,
  listGlobalBranchResources,
  listGlobalBranches,
  promoteGlobalBranch,
  type GlobalBranch,
  type GlobalBranchLink,
  type GlobalBranchSummary,
} from '@/lib/api/global-branches';
import { ApiError } from '@/lib/api/client';

function statusTone(status: GlobalBranchLink['status']): string {
  if (status === 'in_sync') return 'of-status-success';
  if (status === 'drifted') return 'of-status-warning';
  return 'of-status-danger';
}

export function GlobalBranchingPage() {
  const [branches, setBranches] = useState<GlobalBranch[]>([]);
  const [selected, setSelected] = useState<GlobalBranchSummary | null>(null);
  const [resources, setResources] = useState<GlobalBranchLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [createName, setCreateName] = useState('');
  const [createDescription, setCreateDescription] = useState('');
  const [creating, setCreating] = useState(false);

  const [linkResourceType, setLinkResourceType] = useState('dataset');
  const [linkResourceRid, setLinkResourceRid] = useState('');
  const [linkBranchRid, setLinkBranchRid] = useState('');
  const [linking, setLinking] = useState(false);

  const [promoting, setPromoting] = useState(false);
  const [promoteResult, setPromoteResult] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      const next = await listGlobalBranches();
      setBranches(next);
      if (next.length > 0 && !selected) {
        await selectById(next[0].id);
      }
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Failed to load global branches');
    } finally {
      setLoading(false);
    }
  }

  async function selectById(id: string) {
    try {
      const summary = await getGlobalBranch(id);
      setSelected(summary);
      setResources(await listGlobalBranchResources(id));
      setPromoteResult(null);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Failed to load branch detail');
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submitCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    setError('');
    try {
      const created = await createGlobalBranch({
        name: createName.trim(),
        description: createDescription.trim() || undefined,
      });
      setCreateName('');
      setCreateDescription('');
      const next = await listGlobalBranches();
      setBranches(next);
      await selectById(created.id);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Failed to create global branch');
    } finally {
      setCreating(false);
    }
  }

  async function submitLink(e: React.FormEvent) {
    e.preventDefault();
    if (!selected) return;
    setLinking(true);
    setError('');
    try {
      await addGlobalBranchLink(selected.id, {
        resource_type: linkResourceType,
        resource_rid: linkResourceRid.trim(),
        branch_rid: linkBranchRid.trim(),
      });
      setLinkResourceRid('');
      setLinkBranchRid('');
      await selectById(selected.id);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Failed to add link');
    } finally {
      setLinking(false);
    }
  }

  async function promote() {
    if (!selected) return;
    setPromoting(true);
    setError('');
    try {
      const res = await promoteGlobalBranch(selected.id);
      setPromoteResult(`event_id=${res.event_id} → ${res.topic}`);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Promote failed');
    } finally {
      setPromoting(false);
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Developer toolchain</p>
        <h1 className="of-heading-xl" style={{ marginTop: 4 }}>
          Global Branching
        </h1>
        <p className="of-text-muted" style={{ marginTop: 8, fontSize: 14, maxWidth: 720 }}>
          Coordinate cross-plane branches. Every Foundry plane (datasets, ontology, pipelines,
          code repos) keeps owning its local branches; a global branch labels a workstream that
          spans them.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 2fr' }}>
        <aside style={{ display: 'grid', gap: 12 }}>
          <form onSubmit={submitCreate} className="of-panel" style={{ padding: 12, display: 'grid', gap: 8 }}>
            <h2 className="of-heading-sm">Create global branch</h2>
            <input
              className="of-input"
              placeholder="Name (e.g. release-2026-Q3)"
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              required
              style={{ minHeight: 32, fontSize: 13 }}
            />
            <input
              className="of-input"
              placeholder="Description (optional)"
              value={createDescription}
              onChange={(e) => setCreateDescription(e.target.value)}
              style={{ minHeight: 32, fontSize: 13 }}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                type="submit"
                className="of-btn of-btn-primary"
                disabled={creating || !createName.trim()}
                style={{ minHeight: 28, fontSize: 12 }}
              >
                Create
              </button>
            </div>
          </form>

          <div className="of-panel" style={{ overflow: 'hidden' }}>
            {loading ? (
              <p className="of-text-muted" style={{ padding: '8px 12px', fontSize: 13 }}>
                Loading…
              </p>
            ) : branches.length === 0 ? (
              <p className="of-text-muted" style={{ padding: '8px 12px', fontSize: 13 }}>
                No global branches yet.
              </p>
            ) : (
              <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
                {branches.map((b) => (
                  <li key={b.id}>
                    <button
                      type="button"
                      onClick={() => void selectById(b.id)}
                      style={{
                        width: '100%',
                        textAlign: 'left',
                        padding: '8px 12px',
                        background: selected?.id === b.id ? '#dbeafe' : 'transparent',
                        border: 0,
                        cursor: 'pointer',
                        fontSize: 13,
                      }}
                    >
                      <span style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{b.name}</span>
                      <span style={{ display: 'block', fontSize: 11, color: 'var(--text-muted)' }}>
                        {b.rid}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </aside>

        <section style={{ display: 'grid', gap: 12 }}>
          {selected ? (
            <>
              <div className="of-panel" style={{ padding: 16 }}>
                <header style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <h2 className="of-heading-md">{selected.name}</h2>
                    <p style={{ fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                      {selected.rid}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="of-btn"
                    onClick={() => void promote()}
                    disabled={promoting}
                    style={{ background: '#7c3aed', color: '#fff', borderColor: '#6d28d9' }}
                  >
                    Promote
                  </button>
                </header>
                {promoteResult && (
                  <p style={{ marginTop: 8, fontSize: 12, color: 'var(--status-success)' }}>{promoteResult}</p>
                )}
                <dl style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 8, marginTop: 12, fontSize: 12 }}>
                  {[
                    { label: 'Links', value: selected.link_count },
                    { label: 'Drifted', value: selected.drifted_count },
                    { label: 'Archived', value: selected.archived_count },
                  ].map((stat) => (
                    <div key={stat.label}>
                      <dt style={{ color: 'var(--text-muted)' }}>{stat.label}</dt>
                      <dd style={{ margin: 0, fontWeight: 600, color: 'var(--text-strong)' }}>{stat.value}</dd>
                    </div>
                  ))}
                </dl>
              </div>

              <form
                onSubmit={submitLink}
                className="of-panel"
                style={{ padding: 12, display: 'grid', gap: 8, gridTemplateColumns: '1fr 2fr 2fr auto' }}
              >
                <select
                  className="of-select"
                  value={linkResourceType}
                  onChange={(e) => setLinkResourceType(e.target.value)}
                  style={{ minHeight: 30, fontSize: 12 }}
                >
                  <option value="dataset">dataset</option>
                  <option value="pipeline">pipeline</option>
                  <option value="ontology">ontology</option>
                  <option value="code_repo">code_repo</option>
                </select>
                <input
                  className="of-input"
                  placeholder="Resource RID"
                  value={linkResourceRid}
                  onChange={(e) => setLinkResourceRid(e.target.value)}
                  required
                  style={{ minHeight: 30, fontSize: 12 }}
                />
                <input
                  className="of-input"
                  placeholder="Branch RID (ri.foundry.main.branch.…)"
                  value={linkBranchRid}
                  onChange={(e) => setLinkBranchRid(e.target.value)}
                  required
                  style={{ minHeight: 30, fontSize: 12 }}
                />
                <button
                  type="submit"
                  className="of-btn of-btn-primary"
                  disabled={linking}
                  style={{ minHeight: 30, fontSize: 12 }}
                >
                  Link
                </button>
              </form>

              <div className="of-panel" style={{ overflow: 'hidden' }}>
                <table className="of-table" style={{ fontSize: 12 }}>
                  <thead>
                    <tr>
                      <th>Resource type</th>
                      <th>Resource RID</th>
                      <th>Branch RID</th>
                      <th>Status</th>
                      <th>Last synced</th>
                    </tr>
                  </thead>
                  <tbody>
                    {resources.length === 0 ? (
                      <tr>
                        <td colSpan={5} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
                          No resources linked yet.
                        </td>
                      </tr>
                    ) : (
                      resources.map((link) => (
                        <tr key={`${link.resource_type}-${link.resource_rid}-${link.branch_rid}`}>
                          <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{link.resource_type}</td>
                          <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{link.resource_rid}</td>
                          <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{link.branch_rid}</td>
                          <td>
                            <span className={`of-chip ${statusTone(link.status)}`} style={{ fontSize: 10 }}>
                              {link.status}
                            </span>
                          </td>
                          <td style={{ color: 'var(--text-muted)' }}>{link.last_synced_at.slice(0, 16)}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </>
          ) : !loading ? (
            <p className="of-text-muted" style={{ fontSize: 13 }}>
              Select a global branch to inspect.
            </p>
          ) : null}
        </section>
      </div>
    </section>
  );
}
