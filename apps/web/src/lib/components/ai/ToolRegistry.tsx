import { useEffect, useState } from 'react';

import type { ToolDefinition } from '@/lib/api/ai';

export interface ToolDraft {
  id?: string;
  name: string;
  description: string;
  category: string;
  execution_mode: string;
  execution_config_text: string;
  status: string;
  input_schema_text: string;
  output_schema_text: string;
  tags_text: string;
}

interface Props {
  tools: ToolDefinition[];
  draft: ToolDraft;
  busy?: boolean;
  onSelect?: (toolId: string) => void;
  onDraftChange?: (draft: ToolDraft) => void;
  onSave?: () => void;
  onReset?: () => void;
}

const EXECUTION_MODES = [
  'native_sql', 'native_dataset', 'native_ontology', 'native_pipeline', 'native_report',
  'native_workflow', 'native_code_repo', 'knowledge_search', 'openfoundry_api', 'http_json', 'simulated',
];

export function ToolRegistry({ tools, draft, busy = false, onSelect, onDraftChange, onSave, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<ToolDraft>(draft);

  useEffect(() => { setLocalDraft(draft); }, [draft]);

  function update<K extends keyof ToolDraft>(key: K, value: ToolDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Tool Registry</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Manage callable tools for agent execution</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => onReset?.()} disabled={busy} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900">New</button>
          <button type="button" onClick={() => onSave?.()} disabled={busy} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <div className="space-y-3">
          {tools.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No tools registered yet.</div>
          ) : (
            tools.map((tool) => (
              <button
                key={tool.id}
                type="button"
                onClick={() => onSelect?.(tool.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === tool.id ? 'border-cyan-400 bg-cyan-50 dark:border-cyan-700 dark:bg-cyan-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{tool.name}</div>
                <div className="mt-1 text-xs text-slate-500">{tool.category} • {tool.execution_mode}</div>
              </button>
            ))
          )}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <input className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.name} onChange={(e) => update('name', e.target.value)} placeholder="SQL Generator" />
            <input className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.category} onChange={(e) => update('category', e.target.value)} placeholder="analysis" />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <select className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.execution_mode} onChange={(e) => update('execution_mode', e.target.value)}>
              {EXECUTION_MODES.map((m) => <option key={m} value={m}>{m}</option>)}
            </select>
            <input className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.tags_text} onChange={(e) => update('tags_text', e.target.value)} placeholder="sql, copilot" />
          </div>
          <textarea className="h-24 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.description} onChange={(e) => update('description', e.target.value)} />
          <textarea className="h-40 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 font-mono text-xs dark:border-slate-800 dark:bg-slate-900" value={localDraft.execution_config_text} onChange={(e) => update('execution_config_text', e.target.value)} />
          <div className="grid gap-4 md:grid-cols-2">
            <textarea className="h-40 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.input_schema_text} onChange={(e) => update('input_schema_text', e.target.value)} />
            <textarea className="h-40 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900" value={localDraft.output_schema_text} onChange={(e) => update('output_schema_text', e.target.value)} />
          </div>
          <p className="text-xs text-slate-500">native_* + knowledge_search execute inside the agent runtime. openfoundry_api / http_json take execution_config for routes, auth, and payloads.</p>
        </div>
      </div>
    </section>
  );
}
