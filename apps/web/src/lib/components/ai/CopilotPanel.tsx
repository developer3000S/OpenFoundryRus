import { useEffect, useState } from 'react';

import { askCopilot, listKnowledgeBases, type CopilotResponse, type KnowledgeBase } from '@/lib/api/ai';
import { listDatasets, type Dataset } from '@/lib/api/datasets';
import { copilot, useCopilot } from '@/lib/stores/copilot';
import { notifications } from '@/lib/stores/notifications';

export function CopilotPanel() {
  const state = useCopilot();
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [selectedDatasetIds, setSelectedDatasetIds] = useState<string[]>([]);
  const [selectedKnowledgeBaseIds, setSelectedKnowledgeBaseIds] = useState<string[]>([]);
  const [question, setQuestion] = useState('Which provider should take over when latency spikes beyond 500ms?');
  const [includeSql, setIncludeSql] = useState(true);
  const [includePipelinePlan, setIncludePipelinePlan] = useState(true);
  const [response, setResponse] = useState<CopilotResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (state.seedQuestion) setQuestion(state.seedQuestion);
  }, [state.seedQuestion]);

  useEffect(() => {
    (async () => {
      try {
        const [datasetResp, kbResp] = await Promise.all([listDatasets({ per_page: 50 }), listKnowledgeBases()]);
        setDatasets(datasetResp.data);
        setKnowledgeBases(kbResp.data);
        setSelectedKnowledgeBaseIds(kbResp.data.slice(0, 1).map((item) => item.id));
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Failed to load copilot context');
      }
    })();
  }, []);

  function toggle(values: string[], id: string) {
    return values.includes(id) ? values.filter((v) => v !== id) : [...values, id];
  }

  async function submit() {
    if (!question.trim()) {
      notifications.error('Enter a question for the copilot.');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const next = await askCopilot({
        question: question.trim(),
        dataset_ids: selectedDatasetIds,
        knowledge_base_ids: selectedKnowledgeBaseIds,
        include_sql: includeSql,
        include_pipeline_plan: includePipelinePlan,
      });
      setResponse(next);
      notifications.success('Copilot response updated.');
    } catch (cause) {
      const msg = cause instanceof Error ? cause.message : 'Copilot request failed';
      setError(msg);
      notifications.error(msg);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      {!state.open && (
        <button
          type="button"
          onClick={() => copilot.open()}
          className="fixed bottom-6 right-6 z-40 inline-flex h-14 w-14 items-center justify-center rounded-full bg-slate-950 text-sm font-semibold text-white shadow-2xl shadow-cyan-500/20 transition hover:-translate-y-0.5 hover:bg-cyan-500"
        >AI</button>
      )}

      <div className={`fixed inset-y-0 right-0 z-50 w-full max-w-xl transform border-l border-slate-200 bg-white/96 shadow-2xl shadow-slate-950/10 backdrop-blur transition duration-300 dark:border-slate-800 dark:bg-slate-950/96 ${state.open ? 'translate-x-0' : 'translate-x-full'}`}>
        <div className="flex h-full flex-col">
          <div className="border-b border-slate-200 bg-gradient-to-r from-cyan-500 via-sky-500 to-slate-900 p-5 text-white dark:border-slate-800">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-[11px] font-semibold uppercase tracking-[0.3em] text-cyan-100">Platform Copilot</div>
                <h2 className="mt-2 text-2xl font-semibold">Ask for SQL, pipeline steps, or ontology hints</h2>
                <p className="mt-2 max-w-md text-sm text-cyan-50/90">The drawer stays available across the app and routes requests through the AIP backend.</p>
              </div>
              <button type="button" onClick={() => copilot.close()} className="rounded-full border border-white/30 px-3 py-1 text-sm font-medium text-white transition hover:bg-white/10">Close</button>
            </div>
          </div>

          <div className="flex-1 space-y-5 overflow-y-auto p-5">
            <label className="block space-y-2">
              <span className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Question</span>
              <textarea
                rows={5}
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                className="w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-cyan-500 focus:ring-2 focus:ring-cyan-200 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-100"
                placeholder="Explain the failing workflow, ask for starter SQL, or request ontology mapping help."
              />
            </label>

            <div className="grid gap-4 lg:grid-cols-2">
              <CheckList title="Datasets" items={datasets.slice(0, 6).map((d) => ({ id: d.id, label: d.name }))} selected={selectedDatasetIds} onToggle={(id) => setSelectedDatasetIds((prev) => toggle(prev, id))} emptyMessage="No datasets available." />
              <CheckList title="Knowledge Bases" items={knowledgeBases.map((k) => ({ id: k.id, label: k.name }))} selected={selectedKnowledgeBaseIds} onToggle={(id) => setSelectedKnowledgeBaseIds((prev) => toggle(prev, id))} emptyMessage="No knowledge bases available." />
            </div>

            <div className="grid gap-3 rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900/70">
              <label className="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={includeSql} onChange={() => setIncludeSql(!includeSql)} />
                <span>Include starter SQL</span>
              </label>
              <label className="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={includePipelinePlan} onChange={() => setIncludePipelinePlan(!includePipelinePlan)} />
                <span>Include pipeline suggestions</span>
              </label>
            </div>

            <button type="button" onClick={() => void submit()} disabled={loading} className="inline-flex w-full items-center justify-center rounded-2xl bg-slate-950 px-4 py-3 text-sm font-semibold text-white transition hover:bg-cyan-500 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">
              {loading ? 'Thinking...' : 'Ask Copilot'}
            </button>

            {error && <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-200">{error}</div>}

            {response && (
              <div className="space-y-4 rounded-[28px] border border-slate-200 bg-gradient-to-br from-slate-50 to-cyan-50 p-5 dark:border-slate-800 dark:from-slate-900 dark:to-slate-950">
                <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                  <span className="rounded-full bg-white/80 px-2.5 py-1 font-medium dark:bg-slate-800">{response.provider_name}</span>
                  <span className="rounded-full bg-white/80 px-2.5 py-1 font-medium dark:bg-slate-800">Cache {response.cache.hit ? 'hit' : 'miss'}</span>
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Answer</div>
                  <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-slate-700 dark:text-slate-200">{response.answer}</p>
                </div>
                {response.suggested_sql && (
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Suggested SQL</div>
                    <pre className="mt-2 overflow-x-auto rounded-2xl bg-slate-950 px-4 py-3 text-xs text-cyan-100">{response.suggested_sql}</pre>
                  </div>
                )}
                {response.pipeline_suggestions.length > 0 && (
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Pipeline Suggestions</div>
                    <ul className="mt-2 space-y-2 text-sm text-slate-700 dark:text-slate-200">
                      {response.pipeline_suggestions.map((s, i) => (
                        <li key={i} className="rounded-2xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-900">{s}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {response.ontology_hints.length > 0 && (
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Ontology Hints</div>
                    <ul className="mt-2 space-y-2 text-sm text-slate-700 dark:text-slate-200">
                      {response.ontology_hints.map((h, i) => (
                        <li key={i} className="rounded-2xl border border-dashed border-cyan-200 bg-white px-3 py-2 dark:border-cyan-900 dark:bg-slate-900">{h}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  );
}

function CheckList({ title, items, selected, onToggle, emptyMessage }: { title: string; items: Array<{ id: string; label: string }>; selected: string[]; onToggle: (id: string) => void; emptyMessage: string }) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
      <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">{title}</div>
      <div className="mt-3 space-y-2">
        {items.length === 0 ? (
          <p className="text-sm text-slate-500">{emptyMessage}</p>
        ) : (
          items.map((item) => (
            <label key={item.id} className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
              <input type="checkbox" checked={selected.includes(item.id)} onChange={() => onToggle(item.id)} />
              <span>{item.label}</span>
            </label>
          ))
        )}
      </div>
    </div>
  );
}
