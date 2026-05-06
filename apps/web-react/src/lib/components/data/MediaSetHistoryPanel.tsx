import { useEffect, useState } from 'react';

import { createMediaSetBranch, listMediaSetTransactions, type MediaSet, type MediaSetTransactionHistoryEntry } from '@/lib/api/mediaSets';
import { notifications } from '@/lib/stores/notifications';

interface Props {
  mediaSet: MediaSet;
  onBranchCreated?: (branchName: string) => void;
}

function badgeTone(state: string) {
  switch (state) {
    case 'COMMITTED': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300';
    case 'ABORTED': return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300';
    case 'OPEN': return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300';
    default: return 'bg-slate-100 text-slate-700 dark:bg-gray-800 dark:text-slate-300';
  }
}

export function MediaSetHistoryPanel({ mediaSet, onBranchCreated }: Props) {
  const [entries, setEntries] = useState<MediaSetTransactionHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [restoringRid, setRestoringRid] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError('');
    try {
      setEntries(await listMediaSetTransactions(mediaSet.rid));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load history');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaSet.rid]);

  async function restore(entry: MediaSetTransactionHistoryEntry) {
    if (restoringRid) return;
    const branchName = window.prompt(
      `Restore as a new branch off transaction ${entry.rid.slice(-12)}.\nBranch name:`,
      `restore-${entry.rid.split('.').pop()?.slice(0, 8) ?? 'point'}`,
    );
    if (!branchName) return;
    setRestoringRid(entry.rid);
    try {
      const branch = await createMediaSetBranch(mediaSet.rid, {
        name: branchName,
        from_branch: entry.branch,
        from_transaction_rid: entry.rid,
      });
      notifications.success(`Branch '${branch.branch_name}' created at this point`);
      onBranchCreated?.(branch.branch_name);
    } catch (cause) {
      notifications.error(cause instanceof Error ? cause.message : 'Failed to create branch');
    } finally {
      setRestoringRid(null);
    }
  }

  return (
    <section className="space-y-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">History</h2>
          <p className="mt-1 text-xs text-slate-500">
            Every transaction recorded on this media set, newest first. Each entry shows the items added, modified (path-deduplicated) and deleted in that batch. "Restore to this point" mints a new branch off the historical transaction (per Foundry's immutable-history contract).
          </p>
        </div>
        <button type="button" onClick={() => void load()} disabled={loading} className="rounded-xl border border-slate-200 px-3 py-1.5 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800">
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </header>

      {error ? (
        <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
      ) : loading && entries.length === 0 ? (
        <div className="h-24 animate-pulse rounded-xl border border-slate-200 bg-slate-100 dark:border-gray-700 dark:bg-gray-800" />
      ) : entries.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 p-6 text-center text-sm text-slate-500 dark:border-gray-700">
          No transactions yet. Open a transaction on a transactional set or upload directly to a transactionless set to start the history.
        </div>
      ) : (
        <ul className="space-y-2">
          {entries.map((entry) => (
            <li key={entry.rid} className="rounded-xl border border-slate-200 p-3 dark:border-gray-700">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm">
                    <span className="font-medium">{entry.branch}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[10px] uppercase tracking-wide ${badgeTone(entry.state)}`}>{entry.state}</span>
                    <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[10px] uppercase tracking-wide text-slate-600 dark:bg-gray-800 dark:text-slate-300">{entry.write_mode}</span>
                  </div>
                  <div className="mt-0.5 text-[11px] text-slate-500">
                    {entry.opened_by || 'unknown'} · {new Date(entry.opened_at).toLocaleString()}
                    {entry.closed_at && <> · closed {new Date(entry.closed_at).toLocaleString()}</>}
                  </div>
                  <div className="mt-1 font-mono text-[10px] text-slate-400">{entry.rid}</div>
                  <div className="mt-2 flex flex-wrap gap-2 text-[11px]">
                    <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300">+{entry.items_added} added</span>
                    <span className="rounded-full bg-amber-50 px-2 py-0.5 text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">~{entry.items_modified} modified</span>
                    <span className="rounded-full bg-rose-50 px-2 py-0.5 text-rose-700 dark:bg-rose-950/30 dark:text-rose-300">−{entry.items_deleted} deleted</span>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => void restore(entry)}
                  disabled={restoringRid === entry.rid || entry.state !== 'COMMITTED'}
                  title={entry.state !== 'COMMITTED' ? 'Only committed transactions can be used as a restore point.' : 'Create a new branch off this transaction.'}
                  className="rounded-xl border border-slate-200 px-3 py-1.5 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800 disabled:opacity-50"
                >
                  {restoringRid === entry.rid ? 'Restoring…' : 'Restore to this point'}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
