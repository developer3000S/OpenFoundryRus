import { useEffect, useMemo, useRef, useState } from 'react';

import type { MediaSetBranch } from '@/lib/api/mediaSets';

interface Props {
  branches: MediaSetBranch[];
  currentBranch: string;
  onSwitch: (branchName: string) => Promise<void> | void;
  onCreate: (params: { name: string; from_branch: string; from_transaction_rid?: string }) => Promise<void> | void;
  onDelete?: (branchName: string) => Promise<void> | void;
  busy?: boolean;
}

export function MediaSetBranchPicker({ branches, currentBranch, onSwitch, onCreate, onDelete, busy = false }: Props) {
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [newFrom, setNewFrom] = useState('main');
  const rootRef = useRef<HTMLDivElement | null>(null);

  const sortedBranches = useMemo(
    () => [...branches].sort((a, b) => {
      if (a.branch_name === 'main') return -1;
      if (b.branch_name === 'main') return 1;
      return a.branch_name.localeCompare(b.branch_name);
    }),
    [branches],
  );

  useEffect(() => {
    function handler(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('click', handler);
    return () => document.removeEventListener('click', handler);
  }, []);

  async function selectBranch(name: string) {
    if (name === currentBranch) { setOpen(false); return; }
    await onSwitch(name);
    setOpen(false);
  }

  async function submitCreate() {
    if (!newName.trim()) return;
    await onCreate({ name: newName.trim(), from_branch: newFrom });
    setNewName('');
    setCreating(false);
    setOpen(false);
  }

  return (
    <div ref={rootRef} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={busy}
        className="inline-flex items-center gap-2 rounded-xl border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium hover:bg-slate-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800"
      >
        <svg className="h-4 w-4" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
          <path d="M5 3.25a2.25 2.25 0 1 1 2.5 2.236v3.028a2.25 2.25 0 1 1-2.5 0V5.486A2.251 2.251 0 0 1 5 3.25Zm6 0a2.25 2.25 0 0 1 1.25 4.122v.628c0 1.105-.895 2-2 2h-1.5a.75.75 0 0 1 0-1.5h1.5a.5.5 0 0 0 .5-.5v-.628A2.25 2.25 0 0 1 11 3.25Z" />
        </svg>
        <span>{currentBranch}</span>
        <svg className="h-3 w-3" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
          <path d="M3 4.5l3 3 3-3" />
        </svg>
      </button>

      {open && (
        <div className="absolute right-0 z-30 mt-1 w-80 rounded-xl border border-slate-200 bg-white p-2 shadow-lg dark:border-gray-700 dark:bg-gray-900">
          <ul className="space-y-1 text-sm">
            {sortedBranches.map((b) => (
              <li key={b.branch_rid} className="flex items-center justify-between gap-2">
                <button
                  type="button"
                  onClick={() => void selectBranch(b.branch_name)}
                  className={`flex-1 truncate rounded-lg px-2 py-1 text-left hover:bg-slate-100 dark:hover:bg-gray-800 ${b.branch_name === currentBranch ? 'font-semibold text-blue-600 dark:text-blue-300' : ''}`}
                >
                  {b.branch_name}
                  {!b.parent_branch_rid && (
                    <span className="ml-2 rounded-full bg-slate-100 px-1.5 py-0.5 text-[10px] uppercase text-slate-600 dark:bg-gray-800 dark:text-slate-300">root</span>
                  )}
                </button>
                {onDelete && b.branch_name !== 'main' && (
                  <button
                    type="button"
                    onClick={() => void onDelete(b.branch_name)}
                    title="Delete branch"
                    className="rounded p-1 text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                  >×</button>
                )}
              </li>
            ))}
          </ul>

          <div className="mt-2 border-t border-slate-200 pt-2 dark:border-gray-800">
            {creating ? (
              <div className="space-y-2">
                <input type="text" placeholder="branch name" value={newName} onChange={(e) => setNewName(e.target.value)} className="w-full rounded-lg border border-slate-300 px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-800" />
                <select value={newFrom} onChange={(e) => setNewFrom(e.target.value)} className="w-full rounded-lg border border-slate-300 px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-800">
                  {sortedBranches.map((b) => <option key={b.branch_rid} value={b.branch_name}>{b.branch_name}</option>)}
                </select>
                <div className="flex justify-end gap-1">
                  <button type="button" onClick={() => setCreating(false)} className="rounded-lg px-2 py-1 text-xs text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-gray-800">Cancel</button>
                  <button type="button" onClick={() => void submitCreate()} disabled={!newName.trim() || busy} className="rounded-lg bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50">Create</button>
                </div>
              </div>
            ) : (
              <button type="button" onClick={() => setCreating(true)} className="w-full rounded-lg px-2 py-1 text-left text-xs text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-gray-800">+ New branch</button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
