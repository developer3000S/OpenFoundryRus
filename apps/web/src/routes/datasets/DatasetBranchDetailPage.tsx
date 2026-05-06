import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import {
  getBranchMarkings,
  listBranches,
  restoreBranch,
  updateBranchRetention,
  type BranchMarkingsView,
  type DatasetBranch,
} from '@/lib/api/datasets';

type Tab = 'retention' | 'security';
type Policy = 'INHERITED' | 'FOREVER' | 'TTL_DAYS';

export function DatasetBranchDetailPage() {
  const { id = '', branch: branchName = '' } = useParams<{ id: string; branch: string }>();
  const [branches, setBranches] = useState<DatasetBranch[]>([]);
  const [branch, setBranch] = useState<DatasetBranch | null>(null);
  const [markings, setMarkings] = useState<BranchMarkingsView | null>(null);
  const [tab, setTab] = useState<Tab>('retention');
  const [policy, setPolicy] = useState<Policy>('INHERITED');
  const [ttlDays, setTtlDays] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function load() {
    if (!id || !branchName) return;
    setError('');
    try {
      const br = await listBranches(id);
      setBranches(br);
      const found = br.find((b) => b.name === branchName) ?? null;
      setBranch(found);
      setPolicy(found?.retention_policy ?? 'INHERITED');
      setTtlDays(found?.retention_ttl_days ?? null);
      try {
        setMarkings(await getBranchMarkings(id, branchName));
      } catch {
        setMarkings({ effective: [], explicit: [], inherited_from_parent: [] });
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load branch');
    }
  }

  useEffect(() => {
    void load();
  }, [id, branchName]);

  async function saveRetention() {
    setBusy(true);
    setError('');
    try {
      await updateBranchRetention(id, branchName, {
        policy,
        ttl_days: policy === 'TTL_DAYS' ? ttlDays : null,
      });
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function restore() {
    setBusy(true);
    setError('');
    try {
      await restoreBranch(id, branchName);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Restore failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 880 }}>
      <Link to={`/datasets/${id}/branches`} style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Branches</Link>
      <header>
        <h1 className="of-heading-xl">{branchName}</h1>
        {branch && (
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {branches.length} branches · policy: {branch.retention_policy ?? 'INHERITED'}
            {branch.archived_at ? ` · archived at ${branch.archived_at}` : ''}
          </p>
        )}
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs tabs={['retention', 'security'] as const} active={tab} onChange={setTab} />

      {tab === 'retention' && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          {branch?.archived_at && (
            <div style={{ padding: 10, background: '#fef3c7', color: '#92400e', borderRadius: 8, fontSize: 12 }}>
              Branch archived at <code>{branch.archived_at}</code>.
              <button type="button" onClick={() => void restore()} disabled={busy} className="of-button" style={{ marginLeft: 8, fontSize: 11 }}>
                Restore branch
              </button>
            </div>
          )}
          <fieldset style={{ display: 'grid', gap: 6, padding: 0, border: 0 }}>
            <legend style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Policy</legend>
            {(['INHERITED', 'FOREVER', 'TTL_DAYS'] as Policy[]).map((p) => (
              <label key={p} style={{ display: 'flex', gap: 8, padding: 6, border: '1px solid var(--border-default)', borderRadius: 8 }}>
                <input type="radio" checked={policy === p} onChange={() => setPolicy(p)} />
                <span>
                  <strong>{p}</strong>
                  <span style={{ display: 'block', fontSize: 11, color: 'var(--text-muted)' }}>
                    {p === 'INHERITED' && 'Walk up parent_branch chain.'}
                    {p === 'FOREVER' && 'Never archived.'}
                    {p === 'TTL_DAYS' && 'Archive after N days of inactivity.'}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
          {policy === 'TTL_DAYS' && (
            <label style={{ fontSize: 13 }}>
              TTL (days)
              <input
                type="number"
                min={1}
                value={ttlDays ?? ''}
                onChange={(e) => setTtlDays(e.target.value ? Number(e.target.value) : null)}
                className="of-input"
                style={{ marginTop: 4, width: 120 }}
              />
            </label>
          )}
          <div>
            <button type="button" onClick={() => void saveRetention()} disabled={busy} className="of-button of-button--primary">
              Save retention
            </button>
          </div>
        </section>
      )}

      {tab === 'security' && (
        <section className="of-panel" style={{ padding: 16 }}>
          {!markings || markings.effective.length === 0 ? (
            <p className="of-text-muted">No markings on this branch.</p>
          ) : (
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
              {(['effective', 'explicit', 'inherited_from_parent'] as const).map((g) => (
                <div key={g} style={{ padding: 12, border: '1px solid var(--border-default)', borderRadius: 12 }}>
                  <p style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)' }}>
                    {g === 'effective' && 'Effective'}
                    {g === 'explicit' && 'Explicit on this branch'}
                    {g === 'inherited_from_parent' && 'Inherited from parent'}
                  </p>
                  <ul style={{ display: 'flex', flexWrap: 'wrap', gap: 4, paddingLeft: 0, listStyle: 'none', marginTop: 6 }}>
                    {markings[g].length === 0 ? (
                      <li className="of-text-muted">—</li>
                    ) : (
                      markings[g].map((m) => (
                        <li key={m} style={{ fontSize: 11, padding: '2px 8px', background: 'var(--bg-subtle)', borderRadius: 999 }}>
                          <code>{m}</code>
                        </li>
                      ))
                    )}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </section>
  );
}
