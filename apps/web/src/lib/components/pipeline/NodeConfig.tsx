import { useMemo } from 'react';
import { Link } from 'react-router-dom';

import { JsonEditor } from '@/lib/components/JsonEditor';
import { MediaTransformEditor } from '@/lib/components/pipeline/MediaTransformEditor';
import { TransformEditor } from '@/lib/components/pipeline/TransformEditor';
import type { NodeValidationReport, PipelineNode } from '@/lib/api/pipelines';

type TransformOption = 'passthrough' | 'sql' | 'python' | 'pyspark' | 'spark' | 'llm' | 'wasm' | 'external' | 'remote';

interface NodeConfigProps {
  node: PipelineNode | null;
  siblings: PipelineNode[];
  readOnly?: boolean;
  onChange: (next: PipelineNode) => void;
  onDelete?: (nodeId: string) => void;
  validation?: NodeValidationReport | null;
}

const TRANSFORMS: TransformOption[] = ['passthrough', 'sql', 'python', 'pyspark', 'spark', 'llm', 'wasm', 'external', 'remote'];

const BODY_KEY_CANDIDATES: Record<string, string[]> = {
  sql: ['sql'],
  python: ['source', 'python_source'],
  pyspark: ['source', 'python_source'],
  spark: ['source', 'spark_source'],
  llm: ['prompt'],
  wasm: ['wasm_module_b64', 'module'],
};

const DEFAULT_BODY_KEY: Record<string, string> = {
  sql: 'sql',
  python: 'source',
  pyspark: 'source',
  spark: 'source',
  llm: 'prompt',
  wasm: 'wasm_module_b64',
};

const ALL_BODY_KEYS = Array.from(new Set(Object.values(BODY_KEY_CANDIDATES).flat()));

const MEDIA_TRANSFORM_TYPES = new Set([
  'media_set_input',
  'media_set_output',
  'media_transform',
  'convert_media_set_to_table_rows',
  'get_media_references',
]);

function datasetRoute(id: string) {
  return `/datasets/${encodeURIComponent(id)}`;
}

export function NodeConfig({ node, siblings, readOnly = false, onChange, onDelete, validation = null }: NodeConfigProps) {
  const dependencyOptions = useMemo(() => siblings.filter((s) => node && s.id !== node.id), [siblings, node]);

  if (!node) {
    return (
      <aside style={{ padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#94a3b8', fontSize: 12 }}>
        <p style={{ margin: 0 }}>Select a node on the canvas to edit its properties.</p>
      </aside>
    );
  }

  const transformOptions = TRANSFORMS.includes(node.transform_type as TransformOption)
    ? TRANSFORMS
    : [...TRANSFORMS, node.transform_type];

  function bodyValue(n: PipelineNode): string {
    for (const key of BODY_KEY_CANDIDATES[n.transform_type] ?? []) {
      const raw = (n.config ?? {})[key];
      if (typeof raw === 'string') return raw;
    }
    return '';
  }

  function patch(partial: Partial<PipelineNode>) {
    if (!node) return;
    onChange({ ...node, ...partial });
  }

  function setBody(next: string) {
    if (!node) return;
    const candidates = BODY_KEY_CANDIDATES[node.transform_type] ?? [];
    const key = candidates.find((candidate) => typeof (node.config ?? {})[candidate] === 'string') ?? DEFAULT_BODY_KEY[node.transform_type];
    if (!key) return;
    onChange({ ...node, config: { ...(node.config ?? {}), [key]: next } });
  }

  function setTransform(next: string) {
    if (!node) return;
    const cleanConfig: Record<string, unknown> = { ...(node.config ?? {}) };
    for (const key of ALL_BODY_KEYS) delete cleanConfig[key];
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
  const hasTransformBody = (BODY_KEY_CANDIDATES[node.transform_type]?.length ?? 0) > 0;
  const supportsTransformEditor = !isMedia && (hasTransformBody || node.transform_type === 'passthrough');

  return (
    <aside style={{ padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0', display: 'flex', flexDirection: 'column', gap: 12, width: '100%', boxSizing: 'border-box' }}>
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
          <select value={node.transform_type} onChange={(e) => setTransform(e.target.value)} disabled={readOnly} className="of-input" style={{ marginTop: 4 }}>
            {transformOptions.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </label>
      )}

      <label style={{ fontSize: 12 }}>
        Output dataset id
        <input value={node.output_dataset_id ?? ''} onChange={(e) => patch({ output_dataset_id: e.target.value || null })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
        {node.output_dataset_id && (
          <Link to={datasetRoute(node.output_dataset_id)} className="of-link" style={{ display: 'inline-block', marginTop: 4, fontSize: 11 }}>
            Open dataset
          </Link>
        )}
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
        {node.input_dataset_ids.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 6 }}>
            {node.input_dataset_ids.map((datasetId) => (
              <Link key={datasetId} to={datasetRoute(datasetId)} className="of-link" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                {datasetId}
              </Link>
            ))}
          </div>
        )}
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

      {isMedia && (
        <MediaTransformEditor node={node} readOnly={readOnly} onChange={onChange} />
      )}

      {supportsTransformEditor && (
        <TransformEditor
          transformType={node.transform_type}
          value={bodyValue(node)}
          onChange={setBody}
          config={node.config}
          onConfigChange={(next) => patch({ config: next })}
          readOnly={readOnly}
        />
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
