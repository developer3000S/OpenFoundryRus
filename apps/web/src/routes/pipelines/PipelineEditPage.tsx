import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { JsonEditor } from '@/lib/components/JsonEditor';
import { Tabs } from '@/lib/components/Tabs';
import { PipelineCanvas } from '@/lib/components/pipeline/PipelineCanvas';
import { PipelineNodeList } from '@/lib/components/pipeline/PipelineNodeList';
import {
  getPipeline,
  listRuns,
  retryPipelineRun,
  triggerRun,
  updatePipeline,
  validatePipelineById,
  type Pipeline,
  type PipelineNode,
  type PipelineRun,
  type PipelineValidationResponse,
} from '@/lib/api/pipelines';

export function PipelineEditPage() {
  const { id = '' } = useParams<{ id: string }>();
  const [pipeline, setPipeline] = useState<Pipeline | null>(null);
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [validation, setValidation] = useState<PipelineValidationResponse | null>(null);
  const [tab, setTab] = useState<'canvas' | 'nodes' | 'config' | 'runs' | 'validate'>('canvas');

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [statusValue, setStatusValue] = useState('draft');
  const [nodesJson, setNodesJson] = useState('');
  const [scheduleJson, setScheduleJson] = useState('');
  const [retryJson, setRetryJson] = useState('');

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function load() {
    if (!id) return;
    setLoading(true);
    setError('');
    try {
      const p = await getPipeline(id);
      setPipeline(p);
      setName(p.name);
      setDescription(p.description);
      setStatusValue(p.status);
      setNodesJson(JSON.stringify(p.dag, null, 2));
      setScheduleJson(JSON.stringify(p.schedule_config, null, 2));
      setRetryJson(JSON.stringify(p.retry_policy, null, 2));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load pipeline');
    } finally {
      setLoading(false);
    }
  }

  async function loadRuns() {
    if (!id) return;
    try {
      const res = await listRuns(id, { per_page: 50 });
      setRuns(res.data);
    } catch {
      // ignore — runs are non-critical
    }
  }

  useEffect(() => {
    void load();
    void loadRuns();
  }, [id]);

  async function save() {
    if (!pipeline) return;
    setSaving(true);
    setError('');
    try {
      const updated = await updatePipeline(pipeline.id, {
        name,
        description,
        status: statusValue,
        nodes: JSON.parse(nodesJson),
        schedule_config: JSON.parse(scheduleJson),
        retry_policy: JSON.parse(retryJson),
      });
      setPipeline(updated);
      setNodesJson(JSON.stringify(updated.dag, null, 2));
      setScheduleJson(JSON.stringify(updated.schedule_config, null, 2));
      setRetryJson(JSON.stringify(updated.retry_policy, null, 2));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  }

  async function runNow() {
    if (!pipeline) return;
    setBusy(true);
    try {
      await triggerRun(pipeline.id);
      await loadRuns();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Run failed');
    } finally {
      setBusy(false);
    }
  }

  async function retryRun(runId: string) {
    if (!pipeline) return;
    setBusy(true);
    try {
      await retryPipelineRun(pipeline.id, runId);
      await loadRuns();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Retry failed');
    } finally {
      setBusy(false);
    }
  }

  async function runValidate() {
    if (!pipeline) return;
    setBusy(true);
    try {
      const report = await validatePipelineById(pipeline.id);
      setValidation({
        valid: report.all_valid,
        errors: report.nodes.flatMap((n) => n.errors.map((e) => `${n.node_id}: ${e.message}`)),
        warnings: [],
        next_run_at: null,
        summary: { node_count: report.nodes.length, edge_count: 0, root_node_ids: [], leaf_node_ids: [] },
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Validate failed');
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading pipeline…</p>
      </section>
    );
  }

  if (!pipeline) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Pipelines</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Pipeline not found'}</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <header className="of-panel" style={{ display: 'grid', gap: 8, padding: 10 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
          <div style={{ minWidth: 0 }}>
            <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 12 }}>← Pipelines</Link>
            <h1 className="of-heading-lg" style={{ marginTop: 4 }}>{pipeline.name}</h1>
            <p className="of-text-muted" style={{ marginTop: 2, fontSize: 11, fontFamily: 'var(--font-mono)' }}>
              {pipeline.id}
            </p>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
            <span className="of-chip of-chip-active">{pipeline.status}</span>
            <span className="of-chip">{pipeline.pipeline_type ?? 'BATCH'}</span>
          </div>
        </div>
        <div className="of-toolbar" style={{ borderRadius: 0, margin: '0 -10px -10px', borderRight: 0, borderLeft: 0, borderBottom: 0, justifyContent: 'space-between' }}>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            <select value={statusValue} onChange={(e) => setStatusValue(e.target.value)} className="of-select" style={{ width: 120 }}>
              <option value="draft">draft</option>
              <option value="active">active</option>
              <option value="paused">paused</option>
              <option value="archived">archived</option>
            </select>
            <span className="of-text-muted" style={{ alignSelf: 'center', fontSize: 11 }}>
              {runs.length} run{runs.length === 1 ? '' : 's'}
            </span>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <button type="button" onClick={() => void runValidate()} disabled={busy} className="of-button">
            Validate
          </button>
          <button type="button" onClick={() => void runNow()} disabled={busy} className="of-button">
            Run now
          </button>
          <button type="button" onClick={() => void save()} disabled={saving} className="of-button of-button--primary">
            {saving ? 'Saving…' : 'Save'}
          </button>
          </div>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ overflow: 'hidden' }}>
        <Tabs tabs={['canvas', 'nodes', 'config', 'runs', 'validate'] as const} active={tab} onChange={setTab} />

        <div style={{ padding: tab === 'canvas' ? 0 : 10 }}>
          {tab === 'canvas' && (
            <PipelineCanvas
              nodes={(() => {
                try { return JSON.parse(nodesJson) as PipelineNode[]; }
                catch { return []; }
              })()}
              status={statusValue}
              scheduleConfig={(() => {
                try { return JSON.parse(scheduleJson); }
                catch { return { enabled: false, cron: null }; }
              })()}
              onChange={(next) => setNodesJson(JSON.stringify(next, null, 2))}
            />
          )}

          {tab === 'nodes' && (
            <PipelineNodeList
              nodes={(() => {
                try { return JSON.parse(nodesJson) as PipelineNode[]; }
                catch { return []; }
              })()}
              onChange={(next) => setNodesJson(JSON.stringify(next, null, 2))}
            />
          )}

          {tab === 'config' && (
            <section style={{ display: 'grid', gap: 8 }}>
              <label style={{ fontSize: 12 }}>
                Name
                <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 12 }}>
                Description
                <input value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <JsonEditor label="Nodes JSON (DAG)" value={nodesJson} onChange={setNodesJson} minHeight={320} />
              <JsonEditor label="Schedule config JSON" value={scheduleJson} onChange={setScheduleJson} minHeight={80} />
              <JsonEditor label="Retry policy JSON" value={retryJson} onChange={setRetryJson} minHeight={80} />
            </section>
          )}

          {tab === 'runs' && (
            <table className="of-table">
              <thead><tr><th>Status</th><th>Attempt</th><th>Trigger</th><th>Started</th><th /></tr></thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td>{r.status}</td>
                    <td>{r.attempt_number}</td>
                    <td>{r.trigger_type}</td>
                    <td>{new Date(r.started_at).toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>
                      <button type="button" onClick={() => void retryRun(r.id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                        Retry
                      </button>
                    </td>
                  </tr>
                ))}
                {runs.length === 0 && <tr><td colSpan={5} className="of-text-muted">No runs yet.</td></tr>}
              </tbody>
            </table>
          )}

          {tab === 'validate' && (
            <section>
              {validation ? (
                <>
                  <p className="of-eyebrow">{validation.valid ? '✓ Valid' : '✗ Invalid'}</p>
                  {validation.errors.length > 0 && (
                    <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
                      {validation.errors.map((e, i) => (
                        <li key={i} style={{ color: '#b42318' }}>{e}</li>
                      ))}
                    </ul>
                  )}
                </>
              ) : (
                <p className="of-text-muted">Click "Validate" to run server-side DAG validation.</p>
              )}
            </section>
          )}
        </div>
      </section>
    </section>
  );
}
