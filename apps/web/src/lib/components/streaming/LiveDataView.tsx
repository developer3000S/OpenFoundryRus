import type { ConnectorCatalogEntry, LiveTailResponse, TopologyRuntimeSnapshot } from '@/lib/api/streaming';

interface Props {
  connectors: ConnectorCatalogEntry[];
  liveTail: LiveTailResponse | null;
  runtime: TopologyRuntimeSnapshot | null;
}

export function LiveDataView({ connectors, liveTail, runtime }: Props) {
  const aggregates = runtime?.latest_run?.aggregate_windows ?? runtime?.preview?.aggregate_windows ?? [];
  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div>
        <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Live Tail</div>
        <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Connector health, live events, and CEP pattern matches</h2>
      </div>
      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.88fr)_minmax(0,1.12fr)]">
        <div className="space-y-4">
          <div>
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Connectors</div>
            <div className="mt-3 space-y-3">
              {connectors.map((c, i) => (
                <div key={i} className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200">
                  <div className="font-semibold">{c.connector_type} • {c.direction}</div>
                  <div className="mt-1 text-xs text-slate-500">{c.endpoint} • backlog {c.backlog} • {c.throughput_per_second.toFixed(0)}/s</div>
                </div>
              ))}
            </div>
          </div>
          {(runtime?.latest_run || runtime?.preview) && (
            <div className="rounded-[24px] border border-dashed border-cyan-300 bg-cyan-50/60 p-4 dark:border-cyan-900 dark:bg-cyan-950/20">
              <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-cyan-700 dark:text-cyan-300">Latest Aggregates</div>
              <div className="mt-3 space-y-2">
                {aggregates.map((a, i) => (
                  <div key={i} className="rounded-2xl border border-cyan-200 bg-white px-4 py-3 text-sm text-slate-700 dark:border-cyan-900 dark:bg-slate-950 dark:text-slate-200">
                    {a.window_name} • {a.group_key} • {a.measure_name} = {a.value}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
        <div className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[24px] border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Events</div>
            <div className="mt-3 space-y-3">
              {!liveTail || liveTail.events.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No live events captured yet.</div>
              ) : (
                liveTail.events.map((event, i) => (
                  <div key={i} className="rounded-2xl border border-slate-200 bg-white px-4 py-3 dark:border-slate-800 dark:bg-slate-950">
                    <div className="flex items-center justify-between gap-3 text-sm text-slate-700 dark:text-slate-200">
                      <span>{event.stream_name}</span>
                      <span className="text-xs text-slate-500">{new Date(event.processing_time).toLocaleTimeString()}</span>
                    </div>
                    <pre className="mt-2 overflow-x-auto whitespace-pre-wrap text-xs text-slate-600 dark:text-slate-300">{JSON.stringify(event.payload, null, 2)}</pre>
                  </div>
                ))
              )}
            </div>
          </div>
          <div className="rounded-[24px] border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
            <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">CEP Matches</div>
            <div className="mt-3 space-y-3">
              {!liveTail || liveTail.matches.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-slate-300 px-4 py-5 text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">No pattern matches yet.</div>
              ) : (
                liveTail.matches.map((m, i) => (
                  <div key={i} className="rounded-2xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-700 dark:border-slate-800 dark:bg-slate-950 dark:text-slate-200">
                    <div className="font-semibold">{m.pattern_name}</div>
                    <div className="mt-1 text-xs text-slate-500">{m.matched_sequence.join(' → ')} • confidence {m.confidence.toFixed(2)}</div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
