import type { MediaSetSchema } from '@/lib/api/mediaSets';

export interface NodePaletteEntry {
  transform_type: string;
  kind?: string;
  defaultConfig?: Record<string, unknown>;
  label: string;
  description: string;
  icon: string;
  acceptedSchemas?: MediaSetSchema[];
  outputKind?: 'media_items' | 'dataset_rows' | 'sideeffect';
}

interface NodePaletteProps {
  onAdd: (entry: NodePaletteEntry) => void;
  disabled?: boolean;
}

const CATEGORIES: { name: string; entries: NodePaletteEntry[] }[] = [
  {
    name: 'Inputs',
    entries: [
      { transform_type: 'passthrough', label: 'Input dataset', description: 'Bind a Foundry dataset version. Outputs unchanged.', icon: '⊟' },
    ],
  },
  {
    name: 'Transform',
    entries: [
      { transform_type: 'sql', label: 'SQL transform', description: 'DataFusion SQL over upstream datasets.', icon: 'Σ' },
      { transform_type: 'python', label: 'Python transform', description: 'pyo3 user function. Receives Arrow batches per input.', icon: 'py' },
    ],
  },
  {
    name: 'AI / Custom',
    entries: [
      { transform_type: 'llm', label: 'LLM transform', description: 'Call the AI service for row-wise prompt completion.', icon: '✸' },
      { transform_type: 'wasm', label: 'WASM transform', description: 'Sandboxed user module (wasmtime). Offline, no network.', icon: '◇' },
    ],
  },
  {
    name: 'Media: Inputs',
    entries: [
      { transform_type: 'media_set_input', label: 'Media set input', description: 'Read items (or paths) from a Foundry media set.', icon: 'IN', defaultConfig: { media_set_rid: '', branch: 'main' }, outputKind: 'media_items' },
    ],
  },
  {
    name: 'Media: Outputs',
    entries: [
      { transform_type: 'media_set_output', label: 'Media set output', description: 'Write derived media into an existing or fresh media set.', icon: 'OUT', defaultConfig: { media_set_rid: '', branch: 'main', write_mode: 'modify' }, outputKind: 'sideeffect' },
    ],
  },
  {
    name: 'Media: Transforms',
    entries: [
      { transform_type: 'media_transform', kind: 'extract_text_ocr', label: 'Extract text (OCR)', description: 'OCR over IMAGE or DOCUMENT items. Emits dataset rows.', icon: 'OCR', defaultConfig: { kind: 'extract_text_ocr', params: {} }, acceptedSchemas: ['IMAGE', 'DOCUMENT'], outputKind: 'dataset_rows' },
      { transform_type: 'media_transform', kind: 'resize', label: 'Resize image', description: 'Resize IMAGE items to width × height pixels.', icon: '⛶', defaultConfig: { kind: 'resize', params: { width: 256, height: 256 } }, acceptedSchemas: ['IMAGE'], outputKind: 'media_items' },
      { transform_type: 'media_transform', kind: 'rotate', label: 'Rotate image', description: 'Rotate IMAGE items by N degrees clockwise.', icon: '↻', defaultConfig: { kind: 'rotate', params: { degrees: 90 } }, acceptedSchemas: ['IMAGE'], outputKind: 'media_items' },
      { transform_type: 'media_transform', kind: 'crop', label: 'Crop image', description: 'Crop IMAGE items to (x, y, width, height).', icon: '▣', defaultConfig: { kind: 'crop', params: { x: 0, y: 0, width: 256, height: 256 } }, acceptedSchemas: ['IMAGE'], outputKind: 'media_items' },
      { transform_type: 'media_transform', kind: 'transcribe_audio', label: 'Transcribe audio', description: 'Transcribe AUDIO or VIDEO items to text rows.', icon: '🎙', defaultConfig: { kind: 'transcribe_audio', params: {} }, acceptedSchemas: ['AUDIO', 'VIDEO'], outputKind: 'dataset_rows' },
      { transform_type: 'media_transform', kind: 'generate_embedding', label: 'Generate embedding', description: 'Generate vector embedding for any media kind.', icon: '⊕', defaultConfig: { kind: 'generate_embedding', params: {} }, outputKind: 'dataset_rows' },
      { transform_type: 'media_transform', kind: 'render_pdf_page', label: 'Render PDF page', description: 'Render one page of a DOCUMENT item as an image.', icon: 'PDF', defaultConfig: { kind: 'render_pdf_page', params: { page: 1 } }, acceptedSchemas: ['DOCUMENT'], outputKind: 'media_items' },
      { transform_type: 'convert_media_set_to_table_rows', label: 'Convert to table rows', description: 'Flatten a media set into one row per item.', icon: '⊞', defaultConfig: { source_media_set_rid: '', branch: 'main', include_media_reference: true }, outputKind: 'dataset_rows' },
      { transform_type: 'get_media_references', label: 'Get media references', description: 'Walk a dataset of files and register them in a media set.', icon: '🔗', defaultConfig: { source_dataset_id: '', target_media_set_rid: '', force_mime_type: null }, outputKind: 'dataset_rows' },
    ],
  },
  {
    name: 'Data Connection: Sync',
    entries: [
      { transform_type: 'SYNC', label: 'Sync from external source', description: 'Materialise data from a Foundry connector (S3, JDBC, Kafka, …) into a Foundry dataset.', icon: '⇄', defaultConfig: { logic_kind: 'SYNC', source_rid: '', sync_def_id: '', overrides: {} }, outputKind: 'dataset_rows' },
    ],
  },
  {
    name: 'Quality: Health check',
    entries: [
      { transform_type: 'HEALTH_CHECK', label: 'Health check', description: 'Validate row count / schema drift / freshness / custom SQL on a target dataset.', icon: '✓', defaultConfig: { logic_kind: 'HEALTH_CHECK', check_kind: 'ROW_COUNT_NONZERO', target_dataset_rid: '', params: {} }, outputKind: 'sideeffect' },
    ],
  },
  {
    name: 'Analytics: Object set materialize',
    entries: [
      { transform_type: 'ANALYTICAL', label: 'Materialise object set', description: 'Run a Quiver-style object-set query and write the rows into the output dataset.', icon: '◎', defaultConfig: { logic_kind: 'ANALYTICAL', object_set_query: {}, ontology_rid: null, output_schema: null }, outputKind: 'dataset_rows' },
    ],
  },
  {
    name: 'Export: External destination',
    entries: [
      { transform_type: 'EXPORT', label: 'Export to external system', description: 'Push a dataset to S3 / GCS / HTTP / JDBC. Requires an ACL alias.', icon: '↗', defaultConfig: { logic_kind: 'EXPORT', export_target: 'S3', endpoint: '', options: {}, source_dataset_rid: '', acl_alias: '' }, outputKind: 'sideeffect' },
    ],
  },
];

export function NodePalette({ onAdd, disabled = false }: NodePaletteProps) {
  return (
    <aside
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        padding: 12,
        background: '#0b1220',
        border: '1px solid #1f2937',
        borderRadius: 8,
        color: '#e2e8f0',
        width: 240,
        flexShrink: 0,
        maxHeight: '80vh',
        overflowY: 'auto',
      }}
    >
      <h3 style={{ margin: 0, fontSize: 14, color: '#f1f5f9' }}>Add node</h3>
      {CATEGORIES.map((cat) => (
        <section key={cat.name}>
          <h4 style={{ margin: '4px 0 6px', fontSize: 11, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{cat.name}</h4>
          <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
            {cat.entries.map((entry) => (
              <li key={`${entry.transform_type}::${entry.kind ?? ''}`}>
                <button
                  type="button"
                  disabled={disabled}
                  onClick={() => onAdd(entry)}
                  title={entry.description}
                  style={{
                    display: 'flex',
                    gap: 10,
                    alignItems: 'flex-start',
                    width: '100%',
                    textAlign: 'left',
                    background: '#1e293b',
                    color: '#e2e8f0',
                    border: '1px solid #334155',
                    borderRadius: 6,
                    padding: '8px 10px',
                    cursor: disabled ? 'not-allowed' : 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  <span aria-hidden="true" style={{ fontSize: 14, lineHeight: 1, color: '#60a5fa', flexShrink: 0, width: 22, textAlign: 'center' }}>{entry.icon}</span>
                  <span style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <strong style={{ fontSize: 12, fontWeight: 600 }}>{entry.label}</strong>
                    <span style={{ fontSize: 11, color: '#94a3b8' }}>{entry.description}</span>
                    {entry.acceptedSchemas && entry.acceptedSchemas.length > 0 && (
                      <span style={{ fontSize: 10, color: '#38bdf8', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                        accepts: {entry.acceptedSchemas.join(' · ')}
                      </span>
                    )}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </aside>
  );
}
