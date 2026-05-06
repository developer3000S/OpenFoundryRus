import { useEffect, useMemo, useRef, useState } from 'react';

import { executeAgent, type AgentExecutionResponse } from '@/lib/api/ai';
import type { AppWidget, WidgetEvent } from '@/lib/api/apps';
import { getDataset, type Dataset } from '@/lib/api/datasets';
import { evaluateObjectSet, listObjects, type ObjectInstance } from '@/lib/api/ontology';
import { executeQuery, type QueryResult } from '@/lib/api/queries';
import { MediaPreviewWidget } from '@/lib/components/app-builder/MediaPreviewWidget';
import { MediaUploaderWidget } from '@/lib/components/app-builder/MediaUploaderWidget';
import { ChartWidget } from '@/lib/components/dashboard/ChartWidget';
import { TableWidget } from '@/lib/components/dashboard/TableWidget';

interface Props {
  widget: AppWidget;
  globalFilter?: string;
  runtimeParameters?: Record<string, string>;
  interactivePromptSeed?: string;
  primaryInteractiveAgentWidgetId?: string | null;
  onAction?: (event: WidgetEvent, payload?: Record<string, unknown>) => Promise<void> | void;
}

interface FormField {
  name: string;
  label: string;
  type: string;
  options?: string[];
}

interface ScenarioParameter {
  name: string;
  label: string;
  type: string;
  default_value: string;
  description?: string;
}

function interpolate(template: string, params: Record<string, string>) {
  return template.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (_, key: string) => params[key] ?? '');
}

function parseFormFields(value: unknown): FormField[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((entry): entry is Record<string, unknown> => Boolean(entry && typeof entry === 'object'))
    .map((entry) => ({
      name: typeof entry.name === 'string' ? entry.name : 'field',
      label: typeof entry.label === 'string' ? entry.label : (typeof entry.name === 'string' ? entry.name : 'Field'),
      type: typeof entry.type === 'string' ? entry.type : 'text',
      options: Array.isArray(entry.options) ? entry.options.map(String) : undefined,
    }));
}

function parseScenarioParameters(value: unknown): ScenarioParameter[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((entry): entry is Record<string, unknown> => Boolean(entry && typeof entry === 'object'))
    .map((entry) => ({
      name: typeof entry.name === 'string' ? entry.name : 'parameter',
      label: typeof entry.label === 'string' ? entry.label : (typeof entry.name === 'string' ? entry.name : 'Parameter'),
      type: typeof entry.type === 'string' ? entry.type : 'text',
      default_value: typeof entry.default_value === 'string' ? entry.default_value : '',
      description: typeof entry.description === 'string' ? entry.description : undefined,
    }))
    .filter((entry) => entry.name.length > 0);
}

function stringifyValue(value: unknown) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

function objectsToQueryResult(objects: ObjectInstance[]): QueryResult {
  const fieldNames = Array.from(new Set(objects.flatMap((o) => Object.keys(o.properties ?? {}))));
  return {
    columns: ['id', ...fieldNames].map((name) => ({ name, data_type: 'text' })),
    rows: objects.map((o) => [o.id, ...fieldNames.map((f) => stringifyValue(o.properties?.[f]))]),
    total_rows: objects.length,
    execution_time_ms: 0,
  };
}

function datasetToQueryResult(d: Dataset): QueryResult {
  return {
    columns: [{ name: 'attribute', data_type: 'text' }, { name: 'value', data_type: 'text' }],
    rows: [
      ['name', d.name], ['format', d.format], ['rows', String(d.row_count)],
      ['version', String(d.current_version)], ['branch', d.active_branch],
      ['tags', d.tags.join(', ') || 'none'],
    ],
    total_rows: 6,
    execution_time_ms: 0,
  };
}

function objectSetRowsToQueryResult(rows: Record<string, unknown>[]): QueryResult {
  const fieldNames = Array.from(new Set(rows.flatMap((r) => Object.keys(r ?? {}))));
  return {
    columns: fieldNames.map((name) => ({ name, data_type: 'text' })),
    rows: rows.map((r) => fieldNames.map((f) => stringifyValue(r?.[f]))),
    total_rows: rows.length,
    execution_time_ms: 0,
  };
}

export function AppWidgetRenderer({
  widget,
  globalFilter = '',
  runtimeParameters = {},
  interactivePromptSeed = '',
  primaryInteractiveAgentWidgetId = null,
  onAction,
}: Props) {
  const [result, setResult] = useState<QueryResult | null>(null);
  const [dataset, setDataset] = useState<Dataset | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [formState, setFormState] = useState<Record<string, string>>({});
  const [scenarioState, setScenarioState] = useState<Record<string, string>>({});
  const [agentPrompt, setAgentPrompt] = useState('');
  const [agentBusy, setAgentBusy] = useState(false);
  const [agentError, setAgentError] = useState('');
  const [agentResponse, setAgentResponse] = useState<AgentExecutionResponse | null>(null);

  const initialScenarioFiredRef = useRef<string | null>(null);

  const stringProp = (key: string, fallback: string) => {
    const v = widget.props?.[key];
    return typeof v === 'string' ? v : fallback;
  };
  const numberProp = (key: string, fallback: number) => {
    const v = widget.props?.[key];
    if (typeof v === 'number' && Number.isFinite(v)) return v;
    if (typeof v === 'string') { const n = Number(v); return Number.isFinite(n) ? n : fallback; }
    return fallback;
  };
  const booleanProp = (key: string, fallback: boolean) => {
    const v = widget.props?.[key];
    return typeof v === 'boolean' ? v : fallback;
  };
  const arrayProp = (key: string) => {
    const v = widget.props?.[key];
    return Array.isArray(v) ? v : [];
  };

  const formFields = useMemo(() => parseFormFields(widget.props.fields), [widget.props.fields]);
  const scenarioParameters = useMemo(() => parseScenarioParameters(widget.props.parameters), [widget.props.parameters]);
  const content = interpolate(stringProp('content', ''), runtimeParameters);
  const imageUrl = interpolate(stringProp('url', ''), runtimeParameters);
  const imageAlt = stringProp('alt', widget.title);

  // Initialize form state from first row
  useEffect(() => {
    const firstRow = result?.rows[0] ?? [];
    setFormState(Object.fromEntries(formFields.map((f, i) => [f.name, firstRow[i] ?? ''])));
  }, [widget.id, formFields, result]);

  // Initialize scenario state on widget change
  useEffect(() => {
    setScenarioState(Object.fromEntries(scenarioParameters.map((p) => [p.name, p.default_value])));
    setAgentResponse(null);
    setAgentError('');
    setAgentPrompt('');
  }, [widget.id, scenarioParameters]);

  // Sync scenario state from runtime parameters
  useEffect(() => {
    if (widget.widget_type !== 'scenario' || scenarioParameters.length === 0) return;
    setScenarioState((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const p of scenarioParameters) {
        const v = runtimeParameters[p.name];
        if (v !== undefined && next[p.name] !== v) { next[p.name] = v; changed = true; }
      }
      return changed ? next : prev;
    });
  }, [widget.widget_type, scenarioParameters, runtimeParameters]);

  // Seed agent prompt
  useEffect(() => {
    if (widget.widget_type !== 'agent' || !interactivePromptSeed.trim()) return;
    if (primaryInteractiveAgentWidgetId && primaryInteractiveAgentWidgetId !== widget.id) return;
    if (agentPrompt.trim() === interactivePromptSeed.trim()) return;
    setAgentPrompt(interactivePromptSeed);
  }, [widget.widget_type, widget.id, interactivePromptSeed, primaryInteractiveAgentWidgetId, agentPrompt]);

  // Apply default scenario parameters once
  useEffect(() => {
    if (widget.widget_type !== 'scenario' || scenarioParameters.length === 0) return;
    if (initialScenarioFiredRef.current === widget.id) return;
    initialScenarioFiredRef.current = widget.id;
    const initialState = Object.fromEntries(scenarioParameters.map((p) => [p.name, p.default_value]));
    void onAction?.({
      id: `${widget.id}-scenario-initial`,
      trigger: 'scenario_change',
      action: 'set_parameters',
      label: 'Apply default scenario parameters',
      config: {},
    }, initialState);
  }, [widget.id, widget.widget_type, scenarioParameters, onAction]);

  // Load binding when widget or runtime parameters change
  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError('');
      setResult(null);
      setDataset(null);
      try {
        if (!widget.binding) return;
        if (widget.binding.source_type === 'query') {
          if (!widget.binding.query_text) throw new Error('Query binding requires SQL');
          const r = await executeQuery(interpolate(widget.binding.query_text, runtimeParameters), widget.binding.limit ?? 50);
          if (!cancelled) setResult(r);
          return;
        }
        if (widget.binding.source_type === 'ontology') {
          if (!widget.binding.source_id) throw new Error('Ontology binding requires an object type');
          const resp = await listObjects(widget.binding.source_id, { per_page: widget.binding.limit ?? 25 });
          if (!cancelled) setResult(objectsToQueryResult(resp.data));
          return;
        }
        if (widget.binding.source_type === 'object_set') {
          if (!widget.binding.source_id) throw new Error('Object set binding requires a saved object set');
          const resp = await evaluateObjectSet(widget.binding.source_id, { limit: widget.binding.limit ?? 25 });
          if (!cancelled) setResult(objectSetRowsToQueryResult(resp.rows));
          return;
        }
        if (widget.binding.source_type === 'dataset') {
          if (!widget.binding.source_id) throw new Error('Dataset binding requires a dataset');
          const ds = await getDataset(widget.binding.source_id);
          if (cancelled) return;
          setDataset(ds);
          setResult(datasetToQueryResult(ds));
        }
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Binding load failed');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [widget.id, widget.binding, runtimeParameters]);

  async function triggerEvents(trigger: string, payload?: Record<string, unknown>) {
    const events = widget.events.filter((e) => e.trigger === trigger);
    if (events.length === 0 && trigger === 'scenario_change') {
      await onAction?.({
        id: `${widget.id}-scenario-default`, trigger, action: 'set_parameters',
        label: 'Apply scenario parameters', config: {},
      }, payload);
      return;
    }
    for (const event of events) await onAction?.(event, payload);
  }

  async function runAgent() {
    setAgentBusy(true);
    setAgentError('');
    setAgentResponse(null);
    try {
      const agentId = stringProp('agent_id', '').trim();
      if (!agentId) throw new Error('Select an agent before running this widget');
      if (!agentPrompt.trim()) throw new Error('Enter a prompt for the embedded agent');
      const ctx = Object.entries(runtimeParameters).map(([k, v]) => `- ${k}: ${v}`).join('\n');
      const finalPrompt = booleanProp('include_runtime_context', true) && ctx
        ? `${stringProp('runtime_context_intro', 'Current scenario context:')}\n${ctx}\n\nUser request:\n${agentPrompt.trim()}`
        : agentPrompt.trim();
      const resp = await executeAgent(agentId, {
        user_message: finalPrompt,
        knowledge_base_id: stringProp('knowledge_base_id', '').trim() || undefined,
      });
      setAgentResponse(resp);
    } catch (cause) {
      setAgentError(cause instanceof Error ? cause.message : 'Agent execution failed');
    } finally {
      setAgentBusy(false);
    }
  }

  function scenarioDelta(p: ScenarioParameter) {
    const c = Number(scenarioState[p.name] ?? p.default_value);
    const b = Number(p.default_value);
    if (!Number.isFinite(c) || !Number.isFinite(b)) return null;
    return c - b;
  }

  const chartWidget = {
    id: widget.id,
    type: 'chart' as const,
    title: widget.title,
    description: widget.description,
    layout: { colSpan: 1, rowSpan: 1 },
    query: { sql: '', limit: numberProp('limit', 50) },
    chartType: (['bar', 'line', 'area', 'pie', 'scatter'].includes(stringProp('chart_type', 'line')) ? stringProp('chart_type', 'line') : 'line') as 'bar' | 'line' | 'area' | 'pie' | 'scatter',
    categoryColumn: stringProp('x_field', ''),
    seriesColumns: arrayProp('series_fields').map(String).filter(Boolean).length > 0
      ? arrayProp('series_fields').map(String).filter(Boolean)
      : [stringProp('y_field', '')].filter(Boolean),
    stacked: booleanProp('stacked', false),
  };

  const tableWidget = {
    id: widget.id,
    type: 'table' as const,
    title: widget.title,
    description: widget.description,
    layout: { colSpan: 1, rowSpan: 1 },
    query: { sql: '', limit: numberProp('limit', 50) },
    columns: arrayProp('columns')
      .filter((c): c is Record<string, unknown> => Boolean(c && typeof c === 'object'))
      .map((c) => ({
        key: typeof c.key === 'string' ? c.key : '',
        label: typeof c.label === 'string' ? c.label : (typeof c.key === 'string' ? c.key : ''),
      }))
      .filter((c) => c.key.length > 0),
    pageSize: Math.max(1, numberProp('page_size', 10)),
    defaultSortColumn: stringProp('default_sort_column', (arrayProp('columns').find((c): c is Record<string, unknown> => Boolean(c && typeof c === 'object' && typeof c.key === 'string'))?.key as string | undefined) ?? result?.columns[0]?.name ?? ''),
    defaultSortDirection: stringProp('default_sort_direction', 'asc') === 'desc' ? 'desc' as const : 'asc' as const,
  };

  // Map points
  const mapPoints = useMemo(() => {
    if (!result) return [] as Array<{ x: number; y: number; label: string }>;
    const latField = typeof widget.props.latitude_field === 'string' ? widget.props.latitude_field : 'lat';
    const lonField = typeof widget.props.longitude_field === 'string' ? widget.props.longitude_field : 'lon';
    const labelField = typeof widget.props.label_field === 'string' ? widget.props.label_field : result.columns[0]?.name;
    const li = result.columns.findIndex((c) => c.name === latField);
    const oi = result.columns.findIndex((c) => c.name === lonField);
    const lbi = result.columns.findIndex((c) => c.name === labelField);
    if (li < 0 || oi < 0) return [];
    return result.rows
      .map((row) => {
        const lat = Number(row[li]); const lon = Number(row[oi]);
        if (!Number.isFinite(lat) || !Number.isFinite(lon)) return null;
        return {
          x: ((lon + 180) / 360) * 100,
          y: ((90 - lat) / 180) * 100,
          label: lbi >= 0 ? String(row[lbi]) : `${lat.toFixed(2)}, ${lon.toFixed(2)}`,
        };
      })
      .filter((p): p is { x: number; y: number; label: string } => Boolean(p));
  }, [result, widget.props]);

  const contentLines = content.split('\n');

  return (
    <article className="flex h-full min-h-[160px] flex-col rounded-[24px] border border-slate-200 bg-white p-4 shadow-sm">
      <header className="mb-3 flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold text-slate-950">{widget.title}</h3>
            <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[11px] uppercase tracking-[0.2em] text-slate-500">{widget.widget_type}</span>
          </div>
          {widget.description && <p className="mt-1 text-sm text-slate-500">{widget.description}</p>}
        </div>
        {widget.binding && (
          <span className="rounded-full border border-slate-200 px-2 py-1 text-[11px] uppercase tracking-[0.2em] text-slate-400">{widget.binding.source_type}</span>
        )}
      </header>

      {error && <div className="mb-3 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</div>}

      {loading ? (
        <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed border-slate-300 text-sm text-slate-500">Loading binding data...</div>
      ) : widget.widget_type === 'text' ? (
        <div className="flex-1 space-y-2 text-slate-700">
          {contentLines.map((line, i) => {
            if (line.startsWith('### ')) return <h4 key={i} className="text-lg font-semibold">{line.slice(4)}</h4>;
            if (line.startsWith('## ')) return <h3 key={i} className="text-2xl font-semibold">{line.slice(3)}</h3>;
            if (line.startsWith('# ')) return <h2 key={i} className="text-3xl font-semibold">{line.slice(2)}</h2>;
            return <p key={i} className="whitespace-pre-wrap text-sm leading-6 text-slate-600">{line}</p>;
          })}
        </div>
      ) : widget.widget_type === 'image' ? (
        <div className="flex flex-1 items-center justify-center overflow-hidden rounded-2xl bg-slate-100">
          {imageUrl ? <img src={imageUrl} alt={imageAlt} className="h-full w-full object-cover" /> : <div className="text-sm text-slate-500">Add an image URL</div>}
        </div>
      ) : widget.widget_type === 'button' ? (
        <div className="flex flex-1 items-center justify-center">
          <button type="button" onClick={() => void triggerEvents('click')} className="rounded-2xl bg-slate-900 px-5 py-3 text-sm font-medium text-white">
            {interpolate(stringProp('label', widget.title || 'Run action'), runtimeParameters)}
          </button>
        </div>
      ) : widget.widget_type === 'form' ? (
        <form className="grid flex-1 gap-3" onSubmit={(e) => { e.preventDefault(); void triggerEvents('submit', formState); }}>
          {formFields.map((field) => (
            <label key={field.name} className="space-y-1 text-sm">
              <span className="font-medium text-slate-700">{field.label}</span>
              {field.type === 'textarea' ? (
                <textarea rows={3} value={formState[field.name] ?? ''} onChange={(e) => setFormState((s) => ({ ...s, [field.name]: e.target.value }))} className="w-full rounded-xl border border-slate-200 px-3 py-2" />
              ) : field.type === 'select' ? (
                <select value={formState[field.name] ?? ''} onChange={(e) => setFormState((s) => ({ ...s, [field.name]: e.target.value }))} className="w-full rounded-xl border border-slate-200 px-3 py-2">
                  <option value="">Select...</option>
                  {(field.options ?? []).map((opt) => <option key={opt} value={opt}>{opt}</option>)}
                </select>
              ) : (
                <input type={field.type} value={formState[field.name] ?? ''} onChange={(e) => setFormState((s) => ({ ...s, [field.name]: e.target.value }))} className="w-full rounded-xl border border-slate-200 px-3 py-2" />
              )}
            </label>
          ))}
          <div className="pt-2">
            <button type="submit" className="rounded-xl bg-[var(--app-primary,#0f766e)] px-4 py-2 text-sm font-medium text-white">{stringProp('submit_label', 'Submit')}</button>
          </div>
        </form>
      ) : widget.widget_type === 'scenario' ? (
        <form className="flex h-full flex-col gap-4" onSubmit={(e) => { e.preventDefault(); void triggerEvents('scenario_change', scenarioState); }}>
          <div>
            <div className="text-xs uppercase tracking-[0.22em] text-slate-400">Scenario / what-if</div>
            <div className="mt-1 text-sm text-slate-600">{stringProp('headline', 'Scenario controls')}</div>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            {scenarioParameters.map((p) => {
              const delta = scenarioDelta(p);
              return (
                <label key={p.name} className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm">
                  <span className="block font-medium text-slate-700">{p.label}</span>
                  {p.description && <span className="mt-1 block text-xs text-slate-500">{p.description}</span>}
                  <input type={p.type} value={scenarioState[p.name] ?? p.default_value} onChange={(e) => setScenarioState((s) => ({ ...s, [p.name]: e.target.value }))} className="mt-3 w-full rounded-xl border border-slate-200 bg-white px-3 py-2" />
                  {delta !== null && (
                    <div className={`mt-2 text-xs ${delta >= 0 ? 'text-emerald-700' : 'text-rose-700'}`}>Delta {delta.toFixed(2)}</div>
                  )}
                </label>
              );
            })}
          </div>
          <div className="mt-auto flex flex-wrap gap-3">
            <button type="submit" className="rounded-xl bg-[var(--app-primary,#0f766e)] px-4 py-2 text-sm font-medium text-white">{stringProp('apply_label', 'Apply scenario')}</button>
            <button type="button" onClick={() => {
              const next = Object.fromEntries(scenarioParameters.map((p) => [p.name, p.default_value]));
              setScenarioState(next);
              void onAction?.({
                id: `${widget.id}-scenario-reset`, trigger: 'scenario_change', action: 'clear_parameters',
                label: 'Reset scenario parameters', config: {},
              }, next);
            }} className="rounded-xl border border-slate-200 px-4 py-2 text-sm">{stringProp('reset_label', 'Reset')}</button>
          </div>
          {stringProp('summary_template', '').trim() && (
            <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
              {interpolate(stringProp('summary_template', ''), scenarioState)}
            </div>
          )}
        </form>
      ) : widget.widget_type === 'agent' ? (
        <div className="flex h-full flex-col gap-4">
          <div className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-600">{stringProp('welcome_message', 'This widget can run a real OpenFoundry agent.')}</div>
          {booleanProp('include_runtime_context', true) && Object.keys(runtimeParameters).length > 0 && (
            <div className="rounded-2xl border border-slate-200 bg-white px-4 py-3 text-xs text-slate-500">
              <div className="font-semibold uppercase tracking-[0.18em] text-slate-400">{stringProp('runtime_context_intro', 'Current scenario context:')}</div>
              <div className="mt-2 flex flex-wrap gap-2">
                {Object.entries(runtimeParameters).map(([k, v]) => (
                  <span key={k} className="rounded-full border border-slate-200 px-3 py-1">{k}: {v}</span>
                ))}
              </div>
            </div>
          )}
          {agentError && <div className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{agentError}</div>}
          <textarea rows={5} value={agentPrompt} onChange={(e) => setAgentPrompt(e.target.value)} placeholder={stringProp('placeholder', 'Ask the embedded agent a question...')} className="w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm" />
          <div className="flex items-center justify-between gap-3">
            <div className="text-xs uppercase tracking-[0.2em] text-slate-400">Agent {stringProp('agent_id', '').trim() || 'not selected'}</div>
            <button type="button" onClick={() => void runAgent()} disabled={agentBusy} className="rounded-xl bg-[var(--app-primary,#0f766e)] px-4 py-2 text-sm font-medium text-white disabled:opacity-60">{agentBusy ? 'Running...' : stringProp('submit_label', 'Run agent')}</button>
          </div>
          {agentResponse ? (
            <div className="space-y-3 rounded-2xl border border-slate-200 bg-white p-4">
              <div className="text-xs uppercase tracking-[0.22em] text-slate-400">Agent response</div>
              <p className="whitespace-pre-wrap text-sm leading-6 text-slate-700">{agentResponse.final_response}</p>
              {agentResponse.used_tool_names.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {agentResponse.used_tool_names.map((tool) => (
                    <span key={tool} className="rounded-full border border-slate-200 px-3 py-1 text-[11px] uppercase tracking-[0.18em] text-slate-500">{tool}</span>
                  ))}
                </div>
              )}
              {booleanProp('show_traces', true) && (
                <div className="space-y-2">
                  {agentResponse.traces.map((trace, i) => (
                    <div key={i} className="rounded-xl bg-slate-50 px-3 py-3 text-xs text-slate-600">
                      <div className="font-semibold text-slate-700">{trace.title}</div>
                      <div className="mt-1">{trace.observation}</div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <div className="flex flex-1 items-center justify-center rounded-2xl border border-dashed border-slate-300 bg-slate-50 px-4 text-center text-sm text-slate-500">{stringProp('empty_state', 'Configure an agent to turn this panel into an interactive copilot.')}</div>
          )}
        </div>
      ) : widget.widget_type === 'table' ? (
        <div className="min-h-0 flex-1">
          <TableWidget widget={tableWidget} result={result} globalSearch={globalFilter} />
        </div>
      ) : widget.widget_type === 'chart' ? (
        <div className="min-h-0 flex-1">
          <ChartWidget widget={chartWidget} result={result} />
        </div>
      ) : widget.widget_type === 'map' ? (
        <div className="relative flex-1 overflow-hidden rounded-2xl border border-slate-200 bg-[radial-gradient(circle_at_top_left,_rgba(15,118,110,0.18),_transparent_40%),linear-gradient(135deg,_#e0f2fe,_#f8fafc)]">
          <div className="absolute inset-0 bg-[linear-gradient(rgba(15,23,42,0.05)_1px,transparent_1px),linear-gradient(90deg,rgba(15,23,42,0.05)_1px,transparent_1px)] bg-[size:48px_48px]" />
          {mapPoints.map((point, i) => (
            <div key={i} className="absolute -translate-x-1/2 -translate-y-1/2" style={{ left: `${point.x}%`, top: `${point.y}%` }}>
              <div className="flex flex-col items-center gap-1">
                <span className="h-3 w-3 rounded-full border-2 border-white bg-[var(--app-primary,#0f766e)] shadow" />
                <span className="rounded-full bg-white/90 px-2 py-1 text-[11px] font-medium text-slate-700 shadow">{point.label}</span>
              </div>
            </div>
          ))}
          {mapPoints.length === 0 && <div className="flex h-full items-center justify-center text-sm text-slate-500">Map bindings need `lat` and `lon` columns.</div>}
        </div>
      ) : widget.widget_type === 'media_preview' ? (
        <div className="min-h-0 flex-1"><MediaPreviewWidget widget={widget} runtimeParameters={runtimeParameters} /></div>
      ) : widget.widget_type === 'media_uploader' ? (
        <div className="min-h-0 flex-1"><MediaUploaderWidget widget={widget} runtimeParameters={runtimeParameters} onAction={onAction} /></div>
      ) : widget.widget_type === 'container' ? (
        <div className="flex flex-1 flex-col gap-3 rounded-2xl border border-dashed border-slate-300 bg-slate-50 p-3">
          <div className="text-sm font-medium text-slate-600">{stringProp('title', widget.title)}</div>
          {widget.children.length === 0 ? (
            <div className="flex flex-1 items-center justify-center text-sm text-slate-400">Drop related widgets inside this section from a template or nested configuration.</div>
          ) : (
            <div className="grid flex-1 gap-3 md:grid-cols-2">
              {widget.children.map((child) => (
                <AppWidgetRenderer key={child.id} widget={child} globalFilter={globalFilter} runtimeParameters={runtimeParameters} onAction={onAction} />
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed border-slate-300 text-sm text-slate-500">Unsupported widget type.</div>
      )}

      {dataset && widget.binding?.source_type === 'dataset' && (
        <div className="mt-3 rounded-xl bg-slate-50 px-3 py-2 text-xs text-slate-500">Dataset binding currently exposes metadata while row preview is still limited in the dataset service.</div>
      )}
    </article>
  );
}
