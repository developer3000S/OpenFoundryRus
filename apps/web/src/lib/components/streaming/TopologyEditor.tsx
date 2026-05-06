import { useEffect, useState } from 'react';

import type { StreamDefinition, TopologyDefinition, WindowDefinition } from '@/lib/api/streaming';

export interface TopologyDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  state_backend: string;
  source_stream_ids_text: string;
  nodes_text: string;
  edges_text: string;
  join_definition_text: string;
  cep_definition_text: string;
  backpressure_policy_text: string;
  sink_bindings_text: string;
}

interface Props {
  topologies: TopologyDefinition[];
  streams: StreamDefinition[];
  windows: WindowDefinition[];
  draft: TopologyDraft;
  busy?: boolean;
  onSelect?: (topologyId: string) => void;
  onDraftChange?: (draft: TopologyDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

export function TopologyEditor({ topologies, streams, windows, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<TopologyDraft>(draft);

  useEffect(() => {
    setLocalDraft(draft);
  }, [draft]);

  function update<K extends keyof TopologyDraft>(key: K, value: TopologyDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  const jsonPanel = (field: keyof TopologyDraft, label: string, height: string) => (
    <label className="rounded-2xl border border-dashed border-emerald-300 bg-emerald-50/60 px-4 py-3 dark:border-emerald-900 dark:bg-emerald-950/20">
      <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-emerald-700 dark:text-emerald-300">{label}</div>
      <textarea className={`mt-2 ${height} w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100`} value={String(localDraft[field] ?? '')} onChange={(e) => update(field, e.target.value as never)} />
    </label>
  );

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Topology Editor</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">DAG-based processing, joins, CEP, sinks, and backpressure policy</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900" onClick={() => onReset?.()} disabled={busy}>New</button>
          <button type="button" className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950" onClick={() => onSave?.()} disabled={busy}>Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.76fr)_minmax(0,1.24fr)]">
        <div className="space-y-3">
          <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 text-sm text-slate-600 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
            {streams.length} streams available • {windows.length} windows available
          </div>
          {topologies.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No topologies defined yet.</div>
          ) : (
            topologies.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => onSelect?.(t.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === t.id ? 'border-emerald-400 bg-emerald-50 dark:border-emerald-700 dark:bg-emerald-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t.name}</div>
                <div className="mt-1 text-xs text-slate-500">{t.nodes.length} nodes • {t.edges.length} edges • {t.state_backend}</div>
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
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">State Backend</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.state_backend} onChange={(e) => update('state_backend', e.target.value)} />
            </label>
          </div>

          <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Description</div>
            <textarea className="mt-2 h-20 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.description} onChange={(e) => update('description', e.target.value)} />
          </label>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Status</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.status} onChange={(e) => update('status', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Source Stream IDs</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.source_stream_ids_text} onChange={(e) => update('source_stream_ids_text', e.target.value)} placeholder="uuid-1, uuid-2" />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            {jsonPanel('nodes_text', 'Nodes JSON', 'h-44')}
            {jsonPanel('edges_text', 'Edges JSON', 'h-44')}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            {jsonPanel('join_definition_text', 'Join Definition JSON', 'h-36')}
            {jsonPanel('cep_definition_text', 'CEP Definition JSON', 'h-36')}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            {jsonPanel('backpressure_policy_text', 'Backpressure Policy JSON', 'h-32')}
            {jsonPanel('sink_bindings_text', 'Sink Bindings JSON', 'h-32')}
          </div>
        </div>
      </div>
    </section>
  );
}
