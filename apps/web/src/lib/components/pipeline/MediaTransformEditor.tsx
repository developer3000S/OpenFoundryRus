import { JsonEditor } from '@/lib/components/JsonEditor';
import type { PipelineNode } from '@/lib/api/pipelines';

interface MediaTransformEditorProps {
  node: PipelineNode;
  readOnly?: boolean;
  onChange: (next: PipelineNode) => void;
}

const TRANSFORM_KINDS = [
  'extract_text_ocr',
  'resize',
  'rotate',
  'crop',
  'transcribe_audio',
  'generate_embedding',
  'render_pdf_page',
  'extract_layout_aware',
] as const;

export function MediaTransformEditor({ node, readOnly = false, onChange }: MediaTransformEditorProps) {
  function patch(partial: Record<string, unknown>) {
    onChange({ ...node, config: { ...(node.config ?? {}), ...partial } });
  }
  function readString(key: string, fallback = ''): string {
    const raw = (node.config ?? {})[key];
    return typeof raw === 'string' ? raw : fallback;
  }
  function readBool(key: string, fallback = false): boolean {
    const raw = (node.config ?? {})[key];
    return typeof raw === 'boolean' ? raw : fallback;
  }

  return (
    <section style={{ display: 'grid', gap: 8, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <h4 style={{ margin: 0, fontSize: 13 }}>Media node config — {node.transform_type}</h4>

      {node.transform_type === 'media_set_input' && (
        <>
          <label style={{ fontSize: 12 }}>
            Media set RID
            <input value={readString('media_set_rid')} onChange={(e) => patch({ media_set_rid: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Branch
            <input value={readString('branch', 'main')} onChange={(e) => patch({ branch: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
          </label>
        </>
      )}

      {node.transform_type === 'media_set_output' && (
        <>
          <label style={{ fontSize: 12 }}>
            Target media set RID
            <input value={readString('media_set_rid')} onChange={(e) => patch({ media_set_rid: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Branch
            <input value={readString('branch', 'main')} onChange={(e) => patch({ branch: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Write mode
            <select value={readString('write_mode', 'modify')} onChange={(e) => patch({ write_mode: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }}>
              <option value="modify">modify</option>
              <option value="overwrite">overwrite</option>
              <option value="append">append</option>
            </select>
          </label>
        </>
      )}

      {node.transform_type === 'media_transform' && (
        <>
          <label style={{ fontSize: 12 }}>
            Kind
            <select value={readString('kind', 'extract_text_ocr')} onChange={(e) => onChange({ ...node, config: { ...(node.config ?? {}), kind: e.target.value, params: {} } })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }}>
              {TRANSFORM_KINDS.map((k) => <option key={k} value={k}>{k}</option>)}
            </select>
          </label>
          <JsonEditor
            label="Params"
            value={JSON.stringify((node.config ?? {})['params'] ?? {}, null, 2)}
            onChange={(text) => {
              try { patch({ params: JSON.parse(text) }); }
              catch { /* JsonEditor surfaces error */ }
            }}
            disabled={readOnly}
            minHeight={120}
          />
        </>
      )}

      {node.transform_type === 'convert_media_set_to_table_rows' && (
        <>
          <label style={{ fontSize: 12 }}>
            Source media set RID
            <input value={readString('source_media_set_rid')} onChange={(e) => patch({ source_media_set_rid: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Branch
            <input value={readString('branch', 'main')} onChange={(e) => patch({ branch: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 6 }}>
            <input type="checkbox" checked={readBool('include_media_reference', true)} onChange={(e) => patch({ include_media_reference: e.target.checked })} disabled={readOnly} />
            Include media reference column
          </label>
        </>
      )}

      {node.transform_type === 'get_media_references' && (
        <>
          <label style={{ fontSize: 12 }}>
            Source dataset id
            <input value={readString('source_dataset_id')} onChange={(e) => patch({ source_dataset_id: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Target media set RID
            <input value={readString('target_media_set_rid')} onChange={(e) => patch({ target_media_set_rid: e.target.value })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Force MIME type (optional)
            <input value={readString('force_mime_type')} onChange={(e) => patch({ force_mime_type: e.target.value || null })} disabled={readOnly} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
        </>
      )}
    </section>
  );
}
