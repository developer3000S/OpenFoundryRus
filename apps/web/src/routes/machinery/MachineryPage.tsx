import { useEffect, useState } from 'react';

import {
  getMachineryInsights,
  getMachineryQueue,
  listObjectTypes,
  listRules,
  type MachineryInsight,
  type MachineryQueueResponse,
  type ObjectType,
  type OntologyRule,
} from '@/lib/api/ontology';
import {
  listWorkflowApprovals,
  listWorkflowRuns,
  listWorkflows,
  type WorkflowApproval,
  type WorkflowDefinition,
  type WorkflowRun,
} from '@/lib/api/workflows';

export function MachineryPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [selectedObjectTypeId, setSelectedObjectTypeId] = useState('');
  const [rules, setRules] = useState<OntologyRule[]>([]);
  const [insights, setInsights] = useState<MachineryInsight[]>([]);
  const [queue, setQueue] = useState<MachineryQueueResponse | null>(null);
  const [workflows, setWorkflows] = useState<WorkflowDefinition[]>([]);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState('');
  const [approvals, setApprovals] = useState<WorkflowApproval[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [otRes, wfRes] = await Promise.all([
        listObjectTypes({ page: 1, per_page: 100 }),
        listWorkflows({ per_page: 200 }),
      ]);
      setObjectTypes(otRes.data);
      setWorkflows(wfRes.data);
      if (!selectedObjectTypeId && otRes.data[0]) setSelectedObjectTypeId(otRes.data[0].id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load machinery');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function loadType() {
      try {
        const [rRes, iRes, qRes] = await Promise.all([
          listRules(selectedObjectTypeId ? { object_type_id: selectedObjectTypeId, per_page: 100 } : { per_page: 100 }),
          getMachineryInsights(selectedObjectTypeId ? { object_type_id: selectedObjectTypeId } : undefined),
          getMachineryQueue(selectedObjectTypeId ? { object_type_id: selectedObjectTypeId } : undefined),
        ]);
        if (cancelled) return;
        setRules(rRes.data);
        setInsights(iRes.data);
        setQueue(qRes);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load type');
      }
    }
    void loadType();
    return () => {
      cancelled = true;
    };
  }, [selectedObjectTypeId]);

  useEffect(() => {
    if (!selectedWorkflowId) return;
    let cancelled = false;
    async function load() {
      try {
        const [aRes, rRes] = await Promise.all([
          listWorkflowApprovals({ per_page: 50, workflow_id: selectedWorkflowId }),
          listWorkflowRuns(selectedWorkflowId, { per_page: 30 }),
        ]);
        if (cancelled) return;
        setApprovals(aRes.data);
        setRuns(rRes.data);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load workflow');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [selectedWorkflowId]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Machinery</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Inspect ontology rules, machinery insights, queue depth and recommendations, plus connected workflows for the
          selected object type.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading && <p className="of-text-muted">Loading…</p>}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Object type:
          <select
            value={selectedObjectTypeId}
            onChange={(e) => setSelectedObjectTypeId(e.target.value)}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            <option value="">All</option>
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>
                {t.display_name}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Workflow:
          <select
            value={selectedWorkflowId}
            onChange={(e) => setSelectedWorkflowId(e.target.value)}
            className="of-input"
            style={{ marginLeft: 6, width: 'auto' }}
          >
            <option value="">— select —</option>
            {workflows.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Rules ({rules.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {rules.map((r) => (
            <li key={r.id}>
              <strong>{r.display_name}</strong> · {r.evaluation_mode}
            </li>
          ))}
        </ul>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Insights ({insights.length})</p>
        <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
          {insights.map((i) => (
            <li key={i.rule_id}>
              <strong>{i.display_name}</strong> · {i.dynamic_pressure} · matched {i.matched_runs}/{i.total_runs}
            </li>
          ))}
        </ul>
      </section>

      {queue && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Queue summary</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
            <span className="of-chip">Depth {queue.recommendation.queue_depth}</span>
            <span className="of-chip">Overdue {queue.recommendation.overdue_count}</span>
            <span className="of-chip">Total minutes {queue.recommendation.total_estimated_minutes}</span>
            <span className="of-chip">Strategy {queue.recommendation.strategy}</span>
          </div>
          <p className="of-eyebrow" style={{ marginTop: 14 }}>Capability load</p>
          <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
            {queue.recommendation.capability_load.map((c) => (
              <li key={c.capability}>
                {c.capability} · {c.pending_count} pending · {c.total_estimated_minutes}m
              </li>
            ))}
          </ul>
        </section>
      )}

      {selectedWorkflowId && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Pending approvals ({approvals.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {approvals.map((a) => (
                <li key={a.id}>
                  <strong>{a.title}</strong> · {a.status}
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
