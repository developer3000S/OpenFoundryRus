import type { PipelineNodeResult, PipelineRun } from '@/lib/api/pipelines';

interface RunLogsProps {
  run: PipelineRun;
  onClose?: () => void;
}

const PILL: Record<string, { background: string; color: string }> = {
  running: { background: '#1d4ed8', color: '#dbeafe' },
  completed: { background: '#166534', color: '#d1fae5' },
  failed: { background: '#991b1b', color: '#fee2e2' },
  aborted: { background: '#92400e', color: '#fde68a' },
  skipped: { background: '#92400e', color: '#fde68a' },
};

function pill(s: string) {
  return PILL[s] ?? { background: '#334155', color: '#cbd5e1' };
}

export function RunLogs({ run, onClose }: RunLogsProps) {
  const nodes: PipelineNodeResult[] = run.node_results ?? [];
  const durationMs = run.finished_at ? new Date(run.finished_at).getTime() - new Date(run.started_at).getTime() : null;

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 10, padding: 12, background: '#0f172a', border: '1px solid #1f2937', borderRadius: 6, color: '#e2e8f0' }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <h4 style={{ margin: 0, fontSize: 13 }}>Run <code style={{ fontSize: 11, color: '#94a3b8' }}>{run.id.slice(0, 8)}</code></h4>
        {onClose && (
          <button type="button" onClick={onClose} aria-label="Close" style={{ background: 'transparent', color: '#94a3b8', border: 'none', fontSize: 18, cursor: 'pointer', lineHeight: 1 }}>×</button>
        )}
      </header>

      <dl style={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: '4px 12px', margin: 0, fontSize: 12 }}>
        <dt style={{ color: '#94a3b8' }}>Status</dt>
        <dd style={{ margin: 0 }}><span style={{ ...pill(run.status), padding: '2px 8px', borderRadius: 999, fontSize: 11 }}>{run.status}</span></dd>
        <dt style={{ color: '#94a3b8' }}>Trigger</dt><dd style={{ margin: 0 }}>{run.trigger_type}</dd>
        <dt style={{ color: '#94a3b8' }}>Attempt</dt><dd style={{ margin: 0 }}>#{run.attempt_number}</dd>
        <dt style={{ color: '#94a3b8' }}>Started</dt><dd style={{ margin: 0 }}>{new Date(run.started_at).toLocaleString()}</dd>
        <dt style={{ color: '#94a3b8' }}>Finished</dt><dd style={{ margin: 0 }}>{run.finished_at ? new Date(run.finished_at).toLocaleString() : '—'}</dd>
        <dt style={{ color: '#94a3b8' }}>Duration</dt><dd style={{ margin: 0 }}>{durationMs !== null ? `${(durationMs / 1000).toFixed(1)}s` : '—'}</dd>
      </dl>

      {run.error_message && (
        <pre style={{ background: '#020617', color: '#fca5a5', padding: '6px 8px', borderRadius: 4, fontSize: 11, margin: 0, overflow: 'auto', maxHeight: 240, border: '1px solid #7f1d1d' }}>
          {run.error_message}
        </pre>
      )}

      <h5 style={{ margin: '4px 0', fontSize: 12, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Per-node results</h5>
      {nodes.length === 0 ? (
        <p style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 12, margin: 0 }}>No node-level results recorded for this run.</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
          {nodes.map((nr) => (
            <li key={nr.node_id} style={{ background: '#0b1220', border: '1px solid #1f2937', borderRadius: 4, padding: '8px 10px', display: 'flex', flexDirection: 'column', gap: 6 }}>
              <header style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <strong>{nr.label}</strong>
                <code style={{ fontSize: 11, color: '#94a3b8' }}>({nr.transform_type})</code>
                <span style={{ ...pill(nr.status), padding: '2px 8px', borderRadius: 999, fontSize: 11 }}>{nr.status}</span>
              </header>
              <dl style={{ display: 'flex', gap: 10, margin: 0, fontSize: 11, color: '#94a3b8' }}>
                <dt style={{ color: '#64748b' }}>Attempts</dt><dd style={{ margin: '0 8px 0 4px', color: '#cbd5e1' }}>{nr.attempts}</dd>
                <dt style={{ color: '#64748b' }}>Rows</dt><dd style={{ margin: '0 8px 0 4px', color: '#cbd5e1' }}>{nr.rows_affected ?? '—'}</dd>
              </dl>
              {nr.error && (
                <pre style={{ background: '#020617', color: '#fca5a5', padding: '6px 8px', borderRadius: 4, fontSize: 11, margin: 0, overflow: 'auto', maxHeight: 200, border: '1px solid #7f1d1d' }}>
                  {nr.error}
                </pre>
              )}
              {nr.output && Object.keys(nr.output).length > 0 && (
                <details>
                  <summary style={{ cursor: 'pointer', fontSize: 11, color: '#60a5fa' }}>Output</summary>
                  <pre style={{ background: '#020617', color: '#e2e8f0', padding: '6px 8px', borderRadius: 4, fontSize: 11, margin: '4px 0 0', overflow: 'auto', maxHeight: 200 }}>
                    {JSON.stringify(nr.output, null, 2)}
                  </pre>
                </details>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
