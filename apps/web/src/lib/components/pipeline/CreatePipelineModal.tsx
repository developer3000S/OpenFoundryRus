import { useEffect, useState } from 'react';

import { JsonEditor } from '@/lib/components/JsonEditor';
import { listProjects, type OntologyProject } from '@/lib/api/ontology';
import { createPipeline, type PipelineType } from '@/lib/api/pipelines';

interface CreatePipelineModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: (pipelineId: string) => void;
}

interface TypeCard {
  id: PipelineType;
  title: string;
  summary: string;
  latency: string;
  complexity: string;
}

const TYPE_CARDS: TypeCard[] = [
  { id: 'BATCH', title: 'Batch', summary: 'Recompute every dataset on each run.', latency: 'High', complexity: 'Low' },
  { id: 'FASTER', title: 'Faster', summary: 'DataFusion-backed batch for small-to-medium datasets.', latency: 'Medium', complexity: 'Low' },
  { id: 'INCREMENTAL', title: 'Incremental', summary: 'Process only the rows that changed since the last build.', latency: 'Low', complexity: 'Medium' },
  { id: 'STREAMING', title: 'Streaming', summary: 'Run continuously over an upstream stream.', latency: 'Very low', complexity: 'High' },
  { id: 'EXTERNAL', title: 'External', summary: 'Push compute down to Databricks or Snowflake via virtual tables.', latency: 'Variable', complexity: 'Medium' },
];

export function CreatePipelineModal({ open, onClose, onCreated }: CreatePipelineModalProps) {
  const [step, setStep] = useState<1 | 2 | 3>(1);
  const [pipelineType, setPipelineType] = useState<PipelineType | null>(null);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [projectId, setProjectId] = useState<string>('');
  const [projects, setProjects] = useState<OntologyProject[]>([]);
  const [extraConfig, setExtraConfig] = useState('{}');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setStep(1);
    setPipelineType(null);
    setName('');
    setDescription('');
    setProjectId('');
    setExtraConfig('{}');
    setError(null);
    listProjects({ per_page: 100 }).then((r) => setProjects(r.data)).catch(() => {});
  }, [open]);

  async function submit() {
    if (!pipelineType || !name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      let extra: Record<string, unknown> = {};
      try { extra = JSON.parse(extraConfig); } catch { /* ignore */ }
      const created = await createPipeline({
        name: name.trim(),
        description: description.trim() || undefined,
        pipeline_type: pipelineType,
        nodes: [],
        ...(projectId ? { project_id: projectId } : {}),
        ...(pipelineType === 'EXTERNAL' ? { external: extra as unknown as Parameters<typeof createPipeline>[0]['external'] } : {}),
        ...(pipelineType === 'INCREMENTAL' ? { incremental: extra as unknown as Parameters<typeof createPipeline>[0]['incremental'] } : {}),
        ...(pipelineType === 'STREAMING' ? { streaming: extra as unknown as Parameters<typeof createPipeline>[0]['streaming'] } : {}),
      });
      onCreated(created.id);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) return null;

  return (
    <div role="dialog" aria-modal="true" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100, padding: 16 }}>
      <div style={{ width: '100%', maxWidth: 720, maxHeight: '90vh', overflow: 'auto', background: '#0f172a', color: '#e2e8f0', border: '1px solid #1e293b', borderRadius: 16, padding: 20, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>Create pipeline</h2>
          <span className="of-text-muted" style={{ fontSize: 12 }}>Step {step} of 3</span>
        </header>

        {step === 1 && (
          <div style={{ display: 'grid', gap: 8 }}>
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>Pick a pipeline type:</p>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
              {TYPE_CARDS.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => setPipelineType(c.id)}
                  style={{
                    padding: 12,
                    borderRadius: 8,
                    border: pipelineType === c.id ? '2px solid #3b82f6' : '1px solid #334155',
                    background: pipelineType === c.id ? '#1e293b' : '#111827',
                    textAlign: 'left',
                    cursor: 'pointer',
                    color: 'inherit',
                  }}
                >
                  <strong style={{ fontSize: 14 }}>{c.title}</strong>
                  <p className="of-text-muted" style={{ fontSize: 11, margin: '4px 0' }}>{c.summary}</p>
                  <div style={{ fontSize: 10, color: '#94a3b8' }}>latency: {c.latency} · complexity: {c.complexity}</div>
                </button>
              ))}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6 }}>
              <button type="button" onClick={onClose} className="of-button">Cancel</button>
              <button type="button" disabled={!pipelineType} onClick={() => setStep(2)} className="of-button of-button--primary">Next →</button>
            </div>
          </div>
        )}

        {step === 2 && (
          <div style={{ display: 'grid', gap: 8 }}>
            <label style={{ fontSize: 13 }}>
              Name
              <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Project
              <select value={projectId} onChange={(e) => setProjectId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
                <option value="">— none —</option>
                {projects.map((p) => <option key={p.id} value={p.id}>{p.display_name || p.slug}</option>)}
              </select>
            </label>
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 6 }}>
              <button type="button" onClick={() => setStep(1)} className="of-button">← Back</button>
              <button type="button" disabled={!name.trim()} onClick={() => setStep(3)} className="of-button of-button--primary">Next →</button>
            </div>
          </div>
        )}

        {step === 3 && (
          <div style={{ display: 'grid', gap: 8 }}>
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>Type-specific config (optional JSON):</p>
            <JsonEditor value={extraConfig} onChange={setExtraConfig} minHeight={140} placeholder='{ "watermark_columns": ["updated_at"] }' />
            {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 6 }}>
              <button type="button" onClick={() => setStep(2)} className="of-button">← Back</button>
              <button type="button" onClick={() => void submit()} disabled={submitting} className="of-button of-button--primary">
                {submitting ? 'Creating…' : 'Create pipeline'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
