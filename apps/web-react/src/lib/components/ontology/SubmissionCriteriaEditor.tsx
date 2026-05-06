import { JsonEditor } from '@/lib/components/JsonEditor';
import type { ActionInputField, SubmissionNode } from '@/lib/api/ontology';

interface SubmissionCriteriaEditorProps {
  value: SubmissionNode | null;
  parameters: ActionInputField[];
  onChange: (next: SubmissionNode | null) => void;
}

const TEMPLATE_LEAF: SubmissionNode = {
  type: 'leaf',
  left: { kind: 'param', name: '' },
  op: 'is',
  right: { kind: 'static', value: '' },
};

const TEMPLATE_ALL: SubmissionNode = { type: 'all', children: [TEMPLATE_LEAF] };
const TEMPLATE_ANY: SubmissionNode = { type: 'any', children: [TEMPLATE_LEAF] };

export function SubmissionCriteriaEditor({ value, parameters, onChange }: SubmissionCriteriaEditorProps) {
  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 12, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, color: '#e2e8f0' }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <h3 style={{ margin: 0, fontSize: 13 }}>Submission criteria</h3>
        {value === null ? (
          <span style={{ display: 'flex', gap: 4 }}>
            <button type="button" onClick={() => onChange(TEMPLATE_LEAF)} className="of-button" style={{ fontSize: 11 }}>+ Leaf</button>
            <button type="button" onClick={() => onChange(TEMPLATE_ALL)} className="of-button" style={{ fontSize: 11 }}>+ ALL</button>
            <button type="button" onClick={() => onChange(TEMPLATE_ANY)} className="of-button" style={{ fontSize: 11 }}>+ ANY</button>
          </span>
        ) : (
          <button type="button" onClick={() => onChange(null)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>Clear</button>
        )}
      </header>
      <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>
        Edit the criteria JSON below. Available parameters: {parameters.map((p) => p.name).join(', ') || '—'}
      </p>
      {value !== null && (
        <JsonEditor
          value={JSON.stringify(value, null, 2)}
          onChange={(text) => {
            try { onChange(JSON.parse(text) as SubmissionNode); } catch { /* JsonEditor surfaces error */ }
          }}
          minHeight={180}
        />
      )}
    </section>
  );
}
