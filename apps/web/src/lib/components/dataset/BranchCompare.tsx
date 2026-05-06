import { useEffect, useState } from 'react';

import {
  compareBranches,
  type BranchCompareResponse,
  type CompareConflictingFile,
  type CompareTransactionSummary,
  type DatasetBranch,
} from '@/lib/api/datasets';

interface BranchCompareProps {
  datasetRid: string;
  branches: DatasetBranch[];
}

function shortRid(rid: string): string {
  const tail = rid.split('.').pop() ?? '';
  return tail.length > 12 ? `${tail.slice(0, 8)}…` : tail;
}

export function BranchCompare({ datasetRid, branches }: BranchCompareProps) {
  const [baseBranch, setBaseBranch] = useState('master');
  const [compareBranch, setCompareBranch] = useState('');
  const [response, setResponse] = useState<BranchCompareResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!compareBranch && branches.length > 0) {
      const fallback = branches.find((b) => !b.is_default);
      setCompareBranch(fallback?.name ?? '');
    }
  }, [branches, compareBranch]);

  async function run() {
    if (!baseBranch || !compareBranch || baseBranch === compareBranch) {
      setError('Pick two different branches.');
      return;
    }
    setLoading(true);
    setError('');
    setResponse(null);
    try {
      setResponse(await compareBranches(datasetRid, baseBranch, compareBranch));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Compare failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'flex-end' }}>
        <label style={{ fontSize: 12 }}>
          <span style={{ display: 'block', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-muted)' }}>From branch</span>
          <select value={baseBranch} onChange={(e) => setBaseBranch(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
            {branches.map((b) => <option key={b.id} value={b.name}>{b.name}{b.is_default ? ' (default)' : ''}</option>)}
          </select>
        </label>
        <label style={{ fontSize: 12 }}>
          <span style={{ display: 'block', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--text-muted)' }}>Compare branch</span>
          <select value={compareBranch} onChange={(e) => setCompareBranch(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
            {branches.map((b) => <option key={b.id} value={b.name}>{b.name}{b.is_default ? ' (default)' : ''}</option>)}
          </select>
        </label>
        <button type="button" onClick={() => void run()} disabled={loading} className="of-button of-button--primary" style={{ fontSize: 12 }}>
          {loading ? 'Comparing…' : 'Compare'}
        </button>
      </header>

      {error && <p style={{ fontSize: 11, color: '#fca5a5' }}>{error}</p>}

      {response && (
        <>
          <p className="of-text-muted" style={{ fontSize: 11 }}>
            LCA: <code>{response.lca_branch_rid ? shortRid(response.lca_branch_rid) : '—'}</code>
          </p>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 8 }}>
            <CommitColumn title={`Only on ${baseBranch}`} commits={response.a_only_transactions} />
            <ConflictColumn conflicts={response.conflicting_files} />
            <CommitColumn title={`Only on ${compareBranch}`} commits={response.b_only_transactions} />
          </div>
        </>
      )}
    </section>
  );
}

function CommitColumn({ title, commits }: { title: string; commits: CompareTransactionSummary[] }) {
  return (
    <article style={{ padding: 8, background: 'var(--bg-subtle)', borderRadius: 8 }}>
      <p className="of-eyebrow" style={{ fontSize: 10 }}>{title} ({commits.length})</p>
      <ul style={{ marginTop: 6, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, maxHeight: 320, overflow: 'auto' }}>
        {commits.map((c) => (
          <li key={c.transaction_rid} style={{ padding: 4, fontSize: 11, fontFamily: 'var(--font-mono)' }}>
            <strong>{c.tx_type}</strong> · {shortRid(c.transaction_rid)} · {c.committed_at?.slice(0, 16) ?? '—'}
          </li>
        ))}
        {commits.length === 0 && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>none</li>}
      </ul>
    </article>
  );
}

function ConflictColumn({ conflicts }: { conflicts: CompareConflictingFile[] }) {
  return (
    <article style={{ padding: 8, background: 'var(--bg-subtle)', borderRadius: 8 }}>
      <p className="of-eyebrow" style={{ fontSize: 10 }}>Conflicts ({conflicts.length})</p>
      <ul style={{ marginTop: 6, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, maxHeight: 320, overflow: 'auto' }}>
        {conflicts.map((c) => (
          <li key={c.logical_path} style={{ padding: 4, fontSize: 11 }}>
            <code>{c.logical_path}</code>
            <p className="of-text-muted" style={{ fontSize: 10, margin: '2px 0 0' }}>
              A: {shortRid(c.a_transaction_rid)} · B: {shortRid(c.b_transaction_rid)}
            </p>
          </li>
        ))}
        {conflicts.length === 0 && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>none</li>}
      </ul>
    </article>
  );
}
