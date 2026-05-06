import { JsonEditor } from '@/lib/components/JsonEditor';
import { MonacoEditor } from '@/lib/components/MonacoEditor';

type CodeTransform = 'sql' | 'python' | 'llm' | 'wasm' | 'passthrough';
type LogicKind = 'SYNC' | 'HEALTH_CHECK' | 'ANALYTICAL' | 'EXPORT';
export type Transform = CodeTransform | LogicKind;

interface ViewFilterIncompatibility {
  reason: string;
}

interface TransformEditorProps {
  transformType: Transform;
  value: string;
  readOnly?: boolean;
  onChange: (next: string) => void;
  config?: Record<string, unknown>;
  onConfigChange?: (next: Record<string, unknown>) => void;
  viewFilterIncompatibility?: ViewFilterIncompatibility | null;
}

const LANGUAGE: Record<CodeTransform, string> = {
  sql: 'sql',
  python: 'python',
  llm: 'markdown',
  wasm: 'plaintext',
  passthrough: 'plaintext',
};

const LOGIC_KINDS: LogicKind[] = ['SYNC', 'HEALTH_CHECK', 'ANALYTICAL', 'EXPORT'];
function isLogicKind(t: Transform): t is LogicKind {
  return (LOGIC_KINDS as Transform[]).includes(t);
}

export function TransformEditor({
  transformType,
  value,
  readOnly = false,
  onChange,
  config = {},
  onConfigChange = () => {},
  viewFilterIncompatibility = null,
}: TransformEditorProps) {
  if (transformType === 'passthrough') {
    return (
      <p className="of-text-muted" style={{ fontSize: 12, padding: 12, fontStyle: 'italic' }}>
        Passthrough nodes have no body — upstream rows are forwarded unchanged.
      </p>
    );
  }

  if (isLogicKind(transformType)) {
    return (
      <div style={{ display: 'grid', gap: 8 }}>
        <p className="of-text-muted" style={{ fontSize: 12 }}>
          {transformType} job. Configure via the JSON below; no user code body.
        </p>
        <JsonEditor
          label={`${transformType} config`}
          value={JSON.stringify(config, null, 2)}
          onChange={(text) => {
            try { onConfigChange(JSON.parse(text)); }
            catch { /* JsonEditor surfaces error */ }
          }}
          minHeight={160}
          disabled={readOnly}
        />
        {viewFilterIncompatibility && (
          <p style={{ color: '#fca5a5', fontSize: 11, padding: '4px 8px', border: '1px solid #7f1d1d', borderRadius: 4 }}>
            {viewFilterIncompatibility.reason}
          </p>
        )}
      </div>
    );
  }

  const language = LANGUAGE[transformType as CodeTransform] ?? 'plaintext';
  return (
    <div style={{ display: 'grid', gap: 4 }}>
      <span className="of-text-muted" style={{ fontSize: 11 }}>{language.toUpperCase()} body</span>
      <MonacoEditor
        value={value}
        language={language}
        onChange={(next) => { if (!readOnly) onChange(next); }}
        minHeight={240}
      />
      {viewFilterIncompatibility && (
        <p style={{ color: '#fca5a5', fontSize: 11, padding: '4px 8px', border: '1px solid #7f1d1d', borderRadius: 4 }}>
          {viewFilterIncompatibility.reason}
        </p>
      )}
    </div>
  );
}
