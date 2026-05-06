import { useEffect, useMemo, useState } from 'react';

import {
  getCurrentStreamView,
  getStreamConfig,
  resetStream,
  updateStreamConfig,
  type StreamConfig,
  type StreamConsistency,
  type StreamKind,
  type StreamType,
  type StreamView,
} from '@/lib/api/streaming';

interface Props {
  streamId: string;
  streamName?: string;
  streamKind?: StreamKind;
}

const STREAM_TYPES: Array<{ value: StreamType; label: string; hint: string }> = [
  { value: 'STANDARD', label: 'Standard', hint: 'Low latency, no batching tweaks.' },
  { value: 'HIGH_THROUGHPUT', label: 'High throughput', hint: 'Larger batches; introduces some latency.' },
  { value: 'COMPRESSED', label: 'Compressed', hint: 'lz4 compression on producer batches.' },
  { value: 'HIGH_THROUGHPUT_COMPRESSED', label: 'High throughput + compressed', hint: 'Both, for high-volume streams with repetitive payloads.' },
];

function pipelineConsistencyLabel(value: StreamConsistency): string {
  return value === 'EXACTLY_ONCE'
    ? 'EXACTLY_ONCE — records visible only after each checkpoint'
    : 'AT_LEAST_ONCE — lower latency, duplicates possible';
}

function buildPushUrl(viewRid: string): string {
  return `${window.location.origin}/streams-push/${viewRid}/records`;
}

export function StreamSettings({ streamId, streamName = '', streamKind = 'INGEST' }: Props) {
  const [config, setConfig] = useState<StreamConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [savedMessage, setSavedMessage] = useState('');

  const [resetModalOpen, setResetModalOpen] = useState(false);
  const [resetSchemaJson, setResetSchemaJson] = useState('');
  const [resetForceReset, setResetForceReset] = useState(false);
  const [resetConfirmName, setResetConfirmName] = useState('');
  const [resetSubmitting, setResetSubmitting] = useState(false);
  const [resetError, setResetError] = useState('');
  const [resetSuccess, setResetSuccess] = useState<{ newViewRid: string; pushUrl: string } | null>(null);

  const [currentView, setCurrentView] = useState<StreamView | null>(null);
  const [pushUrl, setPushUrl] = useState('');
  const [pushUrlError, setPushUrlError] = useState('');
  const [copiedKey, setCopiedKey] = useState<'url' | 'curl' | ''>('');

  const resetConfirmMatches = useMemo(
    () => streamName.length > 0 && resetConfirmName.trim() === streamName.trim(),
    [streamName, resetConfirmName],
  );

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [cfg, view] = await Promise.all([
        getStreamConfig(streamId),
        getCurrentStreamView(streamId).catch(() => null),
      ]);
      setConfig(cfg);
      if (view) {
        setCurrentView(view);
        setPushUrl(buildPushUrl(view.view_rid));
        setResetSchemaJson(view.schema_json ? JSON.stringify(view.schema_json, null, 2) : '');
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streamId]);

  async function copyToClipboard(text: string, key: 'url' | 'curl') {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedKey(key);
      setTimeout(() => setCopiedKey(''), 1500);
    } catch (cause) {
      setPushUrlError(`Could not copy: ${cause instanceof Error ? cause.message : String(cause)}`);
    }
  }

  function curlSnippet(): string {
    if (!pushUrl) return '';
    return [
      'curl -X POST \\',
      `  "${pushUrl}" \\`,
      '  -H "Authorization: Bearer $ACCESS_TOKEN" \\',
      '  -H "Content-Type: application/json" \\',
      "  -d '[{ \"value\": {\"sensor_id\":\"sensor1\",\"temperature\":4.1} }]'",
    ].join('\n');
  }

  function setStreamType(value: StreamType) {
    if (!config) return;
    setConfig({
      ...config,
      stream_type: value,
      compression: value === 'COMPRESSED' || value === 'HIGH_THROUGHPUT_COMPRESSED',
    });
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!config) return;
    setSaving(true);
    setError('');
    setSavedMessage('');
    try {
      const next = await updateStreamConfig(streamId, {
        stream_type: config.stream_type,
        compression: config.compression,
        partitions: config.partitions,
        retention_ms: config.retention_ms,
        pipeline_consistency: config.pipeline_consistency,
        checkpoint_interval_ms: config.checkpoint_interval_ms,
      });
      setConfig(next);
      setSavedMessage('Saved. Pipelines consuming this stream will need redeploy.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  async function submitReset(e: React.FormEvent) {
    e.preventDefault();
    if (!resetConfirmMatches) {
      setResetError('Type the stream name exactly to confirm.');
      return;
    }
    setResetSubmitting(true);
    setResetError('');
    try {
      let parsedSchema: unknown | undefined;
      if (resetSchemaJson.trim().length > 0) {
        try { parsedSchema = JSON.parse(resetSchemaJson); }
        catch (parseErr) {
          setResetError(`Schema must be valid JSON: ${parseErr instanceof Error ? parseErr.message : String(parseErr)}`);
          setResetSubmitting(false);
          return;
        }
      }
      const new_config = config ? {
        stream_type: config.stream_type,
        compression: config.compression,
        partitions: config.partitions,
        retention_ms: config.retention_ms,
        pipeline_consistency: config.pipeline_consistency,
        checkpoint_interval_ms: config.checkpoint_interval_ms,
      } : undefined;
      const response = await resetStream(streamId, { new_schema: parsedSchema, new_config, force: resetForceReset });
      setResetSuccess({ newViewRid: response.new_view_rid, pushUrl: response.push_url });
      setPushUrl(response.push_url);
      setCurrentView(response.view);
      setSavedMessage(`Stream reset: viewRid is now ${response.new_view_rid}`);
      setResetConfirmName('');
    } catch (cause) {
      setResetError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setResetSubmitting(false);
    }
  }

  return (
    <section style={{ display: 'grid', gap: 16 }}>
      {loading ? (
        <p>Loading stream settings…</p>
      ) : error && !config ? (
        <p style={{ color: '#b00' }}>{error}</p>
      ) : config ? (
        <>
          <form onSubmit={save}>
            <Card>
              <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>Stream type</h3>
              <Hint>Tunes the Kafka producer's batching and compression. Only change after inspecting stream metrics.</Hint>
              <div style={{ display: 'grid', gap: 6 }}>
                {STREAM_TYPES.map((opt) => (
                  <label key={opt.value} style={{ display: 'flex', gap: 8, alignItems: 'flex-start', padding: 6, borderRadius: 4 }}>
                    <input type="radio" name="stream_type" value={opt.value} checked={config.stream_type === opt.value} onChange={() => setStreamType(opt.value)} />
                    <span>
                      <strong>{opt.label}</strong>
                      <small style={{ display: 'block', color: '#666', fontWeight: 'normal' }}>{opt.hint}</small>
                    </span>
                  </label>
                ))}
              </div>
            </Card>

            <Card>
              <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>Partitions</h3>
              <Hint>Each partition adds ~5 MB/s of throughput. Range 1..50.</Hint>
              <input type="range" min={1} max={50} step={1} value={config.partitions} onChange={(e) => setConfig({ ...config, partitions: Number(e.target.value) })} style={{ width: '100%' }} />
              <div style={{ display: 'flex', gap: 16, justifyContent: 'space-between', marginTop: 6, fontSize: 13 }}>
                <span><strong>{config.partitions}</strong> partition{config.partitions === 1 ? '' : 's'}</span>
                <span title="Estimate based on Foundry's ~5 MB/s per partition heuristic">≈ {(config.partitions * 5).toLocaleString()} MB/s ceiling</span>
              </div>
            </Card>

            <Card>
              <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>Streaming consistency guarantees</h3>
              <Hint>Streaming sources only support <code>AT_LEAST_ONCE</code> for extracts and exports. Streaming pipelines may opt into <code>EXACTLY_ONCE</code>.</Hint>
              <ConsistencyRow label="Ingest">
                <span style={{ display: 'inline-block', padding: '3px 8px', background: '#eef', borderRadius: 3, color: '#335' }} title="Foundry streaming sources only support AT_LEAST_ONCE for extracts and exports.">AT_LEAST_ONCE (locked)</span>
              </ConsistencyRow>
              <ConsistencyRow label="Pipeline">
                <select value={config.pipeline_consistency} onChange={(e) => setConfig({ ...config, pipeline_consistency: e.target.value as StreamConsistency })}>
                  <option value="AT_LEAST_ONCE">AT_LEAST_ONCE</option>
                  <option value="EXACTLY_ONCE">EXACTLY_ONCE</option>
                </select>
              </ConsistencyRow>
              <small style={{ color: '#666', fontSize: 13 }}>{pipelineConsistencyLabel(config.pipeline_consistency)}</small>
              <ConsistencyRow label="Checkpoint interval (ms)">
                <input type="number" min={100} step={100} value={config.checkpoint_interval_ms} onChange={(e) => setConfig({ ...config, checkpoint_interval_ms: Number(e.target.value) })} />
              </ConsistencyRow>
            </Card>

            {error && <p style={{ color: '#b00' }}>{error}</p>}
            {savedMessage && <p style={{ color: '#2a7' }}>{savedMessage}</p>}

            <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
              <button type="submit" disabled={saving} className="of-button of-button--primary">{saving ? 'Saving…' : 'Save settings'}</button>
              <small style={{ color: '#666', fontSize: 13 }}>Pipelines consuming this stream will need redeploy.</small>
            </div>
          </form>

          <Card>
            <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>Push URL</h3>
            <Hint>Active POST URL for push consumers. <strong>This URL will rotate on the next reset</strong>; clients should refetch when they receive a <code>stream.reset.v1</code> event.</Hint>
            {pushUrlError && <p style={{ color: '#b00' }}>{pushUrlError}</p>}
            {currentView ? (
              <>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                  <code style={{ flex: '1 1 12rem', padding: '4px 8px', background: '#f3f3f3', borderRadius: 3, wordBreak: 'break-all' }}>{pushUrl}</code>
                  <button type="button" onClick={() => void copyToClipboard(pushUrl, 'url')} className="of-button">{copiedKey === 'url' ? 'Copied!' : 'Copy URL'}</button>
                </div>
                <p style={{ color: '#666', fontSize: 13 }}>
                  viewRid <code>{currentView.view_rid}</code> · generation <strong>{currentView.generation}</strong>
                </p>
                <details>
                  <summary>Example curl</summary>
                  <pre style={{ background: '#f6f6f6', padding: 8, overflow: 'auto', borderRadius: 4 }}><code>{curlSnippet()}</code></pre>
                  <button type="button" onClick={() => void copyToClipboard(curlSnippet(), 'curl')} className="of-button">{copiedKey === 'curl' ? 'Copied!' : 'Copy curl example'}</button>
                </details>
              </>
            ) : (
              <Hint>No active view. Reset the stream to mint one.</Hint>
            )}
          </Card>

          {streamKind === 'INGEST' && (
            <Card style={{ borderColor: '#f1c2c0' }}>
              <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>Reset stream</h3>
              <Banner>Resetting clears existing records and rotates the viewRid. <strong>This is irreversible.</strong></Banner>
              <Hint>Push consumers will get <code>404 PUSH_VIEW_RETIRED</code> against the old URL until they re-fetch. Downstream pipelines must be replayed.</Hint>
              <button type="button" onClick={() => { setResetError(''); setResetSuccess(null); setResetForceReset(false); setResetConfirmName(''); setResetModalOpen(true); }} style={dangerBtn}>Reset stream…</button>
            </Card>
          )}
        </>
      ) : null}

      {resetModalOpen && (
        <div onClick={(e) => { if (e.target === e.currentTarget) setResetModalOpen(false); }} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }} role="presentation">
          <div role="dialog" aria-modal="true" aria-labelledby="reset-modal-title" style={{ background: '#fff', borderRadius: 6, padding: '16px 20px', width: 'min(540px, 95vw)', boxShadow: '0 20px 50px rgba(0,0,0,0.2)' }}>
            <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <h2 id="reset-modal-title" style={{ margin: 0, fontSize: 18 }}>Reset stream</h2>
              <button type="button" onClick={() => setResetModalOpen(false)} aria-label="Close" style={{ background: 'transparent', border: 0, fontSize: 20, cursor: 'pointer' }}>×</button>
            </header>
            <Banner>This deletes existing records and rotates the viewRid. Downstream pipelines must replay.</Banner>
            <form onSubmit={submitReset} style={{ display: 'grid', gap: 12 }}>
              <label style={{ display: 'grid', gap: 4 }}>
                <span>New schema (optional, JSON)</span>
                <textarea rows={8} value={resetSchemaJson} onChange={(e) => setResetSchemaJson(e.target.value)} placeholder="Leave blank to reuse the current schema" />
              </label>
              <label style={{ display: 'grid', gap: 4, gridTemplateColumns: 'auto 1fr', alignItems: 'start' }}>
                <input type="checkbox" checked={resetForceReset} onChange={(e) => setResetForceReset(e.target.checked)} />
                <span>
                  <strong>Force reset</strong>
                  <small style={{ display: 'block', color: '#666' }}>Required when downstream pipelines are still active. They must be replayed against the new viewRid.</small>
                </span>
              </label>
              <label style={{ display: 'grid', gap: 4 }}>
                <span>Type the stream name to confirm {streamName && (<>(<code>{streamName}</code>)</>)}</span>
                <input type="text" value={resetConfirmName} onChange={(e) => setResetConfirmName(e.target.value)} placeholder={streamName} />
              </label>
              {resetError && <p style={{ color: '#b00' }}>{resetError}</p>}
              {resetSuccess && (
                <p style={{ color: '#2a7' }}>
                  New viewRid: <code>{resetSuccess.newViewRid}</code><br />
                  New POST URL: <code>{resetSuccess.pushUrl}</code>
                </p>
              )}
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                <button type="button" onClick={() => setResetModalOpen(false)} className="of-button">Cancel</button>
                <button type="submit" disabled={resetSubmitting || !resetConfirmMatches} style={dangerBtn}>{resetSubmitting ? 'Resetting…' : 'Reset stream'}</button>
              </div>
            </form>
          </div>
        </div>
      )}
    </section>
  );
}

function Card({ children, style }: { children: React.ReactNode; style?: React.CSSProperties }) {
  return <div style={{ background: '#fff', border: '1px solid #e5e5e5', borderRadius: 6, padding: 16, ...style }}>{children}</div>;
}

function Hint({ children }: { children: React.ReactNode }) {
  return <p style={{ color: '#666', fontSize: 13, margin: '0 0 8px' }}>{children}</p>;
}

function Banner({ children }: { children: React.ReactNode }) {
  return <div role="alert" style={{ padding: '8px 12px', borderRadius: 4, marginBottom: 8, background: '#fdecea', borderLeft: '4px solid #b00020', color: '#720010' }}>{children}</div>;
}

function ConsistencyRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '11rem 1fr', alignItems: 'center', gap: 8, margin: '6px 0' }}>
      <span style={{ fontWeight: 600 }}>{label}</span>
      <span>{children}</span>
    </div>
  );
}

const dangerBtn: React.CSSProperties = { background: '#b00020', color: '#fff', border: 0, borderRadius: 4, padding: '7px 14px', cursor: 'pointer' };
