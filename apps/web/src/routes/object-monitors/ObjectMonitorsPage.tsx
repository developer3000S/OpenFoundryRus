import { useEffect, useState } from 'react';

import { listAnomalies, listEvents, type AnomalyAlert, type AuditEvent } from '@/lib/api/audit';
import {
  listActionTypes,
  listFunctionPackages,
  listObjectSets,
  listObjectTypes,
  type ActionType,
  type FunctionPackage,
  type ObjectSetDefinition,
  type ObjectType,
} from '@/lib/api/ontology';
import {
  createWorkflow,
  deleteWorkflow,
  listWorkflowApprovals,
  listWorkflowRuns,
  listWorkflows,
  updateWorkflow,
  type WorkflowApproval,
  type WorkflowDefinition,
  type WorkflowRun,
  type WorkflowStep,
} from '@/lib/api/workflows';

interface MonitorDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  trigger_type: string;
  object_set_id: string;
  object_type_id: string;
  trigger_event_name: string;
  steps_json: string;
}

function emptyDraft(): MonitorDraft {
  return {
    name: 'Object monitor',
    description: 'Monitor object set for anomalous activity.',
    status: 'draft',
    trigger_type: 'event',
    object_set_id: '',
    object_type_id: '',
    trigger_event_name: 'ontology.object_set.changed',
    steps_json: JSON.stringify(
      [
        {
          id: crypto.randomUUID(),
          name: 'Notify ops',
          step_type: 'notification',
          description: '',
          config: { title: 'Monitor fired', channels: ['in_app'], severity: 'medium' },
          next_step_id: null,
          branches: [],
        },
      ],
      null,
      2,
    ),
  };
}

export function ObjectMonitorsPage() {
  const [workflows, setWorkflows] = useState<WorkflowDefinition[]>([]);
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [objectSets, setObjectSets] = useState<ObjectSetDefinition[]>([]);
  const [actions, setActions] = useState<ActionType[]>([]);
  const [functions, setFunctions] = useState<FunctionPackage[]>([]);
  const [anomalies, setAnomalies] = useState<AnomalyAlert[]>([]);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [approvals, setApprovals] = useState<WorkflowApproval[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [selectedId, setSelectedId] = useState('');
  const [draft, setDraft] = useState<MonitorDraft>(emptyDraft);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [wfRes, otRes, osRes, atRes, fnRes, anRes, evRes] = await Promise.all([
        listWorkflows({ per_page: 200 }),
        listObjectTypes({ per_page: 200 }),
        listObjectSets(),
        listActionTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
        listFunctionPackages({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
        listAnomalies().catch(() => []),
        listEvents({}).catch(() => ({ items: [] })),
      ]);
      setWorkflows(wfRes.data.filter((w) => w.trigger_config['object_set_id'] || w.trigger_config['object_type_id']));
      setObjectTypes(otRes.data);
      setObjectSets(osRes.data);
      setActions(atRes.data);
      setFunctions(fnRes.data);
      setAnomalies(anRes);
      setEvents(evRes.items);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load monitors');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedId) return;
    let cancelled = false;
    async function load() {
      try {
        const [appRes, runRes] = await Promise.all([
          listWorkflowApprovals({ workflow_id: selectedId, per_page: 50 }),
          listWorkflowRuns(selectedId, { per_page: 30 }),
        ]);
        if (cancelled) return;
        setApprovals(appRes.data);
        setRuns(runRes.data);
        const wf = workflows.find((w) => w.id === selectedId);
        if (wf) {
          setDraft({
            id: wf.id,
            name: wf.name,
            description: wf.description,
            status: wf.status,
            trigger_type: wf.trigger_type,
            object_set_id: String(wf.trigger_config['object_set_id'] ?? ''),
            object_type_id: String(wf.trigger_config['object_type_id'] ?? ''),
            trigger_event_name: String(wf.trigger_config['event_name'] ?? ''),
            steps_json: JSON.stringify(wf.steps, null, 2),
          });
        }
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load monitor');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [selectedId, workflows]);

  async function saveMonitor() {
    setError('');
    try {
      let steps: WorkflowStep[];
      try {
        steps = JSON.parse(draft.steps_json) as WorkflowStep[];
      } catch {
        throw new Error('Invalid steps JSON');
      }
      const triggerConfig: Record<string, unknown> = {};
      if (draft.object_set_id) triggerConfig.object_set_id = draft.object_set_id;
      if (draft.object_type_id) triggerConfig.object_type_id = draft.object_type_id;
      if (draft.trigger_event_name) triggerConfig.event_name = draft.trigger_event_name;
      const payload = {
        name: draft.name,
        description: draft.description,
        status: draft.status,
        trigger_type: draft.trigger_type,
        trigger_config: triggerConfig,
        steps,
      };
      const saved = draft.id ? await updateWorkflow(draft.id, payload) : await createWorkflow(payload);
      setSelectedId(saved.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save monitor');
    }
  }

  async function removeMonitor() {
    if (!draft.id) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete this monitor?')) return;
    try {
      await deleteWorkflow(draft.id);
      setSelectedId('');
      setDraft(emptyDraft());
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete monitor');
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Object monitors</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Define event/cron-driven monitors over object sets and types. Each monitor is a workflow with notification or
          submit-action steps. Lists active approvals and recent runs for the selected monitor.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading && <p className="of-text-muted">Loading…</p>}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Catalog</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
          <span className="of-chip">Monitors {workflows.length}</span>
          <span className="of-chip">Object types {objectTypes.length}</span>
          <span className="of-chip">Object sets {objectSets.length}</span>
          <span className="of-chip">Actions {actions.length}</span>
          <span className="of-chip">Functions {functions.length}</span>
          <span className="of-chip">Anomalies {anomalies.length}</span>
          <span className="of-chip">Recent events {events.length}</span>
        </div>
      </section>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Monitors</p>
          <button
            type="button"
            onClick={() => {
              setSelectedId('');
              setDraft(emptyDraft());
            }}
            className="of-button"
            style={{ marginTop: 8, fontSize: 12 }}
          >
            New monitor
          </button>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {workflows.map((w) => {
              const active = selectedId === w.id;
              return (
                <li key={w.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(w.id)}
                    style={{
                      width: '100%',
                      textAlign: 'left',
                      padding: 10,
                      borderRadius: 8,
                      border: `1px solid ${active ? '#1d4ed8' : 'var(--border-default)'}`,
                      background: active ? '#eff6ff' : 'transparent',
                      cursor: 'pointer',
                      marginBottom: 4,
                    }}
                  >
                    <strong>{w.name}</strong>{' '}
                    <span className="of-chip" style={{ fontSize: 10 }}>{w.trigger_type}</span>
                    <p className="of-text-muted" style={{ fontSize: 11, marginTop: 2 }}>{w.status}</p>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>

        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Monitor draft</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
            <label style={{ fontSize: 13 }}>
              Name
              <input value={draft.name} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <textarea value={draft.description} onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))} className="of-input" style={{ marginTop: 4, minHeight: 60 }} />
            </label>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <label style={{ fontSize: 13 }}>
                Trigger type
                <select value={draft.trigger_type} onChange={(e) => setDraft((d) => ({ ...d, trigger_type: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                  <option value="event">Event</option>
                  <option value="cron">Cron</option>
                  <option value="manual">Manual</option>
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Status
                <select value={draft.status} onChange={(e) => setDraft((d) => ({ ...d, status: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                  <option value="draft">Draft</option>
                  <option value="active">Active</option>
                  <option value="paused">Paused</option>
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Object set
                <select value={draft.object_set_id} onChange={(e) => setDraft((d) => ({ ...d, object_set_id: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                  <option value="">— none —</option>
                  {objectSets.map((s) => (
                    <option key={s.id} value={s.id}>{s.name}</option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Object type
                <select value={draft.object_type_id} onChange={(e) => setDraft((d) => ({ ...d, object_type_id: e.target.value }))} className="of-input" style={{ marginTop: 4 }}>
                  <option value="">— none —</option>
                  {objectTypes.map((t) => (
                    <option key={t.id} value={t.id}>{t.display_name}</option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                Trigger event name
                <input value={draft.trigger_event_name} onChange={(e) => setDraft((d) => ({ ...d, trigger_event_name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
              </label>
            </div>
            <label style={{ fontSize: 13 }}>
              Steps JSON (workflow steps)
              <textarea
                value={draft.steps_json}
                onChange={(e) => setDraft((d) => ({ ...d, steps_json: e.target.value }))}
                className="of-input"
                style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 220 }}
              />
            </label>
            <div style={{ display: 'flex', gap: 6 }}>
              <button type="button" onClick={() => void saveMonitor()} className="of-button of-button--primary">
                {draft.id ? 'Update monitor' : 'Create monitor'}
              </button>
              {draft.id && (
                <button type="button" onClick={() => void removeMonitor()} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
                  Delete
                </button>
              )}
            </div>
          </div>
        </section>
      </div>

      {selectedId && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Pending approvals ({approvals.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {approvals.map((a) => (
                <li key={a.id}>
                  <strong>{a.title}</strong> · {a.status} · run {a.workflow_run_id.slice(0, 8)}
                </li>
              ))}
            </ul>
          </section>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Recent runs ({runs.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {runs.map((r) => (
                <li key={r.id}>
                  {r.trigger_type} · {r.status} · {new Date(r.started_at).toLocaleString()}
                </li>
              ))}
            </ul>
          </section>
        </>
      )}
    </section>
  );
}
