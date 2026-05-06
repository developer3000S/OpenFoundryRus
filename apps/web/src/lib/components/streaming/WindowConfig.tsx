import { useEffect, useState } from 'react';

import type { WindowDefinition } from '@/lib/api/streaming';

export interface WindowDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  window_type: string;
  duration_seconds: number;
  slide_seconds: number;
  session_gap_seconds: number;
  allowed_lateness_seconds: number;
  aggregation_keys_text: string;
  measure_fields_text: string;
  keyed?: boolean;
  key_columns_text?: string;
  state_ttl_seconds?: number;
}

interface Props {
  windows: WindowDefinition[];
  draft: WindowDraft;
  busy?: boolean;
  onSelect?: (windowId: string) => void;
  onDraftChange?: (draft: WindowDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

export function WindowConfig({ windows, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<WindowDraft>(draft);

  useEffect(() => {
    setLocalDraft(draft);
  }, [draft]);

  function update<K extends keyof WindowDraft>(key: K, value: WindowDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Windowing</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Tumbling, sliding, and session window controls</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900" onClick={() => onReset?.()} disabled={busy}>New</button>
          <button type="button" className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950" onClick={() => onSave?.()} disabled={busy}>Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.78fr)_minmax(0,1.22fr)]">
        <div className="space-y-3">
          {windows.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No window definitions yet.</div>
          ) : (
            windows.map((w) => (
              <button
                key={w.id}
                type="button"
                onClick={() => onSelect?.(w.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === w.id ? 'border-violet-400 bg-violet-50 dark:border-violet-700 dark:bg-violet-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{w.name}</div>
                <div className="mt-1 text-xs text-slate-500">{w.window_type} • {w.duration_seconds}s / {w.slide_seconds}s</div>
              </button>
            ))
          )}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Name</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.name} onChange={(e) => update('name', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Type</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.window_type} onChange={(e) => update('window_type', e.target.value)} />
            </label>
          </div>

          <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Description</div>
            <textarea className="mt-2 h-20 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.description} onChange={(e) => update('description', e.target.value)} />
          </label>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Duration</div>
              <input type="number" className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={String(localDraft.duration_seconds)} onChange={(e) => update('duration_seconds', Number(e.target.value) || 300)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Slide</div>
              <input type="number" className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={String(localDraft.slide_seconds)} onChange={(e) => update('slide_seconds', Number(e.target.value) || 300)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Session Gap</div>
              <input type="number" className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={String(localDraft.session_gap_seconds)} onChange={(e) => update('session_gap_seconds', Number(e.target.value) || 180)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Allowed Lateness</div>
              <input type="number" className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={String(localDraft.allowed_lateness_seconds)} onChange={(e) => update('allowed_lateness_seconds', Number(e.target.value) || 30)} />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="rounded-2xl border border-dashed border-violet-300 bg-violet-50/60 px-4 py-3 dark:border-violet-900 dark:bg-violet-950/20">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-violet-700 dark:text-violet-300">Aggregation Keys</div>
              <textarea className="mt-2 h-32 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.aggregation_keys_text} onChange={(e) => update('aggregation_keys_text', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-dashed border-violet-300 bg-violet-50/60 px-4 py-3 dark:border-violet-900 dark:bg-violet-950/20">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-violet-700 dark:text-violet-300">Measure Fields</div>
              <textarea className="mt-2 h-32 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.measure_fields_text} onChange={(e) => update('measure_fields_text', e.target.value)} />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Keyed</div>
              <input type="checkbox" className="mt-2" checked={localDraft.keyed ?? false} onChange={(e) => update('keyed', e.target.checked)} />
              <small className="block text-xs text-slate-500">When checked, the runtime runs <code>key_by(key_columns)</code> before windowing.</small>
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900 md:col-span-2">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Key columns (comma separated)</div>
              <input type="text" className="mt-2 w-full bg-transparent text-sm outline-none" value={localDraft.key_columns_text ?? ''} onChange={(e) => update('key_columns_text', e.target.value)} placeholder="customer_id, country" />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900 md:col-span-3">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">State TTL (seconds)</div>
              <input type="range" min={0} max={86400} step={60} className="mt-2 w-full" value={String(localDraft.state_ttl_seconds ?? 0)} onChange={(e) => update('state_ttl_seconds', Number(e.target.value) || 0)} />
              <small className="text-xs text-slate-500">{(localDraft.state_ttl_seconds ?? 0)}s — 0 disables TTL.</small>
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
