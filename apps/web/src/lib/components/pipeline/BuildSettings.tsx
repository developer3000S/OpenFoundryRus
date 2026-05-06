import { useState } from 'react';

import type { StreamConsistency } from '@/lib/api/streaming';
import { StreamingProfileSelector } from './StreamingProfileSelector';

export type AbortPolicy = 'DEPENDENT_ONLY' | 'ALL_NON_DEPENDENT';

interface BuildSettingsProps {
  streamingConsistency?: StreamConsistency;
  onStreamingConsistencyChange?: (next: StreamConsistency) => void;
  isStreamingPipeline?: boolean;
  buildBranch?: string;
  onBuildBranchChange?: (next: string) => void;
  jobSpecFallback?: string[];
  onJobSpecFallbackChange?: (next: string[]) => void;
  inputFallbackOverrides?: Array<{ datasetRid: string; fallbackChain: string[] }>;
  onInputFallbackOverridesChange?: (next: Array<{ datasetRid: string; fallbackChain: string[] }>) => void;
  pipelineRid?: string | null;
  outputDatasetRids?: string[];
  projectRid?: string | null;
  forceBuild?: boolean;
  onForceBuildChange?: (next: boolean) => void;
  abortPolicy?: AbortPolicy;
  onAbortPolicyChange?: (next: AbortPolicy) => void;
  lastBuildStaleOutputs?: Array<{ jobSpecRid: string; outputDatasetRid: string }>;
}

const STREAMING_CONSISTENCY_OPTIONS: StreamConsistency[] = ['AT_LEAST_ONCE', 'EXACTLY_ONCE'];

export function BuildSettings({
  streamingConsistency = 'AT_LEAST_ONCE',
  onStreamingConsistencyChange,
  isStreamingPipeline = true,
  buildBranch = 'master',
  onBuildBranchChange,
  jobSpecFallback = [],
  onJobSpecFallbackChange,
  inputFallbackOverrides = [],
  onInputFallbackOverridesChange,
  pipelineRid = null,
  outputDatasetRids = [],
  projectRid = null,
  forceBuild = false,
  onForceBuildChange,
  abortPolicy = 'DEPENDENT_ONLY',
  onAbortPolicyChange,
  lastBuildStaleOutputs = [],
}: BuildSettingsProps) {
  const [fallbackText, setFallbackText] = useState(jobSpecFallback.join(', '));

  function commitFallback() {
    if (!onJobSpecFallbackChange) return;
    onJobSpecFallbackChange(fallbackText.split(',').map((s) => s.trim()).filter(Boolean));
  }

  function addOverride() {
    if (!onInputFallbackOverridesChange) return;
    onInputFallbackOverridesChange([...inputFallbackOverrides, { datasetRid: '', fallbackChain: [] }]);
  }

  function patchOverride(idx: number, patch: Partial<{ datasetRid: string; fallbackChain: string[] }>) {
    if (!onInputFallbackOverridesChange) return;
    onInputFallbackOverridesChange(inputFallbackOverrides.map((o, i) => (i === idx ? { ...o, ...patch } : o)));
  }

  function removeOverride(idx: number) {
    if (!onInputFallbackOverridesChange) return;
    onInputFallbackOverridesChange(inputFallbackOverrides.filter((_, i) => i !== idx));
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Build settings</h3>

      <fieldset style={{ display: 'grid', gap: 6, padding: 8, border: '1px solid #1f2937', borderRadius: 6 }}>
        <legend style={{ fontSize: 11, color: '#94a3b8', padding: '0 6px' }}>Streaming consistency</legend>
        {!isStreamingPipeline ? (
          <p className="of-text-muted" style={{ fontSize: 12, margin: 0 }}>
            Pipeline does not produce a streaming output — consistency setting hidden.
          </p>
        ) : (
          <select value={streamingConsistency} onChange={(e) => onStreamingConsistencyChange?.(e.target.value as StreamConsistency)} className="of-input">
            {STREAMING_CONSISTENCY_OPTIONS.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        )}
      </fieldset>

      <fieldset style={{ display: 'grid', gap: 6, padding: 8, border: '1px solid #1f2937', borderRadius: 6 }}>
        <legend style={{ fontSize: 11, color: '#94a3b8', padding: '0 6px' }}>Branches</legend>
        <label style={{ fontSize: 12 }}>
          Build branch
          <input value={buildBranch} onChange={(e) => onBuildBranchChange?.(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          JobSpec fallback (comma-separated branches)
          <input
            value={fallbackText}
            onChange={(e) => setFallbackText(e.target.value)}
            onBlur={commitFallback}
            className="of-input"
            style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
          />
        </label>
        <div>
          <p className="of-eyebrow" style={{ fontSize: 10 }}>Per-input overrides</p>
          <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 4 }}>
            {inputFallbackOverrides.map((o, i) => (
              <li key={i} style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 4 }}>
                <input value={o.datasetRid} onChange={(e) => patchOverride(i, { datasetRid: e.target.value })} placeholder="dataset rid" className="of-input" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }} />
                <input value={o.fallbackChain.join(', ')} onChange={(e) => patchOverride(i, { fallbackChain: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} placeholder="branch1, branch2" className="of-input" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }} />
                <button type="button" onClick={() => removeOverride(i)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>×</button>
              </li>
            ))}
          </ul>
          <button type="button" onClick={addOverride} className="of-button" style={{ marginTop: 4, fontSize: 11 }}>+ Override</button>
        </div>
      </fieldset>

      <fieldset style={{ display: 'grid', gap: 6, padding: 8, border: '1px solid #1f2937', borderRadius: 6 }}>
        <legend style={{ fontSize: 11, color: '#94a3b8', padding: '0 6px' }}>Execution</legend>
        <label style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 6 }}>
          <input type="checkbox" checked={forceBuild} onChange={(e) => onForceBuildChange?.(e.target.checked)} />
          Force build (recompute all outputs, skip staleness check)
        </label>
        <label style={{ fontSize: 12 }}>
          Abort policy
          <select value={abortPolicy} onChange={(e) => onAbortPolicyChange?.(e.target.value as AbortPolicy)} className="of-input" style={{ marginTop: 4 }}>
            <option value="DEPENDENT_ONLY">DEPENDENT_ONLY</option>
            <option value="ALL_NON_DEPENDENT">ALL_NON_DEPENDENT</option>
          </select>
        </label>
      </fieldset>

      {lastBuildStaleOutputs.length > 0 && (
        <fieldset style={{ display: 'grid', gap: 6, padding: 8, border: '1px solid #1f2937', borderRadius: 6 }}>
          <legend style={{ fontSize: 11, color: '#94a3b8', padding: '0 6px' }}>Stale outputs in last build ({lastBuildStaleOutputs.length})</legend>
          <ul style={{ paddingLeft: 18, fontSize: 11, fontFamily: 'var(--font-mono)', color: '#94a3b8' }}>
            {lastBuildStaleOutputs.map((o, i) => (
              <li key={i}>{o.outputDatasetRid} ← {o.jobSpecRid.slice(0, 12)}…</li>
            ))}
          </ul>
        </fieldset>
      )}

      {pipelineRid && projectRid && (
        <StreamingProfileSelector pipelineRid={pipelineRid} projectRid={projectRid} />
      )}

      {outputDatasetRids.length > 0 && (
        <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>
          {outputDatasetRids.length} output dataset{outputDatasetRids.length === 1 ? '' : 's'}
        </p>
      )}
    </section>
  );
}
