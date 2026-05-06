import { useEffect, useRef, useState } from 'react';

import { getMediaSetUsage, type MediaSet, type UsageBucketByKind, type UsageResponse } from '@/lib/api/mediaSets';

interface Props {
  mediaSet: MediaSet;
}

const PALETTE = ['#0f766e', '#0369a1', '#c2410c', '#7c3aed', '#be123c', '#15803d', '#9333ea', '#0891b2'];

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MiB`;
  return `${(n / 1024 ** 3).toFixed(2)} GiB`;
}

function hitRate(b: UsageBucketByKind) {
  if (b.invocations === 0) return '—';
  return `${((b.cache_hits / b.invocations) * 100).toFixed(0)}%`;
}

function shape(usage: UsageResponse) {
  const days = Array.from(new Set(usage.by_day_kind.map((p) => p.day))).sort();
  const kinds = Array.from(new Set(usage.by_day_kind.map((p) => p.kind))).sort();
  const seriesByKind: Record<string, number[]> = {};
  for (const kind of kinds) {
    seriesByKind[kind] = days.map((day) => {
      const hit = usage.by_day_kind.find((p) => p.day === day && p.kind === kind);
      return hit ? hit.compute_seconds : 0;
    });
  }
  const totalsPerDay = days.map((day) => kinds.reduce((acc, kind) => {
    const hit = usage.by_day_kind.find((p) => p.day === day && p.kind === kind);
    return acc + (hit ? hit.compute_seconds : 0);
  }, 0));
  return { days, kinds, seriesByKind, totalsPerDay };
}

export function MediaSetUsagePanel({ mediaSet }: Props) {
  const [usage, setUsage] = useState<UsageResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const lineRef = useRef<HTMLDivElement | null>(null);
  const stackRef = useRef<HTMLDivElement | null>(null);
  const lineChart = useRef<{ setOption: (o: unknown, replace?: boolean) => void; dispose: () => void } | null>(null);
  const stackChart = useRef<{ setOption: (o: unknown, replace?: boolean) => void; dispose: () => void } | null>(null);

  async function load() {
    setLoading(true);
    setError('');
    try {
      setUsage(await getMediaSetUsage(mediaSet.rid));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load usage');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    return () => {
      lineChart.current?.dispose();
      stackChart.current?.dispose();
      lineChart.current = null;
      stackChart.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaSet.rid]);

  useEffect(() => {
    if (!usage || !lineRef.current || !stackRef.current) return;
    let cancelled = false;
    (async () => {
      const echarts = await import('echarts');
      if (cancelled) return;
      const { days, kinds, seriesByKind, totalsPerDay } = shape(usage);
      if (!lineChart.current) lineChart.current = echarts.init(lineRef.current!) as unknown as typeof lineChart.current;
      lineChart.current!.setOption({
        color: PALETTE,
        tooltip: { trigger: 'axis' },
        legend: { top: 0, textStyle: { color: '#64748b' } },
        grid: { left: 40, right: 16, top: 28, bottom: 30 },
        xAxis: { type: 'category', data: days },
        yAxis: { type: 'value', name: 'compute-seconds' },
        series: [{ name: 'Compute-seconds (all kinds)', type: 'line', smooth: true, data: totalsPerDay }],
      }, true);
      if (!stackChart.current) stackChart.current = echarts.init(stackRef.current!) as unknown as typeof stackChart.current;
      stackChart.current!.setOption({
        color: PALETTE,
        tooltip: { trigger: 'axis' },
        legend: { top: 0, textStyle: { color: '#64748b' } },
        grid: { left: 40, right: 16, top: 28, bottom: 30 },
        xAxis: { type: 'category', data: days },
        yAxis: { type: 'value', name: 'compute-seconds' },
        series: kinds.map((kind) => ({ name: kind, type: 'bar', stack: 'kinds', emphasis: { focus: 'series' }, data: seriesByKind[kind] })),
      }, true);
    })();
    return () => { cancelled = true; };
  }, [usage]);

  return (
    <section className="space-y-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Usage</h2>
          <p className="mt-1 text-xs text-slate-500">
            Compute-seconds and bytes processed by access patterns over the last 30 days. Rates follow the Foundry "Media usage costs and limits" table.
          </p>
        </div>
        <button type="button" onClick={() => void load()} disabled={loading} className="rounded-xl border border-slate-200 px-3 py-1.5 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800">
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </header>

      {error ? (
        <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
      ) : loading && !usage ? (
        <div className="h-48 animate-pulse rounded-xl border border-slate-200 bg-slate-100 dark:border-gray-700 dark:bg-gray-800" />
      ) : usage ? (
        <>
          <div className="grid gap-3 md:grid-cols-3">
            <div className="rounded-2xl border border-slate-200 p-4 dark:border-gray-700">
              <div className="text-xs uppercase tracking-[0.18em] text-slate-400">Total compute-seconds</div>
              <div className="mt-1 text-2xl font-semibold tabular-nums">{usage.total_compute_seconds.toLocaleString()}</div>
            </div>
            <div className="rounded-2xl border border-slate-200 p-4 dark:border-gray-700">
              <div className="text-xs uppercase tracking-[0.18em] text-slate-400">Total bytes processed</div>
              <div className="mt-1 text-2xl font-semibold tabular-nums">{formatBytes(usage.total_input_bytes)}</div>
            </div>
            <div className="rounded-2xl border border-slate-200 p-4 dark:border-gray-700">
              <div className="text-xs uppercase tracking-[0.18em] text-slate-400">Window</div>
              <div className="mt-1 text-sm">{new Date(usage.since).toLocaleDateString()} – {new Date(usage.until).toLocaleDateString()}</div>
            </div>
          </div>

          <div className="rounded-2xl border border-slate-200 p-4 dark:border-gray-700">
            <div className="text-xs uppercase tracking-[0.18em] text-slate-400">Compute-seconds per day</div>
            <div ref={lineRef} className="mt-2 h-56 w-full" />
          </div>
          <div className="rounded-2xl border border-slate-200 p-4 dark:border-gray-700">
            <div className="text-xs uppercase tracking-[0.18em] text-slate-400">Per-kind breakdown</div>
            <div ref={stackRef} className="mt-2 h-56 w-full" />
          </div>

          <div className="overflow-hidden rounded-2xl border border-slate-200 dark:border-gray-700">
            <table className="min-w-full divide-y divide-slate-200 text-sm dark:divide-gray-700">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500 dark:bg-gray-800/40">
                <tr>
                  <th className="px-3 py-2 text-left">Kind</th>
                  <th className="px-3 py-2 text-right">Invocations</th>
                  <th className="px-3 py-2 text-right">Cache hits</th>
                  <th className="px-3 py-2 text-right">Hit rate</th>
                  <th className="px-3 py-2 text-right">Compute-seconds</th>
                  <th className="px-3 py-2 text-right">Input bytes</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-gray-800">
                {usage.by_kind.length === 0 ? (
                  <tr><td colSpan={6} className="px-3 py-6 text-center text-slate-400">No invocations recorded in this window.</td></tr>
                ) : (
                  usage.by_kind.map((b) => (
                    <tr key={b.kind} data-kind={b.kind}>
                      <td className="px-3 py-2 font-mono">{b.kind}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{b.invocations}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{b.cache_hits}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{hitRate(b)}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{b.compute_seconds.toLocaleString()}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{formatBytes(b.input_bytes)}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      ) : null}
    </section>
  );
}
