import { useMemo } from 'react';

import { JsonEditor } from '@/lib/components/JsonEditor';
import type { NodeValidationReport, PipelineNode } from '@/lib/api/pipelines';

type Transform = 'sql' | 'python' | 'llm' | 'wasm' | 'passthrough';

interface NodeConfigProps {
  node: PipelineNode | null;
  siblings: PipelineNode[];
  readOnly?: boolean;
  onChange: (next: PipelineNode) => void;
  onDelete?: (nodeId: string) => void;
  validation?: NodeValidationReport | null;
}

const TRANSFORMS: Transform[] = ['passthrough', 'sql', 'python', 'llm', 'wasm'];

const BODY_KEY: Record<Transform, string | null> = {
  sql: 'sql',
  python: 'python_source',
  llm: 'prompt',
  wasm: 'wasm_module_b64',
  passthrough: null,
};

const MEDIA_TRANSFORM_TYPES = new Set([
  'media_set_input',
  'media_set_output',
  'media_transform',
  'convert_media_set_to_table_rows',
  'get_media_references',
]);

export function NodeConfig({ node, siblings, readOnly = false, onChange, onDelete, validation = null }: NodeConfigProps) {
  const dependencyOptions = useMemo(() => siblings.filter((s) => node && s.id !== node.id), [siblings, node]);

  if (!node) {
    return (
      <aside style={{ padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#94a3b8', fontSize: 12 }}>
        <p style={{ margin: 0 }}>Select a node on the canvas to edit its properties.</p>
      </aside>
    );
  }

  function bodyValue(n: PipelineNode): string {
    const key = BODY_KEY[n.transform_type as Transform];
    if (!key) return '';
    const raw = (n.config ?? {})[key];
    return typeof raw === 'string' ? raw : '';
  }

  function patch(partial: Partial<PipelineNode>) {
    if (!node) return;
    onChange({ ...node, ...partial });
  }

  function setBody(next: string) {
    if (!node) return;
    const key = BODY_KEY[node.transform_type as Transform];
    if (!key) return;
    onChange({ ...node, config: { ...(node.config ?? {}), [key]: next } });
  }

  function setTransform(next: Transform) {
    if (!node) return;
    const cleanConfig: Record<string, unknown> = { ...(node.config ?? {}) };
    for (const v of Object.values(BODY_KEY)) {
      if (v) delete cleanConfig[v];
    }
    onChange({ ...node, transform_type: next, config: cleanConfig });
  }

  function toggleDependency(id: string) {
    if (!node) return;
    const set = new Set(node.depends_on);
    if (set.has(id)) set.delete(id);
    else set.add(id);
    patch({ depends_on: [...set] });
  }

  const isMedia = MEDIA_TRANSFORM_TYPES.has(node.transform_type);
  const bodyKey = BODY_KEY[node.transform_type as Transform];

  return (
    <aside style={{ padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0', display: 'flex', flexDirection: 'column', gap: 12, width: 320 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Node properties</h3>
        {onDelete && !readOnly && (
          <button type="button" onClick={() => onDelete(node.id)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>
            Delete
          </button>
        )}
      </header>

      {validation && validation.status !== 'VALID' && validation.errors.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 4 }}>
          {validation.errors.map((err, i) => (
            <li key={i} style={{ background: '#7f1d1d', color: '#fecaca', padding: '4px 8px', borderRadius: 4, fontSize: 11 }}>
              {err.column && <code style={{ marginRight: 4 }}>{err.column}</code>}
              {err.message}
            </li>
          ))}
        </ul>
      )}

      <label style={{ fontSize: 12 }}>
        Node id
        <input value={node.id} disabled className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
      </label>

      <label style={{ fontSize: 12 }}>
        Label
        <input value={node.label} onChange={(e) => patch({ label: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
      </label>

      {!isMedia && (
        <label style={{ fontSize: 12 }}>
          Transform type
          <select value={node.transform_type} onChange={(e) => setTransform(e.target.value as Transform)} disabled={readOnly} className="of-input" style={{ marginTop: 4 }}>
            {TRANSFORMS.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </label>
      )}

      <label style={{ fontSize: 12 }}>
        Output dataset id
        <input value={node.output_dataset_id ?? ''} onChange={(e) => patch({ output_dataset_id: e.target.value || null })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
      </label>

      <label style={{ fontSize: 12 }}>
        Input dataset ids (comma)
        <input
          value={node.input_dataset_ids.join(', ')}
          onChange={(e) => patch({ input_dataset_ids: e.target.value.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean) })}
          disabled={readOnly}
          className="of-input"
          style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
        />
      </label>

      <div style={{ fontSize: 12 }}>
        Depends on
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 4 }}>
          {dependencyOptions.map((s) => (
            <label key={s.id} style={{ fontSize: 11, padding: '2px 8px', border: '1px solid #334155', borderRadius: 999, cursor: readOnly ? 'not-allowed' : 'pointer', display: 'flex', alignItems: 'center', gap: 4 }}>
              <input type="checkbox" checked={node.depends_on.includes(s.id)} onChange={() => toggleDependency(s.id)} disabled={readOnly} />
              {s.label}
            </label>
          ))}
          {dependencyOptions.length === 0 && <span style={{ color: '#94a3b8', fontSize: 11 }}>No other nodes.</span>}
        </div>
      </div>

      {bodyKey && !isMedia && (
        <label style={{ fontSize: 12 }}>
          Body ({bodyKey})
          <textarea
            value={bodyValue(node)}
            onChange={(e) => setBody(e.target.value)}
            disabled={readOnly}
            rows={10}
            className="of-input"
            style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11 }}
          />
        </label>
      )}

      <JsonEditor
        label="Config (raw JSON)"
        value={JSON.stringify(node.config, null, 2)}
        onChange={(text) => {
          try { patch({ config: JSON.parse(text) }); }
          catch { /* JsonEditor surfaces error */ }
        }}
        disabled={readOnly}
        minHeight={120}
      />
    </aside>
  );
}
