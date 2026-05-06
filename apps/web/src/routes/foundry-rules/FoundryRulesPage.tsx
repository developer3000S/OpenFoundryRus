import { useEffect, useState } from 'react';

import {
  applyRule,
  createRule,
  deleteRule,
  getMachineryInsights,
  getMachineryQueue,
  listObjectTypes,
  listRules,
  simulateRule,
  updateRule,
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
import { JsonEditor } from '@/lib/components/JsonEditor';

interface RuleDraft {
  id?: string;
  name: string;
  display_name: string;
  description: string;
  object_type_id: string;
  evaluation_mode: 'advisory' | 'automatic';
  trigger_text: string;
  effect_text: string;
}

function emptyDraft(typeId = ''): RuleDraft {
  return {
    name: 'rule_threshold_breach',
    display_name: 'Threshold breach',
    description: '',
    object_type_id: typeId,
    evaluation_mode: 'advisory',
    trigger_text: JSON.stringify({ numeric_gte: { score: 0.8 } }, null, 2),
    effect_text: JSON.stringify({ alert: { severity: 'high', title: 'Threshold breach' } }, null, 2),
  };
}

export function FoundryRulesPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [selectedTypeId, setSelectedTypeId] = useState('');
  const [rules, setRules] = useState<OntologyRule[]>([]);
  const [insights, setInsights] = useState<MachineryInsight[]>([]);
  const [queue, setQueue] = useState<MachineryQueueResponse | null>(null);
  const [workflows, setWorkflows] = useState<WorkflowDefinition[]>([]);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState('');
  const [approvals, setApprovals] = useState<WorkflowApproval[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [draft, setDraft] = useState<RuleDraft>(emptyDraft());
  const [simulationObjectId, setSimulationObjectId] = useState('');
  const [simulationResult, setSimulationResult] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function refresh() {
    setError('');
    try {
      const [otRes, wfRes] = await Promise.all([
        listObjectTypes({ per_page: 200 }),
        listWorkflows({ per_page: 200 }),
      ]);
      setObjectTypes(otRes.data);
      setWorkflows(wfRes.data);
      if (!selectedTypeId && otRes.data[0]) {
        setSelectedTypeId(otRes.data[0].id);
        setDraft(emptyDraft(otRes.data[0].id));
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!selectedTypeId) return;
    let cancelled = false;
    async function load() {
      try {
        const params = { object_type_id: selectedTypeId, per_page: 100 };
        const [rRes, iRes, qRes] = await Promise.all([
          listRules(params),
          getMachineryInsights({ object_type_id: selectedTypeId }),
          getMachineryQueue({ object_type_id: selectedTypeId }),
        ]);
        if (cancelled) return;
        setRules(rRes.data);
        setInsights(iRes.data);
        setQueue(qRes);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load type');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [selectedTypeId]);

  useEffect(() => {
    if (!selectedWorkflowId) return;
    let cancelled = false;
    async function load() {
      try {
        const [aRes, rRes] = await Promise.all([
          listWorkflowApprovals({ per_page: 50, status: 'pending', workflow_id: selectedWorkflowId }),
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

  function loadRule(rule: OntologyRule) {
    setDraft({
      id: rule.id,
      name: rule.name,
      display_name: rule.display_name,
      description: rule.description,
      object_type_id: rule.object_type_id,
      evaluation_mode: rule.evaluation_mode,
      trigger_text: JSON.stringify(rule.trigger_spec ?? {}, null, 2),
      effect_text: JSON.stringify(rule.effect_spec ?? {}, null, 2),
    });
  }

  async function saveRule() {
    setBusy(true);
    setError('');
    try {
      const trigger_spec = JSON.parse(draft.trigger_text);
      const effect_spec = JSON.parse(draft.effect_text);
      if (draft.id) {
        await updateRule(draft.id, {
          display_name: draft.display_name,
          description: draft.description,
          evaluation_mode: draft.evaluation_mode,
          trigger_spec,
          effect_spec,
        });
      } else {
        await createRule({
          name: draft.name,
          display_name: draft.display_name,
          description: draft.description,
          object_type_id: draft.object_type_id,
          evaluation_mode: draft.evaluation_mode,
          trigger_spec,
          effect_spec,
        });
      }
      const params = { object_type_id: selectedTypeId, per_page: 100 };
      const rRes = await listRules(params);
      setRules(rRes.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save rule');
    } finally {
      setBusy(false);
    }
  }

  async function removeRule() {
    if (!draft.id) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete rule?')) return;
    setBusy(true);
    try {
      await deleteRule(draft.id);
      setDraft(emptyDraft(selectedTypeId));
      const params = { object_type_id: selectedTypeId, per_page: 100 };
      const rRes = await listRules(params);
      setRules(rRes.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete');
    } finally {
      setBusy(false);
    }
  }

  async function simulate() {
    if (!draft.id) return;
    setBusy(true);
    try {
      const res = await simulateRule(draft.id, { object_id: simulationObjectId });
      setSimulationResult(res);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Simulate failed');
    } finally {
      setBusy(false);
    }
  }

  async function apply() {
    if (!draft.id) return;
    setBusy(true);
    try {
      const res = await applyRule(draft.id, { object_id: simulationObjectId });
      setSimulationResult(res);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Apply failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Foundry rules</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Per object-type rules with trigger and effect specs. Simulate or apply against a target object id.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Object type:
          <select value={selectedTypeId} onChange={(e) => { setSelectedTypeId(e.target.value); setDraft(emptyDraft(e.target.value)); }} className="of-input" style={{ marginLeft: 6, width: 'auto' }}>
            {objectTypes.map((t) => (
              <option key={t.id} value={t.id}>{t.display_name}</option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          Workflow:
          <select value={selectedWorkflowId} onChange={(e) => setSelectedWorkflowId(e.target.value)} className="of-input" style={{ marginLeft: 6, width: 'auto' }}>
            <option value="">— select —</option>
            {workflows.map((w) => (
              <option key={w.id} value={w.id}>{w.name}</option>
            ))}
          </select>
        </label>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Rules ({rules.length})</p>
          <button
            type="button"
            onClick={() => setDraft(emptyDraft(selectedTypeId))}
            className="of-button"
            style={{ marginTop: 8, fontSize: 12 }}
          >
            New rule
          </button>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {rules.map((r) => (
              <li key={r.id}>
                <button
                  type="button"
                  onClick={() => loadRule(r)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 10,
                    borderRadius: 8,
                    border: `1px solid ${draft.id === r.id ? '#1d4ed8' : 'var(--border-default)'}`,
                    background: draft.id === r.id ? '#eff6ff' : 'transparent',
                    cursor: 'pointer',
                    marginBottom: 4,
                  }}
                >
                  <strong>{r.display_name}</strong>{' '}
                  <span className="of-chip">{r.evaluation_mode}</span>
                </button>
              </li>
            ))}
          </ul>

          {insights.length > 0 && (
            <>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Insights</p>
              <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
                {insights.slice(0, 5).map((i) => (
                  <li key={i.rule_id}>
                    <strong>{i.display_name}</strong> · {i.dynamic_pressure} · {i.matched_runs}/{i.total_runs}
                  </li>
                ))}
              </ul>
            </>
          )}

          {queue && (
            <p className="of-text-muted" style={{ marginTop: 14, fontSize: 12 }}>
              Queue: {queue.recommendation.queue_depth} pending · {queue.recommendation.overdue_count} overdue
            </p>
          )}
        </section>

        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Rule draft</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
            <label style={{ fontSize: 13 }}>
              Name (snake_case)
              <input value={draft.name} disabled={Boolean(draft.id)} onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Display name
              <input value={draft.display_name} onChange={(e) => setDraft((d) => ({ ...d, display_name: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <input value={draft.description} onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Evaluation mode
              <select value={draft.evaluation_mode} onChange={(e) => setDraft((d) => ({ ...d, evaluation_mode: e.target.value as 'advisory' | 'automatic' }))} className="of-input" style={{ marginTop: 4 }}>
                <option value="advisory">advisory</option>
                <option value="automatic">automatic</option>
              </select>
            </label>
            <JsonEditor
              label="Trigger spec JSON"
              value={draft.trigger_text}
              onChange={(v) => setDraft((d) => ({ ...d, trigger_text: v }))}
              minHeight={140}
            />
            <JsonEditor
              label="Effect spec JSON"
              value={draft.effect_text}
              onChange={(v) => setDraft((d) => ({ ...d, effect_text: v }))}
              minHeight={140}
            />
            <div style={{ display: 'flex', gap: 6 }}>
              <button type="button" onClick={() => void saveRule()} disabled={busy} className="of-button of-button--primary">
                {draft.id ? 'Update' : 'Create'}
              </button>
              {draft.id && (
                <button type="button" onClick={() => void removeRule()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
                  Delete
                </button>
              )}
            </div>
          </div>

          {draft.id && (
            <>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Simulate / apply</p>
              <input
                value={simulationObjectId}
                onChange={(e) => setSimulationObjectId(e.target.value)}
                placeholder="object_id"
                className="of-input"
                style={{ marginTop: 6 }}
              />
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button type="button" onClick={() => void simulate()} disabled={busy || !simulationObjectId} className="of-button">
                  Simulate
                </button>
                <button type="button" onClick={() => void apply()} disabled={busy || !simulationObjectId} className="of-button of-button--primary">
                  Apply
                </button>
              </div>
              {!!simulationResult && (
                <pre style={{ marginTop: 10, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 280 }}>
                  {JSON.stringify(simulationResult, null, 2)}
                </pre>
              )}
            </>
          )}
        </section>
      </div>

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
