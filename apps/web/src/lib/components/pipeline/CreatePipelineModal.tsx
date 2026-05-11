import { useEffect, useState } from 'react';

import { JsonEditor } from '@/lib/components/JsonEditor';
import { Glyph } from '@/lib/components/ui/Glyph';
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
  { id: 'FASTER', title: 'Lightweight / Faster', summary: 'OpenFoundry local table execution for fast interactive previews.', latency: 'Low', complexity: 'Low' },
  { id: 'DISTRIBUTED', title: 'Spark / Flink distributed', summary: 'Submit graph nodes through the distributed runtime adapter.', latency: 'Cluster', complexity: 'Medium' },
  { id: 'INCREMENTAL', title: 'Incremental', summary: 'Process only the rows that changed since the last build.', latency: 'Low', complexity: 'Medium' },
  { id: 'STREAMING', title: 'Streaming', summary: 'Run continuously over an upstream stream.', latency: 'Very low', complexity: 'High' },
  { id: 'EXTERNAL', title: 'External', summary: 'Push compute down to Databricks or Snowflake via virtual tables.', latency: 'Variable', complexity: 'Medium' },
];

const STEPS: Array<{ id: 1 | 2 | 3; label: string }> = [
  { id: 1, label: 'Type' },
  { id: 2, label: 'Identity' },
  { id: 3, label: 'Configuration' },
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
        ...(pipelineType === 'DISTRIBUTED' ? { distributed: { engine: 'spark', ...extra } as unknown as Parameters<typeof createPipeline>[0]['distributed'] } : {}),
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
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-pipeline-title"
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(15, 23, 36, 0.45)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        padding: 16,
      }}
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="of-panel"
        style={{
          width: '100%',
          maxWidth: 760,
          maxHeight: '90vh',
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: 'var(--shadow-popover)',
        }}
      >
        {/* Modal header */}
        <header
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '12px 16px',
            borderBottom: '1px solid var(--border-subtle)',
            background: 'var(--bg-panel-muted)',
          }}
        >
          <div>
            <p className="of-eyebrow" style={{ margin: 0 }}>Pipelines</p>
            <h2 id="create-pipeline-title" className="of-heading-md" style={{ margin: '2px 0 0' }}>
              Create pipeline
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="of-button of-button--ghost"
            aria-label="Close"
            style={{ minHeight: 28, padding: '0 6px' }}
          >
            <Glyph name="x" size={14} />
          </button>
        </header>

        {/* Step indicator */}
        <nav
          aria-label="Progress"
          style={{
            display: 'flex',
            gap: 6,
            padding: '8px 16px',
            borderBottom: '1px solid var(--border-subtle)',
            background: 'var(--bg-panel)',
          }}
        >
          {STEPS.map((s) => {
            const reached = step >= s.id;
            const current = step === s.id;
            return (
              <div
                key={s.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: '4px 8px',
                  borderRadius: 'var(--radius-sm)',
                  fontSize: 11,
                  fontWeight: current ? 700 : 500,
                  color: reached ? 'var(--status-info)' : 'var(--text-muted)',
                  background: current ? 'var(--bg-chip-active)' : 'transparent',
                }}
              >
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minWidth: 18,
                    height: 18,
                    borderRadius: 999,
                    background: reached ? 'var(--status-info)' : 'var(--bg-chip)',
                    color: reached ? '#fff' : 'var(--text-muted)',
                    fontSize: 10,
                    fontWeight: 700,
                  }}
                >
                  {s.id}
                </span>
                {s.label}
              </div>
            );
          })}
        </nav>

        {/* Modal body */}
        <div style={{ padding: 16, display: 'grid', gap: 14 }}>
          {step === 1 && (
            <div style={{ display: 'grid', gap: 10 }}>
              <div>
                <p className="of-heading-sm" style={{ margin: 0 }}>Choose a pipeline type</p>
                <p className="of-text-muted" style={{ margin: '2px 0 0', fontSize: 12 }}>
                  Each strategy trades off latency, complexity and resource use. You can change configuration later in the builder.
                </p>
              </div>
              <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
                {TYPE_CARDS.map((c) => {
                  const selected = pipelineType === c.id;
                  return (
                    <button
                      key={c.id}
                      type="button"
                      onClick={() => setPipelineType(c.id)}
                      className="of-panel"
                      style={{
                        padding: 10,
                        textAlign: 'left',
                        cursor: 'pointer',
                        background: selected ? 'var(--bg-chip-active)' : 'var(--bg-panel)',
                        borderColor: selected ? 'var(--border-focus)' : 'var(--border-default)',
                        borderWidth: selected ? 2 : 1,
                      }}
                    >
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <strong style={{ fontSize: 13, color: 'var(--text-strong)' }}>{c.title}</strong>
                        {selected ? <Glyph name="cube" size={12} /> : null}
                      </div>
                      <p className="of-text-muted" style={{ fontSize: 11, margin: '4px 0 6px' }}>{c.summary}</p>
                      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                        <span className="of-chip" style={{ fontSize: 10 }}>latency · {c.latency}</span>
                        <span className="of-chip" style={{ fontSize: 10 }}>complexity · {c.complexity}</span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {step === 2 && (
            <div style={{ display: 'grid', gap: 10 }}>
              <div>
                <p className="of-heading-sm" style={{ margin: 0 }}>Identify your pipeline</p>
                <p className="of-text-muted" style={{ margin: '2px 0 0', fontSize: 12 }}>
                  Pipelines live inside a project. Pick a name your team will recognise — descriptions help auditors.
                </p>
              </div>
              <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                <span className="of-eyebrow">Name</span>
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="of-input"
                  placeholder="e.g. flight_alerts_clean"
                />
              </label>
              <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                <span className="of-eyebrow">Description</span>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  rows={3}
                  className="of-input"
                  style={{ minHeight: 64 }}
                  placeholder="What does this pipeline produce?"
                />
              </label>
              <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
                <span className="of-eyebrow">Project</span>
                <select
                  value={projectId}
                  onChange={(e) => setProjectId(e.target.value)}
                  className="of-input of-select"
                >
                  <option value="">— none —</option>
                  {projects.map((p) => <option key={p.id} value={p.id}>{p.display_name || p.slug}</option>)}
                </select>
              </label>
            </div>
          )}

          {step === 3 && (
            <div style={{ display: 'grid', gap: 10 }}>
              <div>
                <p className="of-heading-sm" style={{ margin: 0 }}>Type-specific configuration</p>
                <p className="of-text-muted" style={{ margin: '2px 0 0', fontSize: 12 }}>
                  Optional JSON blob applied to {pipelineType ?? 'the selected type'}. Leave empty to start with sensible defaults.
                </p>
              </div>
              <JsonEditor
                value={extraConfig}
                onChange={setExtraConfig}
                minHeight={160}
                placeholder='{ "watermark_columns": ["updated_at"] }'
              />
              {error && (
                <div className="of-status-danger" style={{ padding: '6px 10px', borderRadius: 'var(--radius-sm)', fontSize: 12 }}>
                  {error}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer actions */}
        <footer
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: 6,
            padding: '10px 16px',
            borderTop: '1px solid var(--border-subtle)',
            background: 'var(--bg-panel-muted)',
          }}
        >
          <span className="of-text-muted" style={{ fontSize: 11 }}>Step {step} of {STEPS.length}</span>
          <div style={{ display: 'flex', gap: 6 }}>
            {step === 1 ? (
              <button type="button" onClick={onClose} className="of-button">Cancel</button>
            ) : (
              <button type="button" onClick={() => setStep((step - 1) as 1 | 2 | 3)} className="of-button">
                ← Back
              </button>
            )}
            {step === 1 && (
              <button
                type="button"
                disabled={!pipelineType}
                onClick={() => setStep(2)}
                className="of-button of-button--primary"
              >
                Next →
              </button>
            )}
            {step === 2 && (
              <button
                type="button"
                disabled={!name.trim()}
                onClick={() => setStep(3)}
                className="of-button of-button--primary"
              >
                Next →
              </button>
            )}
            {step === 3 && (
              <button
                type="button"
                onClick={() => void submit()}
                disabled={submitting}
                className="of-button of-button--success"
              >
                {submitting ? 'Creating…' : 'Create pipeline'}
              </button>
            )}
          </div>
        </footer>
      </div>
    </div>
  );
}
