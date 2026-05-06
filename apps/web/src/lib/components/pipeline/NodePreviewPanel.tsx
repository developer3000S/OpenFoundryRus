import { useEffect, useState } from 'react';

import { previewPipelineNode, type PipelineNode, type PipelinePreviewOutput } from '@/lib/api/pipelines';
import { VirtualizedPreviewTable } from '@/lib/components/dataset/VirtualizedPreviewTable';

interface NodePreviewPanelProps {
  pipelineId: string;
  node: PipelineNode | null;
}

export function NodePreviewPanel({ pipelineId, node }: NodePreviewPanelProps) {
  const [preview, setPreview] = useState<PipelinePreviewOutput | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!node || !pipelineId) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    previewPipelineNode(pipelineId, node.id)
      .then((p) => { if (!cancelled) setPreview(p); })
      .catch((cause: unknown) => { if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load preview'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [pipelineId, node?.id]);

  let freshnessLabel = '';
  if (preview) {
    const generated = new Date(preview.generated_at).getTime();
    const elapsed = Math.max(0, Math.round((Date.now() - generated) / 1000));
    if (elapsed === 0) freshnessLabel = 'just now';
    else if (elapsed < 60) freshnessLabel = `${elapsed}s ago`;
    else freshnessLabel = `${Math.floor(elapsed / 60)}m ago`;
  }

  const columns = (preview?.columns ?? []).map((name) => ({ name }));
  const rows = preview?.rows ?? [];

  async function refresh() {
    if (!node || !pipelineId) return;
    setLoading(true);
    setError(null);
    try {
      setPreview(await previewPipelineNode(pipelineId, node.id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load preview');
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="of-panel" style={{ padding: '12px 16px', display: 'flex', flexDirection: 'column', gap: 8, marginTop: 12 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <div className="of-text-muted" style={{ fontSize: 11, fontWeight: 600, letterSpacing: '0.06em', textTransform: 'uppercase' }}>Preview</div>
          <h3 style={{ margin: '4px 0 0', fontSize: 14, fontWeight: 600 }}>{node ? node.label || node.id : 'No node selected'}</h3>
          {preview && (
            <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
              {preview.sample_size} rows · chain {preview.source_chain.join(' → ')} · last refreshed {freshnessLabel}
            </p>
          )}
        </div>
        <button type="button" disabled={loading || !node} onClick={() => void refresh()} className="of-button" style={{ fontSize: 12 }}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </header>
      {error ? (
        <div className="of-status-danger" style={{ padding: '8px 12px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>{error}</div>
      ) : !node ? (
        <div className="of-text-muted" style={{ border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', padding: 14, fontSize: 12, textAlign: 'center' }}>
          Select a node on the canvas to preview the data after that step.
        </div>
      ) : loading && !preview ? (
        <div className="of-text-muted" style={{ border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', padding: 14, fontSize: 12, textAlign: 'center' }}>
          Loading preview…
        </div>
      ) : preview && rows.length === 0 ? (
        <div className="of-text-muted" style={{ border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', padding: 14, fontSize: 12, textAlign: 'center' }}>
          No rows match the upstream chain at this step.
        </div>
      ) : preview ? (
        <div style={{ maxHeight: 280, overflow: 'auto' }}>
          <VirtualizedPreviewTable columns={columns} rows={rows} transactions={[]} />
        </div>
      ) : null}
    </section>
  );
}
