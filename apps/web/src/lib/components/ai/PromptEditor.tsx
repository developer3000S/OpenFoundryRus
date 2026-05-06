import { useEffect, useState } from 'react';

import type { PromptTemplate } from '@/lib/api/ai';

export interface PromptDraft {
  id?: string;
  name: string;
  description: string;
  category: string;
  status: string;
  tags_text: string;
  content: string;
  input_variables_text: string;
  notes: string;
}

interface Props {
  prompts: PromptTemplate[];
  draft: PromptDraft;
  renderedPreview: string;
  missingVariables: string[];
  busy?: boolean;
  onSelect?: (promptId: string) => void;
  onDraftChange?: (draft: PromptDraft) => void;
  onSave?: () => void;
  onRender?: () => void;
  onReset?: () => void;
}

export function PromptEditor({ prompts, draft, renderedPreview, missingVariables, busy = false, onSelect, onDraftChange, onSave, onRender, onReset }: Props) {
  const [localDraft, setLocalDraft] = useState<PromptDraft>(draft);

  useEffect(() => { setLocalDraft(draft); }, [draft]);

  function update<K extends keyof PromptDraft>(key: K, value: PromptDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Prompt Management</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Versioned prompt templates with live rendering</h2>
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => onReset?.()} disabled={busy} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900">New</button>
          <button type="button" onClick={() => onRender?.()} disabled={busy} className="rounded-full border border-cyan-300 px-3 py-1.5 text-sm text-cyan-700 hover:bg-cyan-50 dark:border-cyan-800 dark:text-cyan-300 dark:hover:bg-cyan-950/40">Render</button>
          <button type="button" onClick={() => onSave?.()} disabled={busy} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Save</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <div className="space-y-3">
          {prompts.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No prompts yet.</div>
          ) : (
            prompts.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => onSelect?.(p.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${localDraft.id === p.id ? 'border-cyan-400 bg-cyan-50 dark:border-cyan-700 dark:bg-cyan-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{p.name}</div>
                    <div className="mt-1 text-xs text-slate-500">{p.category} • v{p.current_version.version_number}</div>
                  </div>
                  <span className="rounded-full bg-white px-2 py-1 text-[11px] font-medium uppercase tracking-[0.2em] text-slate-500 dark:bg-slate-950">{p.status}</span>
                </div>
              </button>
            ))
          )}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Name"><input className={inputCls} value={localDraft.name} onChange={(e) => update('name', e.target.value)} /></Field>
            <Field label="Category"><input className={inputCls} value={localDraft.category} onChange={(e) => update('category', e.target.value)} /></Field>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Description"><input className={inputCls} value={localDraft.description} onChange={(e) => update('description', e.target.value)} /></Field>
            <Field label="Tags"><input className={inputCls} value={localDraft.tags_text} onChange={(e) => update('tags_text', e.target.value)} placeholder="copilot, operations" /></Field>
          </div>
          <Field label="Template">
            <textarea className={`${inputCls} h-40`} value={localDraft.content} onChange={(e) => update('content', e.target.value)} />
          </Field>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Input Variables"><input className={inputCls} value={localDraft.input_variables_text} onChange={(e) => update('input_variables_text', e.target.value)} placeholder="team_name, region" /></Field>
            <Field label="Version Notes"><input className={inputCls} value={localDraft.notes} onChange={(e) => update('notes', e.target.value)} /></Field>
          </div>

          <div className="rounded-2xl border border-dashed border-slate-300 bg-white px-4 py-4 dark:border-slate-700 dark:bg-slate-950">
            <div className="flex items-center justify-between gap-3">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Rendered Preview</div>
              {missingVariables.length > 0 && <div className="text-xs text-amber-600 dark:text-amber-300">Missing: {missingVariables.join(', ')}</div>}
            </div>
            <pre className="mt-3 whitespace-pre-wrap text-sm leading-6 text-slate-700 dark:text-slate-200">{renderedPreview || 'Render a template to preview interpolation.'}</pre>
          </div>
        </div>
      </div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
      <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">{label}</div>
      <div className="mt-2">{children}</div>
    </label>
  );
}

const inputCls = 'w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100';
