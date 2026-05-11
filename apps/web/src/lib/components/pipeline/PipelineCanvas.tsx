import { useEffect, useMemo, useState } from 'react';

import {
  validatePipeline,
  type NodeValidationReport,
  type PipelineNode,
  type PipelineScheduleConfig,
  type PipelineValidationResponse,
} from '@/lib/api/pipelines';
import { Glyph } from '@/lib/components/ui/Glyph';

interface PipelineCanvasProps {
  nodes: PipelineNode[];
  status?: string;
  scheduleConfig?: PipelineScheduleConfig;
  readOnly?: boolean;
  onChange?: (nodes: PipelineNode[]) => void;
  onSelect?: (node: PipelineNode | null) => void;
  onValidate?: (result: PipelineValidationResponse) => void;
  onTransform?: (node: PipelineNode) => void;
  onJoinStart?: (left: PipelineNode, right: PipelineNode) => void;
  onUnionStart?: (inputs: PipelineNode[]) => void;
  onAddOutput?: (source: PipelineNode, kind: 'dataset' | 'object_type' | 'link_type' | 'time_series' | 'virtual_table') => void;
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
  onTransform,
  onJoinStart,
  onUnionStart,
  onAddOutput,
  nodeReports = {},
}: PipelineCanvasProps) {
  const [addOutputMenuOpen, setAddOutputMenuOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [pendingSourceId, setPendingSourceId] = useState<string | null>(null);
  const [pendingJoinLeftId, setPendingJoinLeftId] = useState<string | null>(null);
  const [pendingJoinRightId, setPendingJoinRightId] = useState<string | null>(null);
  const [pendingUnionIds, setPendingUnionIds] = useState<string[]>([]);
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
    if (pendingUnionIds.length > 0) {
      setPendingUnionIds((current) => {
        if (current.includes(id)) return current.filter((entry) => entry !== id);
        return [...current, id];
      });
      return;
    }
    if (pendingJoinLeftId && id !== pendingJoinLeftId) {
      setPendingJoinRightId(id);
      return;
    }
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

  function startJoin(node: PipelineNode) {
    setPendingJoinLeftId(node.id);
    setPendingJoinRightId(null);
    setSelectedId(null);
  }

  function cancelJoin() {
    setPendingJoinLeftId(null);
    setPendingJoinRightId(null);
  }

  function commitJoin() {
    if (!pendingJoinLeftId || !pendingJoinRightId) return;
    const left = nodes.find((entry) => entry.id === pendingJoinLeftId);
    const right = nodes.find((entry) => entry.id === pendingJoinRightId);
    if (left && right) onJoinStart?.(left, right);
    setPendingJoinLeftId(null);
    setPendingJoinRightId(null);
  }

  function startUnion(node: PipelineNode) {
    setPendingUnionIds([node.id]);
    setSelectedId(null);
  }

  function cancelUnion() {
    setPendingUnionIds([]);
  }

  function commitUnion() {
    if (pendingUnionIds.length < 2) return;
    const inputs = pendingUnionIds
      .map((id) => nodes.find((entry) => entry.id === id))
      .filter((entry): entry is PipelineNode => Boolean(entry));
    if (inputs.length >= 2) onUnionStart?.(inputs);
    setPendingUnionIds([]);
  }

  function disconnect(targetId: string, sourceId: string) {
    if (readOnly) return;
    onChange?.(nodes.map((n) => (n.id === targetId ? { ...n, depends_on: n.depends_on.filter((d) => d !== sourceId) } : n)));
  }

  function nodeFill(node: PipelineNode) {
    if (selectedId === node.id) return '#e8f1ff';
    if (pendingSourceId === node.id) return '#fff3df';
    return '#ffffff';
  }

  function nodeStroke(node: PipelineNode) {
    const errored = validation?.errors.some((e) => e.includes(`'${node.id}'`)) ?? false;
    if (errored) return '#ef4444';
    const tone = nodeReports[node.id]?.status;
    if (tone === 'INVALID') return '#ef4444';
    if (tone === 'PENDING') return '#eab308';
    if (tone === 'VALID') return '#10b981';
    if (selectedId === node.id) return '#2d72d2';
    return '#aeb8c5';
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
    <div style={{ display: 'grid', gap: 0 }}>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center', padding: 8, borderBottom: '1px solid var(--border-default)', background: 'var(--bg-topbar)' }}>
        {!readOnly && TRANSFORM_OPTIONS.map((t) => (
          <button key={t.value} type="button" onClick={() => addNode(t.value)} className="of-button" style={{ fontSize: 11 }}>
            + {t.label}
          </button>
        ))}
        {!readOnly && selectedId && (
          <>
            <button type="button" onClick={startConnect} className="of-button" style={{ fontSize: 11, background: pendingSourceId ? '#2d72d2' : undefined, color: pendingSourceId ? '#fff' : undefined }}>
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

      <div style={{ overflow: 'auto', border: 0, background: '#eef1f5', position: 'relative' }}>
        {pendingUnionIds.length > 0 ? (
          (() => {
            const primary = nodes.find((entry) => entry.id === pendingUnionIds[0]);
            const others = pendingUnionIds.length - 1;
            return (
              <div
                role="status"
                style={{
                  position: 'absolute',
                  top: 12,
                  left: 12,
                  right: 12,
                  zIndex: 6,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 14px',
                  background: '#fff',
                  border: '1px solid var(--border-default)',
                  borderRadius: 6,
                  boxShadow: '0 6px 16px rgba(15, 23, 42, 0.12)',
                  fontSize: 13,
                }}
              >
                <span style={{ flex: 1 }}>
                  Select additional nodes on the graph to start configuring their union. Hold ⌘ and click on a node to add or remove it from your selection.
                  <div style={{ marginTop: 4, fontSize: 12, color: 'var(--text-muted)', display: 'flex', gap: 6, alignItems: 'center' }}>
                    <UnionGlyph />
                    <strong style={{ color: 'var(--text-strong)' }}>{primary ? primary.label : '—'}</strong>
                    <span>with {others} other node{others === 1 ? '' : 's'}</span>
                  </div>
                </span>
                <button
                  type="button"
                  onClick={commitUnion}
                  disabled={pendingUnionIds.length < 2}
                  style={{
                    padding: '8px 16px',
                    border: 0,
                    borderRadius: 4,
                    background: '#2d72d2',
                    color: '#fff',
                    fontSize: 13,
                    fontWeight: 600,
                    cursor: pendingUnionIds.length < 2 ? 'not-allowed' : 'pointer',
                    opacity: pendingUnionIds.length < 2 ? 0.6 : 1,
                  }}
                >
                  Start
                </button>
                <button type="button" className="of-button" aria-label="Cancel union" onClick={cancelUnion}>
                  <Glyph name="x" size={12} />
                </button>
              </div>
            );
          })()
        ) : null}
        {pendingJoinLeftId ? (
          (() => {
            const left = nodes.find((entry) => entry.id === pendingJoinLeftId);
            const right = pendingJoinRightId ? nodes.find((entry) => entry.id === pendingJoinRightId) : null;
            return (
              <div
                role="status"
                style={{
                  position: 'absolute',
                  top: 12,
                  right: 12,
                  zIndex: 6,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 14px',
                  background: '#fff',
                  border: '1px solid var(--border-default)',
                  borderRadius: 6,
                  boxShadow: '0 6px 16px rgba(15, 23, 42, 0.12)',
                  fontSize: 13,
                  maxWidth: 520,
                }}
              >
                <span style={{ flex: 1 }}>
                  Select another table on the graph to start configuring the join.
                  <div style={{ marginTop: 4, fontSize: 12, color: 'var(--text-muted)', display: 'flex', gap: 18 }}>
                    <span>
                      <strong style={{ color: 'var(--text-strong)' }}>Left:</strong> {left ? left.label : '—'}
                    </span>
                    <span>
                      <strong style={{ color: 'var(--text-strong)' }}>Right:</strong> {right ? right.label : '—'}
                    </span>
                  </div>
                </span>
                <button
                  type="button"
                  onClick={commitJoin}
                  disabled={!right}
                  style={{
                    padding: '8px 16px',
                    border: 0,
                    borderRadius: 4,
                    background: '#2d72d2',
                    color: '#fff',
                    fontSize: 13,
                    fontWeight: 600,
                    cursor: right ? 'pointer' : 'not-allowed',
                    opacity: right ? 1 : 0.6,
                  }}
                >
                  Start
                </button>
                <button type="button" className="of-button" aria-label="Cancel join" onClick={cancelJoin}>
                  <Glyph name="x" size={12} />
                </button>
              </div>
            );
          })()
        ) : null}
        {selectedId && !readOnly ? (
          (() => {
            const selectedNode = nodes.find((entry) => entry.id === selectedId);
            const pos = positions.get(selectedId);
            if (!selectedNode || !pos) return null;
            return (
              <div
                style={{
                  position: 'absolute',
                  top: pos.y,
                  left: pos.x + NODE_W + 6,
                  zIndex: 5,
                  display: 'grid',
                  gap: 2,
                  padding: 4,
                  background: '#fff',
                  border: '1px solid var(--border-default)',
                  borderRadius: 4,
                  boxShadow: '0 6px 16px rgba(15, 23, 42, 0.12)',
                }}
                onClick={(event) => event.stopPropagation()}
              >
                <FloatingActionButton
                  label="Transform"
                  tone="#2d72d2"
                  onClick={() => onTransform?.(selectedNode)}
                  glyph={<TransformGlyph />}
                />
                <FloatingActionButton
                  label="Join"
                  tone="#15803d"
                  onClick={() => startJoin(selectedNode)}
                  glyph={<JoinGlyph />}
                />
                <FloatingActionButton
                  label="Union"
                  tone="#b42318"
                  onClick={() => startUnion(selectedNode)}
                  glyph={<UnionGlyph />}
                />
                <FloatingActionButton label="Pivot" tone="#b42318" disabled glyph={<PivotGlyph />} />
                <FloatingActionButton label="Filter" tone="#7c5dd6" disabled glyph={<DotsGlyph />} />
                <FloatingActionButton label="AI suggest" tone="#7c5dd6" disabled glyph={<SparkGlyph />} />
                <FloatingActionButton label="Hint" tone="#cf923f" disabled glyph={<BulbGlyph />} />
                <div style={{ position: 'relative' }}>
                  <FloatingActionButton
                    label="Add output"
                    tone="#cf923f"
                    onClick={() => setAddOutputMenuOpen((open) => !open)}
                    glyph={<PlusCircleGlyph />}
                  />
                  {addOutputMenuOpen ? (
                    <div
                      role="menu"
                      style={{
                        position: 'absolute',
                        top: 0,
                        left: 'calc(100% + 6px)',
                        background: '#fff',
                        border: '1px solid var(--border-default)',
                        borderRadius: 6,
                        boxShadow: '0 8px 24px rgba(15, 23, 42, 0.16)',
                        padding: 6,
                        minWidth: 220,
                        zIndex: 7,
                      }}
                    >
                      <AddOutputMenuItem
                        enabled
                        glyph={<NewDatasetGlyph />}
                        label="New dataset"
                        onClick={() => {
                          setAddOutputMenuOpen(false);
                          onAddOutput?.(selectedNode, 'dataset');
                        }}
                      />
                      <AddOutputMenuItem
                        enabled
                        glyph={<NewObjectTypeGlyph />}
                        label="New object type"
                        onClick={() => {
                          setAddOutputMenuOpen(false);
                          onAddOutput?.(selectedNode, 'object_type');
                        }}
                      />
                      <AddOutputMenuItem
                        enabled
                        glyph={<Glyph name="link" size={14} />}
                        label="New link type"
                        onClick={() => {
                          setAddOutputMenuOpen(false);
                          onAddOutput?.(selectedNode, 'link_type');
                        }}
                      />
                      <AddOutputMenuItem disabled glyph={<NewTimeSeriesGlyph />} label="New time series sync" />
                      <AddOutputMenuItem disabled glyph={<NewVirtualTableGlyph />} label="New virtual table" />
                    </div>
                  ) : null}
                </div>
              </div>
            );
          })()
        ) : null}
        <svg width={Math.max(canvasW, 600)} height={Math.max(canvasH, 200)} role="img" aria-label="Pipeline DAG" style={{ display: 'block' }}>
          <defs>
            <pattern id="pipeline-grid" width="18" height="18" patternUnits="userSpaceOnUse">
              <path d="M 18 0 L 0 0 0 18" fill="none" stroke="#d7dde5" strokeWidth="1" />
            </pattern>
            <marker id="arrowhead" markerWidth="10" markerHeight="10" refX="10" refY="3.5" orient="auto" fill="#7b8794">
              <polygon points="0 0, 10 3.5, 0 7" />
            </marker>
          </defs>
          <rect width="100%" height="100%" fill="url(#pipeline-grid)" />
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
                  <path d={path} fill="none" stroke="#7b8794" strokeWidth={1.4} markerEnd="url(#arrowhead)" />
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
                <rect width={NODE_W} height={NODE_H} rx={2} ry={2} fill={nodeFill(n)} stroke={nodeStroke(n)} strokeWidth={1.5} />
                <rect width={4} height={NODE_H} rx={2} ry={2} fill={selectedId === n.id ? '#2d72d2' : '#6f7d8c'} />
                <text x={12} y={22} fill="#1f252d" fontSize={12} fontWeight={600}>
                  {n.label.length > 22 ? `${n.label.slice(0, 22)}…` : n.label}
                </text>
                <text x={12} y={42} fill="#5f6b7a" fontSize={11}>
                  {n.transform_type}
                </text>
                <text x={12} y={56} fill="#8b96a5" fontSize={10} fontFamily="ui-monospace, monospace">
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
            <text x={20} y={40} fill="#5f6b7a" fontSize={12}>
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

function FloatingActionButton({
  label,
  tone,
  glyph,
  onClick,
  disabled,
}: {
  label: string;
  tone: string;
  glyph: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={onClick}
      disabled={disabled}
      style={{
        width: 28,
        height: 28,
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        border: 0,
        borderRadius: 4,
        background: 'transparent',
        color: tone,
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.45 : 1,
      }}
    >
      {glyph}
    </button>
  );
}

function TransformGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M5 6h14M7 12h10M9 18h6" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}

function JoinGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="6" width="9" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <rect x="11" y="6" width="9" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M11 11h2" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}

function UnionGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="6" width="16" height="5" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <rect x="4" y="13" width="16" height="5" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}

function PivotGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="6" width="16" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M4 11h16M11 6v12" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}

function DotsGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="6" cy="12" r="2" fill="currentColor" />
      <circle cx="12" cy="12" r="2" fill="currentColor" />
      <circle cx="18" cy="12" r="2" fill="currentColor" />
    </svg>
  );
}

function SparkGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M12 4l1.5 4.5L18 10l-4.5 1.5L12 16l-1.5-4.5L6 10l4.5-1.5z" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
    </svg>
  );
}

function BulbGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M9 18h6M10 20.5h4M8.2 13.6A6 6 0 1 1 17 9a6 6 0 0 1-2 4.6V16H10v-2.4z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

function PlusCircleGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="1.6" />
      <path d="M12 8v8M8 12h8" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

function AddOutputMenuItem({
  glyph,
  label,
  enabled,
  disabled,
  onClick,
}: {
  glyph: React.ReactNode;
  label: string;
  enabled?: boolean;
  disabled?: boolean;
  onClick?: () => void;
}) {
  const isDisabled = disabled || !enabled;
  return (
    <button
      type="button"
      onClick={isDisabled ? undefined : onClick}
      disabled={isDisabled}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        width: '100%',
        padding: '8px 10px',
        border: 0,
        borderRadius: 4,
        background: 'transparent',
        cursor: isDisabled ? 'not-allowed' : 'pointer',
        color: isDisabled ? 'var(--text-muted)' : 'var(--text-strong)',
        fontSize: 13,
        textAlign: 'left',
      }}
      onMouseEnter={(event) => {
        if (!isDisabled) event.currentTarget.style.background = 'rgba(45, 114, 210, 0.06)';
      }}
      onMouseLeave={(event) => (event.currentTarget.style.background = 'transparent')}
    >
      <span style={{ color: '#cf923f', display: 'inline-flex' }}>{glyph}</span>
      <span>{label}</span>
    </button>
  );
}

function NewDatasetGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="5" width="13" height="14" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M4 9h13" stroke="currentColor" strokeWidth="1.6" />
      <circle cx="19" cy="18" r="3.6" fill="currentColor" />
      <path d="M19 16.4v3.2M17.4 18h3.2" stroke="#fff" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

function NewObjectTypeGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M12 4l8 4.5v7L12 20l-8-4.5v-7z" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round" />
      <path d="M4 8.5L12 13l8-4.5M12 13v7" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}

function NewTimeSeriesGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M4 17l4-6 4 3 4-7 4 4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function NewVirtualTableGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="5" width="13" height="14" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M14 9l4 4-4 4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
