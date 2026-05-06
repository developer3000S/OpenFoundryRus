import { useEffect, useMemo, useState } from 'react';

import {
  validatePipeline,
  type NodeValidationReport,
  type PipelineNode,
  type PipelineScheduleConfig,
  type PipelineValidationResponse,
} from '@/lib/api/pipelines';

interface PipelineCanvasProps {
  nodes: PipelineNode[];
  status?: string;
  scheduleConfig?: PipelineScheduleConfig;
  readOnly?: boolean;
  onChange?: (nodes: PipelineNode[]) => void;
  onSelect?: (node: PipelineNode | null) => void;
  onValidate?: (result: PipelineValidationResponse) => void;
  nodeReports?: Record<string, NodeValidationReport>;
}

const NODE_W = 180;
const NODE_H = 64;
const STAGE_GAP_X = 80;
const STAGE_GAP_Y = 24;

const TRANSFORM_OPTIONS = [
  { value: 'passthrough', label: 'Passthrough' },
  { value: 'sql', label: 'SQL' },
  { value: 'python', label: 'Python' },
  { value: 'llm', label: 'LLM' },
  { value: 'wasm', label: 'WASM' },
];

function uniqueId(base: string, nodes: PipelineNode[]) {
  const candidate = base.replace(/[^a-zA-Z0-9_]+/g, '_').toLowerCase() || 'node';
  if (!nodes.some((n) => n.id === candidate)) return candidate;
  let i = 2;
  while (nodes.some((n) => n.id === `${candidate}_${i}`)) i += 1;
  return `${candidate}_${i}`;
}

function topologicalStages(nodes: PipelineNode[]): string[][] {
  const indeg = new Map<string, number>();
  const adj = new Map<string, string[]>();
  for (const n of nodes) {
    indeg.set(n.id, 0);
    adj.set(n.id, []);
  }
  for (const n of nodes) {
    for (const dep of n.depends_on) {
      if (!indeg.has(dep)) continue;
      indeg.set(n.id, (indeg.get(n.id) ?? 0) + 1);
      adj.get(dep)!.push(n.id);
    }
  }
  const result: string[][] = [];
  let frontier = nodes.filter((n) => (indeg.get(n.id) ?? 0) === 0).map((n) => n.id);
  const visited = new Set<string>();
  while (frontier.length > 0) {
    result.push(frontier);
    frontier.forEach((id) => visited.add(id));
    const next: string[] = [];
    for (const id of frontier) {
      for (const child of adj.get(id) ?? []) {
        const v = (indeg.get(child) ?? 0) - 1;
        indeg.set(child, v);
        if (v === 0) next.push(child);
      }
    }
    frontier = next;
  }
  const orphan = nodes.map((n) => n.id).filter((id) => !visited.has(id));
  if (orphan.length > 0) result.push(orphan);
  return result;
}

export function PipelineCanvas({
  nodes,
  status = 'draft',
  scheduleConfig = { enabled: false, cron: null },
  readOnly = false,
  onChange,
  onSelect,
  onValidate,
  nodeReports = {},
}: PipelineCanvasProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [pendingSourceId, setPendingSourceId] = useState<string | null>(null);
  const [validation, setValidation] = useState<PipelineValidationResponse | null>(null);
  const [validating, setValidating] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);

  const stages = useMemo(() => topologicalStages(nodes), [nodes]);

  const positions = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>();
    stages.forEach((stage, sIdx) => {
      stage.forEach((id, idx) => {
        map.set(id, {
          x: sIdx * (NODE_W + STAGE_GAP_X) + 20,
          y: idx * (NODE_H + STAGE_GAP_Y) + 20,
        });
      });
    });
    return map;
  }, [stages]);

  const canvasW = stages.length * (NODE_W + STAGE_GAP_X) + 40;
  const canvasH = Math.max(1, ...stages.map((s) => s.length)) * (NODE_H + STAGE_GAP_Y) + 40;

  // Debounced validate when nodes/status/scheduleConfig change.
  useEffect(() => {
    const timer = setTimeout(async () => {
      setValidating(true);
      setValidationError(null);
      try {
        const result = await validatePipeline({ status, schedule_config: scheduleConfig, nodes });
        setValidation(result);
        onValidate?.(result);
      } catch (cause) {
        const body = (cause as { body?: PipelineValidationResponse })?.body;
        if (body) {
          setValidation(body);
          onValidate?.(body);
        } else {
          setValidationError(cause instanceof Error ? cause.message : 'validation failed');
        }
      } finally {
        setValidating(false);
      }
    }, 300);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes, status, scheduleConfig.enabled, scheduleConfig.cron]);

  function selectNode(id: string | null) {
    if (readOnly && id !== null) return;
    setSelectedId(id);
    setPendingSourceId(null);
    onSelect?.(id ? (nodes.find((n) => n.id === id) ?? null) : null);
  }

  function addNode(transform: string) {
    if (readOnly) return;
    const id = uniqueId(`${transform}_node`, nodes);
    const node: PipelineNode = {
      id,
      label: `New ${transform} node`,
      transform_type: transform,
      config: {},
      depends_on: selectedId ? [selectedId] : [],
      input_dataset_ids: [],
      output_dataset_id: null,
    };
    onChange?.([...nodes, node]);
    setSelectedId(id);
    onSelect?.(node);
  }

  function removeSelected() {
    if (readOnly || !selectedId) return;
    const removed = selectedId;
    onChange?.(nodes.filter((n) => n.id !== removed).map((n) => ({ ...n, depends_on: n.depends_on.filter((d) => d !== removed) })));
    setSelectedId(null);
    onSelect?.(null);
  }

  function startConnect() {
    if (readOnly || !selectedId) return;
    setPendingSourceId(selectedId);
  }

  function nodeClick(id: string) {
    if (pendingSourceId && pendingSourceId !== id) {
      onChange?.(nodes.map((n) => {
        if (n.id !== id) return n;
        if (n.depends_on.includes(pendingSourceId)) return n;
        return { ...n, depends_on: [...n.depends_on, pendingSourceId] };
      }));
      setPendingSourceId(null);
      return;
    }
    selectNode(id);
  }

  function disconnect(targetId: string, sourceId: string) {
    if (readOnly) return;
    onChange?.(nodes.map((n) => (n.id === targetId ? { ...n, depends_on: n.depends_on.filter((d) => d !== sourceId) } : n)));
  }

  function nodeFill(node: PipelineNode) {
    if (selectedId === node.id) return '#1e40af';
    if (pendingSourceId === node.id) return '#7c3aed';
    return '#1f2937';
  }

  function nodeStroke(node: PipelineNode) {
    const errored = validation?.errors.some((e) => e.includes(`'${node.id}'`)) ?? false;
    if (errored) return '#ef4444';
    const tone = nodeReports[node.id]?.status;
    if (tone === 'INVALID') return '#ef4444';
    if (tone === 'PENDING') return '#eab308';
    if (tone === 'VALID') return '#10b981';
    if (selectedId === node.id) return '#3b82f6';
    return '#374151';
  }

  function statusGlyph(nodeId: string) {
    const r = nodeReports[nodeId];
    if (!r) return '';
    if (r.status === 'VALID') return '✓';
    if (r.status === 'PENDING') return '⚠';
    return '✗';
  }

  function statusTooltip(nodeId: string) {
    const r = nodeReports[nodeId];
    if (!r || r.errors.length === 0) return '';
    return r.errors.map((e) => e.message).join('; ');
  }

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
        {!readOnly && TRANSFORM_OPTIONS.map((t) => (
          <button key={t.value} type="button" onClick={() => addNode(t.value)} className="of-button" style={{ fontSize: 11 }}>
            + {t.label}
          </button>
        ))}
        {!readOnly && selectedId && (
          <>
            <button type="button" onClick={startConnect} className="of-button" style={{ fontSize: 11, background: pendingSourceId ? '#7c3aed' : undefined, color: pendingSourceId ? '#fff' : undefined }}>
              {pendingSourceId ? 'Click target to connect…' : 'Connect →'}
            </button>
            <button type="button" onClick={removeSelected} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
              Remove
            </button>
          </>
        )}
        <span className="of-text-muted" style={{ fontSize: 11, marginLeft: 'auto' }}>
          {validating && '· validating…'}
          {!validating && validation && validation.valid && '· ✓ valid'}
          {!validating && validation && !validation.valid && `· ${validation.errors.length} error${validation.errors.length === 1 ? '' : 's'}`}
          {!validating && validationError && ` · ${validationError}`}
        </span>
      </div>

      <div style={{ overflow: 'auto', border: '1px solid var(--border-default)', borderRadius: 12, background: '#0b1220' }}>
        <svg width={Math.max(canvasW, 600)} height={Math.max(canvasH, 200)} role="img" aria-label="Pipeline DAG" style={{ display: 'block' }}>
          <defs>
            <marker id="arrowhead" markerWidth="10" markerHeight="10" refX="10" refY="3.5" orient="auto" fill="#94a3b8">
              <polygon points="0 0, 10 3.5, 0 7" />
            </marker>
          </defs>
          {/* Edges */}
          {nodes.flatMap((n) =>
            n.depends_on.map((dep) => {
              const src = positions.get(dep);
              const tgt = positions.get(n.id);
              if (!src || !tgt) return null;
              const x1 = src.x + NODE_W;
              const y1 = src.y + NODE_H / 2;
              const x2 = tgt.x;
              const y2 = tgt.y + NODE_H / 2;
              const cx = (x1 + x2) / 2;
              const path = `M ${x1} ${y1} C ${cx} ${y1}, ${cx} ${y2}, ${x2} ${y2}`;
              return (
                <g key={`${dep}->${n.id}`}>
                  <path d={path} fill="none" stroke="#475569" strokeWidth={1.5} markerEnd="url(#arrowhead)" />
                  {!readOnly && (
                    <path
                      d={path}
                      fill="none"
                      stroke="transparent"
                      strokeWidth={10}
                      style={{ cursor: 'pointer' }}
                      onClick={() => disconnect(n.id, dep)}
                    >
                      <title>Click to disconnect</title>
                    </path>
                  )}
                </g>
              );
            }),
          )}
          {/* Nodes */}
          {nodes.map((n) => {
            const pos = positions.get(n.id);
            if (!pos) return null;
            return (
              <g
                key={n.id}
                transform={`translate(${pos.x}, ${pos.y})`}
                onClick={() => nodeClick(n.id)}
                style={{ cursor: readOnly ? 'default' : 'pointer' }}
              >
                <rect width={NODE_W} height={NODE_H} rx={8} ry={8} fill={nodeFill(n)} stroke={nodeStroke(n)} strokeWidth={2} />
                <text x={12} y={22} fill="#f1f5f9" fontSize={13} fontWeight={600}>
                  {n.label.length > 22 ? `${n.label.slice(0, 22)}…` : n.label}
                </text>
                <text x={12} y={42} fill="#94a3b8" fontSize={11}>
                  {n.transform_type}
                </text>
                <text x={12} y={56} fill="#64748b" fontSize={10} fontFamily="ui-monospace, monospace">
                  {n.id}
                </text>
                {statusGlyph(n.id) && (
                  <text x={NODE_W - 16} y={20} textAnchor="end" fontSize={14}>
                    {statusGlyph(n.id)}
                    <title>{statusTooltip(n.id)}</title>
                  </text>
                )}
              </g>
            );
          })}
          {nodes.length === 0 && (
            <text x={20} y={40} fill="#94a3b8" fontSize={12}>
              Empty DAG. Click "+ SQL" or another transform to add a node.
            </text>
          )}
        </svg>
      </div>

      {validation && validation.errors.length > 0 && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          <strong>Errors</strong>
          <ul style={{ marginTop: 4, paddingLeft: 18 }}>
            {validation.errors.map((e, i) => <li key={i}>{e}</li>)}
          </ul>
        </div>
      )}
      {validation && validation.warnings.length > 0 && (
        <div style={{ padding: '10px 14px', background: '#fef3c7', color: '#92400e', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          <strong>Warnings</strong>
          <ul style={{ marginTop: 4, paddingLeft: 18 }}>
            {validation.warnings.map((w, i) => <li key={i}>{w}</li>)}
          </ul>
        </div>
      )}
    </div>
  );
}
