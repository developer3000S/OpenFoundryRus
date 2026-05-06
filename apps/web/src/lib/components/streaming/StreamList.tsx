import { useEffect, useState } from 'react';

import type { StreamDefinition } from '@/lib/api/streaming';

export interface StreamDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  retention_hours: number;
  connector_type: string;
  endpoint: string;
  format: string;
  schema_text: string;
}

interface Props {
  streams: StreamDefinition[];
  draft: StreamDraft;
  busy?: boolean;
  onSelect?: (streamId: string) => void;
  onDraftChange?: (draft: StreamDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

export function StreamList({ streams, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<StreamDraft>(draft);

  useEffect(() => {
    setLocalDraft(draft);
  }, [draft]);

  function update<K extends keyof StreamDraft>(key: K, value: StreamDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Stream Definitions</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Named streams with schemas and source connectors</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900" onClick={() => onReset?.()} disabled={busy}>New</button>
          <button type="button" className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950" onClick={() => onSave?.()} disabled={busy}>Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.78fr)_minmax(0,1.22fr)]">
        <div className="space-y-3">
          {streams.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No streams defined yet.</div>
          ) : (
            streams.map((stream) => (
              <button
                key={stream.id}
                type="button"
                onClick={() => onSelect?.(stream.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === stream.id ? 'border-sky-400 bg-sky-50 dark:border-sky-700 dark:bg-sky-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{stream.name}</div>
                  <div className="flex flex-wrap gap-1">
                    {stream.stream_profile?.high_throughput && (
                      <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium uppercase text-amber-800 dark:bg-amber-950/40 dark:text-amber-300" title="linger.ms=25, batch.size=256KiB">HT</span>
                    )}
                    {stream.stream_profile?.compressed && (
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium uppercase text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300" title="compression.type=lz4">LZ4</span>
                    )}
                  </div>
                </div>
                <div className="mt-1 text-xs text-slate-500">{stream.source_binding.connector_type} • {stream.schema.fields.length} fields • {stream.retention_hours}h retention • {stream.stream_profile?.partitions ?? stream.partitions} part.</div>
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
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Status</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.status} onChange={(e) => update('status', e.target.value)} />
            </label>
          </div>

          <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Description</div>
            <textarea className="mt-2 h-20 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.description} onChange={(e) => update('description', e.target.value)} />
          </label>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Connector</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.connector_type} onChange={(e) => update('connector_type', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Endpoint</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.endpoint} onChange={(e) => update('endpoint', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Format</div>
              <input className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.format} onChange={(e) => update('format', e.target.value)} />
            </label>
            <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Retention Hours</div>
              <input type="number" className="mt-2 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={String(localDraft.retention_hours)} onChange={(e) => update('retention_hours', Number(e.target.value) || 72)} />
            </label>
          </div>

          <label className="rounded-2xl border border-dashed border-sky-300 bg-sky-50/60 px-4 py-3 dark:border-sky-900 dark:bg-sky-950/20">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-sky-700 dark:text-sky-300">Schema JSON</div>
            <textarea className="mt-2 h-56 w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100" value={localDraft.schema_text} onChange={(e) => update('schema_text', e.target.value)} />
          </label>
        </div>
      </div>
    </section>
  );
}
