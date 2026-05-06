import { useEffect, useState } from 'react';

import { listUsers, type UserProfile } from '@/lib/api/auth';
import {
  createWorkflow,
  decideWorkflowApproval,
  deleteWorkflow,
  evaluateCronWorkflows,
  getWorkflow,
  listWorkflowApprovals,
  listWorkflowRuns,
  listWorkflows,
  startWorkflowRun,
  triggerWorkflowEvent,
  updateWorkflow,
  type WorkflowActionProposal,
  type WorkflowApproval,
  type WorkflowDefinition,
  type WorkflowRun,
  type WorkflowStep,
} from '@/lib/api/workflows';
import { notifications } from '@stores/notifications';

interface WorkflowDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  trigger_type: string;
  trigger_config: Record<string, unknown>;
  steps: WorkflowStep[];
}

function defaultStepConfig(stepType: string): Record<string, unknown> {
  if (stepType === 'submit_action') {
    return {
      action_id: '',
      target_object_id_field: 'event.object_id',
      parameters: {},
      justification: 'Automated workflow submit action from {{trigger.type}}',
      result_key: 'automation.last_submit_action',
    };
  }
  if (stepType === 'notification') {
    return {
      title: 'Workflow notification',
      message: 'Workflow condition met for {{trigger.type}}',
      channels: ['in_app'],
      severity: 'info',
    };
  }
  return {};
}

function createProposalApprovalConfig(): Record<string, unknown> {
  return {
    title: 'Review proposed ontology action',
    instructions: 'Inspect the proposal and approve to auto-apply it into the Ontology.',
    channels: ['in_app', 'email'],
    proposal: {
      action_id: '',
      target_object_id_field: 'event.object_id',
      parameters: {},
      summary: 'Review ontology action proposal for {{event.object_id}}',
      auto_apply_on_approval: true,
      execution_identity: 'approver',
    },
  };
}

function createStep(stepType = 'action', config: Record<string, unknown> = defaultStepConfig(stepType)): WorkflowStep {
  return {
    id: crypto.randomUUID(),
    name:
      stepType === 'submit_action'
        ? 'Submit action'
        : stepType === 'approval' && 'proposal' in config
          ? 'Proposal review'
          : `New ${stepType}`,
    step_type: stepType,
    description: '',
    config,
    next_step_id: null,
    branches: [],
  };
}

function createEmptyWorkflow(): WorkflowDraft {
  return {
    name: 'New workflow',
    description: '',
    status: 'draft',
    trigger_type: 'manual',
    trigger_config: {},
    steps: [createStep('action')],
  };
}

function normalizeWorkflow(workflow: WorkflowDefinition): WorkflowDraft {
  return {
    id: workflow.id,
    name: workflow.name,
    description: workflow.description,
    status: workflow.status,
    trigger_type: workflow.trigger_type,
    trigger_config: workflow.trigger_config ?? {},
    steps: Array.isArray(workflow.steps) ? workflow.steps : [],
  };
}

function formatPreview(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}

export function WorkflowsPage() {
  const [workflows, setWorkflows] = useState<WorkflowDefinition[]>([]);
  const [approvals, setApprovals] = useState<WorkflowApproval[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [users, setUsers] = useState<UserProfile[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState('');
  const [eventPayloadText, setEventPayloadText] = useState('{\n  "source": "workflow-builder"\n}');
  const [error, setError] = useState('');
  const [stepConfigText, setStepConfigText] = useState<Record<string, string>>({});
  const [approvalNotes, setApprovalNotes] = useState<Record<string, string>>({});
  const [draft, setDraft] = useState<WorkflowDraft>(createEmptyWorkflow);

  function syncStepConfigText(steps: WorkflowStep[]) {
    setStepConfigText(
      Object.fromEntries(steps.map((step) => [step.id, JSON.stringify(step.config ?? {}, null, 2)])),
    );
  }

  async function loadWorkflows(searchValue = search) {
    const response = await listWorkflows({ search: searchValue || undefined, per_page: 50 });
    setWorkflows(response.data);
    return response.data;
  }

  async function loadApprovals(workflowId: string) {
    const response = await listWorkflowApprovals({
      per_page: 50,
      status: 'pending',
      workflow_id: workflowId || undefined,
    });
    setApprovals(response.data);
  }

  async function loadRuns(workflowId: string) {
    if (!workflowId) {
      setRuns([]);
      return;
    }
    const response = await listWorkflowRuns(workflowId, { per_page: 20 });
    setRuns(response.data);
  }

  async function selectWorkflow(id: string) {
    setSelectedWorkflowId(id);
    const workflow = await getWorkflow(id);
    const next = normalizeWorkflow(workflow);
    setDraft(next);
    syncStepConfigText(next.steps);
    await Promise.all([loadRuns(id), loadApprovals(id)]);
  }

  async function load() {
    setLoading(true);
    setError('');
    try {
      const [allUsers, list] = await Promise.all([
        listUsers().catch(() => [] as UserProfile[]),
        loadWorkflows(),
      ]);
      setUsers(allUsers);

      if (selectedWorkflowId) {
        await selectWorkflow(selectedWorkflowId);
      } else if (list.length > 0) {
        await selectWorkflow(list[0].id);
      } else {
        const empty = createEmptyWorkflow();
        setDraft(empty);
        setSelectedWorkflowId('');
        syncStepConfigText(empty.steps);
      }
    } catch (cause) {
      console.error('Failed to load workflows', cause);
      setError(cause instanceof Error ? cause.message : 'Failed to load workflows');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function newWorkflow() {
    const empty = createEmptyWorkflow();
    setSelectedWorkflowId('');
    setDraft(empty);
    setRuns([]);
    setApprovals([]);
    syncStepConfigText(empty.steps);
  }

  async function saveWorkflow() {
    setSaving(true);
    setError('');
    try {
      const payload = {
        name: draft.name,
        description: draft.description,
        status: draft.status,
        trigger_type: draft.trigger_type,
        trigger_config: draft.trigger_config,
        steps: draft.steps,
      };
      const workflow = draft.id ? await updateWorkflow(draft.id, payload) : await createWorkflow(payload);
      notifications.success(`Workflow ${draft.id ? 'updated' : 'created'}`);
      await loadWorkflows();
      await selectWorkflow(workflow.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save workflow');
    } finally {
      setSaving(false);
    }
  }

  async function removeCurrentWorkflow() {
    if (!draft.id || !confirm('Delete this workflow?')) return;
    await deleteWorkflow(draft.id);
    notifications.success('Workflow deleted');
    newWorkflow();
    const list = await loadWorkflows();
    if (list.length > 0) {
      await selectWorkflow(list[0].id);
    }
  }

  function addStep(stepType = 'action') {
    const step = createStep(stepType);
    setDraft((current) => ({ ...current, steps: [...current.steps, step] }));
    setStepConfigText((current) => ({ ...current, [step.id]: JSON.stringify(step.config ?? {}, null, 2) }));
  }

  function addProposalApprovalStep() {
    const step = createStep('approval', createProposalApprovalConfig());
    setDraft((current) => ({ ...current, steps: [...current.steps, step] }));
    setStepConfigText((current) => ({ ...current, [step.id]: JSON.stringify(step.config ?? {}, null, 2) }));
  }

  function duplicateStep(stepId: string) {
    setDraft((current) => {
      const source = current.steps.find((step) => step.id === stepId);
      if (!source) return current;
      const copy: WorkflowStep = {
        ...structuredClone(source),
        id: crypto.randomUUID(),
        name: `${source.name} copy`,
      };
      setStepConfigText((textCurrent) => ({
        ...textCurrent,
        [copy.id]: JSON.stringify(copy.config ?? {}, null, 2),
      }));
      return { ...current, steps: [...current.steps, copy] };
    });
  }

  function removeStep(stepId: string) {
    setDraft((current) => {
      if (current.steps.length <= 1) return current;
      const filtered = current.steps
        .filter((step) => step.id !== stepId)
        .map((step) => ({
          ...step,
          next_step_id: step.next_step_id === stepId ? null : step.next_step_id,
          branches: step.branches.filter((branch) => branch.next_step_id !== stepId),
        }));
      setStepConfigText((textCurrent) => {
        const next = { ...textCurrent };
        delete next[stepId];
        return next;
      });
      return { ...current, steps: filtered };
    });
  }

  function patchStep(stepId: string, patch: Partial<WorkflowStep>) {
    setDraft((current) => ({
      ...current,
      steps: current.steps.map((step) => (step.id === stepId ? { ...step, ...patch } : step)),
    }));
  }

  function addBranch(stepId: string) {
    setDraft((current) => ({
      ...current,
      steps: current.steps.map((step) =>
        step.id === stepId
          ? {
              ...step,
              branches: [
                ...step.branches,
                {
                  condition: { field: 'last_approval_decision.decision', operator: 'eq', value: 'approved' },
                  next_step_id: '',
                },
              ],
            }
          : step,
      ),
    }));
  }

  function removeBranch(stepId: string, branchIndex: number) {
    setDraft((current) => ({
      ...current,
      steps: current.steps.map((step) =>
        step.id === stepId
          ? { ...step, branches: step.branches.filter((_, index) => index !== branchIndex) }
          : step,
      ),
    }));
  }

  function patchBranch(stepId: string, branchIndex: number, patch: Partial<{ field: string; operator: string; value: unknown; next_step_id: string }>) {
    setDraft((current) => ({
      ...current,
      steps: current.steps.map((step) => {
        if (step.id !== stepId) return step;
        return {
          ...step,
          branches: step.branches.map((branch, index) => {
            if (index !== branchIndex) return branch;
            return {
              condition: {
                ...branch.condition,
                ...(patch.field !== undefined ? { field: patch.field } : {}),
                ...(patch.operator !== undefined ? { operator: patch.operator } : {}),
                ...(patch.value !== undefined ? { value: patch.value } : {}),
              },
              next_step_id: patch.next_step_id !== undefined ? patch.next_step_id : branch.next_step_id,
            };
          }),
        };
      }),
    }));
  }

  function updateStepConfig(stepId: string, raw: string) {
    setStepConfigText((current) => ({ ...current, [stepId]: raw }));
    try {
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      patchStep(stepId, { config: parsed });
    } catch {
      // Keep raw text until valid JSON.
    }
  }

  function ownerName(userId: string | null) {
    if (!userId) return 'Unassigned';
    return users.find((user) => user.id === userId)?.name ?? userId.slice(0, 8);
  }

  function stepName(stepId: string | null) {
    return draft.steps.find((step) => step.id === stepId)?.name ?? 'End';
  }

  function proposalForApproval(approval: WorkflowApproval): WorkflowActionProposal | null {
    return approval.payload?.proposal ?? null;
  }

  function proposalExecutionStatus(approval: WorkflowApproval) {
    return approval.payload?.proposal_execution?.status ?? null;
  }

  function updateTriggerField(key: string, value: string) {
    setDraft((current) => ({
      ...current,
      trigger_config: { ...current.trigger_config, [key]: value },
    }));
  }

  async function runManual() {
    if (!draft.id) return;
    setTriggering(true);
    try {
      await startWorkflowRun(draft.id, { initiated_from: 'workflow-builder' });
      notifications.success('Manual run started');
      await Promise.all([loadRuns(draft.id), loadApprovals(draft.id)]);
    } finally {
      setTriggering(false);
    }
  }

  async function fireEvent() {
    const eventName =
      typeof draft.trigger_config['event_name'] === 'string' ? String(draft.trigger_config['event_name']) : '';
    if (!eventName) {
      setError('Set an event name before firing an event trigger');
      return;
    }
    setTriggering(true);
    setError('');
    try {
      const payload = JSON.parse(eventPayloadText) as Record<string, unknown>;
      await triggerWorkflowEvent(eventName, payload);
      notifications.success('Event trigger dispatched');
      await Promise.all([loadRuns(selectedWorkflowId), loadApprovals(selectedWorkflowId), loadWorkflows()]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to trigger workflow event');
    } finally {
      setTriggering(false);
    }
  }

  async function runCronEvaluation() {
    setTriggering(true);
    try {
      const response = await evaluateCronWorkflows();
      notifications.info(`Triggered ${response.triggered_runs} due cron workflow(s)`);
      await Promise.all([loadRuns(selectedWorkflowId), loadApprovals(selectedWorkflowId), loadWorkflows()]);
    } finally {
      setTriggering(false);
    }
  }

  async function decide(approval: WorkflowApproval, decision: 'approved' | 'rejected') {
    const comment = approvalNotes[approval.id]?.trim();
    await decideWorkflowApproval(approval.id, { decision, comment: comment || undefined, payload: {} });
    notifications.success(`Approval ${decision}`);
    setApprovalNotes((current) => {
      const next = { ...current };
      delete next[approval.id];
      return next;
    });
    await Promise.all([loadApprovals(selectedWorkflowId), loadRuns(selectedWorkflowId)]);
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <h1 className="of-heading-xl">Workflows &amp; Notifications</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Build event-driven automations, submit ontology actions automatically, and route
            proposal-based HITL reviews through one queue.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" onClick={newWorkflow} className="of-button">
            New workflow
          </button>
          <button type="button" onClick={() => void saveWorkflow()} disabled={saving} className="of-button of-button--primary">
            {saving ? 'Saving…' : draft.id ? 'Save changes' : 'Create workflow'}
          </button>
        </div>
      </div>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)' }}>
        <section className="of-panel" style={{ padding: 20, display: 'grid', gap: 16 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p className="of-eyebrow">Workflow registry</p>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                Definitions, trigger modes, and approval queue entry points.
              </p>
            </div>
            <span className="of-chip">{workflows.length} workflows</span>
          </div>

          <input
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              void loadWorkflows(e.target.value);
            }}
            placeholder="Search workflows…"
            className="of-input"
          />

          {loading ? (
            <div style={{ padding: 48, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
              Loading workflows…
            </div>
          ) : workflows.length === 0 ? (
            <div style={{ border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', padding: 32, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
              No workflows yet. Create the first workflow from the builder.
            </div>
          ) : (
            <div style={{ display: 'grid', gap: 8 }}>
              {workflows.map((workflow) => {
                const active = selectedWorkflowId === workflow.id;
                return (
                  <button
                    key={workflow.id}
                    type="button"
                    onClick={() => void selectWorkflow(workflow.id)}
                    style={{
                      width: '100%',
                      textAlign: 'left',
                      padding: 14,
                      border: `1px solid ${active ? '#2563eb' : 'var(--border-default)'}`,
                      background: active ? '#eff6ff' : 'var(--bg-elevated)',
                      borderRadius: 'var(--radius-md)',
                      cursor: 'pointer',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <div>
                        <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{workflow.name}</div>
                        <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          {workflow.description || 'No description'}
                        </div>
                      </div>
                      <span className="of-chip">{workflow.trigger_type}</span>
                    </div>
                    <div style={{ marginTop: 10, display: 'flex', flexWrap: 'wrap', gap: 8, fontSize: 12, color: 'var(--text-muted)' }}>
                      <span>Status {workflow.status}</span>
                      <span>{Array.isArray(workflow.steps) ? workflow.steps.length : 0} steps</span>
                      <span>Owner {ownerName(workflow.owner_id)}</span>
                    </div>
                  </button>
                );
              })}
            </div>
          )}

          <div className="of-panel-muted" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div>
                <div style={{ fontWeight: 600 }}>Pending approvals</div>
                <div className="of-text-muted" style={{ fontSize: 13 }}>
                  Human-in-the-loop queue for the selected workflow.
                </div>
              </div>
              <span className="of-chip" style={{ background: '#fef3c7', color: '#b45309' }}>
                {approvals.length}
              </span>
            </div>

            <div style={{ marginTop: 12, display: 'grid', gap: 10 }}>
              {approvals.map((approval) => {
                const proposal = proposalForApproval(approval);
                return (
                  <div key={approval.id} className="of-panel" style={{ padding: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                      <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: 600 }}>{approval.title}</div>
                        <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          {approval.instructions || 'No instructions'}
                        </div>
                        <div className="of-text-muted" style={{ marginTop: 6, fontSize: 12 }}>
                          Assigned to {ownerName(approval.assigned_to)}
                        </div>
                        {proposal && (
                          <div style={{ marginTop: 10, padding: 10, border: '1px solid #bfdbfe', background: '#eff6ff', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
                            <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#1d4ed8' }}>
                              Proposal
                            </div>
                            <div style={{ marginTop: 6, fontWeight: 600 }}>{proposal.summary}</div>
                            <div className="of-text-muted" style={{ marginTop: 6, fontSize: 12 }}>
                              Action {proposal.action_id.slice(0, 8)}
                              {proposal.target_object_id && (
                                <> · Target {proposal.target_object_id.slice(0, 8)}</>
                              )}
                              {' · Identity '}
                              {proposal.execution_identity}
                            </div>
                            {!!proposal.reasoning && (
                              <pre style={{ marginTop: 8, padding: 8, background: '#fff', borderRadius: 'var(--radius-md)', fontSize: 11, overflowX: 'auto' }}>
                                {formatPreview(proposal.reasoning)}
                              </pre>
                            )}
                            <pre style={{ marginTop: 8, padding: 8, background: '#fff', borderRadius: 'var(--radius-md)', fontSize: 11, overflowX: 'auto' }}>
                              {formatPreview(proposal.preview)}
                            </pre>
                            {proposalExecutionStatus(approval) && (
                              <div className="of-text-muted" style={{ marginTop: 6, fontSize: 12 }}>
                                Execution state: {proposalExecutionStatus(approval)}
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                      <div style={{ width: 200, display: 'grid', gap: 8 }}>
                        <textarea
                          value={approvalNotes[approval.id] ?? ''}
                          onChange={(e) =>
                            setApprovalNotes((current) => ({ ...current, [approval.id]: e.target.value }))
                          }
                          rows={4}
                          placeholder="Optional review note"
                          className="of-input"
                          style={{ fontSize: 12 }}
                        />
                        <div style={{ display: 'flex', gap: 8 }}>
                          <button
                            type="button"
                            onClick={() => void decide(approval, 'approved')}
                            className="of-button of-button--primary"
                            style={{ flex: 1, fontSize: 12, background: '#059669' }}
                          >
                            Approve
                          </button>
                          <button
                            type="button"
                            onClick={() => void decide(approval, 'rejected')}
                            className="of-button of-button--primary"
                            style={{ flex: 1, fontSize: 12, background: '#dc2626' }}
                          >
                            Reject
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
              {approvals.length === 0 && (
                <div className="of-text-muted" style={{ fontSize: 13 }}>No pending approvals.</div>
              )}
            </div>
          </div>
        </section>

        <section style={{ display: 'grid', gap: 16 }}>
          <div className="of-panel" style={{ padding: 20 }}>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
              <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
                <span style={{ fontWeight: 600 }}>Workflow name</span>
                <input
                  value={draft.name}
                  onChange={(e) => setDraft((current) => ({ ...current, name: e.target.value }))}
                  className="of-input"
                />
              </label>
              <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
                <span style={{ fontWeight: 600 }}>Status</span>
                <select
                  value={draft.status}
                  onChange={(e) => setDraft((current) => ({ ...current, status: e.target.value }))}
                  className="of-input"
                >
                  <option value="draft">Draft</option>
                  <option value="active">Active</option>
                  <option value="paused">Paused</option>
                </select>
              </label>
            </div>

            <label style={{ fontSize: 13, display: 'grid', gap: 4, marginTop: 12 }}>
              <span style={{ fontWeight: 600 }}>Description</span>
              <textarea
                value={draft.description}
                onChange={(e) => setDraft((current) => ({ ...current, description: e.target.value }))}
                rows={3}
                className="of-input"
              />
            </label>

            <div style={{ marginTop: 12, display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
              <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
                <span style={{ fontWeight: 600 }}>Trigger type</span>
                <select
                  value={draft.trigger_type}
                  onChange={(e) => setDraft((current) => ({ ...current, trigger_type: e.target.value }))}
                  className="of-input"
                >
                  <option value="manual">Manual</option>
                  <option value="cron">Cron</option>
                  <option value="event">Event-driven</option>
                  <option value="webhook">Webhook</option>
                </select>
              </label>
              <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
                <span style={{ fontWeight: 600 }}>Trigger config</span>
                {draft.trigger_type === 'cron' ? (
                  <input
                    value={String(draft.trigger_config['cron'] ?? '')}
                    onChange={(e) => updateTriggerField('cron', e.target.value)}
                    placeholder="*/15 * * * * *"
                    className="of-input"
                    style={{ fontFamily: 'var(--font-mono)' }}
                  />
                ) : draft.trigger_type === 'event' ? (
                  <input
                    value={String(draft.trigger_config['event_name'] ?? '')}
                    onChange={(e) => updateTriggerField('event_name', e.target.value)}
                    placeholder="dataset.quality.degraded"
                    className="of-input"
                  />
                ) : draft.trigger_type === 'webhook' ? (
                  <input
                    value={String(draft.trigger_config['secret'] ?? '')}
                    onChange={(e) => updateTriggerField('secret', e.target.value)}
                    placeholder="Shared secret"
                    className="of-input"
                  />
                ) : (
                  <div
                    style={{
                      border: '1px dashed var(--border-default)',
                      borderRadius: 'var(--radius-md)',
                      padding: '8px 12px',
                      fontSize: 13,
                      color: 'var(--text-muted)',
                    }}
                  >
                    Manual trigger uses the run button below.
                  </div>
                )}
              </label>
            </div>

            <div style={{ marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              <button
                type="button"
                onClick={() => void runManual()}
                disabled={!draft.id || triggering}
                className="of-button of-button--primary"
              >
                {triggering ? 'Running…' : 'Run manually'}
              </button>
              <button
                type="button"
                onClick={() => void fireEvent()}
                disabled={triggering || draft.trigger_type !== 'event'}
                className="of-button"
              >
                Fire event trigger
              </button>
              <button
                type="button"
                onClick={() => void runCronEvaluation()}
                disabled={triggering || draft.trigger_type !== 'cron'}
                className="of-button"
              >
                Run due cron workflows
              </button>
              {draft.id && (
                <button
                  type="button"
                  onClick={() => void removeCurrentWorkflow()}
                  className="of-button"
                  style={{ color: '#dc2626', borderColor: '#fecaca' }}
                >
                  Delete workflow
                </button>
              )}
            </div>

            {draft.trigger_type === 'event' && (
              <label style={{ fontSize: 13, display: 'grid', gap: 4, marginTop: 12 }}>
                <span style={{ fontWeight: 600 }}>Event payload</span>
                <textarea
                  value={eventPayloadText}
                  onChange={(e) => setEventPayloadText(e.target.value)}
                  rows={5}
                  className="of-input"
                  style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}
                />
              </label>
            )}
          </div>

          <div className="of-panel" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
              <div>
                <p className="of-eyebrow">Workflow builder</p>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  A visual step lane with config JSON, branching, notifications, direct submit
                  actions, and proposal-based approvals.
                </p>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                <button type="button" onClick={() => addStep('action')} className="of-button" style={{ fontSize: 12 }}>
                  Add action
                </button>
                <button type="button" onClick={() => addStep('approval')} className="of-button" style={{ fontSize: 12 }}>
                  Add approval
                </button>
                <button type="button" onClick={addProposalApprovalStep} className="of-button" style={{ fontSize: 12 }}>
                  Add proposal review
                </button>
                <button type="button" onClick={() => addStep('submit_action')} className="of-button" style={{ fontSize: 12 }}>
                  Add submit action
                </button>
                <button type="button" onClick={() => addStep('notification')} className="of-button" style={{ fontSize: 12 }}>
                  Add notification
                </button>
              </div>
            </div>

            <div style={{ marginTop: 16, overflowX: 'auto', paddingBottom: 8 }}>
              <div style={{ display: 'flex', gap: 16, minWidth: 'max-content' }}>
                {draft.steps.map((step, index) => (
                  <div
                    key={step.id}
                    style={{
                      width: 352,
                      padding: 14,
                      background: 'var(--bg-subtle)',
                      border: '1px solid var(--border-default)',
                      borderRadius: 'var(--radius-md)',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span className="of-chip" style={{ textTransform: 'uppercase', letterSpacing: '0.16em', fontSize: 11 }}>
                        {step.step_type}
                      </span>
                      <div style={{ display: 'flex', gap: 8, fontSize: 12 }}>
                        <button type="button" onClick={() => duplicateStep(step.id)} style={{ color: '#2563eb', background: 'none', border: 'none', cursor: 'pointer' }}>
                          Duplicate
                        </button>
                        <button type="button" onClick={() => removeStep(step.id)} style={{ color: '#dc2626', background: 'none', border: 'none', cursor: 'pointer' }}>
                          Remove
                        </button>
                      </div>
                    </div>

                    <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                      <input
                        value={step.name}
                        onChange={(e) => patchStep(step.id, { name: e.target.value })}
                        placeholder="Step name"
                        className="of-input"
                      />
                      <textarea
                        value={step.description}
                        onChange={(e) => patchStep(step.id, { description: e.target.value })}
                        rows={2}
                        placeholder="Step description"
                        className="of-input"
                      />
                      <select
                        value={step.next_step_id ?? ''}
                        onChange={(e) => patchStep(step.id, { next_step_id: e.target.value || null })}
                        className="of-input"
                      >
                        <option value="">End workflow</option>
                        {draft.steps
                          .filter((candidate) => candidate.id !== step.id)
                          .map((option) => (
                            <option key={option.id} value={option.id}>
                              {option.name}
                            </option>
                          ))}
                      </select>
                      <textarea
                        rows={8}
                        value={stepConfigText[step.id] ?? '{}'}
                        onChange={(e) => updateStepConfig(step.id, e.target.value)}
                        className="of-input"
                        style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}
                      />
                    </div>

                    <div className="of-panel" style={{ padding: 10, marginTop: 12 }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <div style={{ fontSize: 13, fontWeight: 600 }}>Branches</div>
                        <button
                          type="button"
                          onClick={() => addBranch(step.id)}
                          style={{ fontSize: 12, color: '#2563eb', background: 'none', border: 'none', cursor: 'pointer' }}
                        >
                          Add branch
                        </button>
                      </div>
                      <div style={{ marginTop: 10, display: 'grid', gap: 8 }}>
                        {step.branches.map((branch, branchIndex) => (
                          <div key={branchIndex} style={{ padding: 8, border: '1px solid var(--border-default)', borderRadius: 'var(--radius-md)' }}>
                            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
                              <input
                                value={branch.condition.field}
                                onChange={(e) => patchBranch(step.id, branchIndex, { field: e.target.value })}
                                placeholder="Context path"
                                className="of-input"
                                style={{ fontSize: 12 }}
                              />
                              <select
                                value={branch.condition.operator}
                                onChange={(e) => patchBranch(step.id, branchIndex, { operator: e.target.value })}
                                className="of-input"
                                style={{ fontSize: 12 }}
                              >
                                <option value="eq">equals</option>
                                <option value="ne">not equal</option>
                                <option value="contains">contains</option>
                                <option value="gt">&gt;</option>
                                <option value="gte">&gt;=</option>
                                <option value="lt">&lt;</option>
                                <option value="lte">&lt;=</option>
                              </select>
                              <input
                                value={
                                  typeof branch.condition.value === 'string'
                                    ? branch.condition.value
                                    : JSON.stringify(branch.condition.value)
                                }
                                onChange={(e) => patchBranch(step.id, branchIndex, { value: e.target.value })}
                                placeholder="Match value"
                                className="of-input"
                                style={{ fontSize: 12 }}
                              />
                              <select
                                value={branch.next_step_id}
                                onChange={(e) => patchBranch(step.id, branchIndex, { next_step_id: e.target.value })}
                                className="of-input"
                                style={{ fontSize: 12 }}
                              >
                                <option value="">Choose target step</option>
                                {draft.steps
                                  .filter((candidate) => candidate.id !== step.id)
                                  .map((option) => (
                                    <option key={option.id} value={option.id}>
                                      {option.name}
                                    </option>
                                  ))}
                              </select>
                            </div>
                            <div style={{ marginTop: 6, textAlign: 'right' }}>
                              <button
                                type="button"
                                onClick={() => removeBranch(step.id, branchIndex)}
                                style={{ fontSize: 12, color: '#dc2626', background: 'none', border: 'none', cursor: 'pointer' }}
                              >
                                Remove branch
                              </button>
                            </div>
                          </div>
                        ))}
                        {step.branches.length === 0 && (
                          <div className="of-text-muted" style={{ fontSize: 12 }}>
                            No conditional branches on this step.
                          </div>
                        )}
                      </div>
                    </div>

                    <div className="of-text-muted" style={{ marginTop: 12, fontSize: 12 }}>
                      <span style={{ fontWeight: 600 }}>Next:</span> {stepName(step.next_step_id)}
                    </div>

                    {index < draft.steps.length - 1 && (
                      <div className="of-text-muted" style={{ marginTop: 10, textAlign: 'center', fontSize: 12 }}>
                        ↓ then
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className="of-panel" style={{ padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <div>
                <p className="of-eyebrow">Run history</p>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  Latest executions, trigger source, and workflow state transitions.
                </p>
              </div>
              <span className="of-chip">{runs.length} recent runs</span>
            </div>

            <div style={{ marginTop: 12, display: 'grid', gap: 10 }}>
              {runs.map((run) => (
                <div key={run.id} className="of-panel-muted" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' }}>
                    <div style={{ fontWeight: 600 }}>{run.trigger_type} trigger</div>
                    <span className="of-chip">{run.status}</span>
                  </div>
                  <div className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                    Started {new Date(run.started_at).toLocaleString()}
                  </div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                    Current step: {stepName(run.current_step_id)}
                  </div>
                  {run.error_message && (
                    <div
                      className="of-status-danger"
                      style={{ marginTop: 8, padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}
                    >
                      {run.error_message}
                    </div>
                  )}
                </div>
              ))}
              {runs.length === 0 && (
                <div className="of-text-muted" style={{ fontSize: 13 }}>No runs yet for this workflow.</div>
              )}
            </div>
          </div>
        </section>
      </div>
    </section>
  );
}
