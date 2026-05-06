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
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Pipelines</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">{pipeline.name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {pipeline.id} · {pipeline.status} · {pipeline.pipeline_type ?? 'BATCH'}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
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
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs tabs={['canvas', 'nodes', 'config', 'runs', 'validate'] as const} active={tab} onChange={setTab} />

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
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Status
            <select value={statusValue} onChange={(e) => setStatusValue(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
              <option value="draft">draft</option>
              <option value="active">active</option>
              <option value="paused">paused</option>
              <option value="archived">archived</option>
            </select>
          </label>
          <JsonEditor label="Nodes JSON (DAG)" value={nodesJson} onChange={setNodesJson} minHeight={320} />
          <JsonEditor label="Schedule config JSON" value={scheduleJson} onChange={setScheduleJson} minHeight={80} />
          <JsonEditor label="Retry policy JSON" value={retryJson} onChange={setRetryJson} minHeight={80} />
        </section>
      )}

      {tab === 'runs' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Runs ({runs.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {runs.map((r) => (
              <li
                key={r.id}
                style={{
                  padding: 10,
                  borderBottom: '1px solid var(--border-default)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 8,
                }}
              >
                <div>
                  <strong>{r.status}</strong> · attempt {r.attempt_number} · {r.trigger_type} · {new Date(r.started_at).toLocaleString()}
                  {r.error_message && <p className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>{r.error_message}</p>}
                </div>
                <button type="button" onClick={() => void retryRun(r.id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                  Retry
                </button>
              </li>
            ))}
            {runs.length === 0 && <li className="of-text-muted">No runs yet.</li>}
          </ul>
        </section>
      )}

      {tab === 'validate' && (
        <section className="of-panel" style={{ padding: 16 }}>
          {validation ? (
            <>
              <p className="of-eyebrow">{validation.valid ? '✓ Valid' : '✗ Invalid'}</p>
              {validation.errors.length > 0 && (
                <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
                  {validation.errors.map((e, i) => (
                    <li key={i} style={{ color: '#b91c1c' }}>{e}</li>
                  ))}
                </ul>
              )}
            </>
          ) : (
            <p className="of-text-muted">Click "Validate" to run server-side DAG validation.</p>
          )}
        </section>
      )}
    </section>
  );
}
