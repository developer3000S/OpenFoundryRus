import { useEffect, useState } from 'react';

import type { AgentDefinition, AgentExecutionResponse, KnowledgeBase, ToolDefinition } from '@/lib/api/ai';

export interface AgentDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  system_prompt: string;
  objective: string;
  tool_ids: string[];
  planning_strategy: string;
  max_iterations: number;
  memory_text: string;
}

export interface ExecutionDraft {
  user_message: string;
  objective: string;
  knowledge_base_id: string;
  context_text: string;
}

interface Props {
  agents: AgentDefinition[];
  tools: ToolDefinition[];
  knowledgeBases: KnowledgeBase[];
  draft: AgentDraft;
  executionDraft: ExecutionDraft;
  executionResponse: AgentExecutionResponse | null;
  busy?: boolean;
  onSelect?: (agentId: string) => void;
  onDraftChange?: (draft: AgentDraft) => void;
  onExecutionDraftChange?: (draft: ExecutionDraft) => void;
  onSave?: () => void;
  onExecute?: () => void;
  onReset?: () => void;
}

export function AgentBuilder({ agents, tools, knowledgeBases, draft, executionDraft, executionResponse, busy = false, onSelect, onDraftChange, onExecutionDraftChange, onSave, onExecute, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<AgentDraft>(draft);
  const [localExec, setLocalExec] = useState<ExecutionDraft>(executionDraft);

  useEffect(() => { setLocalDraft(draft); }, [draft]);
  useEffect(() => { setLocalExec(executionDraft); }, [executionDraft]);

  function update<K extends keyof AgentDraft>(key: K, value: AgentDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }
  function updateExec<K extends keyof ExecutionDraft>(key: K, value: ExecutionDraft[K]) {
    const next = { ...localExec, [key]: value };
    setLocalExec(next);
    onExecutionDraftChange?.(next);
  }
  function toggleTool(toolId: string) {
    const next = localDraft.tool_ids.includes(toolId) ? localDraft.tool_ids.filter((v) => v !== toolId) : [...localDraft.tool_ids, toolId];
    update('tool_ids', next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Agent Builder</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Configure plan-act-observe agents and execute traces</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => onReset?.()} disabled={busy} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900">New</button>
          <button type="button" onClick={() => onSave?.()} disabled={busy} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <div className="space-y-3">
          {agents.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => onSelect?.(a.id)}
              className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === a.id ? 'border-cyan-400 bg-cyan-50 dark:border-cyan-700 dark:bg-cyan-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
            >
              <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{a.name}</div>
              <div className="mt-1 text-xs text-slate-500">{a.planning_strategy} • {a.max_iterations} iterations</div>
            </button>
          ))}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <input className={inputCls} value={localDraft.name} onChange={(e) => update('name', e.target.value)} placeholder="Platform Analyst" />
            <input className={inputCls} value={localDraft.planning_strategy} onChange={(e) => update('planning_strategy', e.target.value)} placeholder="plan-act-observe" />
          </div>
          <textarea className={`${inputCls} h-24`} value={localDraft.description} onChange={(e) => update('description', e.target.value)} />
          <textarea className={`${inputCls} h-24`} value={localDraft.system_prompt} onChange={(e) => update('system_prompt', e.target.value)} />
          <textarea className={`${inputCls} h-24`} value={localDraft.objective} onChange={(e) => update('objective', e.target.value)} />
          <input type="number" className={inputCls} value={String(localDraft.max_iterations)} onChange={(e) => update('max_iterations', Number(e.target.value) || 3)} />

          <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Tool Access</div>
            <div className="mt-3 grid gap-2 md:grid-cols-2">
              {tools.map((tool) => (
                <label key={tool.id} className="flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-800 dark:bg-slate-950">
                  <input type="checkbox" checked={localDraft.tool_ids.includes(tool.id)} onChange={() => toggleTool(tool.id)} />
                  <span>{tool.name}</span>
                </label>
              ))}
            </div>
          </div>
          <textarea className={`${inputCls} h-28`} value={localDraft.memory_text} onChange={(e) => update('memory_text', e.target.value)} />

          <div className="rounded-[24px] border border-dashed border-cyan-300 bg-cyan-50/60 p-4 dark:border-cyan-900 dark:bg-cyan-950/20">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-cyan-700 dark:text-cyan-300">Execution Sandbox</div>
                <p className="mt-1 text-sm text-slate-600 dark:text-slate-300">Run the current agent against a selected knowledge base.</p>
              </div>
              <button type="button" onClick={() => onExecute?.()} disabled={busy || !localDraft.id} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Execute</button>
            </div>
            <div className="mt-4 grid gap-3">
              <textarea className="h-24 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950" value={localExec.user_message} onChange={(e) => updateExec('user_message', e.target.value)} />
              <input className="rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950" value={localExec.objective} onChange={(e) => updateExec('objective', e.target.value)} placeholder="Investigate provider failover" />
              <select className="rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950" value={localExec.knowledge_base_id} onChange={(e) => updateExec('knowledge_base_id', e.target.value)}>
                <option value="">No knowledge base</option>
                {knowledgeBases.map((kb) => <option key={kb.id} value={kb.id}>{kb.name}</option>)}
              </select>
              <textarea className="h-40 rounded-2xl border border-slate-200 bg-white px-4 py-3 font-mono text-xs dark:border-slate-800 dark:bg-slate-950" value={localExec.context_text} onChange={(e) => updateExec('context_text', e.target.value)} />
            </div>

            {executionResponse && (
              <div className="mt-4 space-y-3">
                <div className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">
                  <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Final Response</div>
                  <p className="mt-2 text-sm leading-6 text-slate-700 dark:text-slate-200">{executionResponse.final_response}</p>
                </div>
                {executionResponse.traces.map((trace, i) => (
                  <div key={i} className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">
                    <div className="flex items-center justify-between gap-3 text-sm">
                      <span className="font-semibold text-slate-900 dark:text-slate-100">{trace.title}</span>
                      <span className="text-xs text-slate-500">{trace.tool_name ?? 'reasoning'}</span>
                    </div>
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">{trace.observation}</p>
                    <pre className="mt-3 overflow-x-auto rounded-2xl bg-slate-950 px-3 py-3 text-xs text-cyan-100">{JSON.stringify(trace.output, null, 2)}</pre>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}

const inputCls = 'rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900';
