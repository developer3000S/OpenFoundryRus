import { useMemo, useState } from 'react';

import { JsonEditor } from '@/lib/components/JsonEditor';
import type { PipelineNode } from '@/lib/api/pipelines';

interface PipelineNodeListProps {
  nodes: PipelineNode[];
  onChange: (next: PipelineNode[]) => void;
  disabled?: boolean;
}

const TRANSFORM_TYPES = ['sql', 'python', 'pyspark', 'spark', 'wasm', 'llm', 'external', 'identity'] as const;

function makeId() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `node_${Date.now()}_${Math.floor(Math.random() * 10_000)}`;
}

function defaultConfigFor(transformType: string): Record<string, unknown> {
  switch (transformType) {
    case 'sql':
      return { sql: 'SELECT 1 AS value' };
    case 'python':
      return { source: 'rows_affected = 0\nresult = "python transform ready"' };
    case 'wasm':
      return { module: '', function: 'run' };
    case 'llm':
      return { prompt: 'Classify the record\n\n{{input_json}}', output_field: 'llm_response' };
    case 'pyspark':
    case 'spark':
      return { entrypoint: 'main', cluster_profile: 'shared' };
    case 'external':
      return { endpoint: '', execution_mode: 'sync' };
    default:
      return {};
  }
}

export function PipelineNodeList({ nodes, onChange, disabled }: PipelineNodeListProps) {
  const [selectedId, setSelectedId] = useState<string>(nodes[0]?.id ?? '');

  const selectedNode = useMemo(() => nodes.find((n) => n.id === selectedId) ?? null, [nodes, selectedId]);

  function patchNode(id: string, patch: Partial<PipelineNode>) {
    onChange(nodes.map((n) => (n.id === id ? { ...n, ...patch } : n)));
  }

  function addNode(transformType: string) {
    const id = makeId();
    const node: PipelineNode = {
      id,
      label: `${transformType} transform`,
      transform_type: transformType,
      config: defaultConfigFor(transformType),
      depends_on: [],
      input_dataset_ids: [],
      output_dataset_id: null,
    };
    onChange([...nodes, node]);
    setSelectedId(id);
  }

  function deleteNode(id: string) {
    onChange(nodes.filter((n) => n.id !== id).map((n) => ({ ...n, depends_on: n.depends_on.filter((d) => d !== id) })));
    if (selectedId === id) setSelectedId('');
  }

  function toggleDependency(targetId: string, depId: string) {
    const target = nodes.find((n) => n.id === targetId);
    if (!target) return;
    const has = target.depends_on.includes(depId);
    patchNode(targetId, {
      depends_on: has ? target.depends_on.filter((d) => d !== depId) : [...target.depends_on, depId],
    });
  }

  return (
    <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 0.7fr) minmax(0, 1.3fr)' }}>
      <section className="of-panel" style={{ padding: 12 }}>
        <p className="of-eyebrow">Nodes ({nodes.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
          {nodes.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                onClick={() => setSelectedId(n.id)}
                disabled={disabled}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 8,
                  borderRadius: 6,
                  border: `1px solid ${selectedId === n.id ? '#1d4ed8' : 'var(--border-default)'}`,
                  background: selectedId === n.id ? '#eff6ff' : 'transparent',
                  cursor: 'pointer',
                  fontSize: 12,
                }}
              >
                <strong>{n.label}</strong>
                <p className="of-text-muted" style={{ fontSize: 10, margin: 0 }}>
                  {n.transform_type} · deps: {n.depends_on.length}
                </p>
              </button>
            </li>
          ))}
          {nodes.length === 0 && <li className="of-text-muted">No nodes.</li>}
        </ul>
        <div style={{ marginTop: 8 }}>
          <p className="of-eyebrow" style={{ fontSize: 10 }}>Add node</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 4 }}>
            {TRANSFORM_TYPES.map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => addNode(t)}
                disabled={disabled}
                className="of-button"
                style={{ fontSize: 10, padding: '2px 8px' }}
              >
                + {t}
              </button>
            ))}
          </div>
        </div>
      </section>

      <section className="of-panel" style={{ padding: 12 }}>
        {selectedNode ? (
          <div style={{ display: 'grid', gap: 8 }}>
            <p className="of-eyebrow">Node {selectedNode.id.slice(0, 12)}…</p>
            <label style={{ fontSize: 13 }}>
              Label
              <input
                value={selectedNode.label}
                onChange={(e) => patchNode(selectedNode.id, { label: e.target.value })}
                disabled={disabled}
                className="of-input"
                style={{ marginTop: 4 }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              Transform type
              <select
                value={selectedNode.transform_type}
                onChange={(e) => patchNode(selectedNode.id, { transform_type: e.target.value })}
                disabled={disabled}
                className="of-input"
                style={{ marginTop: 4 }}
              >
                {TRANSFORM_TYPES.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
                {!TRANSFORM_TYPES.includes(selectedNode.transform_type as typeof TRANSFORM_TYPES[number]) && (
                  <option value={selectedNode.transform_type}>{selectedNode.transform_type}</option>
                )}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              Output dataset id
              <input
                value={selectedNode.output_dataset_id ?? ''}
                onChange={(e) => patchNode(selectedNode.id, { output_dataset_id: e.target.value || null })}
                disabled={disabled}
                placeholder="optional"
                className="of-input"
                style={{ marginTop: 4 }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              Input dataset ids (comma-separated)
              <input
                value={selectedNode.input_dataset_ids.join(', ')}
                onChange={(e) => patchNode(selectedNode.id, {
                  input_dataset_ids: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                })}
                disabled={disabled}
                className="of-input"
                style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
              />
            </label>
            <div style={{ fontSize: 13 }}>
              Depends on
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 4 }}>
                {nodes.filter((n) => n.id !== selectedNode.id).map((n) => (
                  <label
                    key={n.id}
                    style={{
                      fontSize: 11,
                      padding: '2px 8px',
                      border: '1px solid var(--border-default)',
                      borderRadius: 999,
                      cursor: disabled ? 'not-allowed' : 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                    }}
                  >
                    <input
                      type="checkbox"
                      disabled={disabled}
                      checked={selectedNode.depends_on.includes(n.id)}
                      onChange={() => toggleDependency(selectedNode.id, n.id)}
                    />
                    {n.label}
                  </label>
                ))}
                {nodes.length <= 1 && <span className="of-text-muted">No other nodes.</span>}
              </div>
            </div>
            <JsonEditor
              label="Config JSON"
              value={JSON.stringify(selectedNode.config, null, 2)}
              onChange={(text) => {
                try {
                  patchNode(selectedNode.id, { config: JSON.parse(text) });
                } catch {
                  // ignore — JsonEditor surfaces parse error
                }
              }}
              minHeight={140}
              disabled={disabled}
            />
            <div>
              <button
                type="button"
                onClick={() => deleteNode(selectedNode.id)}
                disabled={disabled}
                className="of-button"
                style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}
              >
                Delete node
              </button>
            </div>
          </div>
        ) : (
          <p className="of-text-muted">Pick a node from the left or add one.</p>
        )}
      </section>
    </div>
  );
}
