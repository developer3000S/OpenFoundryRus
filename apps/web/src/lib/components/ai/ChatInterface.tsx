import { useEffect, useState } from 'react';

import type {
  ChatCompletionResponse,
  Conversation,
  ConversationSummary,
  KnowledgeBase,
  LlmProvider,
  ProviderBenchmarkResponse,
  PromptTemplate,
} from '@/lib/api/ai';

export interface ChatDraft {
  conversation_id: string;
  user_message: string;
  system_prompt: string;
  prompt_template_id: string;
  prompt_variables_text: string;
  knowledge_base_id: string;
  preferred_provider_id: string;
  attachments_text: string;
  max_tokens: number;
  fallback_enabled: boolean;
  require_private_network: boolean;
}

interface Props {
  conversations: ConversationSummary[];
  conversation: Conversation | null;
  providers: LlmProvider[];
  prompts: PromptTemplate[];
  knowledgeBases: KnowledgeBase[];
  draft: ChatDraft;
  response: ChatCompletionResponse | null;
  benchmarkResponse?: ProviderBenchmarkResponse | null;
  busy?: boolean;
  onSelectConversation?: (conversationId: string) => void;
  onDraftChange?: (draft: ChatDraft) => void;
  onSend?: () => void;
  onBenchmark?: () => void;
  onResetConversation?: () => void;
}

export function ChatInterface({ conversations, conversation, providers, prompts, knowledgeBases, draft, response, benchmarkResponse = null, busy = false, onSelectConversation, onDraftChange, onSend, onBenchmark, onResetConversation }: Props) {
  const [localDraft, setLocalDraft] = useState<ChatDraft>(draft);
  useEffect(() => { setLocalDraft(draft); }, [draft]);

  function update<K extends keyof ChatDraft>(key: K, value: ChatDraft[K]) {
    const next = { ...localDraft, [key]: value };
    setLocalDraft(next);
    onDraftChange?.(next);
  }

  return (
    <section className="rounded-[32px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Chat Workspace</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Conversations with prompt, provider, and retrieval controls</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => onResetConversation?.()} disabled={busy} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900">New conversation</button>
          <button type="button" onClick={() => onBenchmark?.()} disabled={busy} className="rounded-full border border-cyan-300 px-3 py-1.5 text-sm text-cyan-700 hover:bg-cyan-50 dark:border-cyan-800 dark:text-cyan-300 dark:hover:bg-cyan-950/40">Benchmark</button>
          <button type="button" onClick={() => onSend?.()} disabled={busy} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Send</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.75fr)_minmax(0,1.25fr)]">
        <div className="space-y-3">
          {conversations.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No conversations yet.</div>
          ) : (
            conversations.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => onSelectConversation?.(item.id)}
                className={`w-full rounded-2xl border px-4 py-3 text-left transition ${conversation?.id === item.id ? 'border-cyan-400 bg-cyan-50 dark:border-cyan-700 dark:bg-cyan-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
              >
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{item.title}</div>
                <div className="mt-1 text-xs text-slate-500">{item.message_count} messages • {item.last_cache_hit ? 'cache hit' : 'fresh'}</div>
                <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">{item.last_message_preview}</p>
              </button>
            ))
          )}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-4">
            <select className={inputCls} value={localDraft.prompt_template_id} onChange={(e) => update('prompt_template_id', e.target.value)}>
              <option value="">No prompt template</option>
              {prompts.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            <select className={inputCls} value={localDraft.knowledge_base_id} onChange={(e) => update('knowledge_base_id', e.target.value)}>
              <option value="">No knowledge base</option>
              {knowledgeBases.map((kb) => <option key={kb.id} value={kb.id}>{kb.name}</option>)}
            </select>
            <select className={inputCls} value={localDraft.preferred_provider_id} onChange={(e) => update('preferred_provider_id', e.target.value)}>
              <option value="">Automatic routing</option>
              {providers.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            <input type="number" className={inputCls} value={String(localDraft.max_tokens)} onChange={(e) => update('max_tokens', Number(e.target.value) || 512)} />
          </div>

          <textarea className={`${inputCls} h-20`} value={localDraft.system_prompt} onChange={(e) => update('system_prompt', e.target.value)} />
          <textarea className={`${inputCls} h-20`} value={localDraft.prompt_variables_text} onChange={(e) => update('prompt_variables_text', e.target.value)} />
          <textarea className="h-28 rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950" value={localDraft.user_message} onChange={(e) => update('user_message', e.target.value)} />
          <textarea className={`${inputCls} h-24`} value={localDraft.attachments_text} onChange={(e) => update('attachments_text', e.target.value)} />

          <div className="grid gap-3 lg:grid-cols-2">
            <label className="flex items-center gap-3 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
              <input type="checkbox" checked={localDraft.fallback_enabled} onChange={() => update('fallback_enabled', !localDraft.fallback_enabled)} />
              <span>Enable fallback routing</span>
            </label>
            <label className="flex items-center gap-3 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
              <input type="checkbox" checked={localDraft.require_private_network} onChange={() => update('require_private_network', !localDraft.require_private_network)} />
              <span>Private network only</span>
            </label>
          </div>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
            <div className="rounded-[24px] border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Transcript</div>
              <div className="mt-3 max-h-[28rem] space-y-3 overflow-y-auto">
                {conversation?.messages.length ? (
                  conversation.messages.map((message, i) => (
                    <div key={i} className={`rounded-2xl px-4 py-3 ${message.role === 'assistant' ? 'bg-cyan-50 text-slate-700 dark:bg-cyan-950/20 dark:text-slate-200' : 'bg-white text-slate-700 dark:bg-slate-950 dark:text-slate-200'}`}>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">{message.role}</div>
                      <p className="mt-2 whitespace-pre-wrap text-sm leading-6">{message.content}</p>
                      {message.attachments.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-500 dark:text-slate-400">
                          {message.attachments.map((a, j) => (
                            <span key={j} className="rounded-full bg-white/80 px-2 py-1 dark:bg-slate-900">{a.kind}{a.name ? ` · ${a.name}` : ''}</span>
                          ))}
                        </div>
                      )}
                      {message.citations.length > 0 && (
                        <div className="mt-3 flex flex-wrap gap-2 text-xs text-cyan-700 dark:text-cyan-300">
                          {message.citations.map((c, j) => (
                            <span key={j} className="rounded-full bg-white/80 px-2 py-1 dark:bg-slate-900">{c.document_title}</span>
                          ))}
                        </div>
                      )}
                    </div>
                  ))
                ) : (
                  <p className="text-sm text-slate-500">Start a conversation to see the transcript.</p>
                )}
              </div>
            </div>

            <div className="space-y-4 rounded-[24px] border border-slate-200 bg-gradient-to-br from-slate-50 to-cyan-50 p-4 dark:border-slate-800 dark:from-slate-900 dark:to-slate-950">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Latest Response</div>
              {response ? (
                <div className="space-y-3 text-sm text-slate-700 dark:text-slate-200">
                  <div className="rounded-2xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-950">Provider: {response.provider_name}</div>
                  <div className="rounded-2xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-950">Cache: {response.cache.hit ? 'hit' : 'miss'} • {response.usage.total_tokens} tokens</div>
                  <div className="rounded-2xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-950">Cost: ${response.usage.estimated_cost_usd.toFixed(4)} • {response.usage.latency_ms} ms • {response.usage.network_scope}</div>
                  <div className="rounded-2xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-950">Routing: {response.routing.used_private_network ? 'private' : 'standard'} • {response.routing.required_modalities.join(', ')}</div>
                  <div className="rounded-2xl border border-slate-200 bg-white px-3 py-3 dark:border-slate-800 dark:bg-slate-950">
                    <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Guardrails</div>
                    <p className="mt-2">{response.guardrail.blocked ? 'Blocked' : 'Passed'} • {response.guardrail.flags.length} flags</p>
                  </div>
                </div>
              ) : (
                <p className="text-sm text-slate-500">Response metadata appears here after sending.</p>
              )}
              {benchmarkResponse && (
                <div className="rounded-2xl border border-cyan-200 bg-white px-3 py-3 dark:border-cyan-900 dark:bg-slate-950">
                  <div className="text-xs font-semibold uppercase tracking-[0.24em] text-slate-500">Benchmark</div>
                  <div className="mt-2 space-y-2 text-xs text-slate-600 dark:text-slate-300">
                    {benchmarkResponse.results.map((result, i) => (
                      <div key={i} className="rounded-2xl border border-slate-200 px-3 py-2 dark:border-slate-800">
                        <div className="font-semibold text-slate-900 dark:text-slate-100">{result.provider_name} • {(result.score.overall * 100).toFixed(0)}%</div>
                        <div className="mt-1">{result.error ?? `${result.latency_ms} ms • $${result.estimated_cost_usd.toFixed(4)} • ${result.network_scope}`}</div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

const inputCls = 'rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-900';
