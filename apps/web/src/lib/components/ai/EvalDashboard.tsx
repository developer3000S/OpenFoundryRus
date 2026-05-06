import type { AiPlatformOverview, EvaluateGuardrailsResponse, LlmProvider } from '@/lib/api/ai';

interface Props {
  overview: AiPlatformOverview | null;
  providers: LlmProvider[];
  guardrailInput: string;
  guardrailResponse: EvaluateGuardrailsResponse | null;
  busy?: boolean;
  onGuardrailInputChange?: (value: string) => void;
  onEvaluate?: () => void;
}

function statusBadge(status: string) {
  if (status === 'healthy') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300';
  if (status === 'degraded') return 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300';
  return 'bg-rose-100 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300';
}

export function EvalDashboard({ overview, providers, guardrailInput, guardrailResponse, busy = false, onGuardrailInputChange, onEvaluate }: Props) {
  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Evaluation</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Provider health, cache efficiency, and guardrails</h2>
        </div>
        <button type="button" onClick={() => onEvaluate?.()} disabled={busy} className="rounded-full border border-cyan-300 px-3 py-1.5 text-sm text-cyan-700 hover:bg-cyan-50 dark:border-cyan-800 dark:text-cyan-300 dark:hover:bg-cyan-950/40">Evaluate guardrails</button>
      </div>

      <div className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-6">
        <div className="rounded-2xl bg-slate-950 px-4 py-4 text-white">
          <div className="text-[11px] uppercase tracking-[0.24em] text-slate-300">Providers</div>
          <div className="mt-2 text-3xl font-semibold">{overview?.provider_count ?? 0}</div>
        </div>
        {[
          ['Prompts', overview?.prompt_count ?? 0],
          ['KB Chunks', overview?.indexed_chunk_count ?? 0],
          ['Agents', overview?.agent_count ?? 0],
          ['Cache Hit Rate', overview ? `${Math.round(overview.cache_hit_rate * 100)}%` : '0%'],
          ['LLM Cost', `$${(overview?.estimated_llm_cost_usd ?? 0).toFixed(2)}`],
        ].map(([label, value]) => (
          <div key={label as string} className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] uppercase tracking-[0.24em] text-slate-500">{label}</div>
            <div className="mt-2 text-3xl font-semibold text-slate-900 dark:text-slate-100">{value}</div>
            {label === 'LLM Cost' && (
              <div className="mt-1 text-xs text-slate-500">{overview?.benchmark_run_count ?? 0} benchmarks</div>
            )}
          </div>
        ))}
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <div className="rounded-[24px] border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
          <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Provider Routing Health</div>
          <div className="mt-3 space-y-3">
            {providers.map((p) => (
              <div key={p.id} className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{p.name}</div>
                    <div className="mt-1 text-xs text-slate-500">{p.provider_type} • {p.model_name}</div>
                  </div>
                  <span className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] ${statusBadge(p.health_state.status)}`}>{p.health_state.status}</span>
                </div>
                <div className="mt-3 grid gap-2 text-sm text-slate-600 dark:text-slate-300 md:grid-cols-3">
                  <div>Latency: {p.health_state.avg_latency_ms} ms</div>
                  <div>Error rate: {(p.health_state.error_rate * 100).toFixed(1)}%</div>
                  <div>Weight: {p.route_rules.weight}</div>
                </div>
                <div className="mt-2 grid gap-2 text-xs text-slate-500 dark:text-slate-400 md:grid-cols-3">
                  <div>Network: {p.route_rules.network_scope}</div>
                  <div>Modalities: {p.route_rules.supported_modalities.join(', ')}</div>
                  <div>Cost: ${p.route_rules.input_cost_per_1k_tokens_usd.toFixed(4)} / ${p.route_rules.output_cost_per_1k_tokens_usd.toFixed(4)}</div>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="rounded-[24px] border border-slate-200 bg-gradient-to-br from-cyan-50 to-white p-4 dark:border-slate-800 dark:from-cyan-950/20 dark:to-slate-950">
          <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Guardrail Tester</div>
          <textarea className="mt-3 h-32 w-full rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm dark:border-slate-800 dark:bg-slate-950" value={guardrailInput} onChange={(e) => onGuardrailInputChange?.(e.target.value)} />
          {guardrailResponse && (
            <div className="mt-4 space-y-3 text-sm text-slate-700 dark:text-slate-200">
              <div className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">Risk score: {guardrailResponse.risk_score.toFixed(2)}</div>
              <div className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">Verdict: {guardrailResponse.verdict.blocked ? 'Blocked' : 'Passed'} • {guardrailResponse.verdict.flags.length} flags</div>
              {guardrailResponse.recommendations.length > 0 && (
                <ul className="space-y-2">
                  {guardrailResponse.recommendations.map((r, i) => (
                    <li key={i} className="rounded-2xl border border-dashed border-cyan-200 bg-white px-4 py-3 dark:border-cyan-900 dark:bg-slate-950">{r}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
