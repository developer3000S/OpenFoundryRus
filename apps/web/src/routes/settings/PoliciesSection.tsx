import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  createPolicy,
  deletePolicy,
  evaluatePolicy,
  type PolicyEvaluationResult,
} from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { policiesQuery, settingsQueryKeys } from './queries';
import { parseJson, toOptionalString } from './utils';

interface PoliciesSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

const DEFAULT_POLICY_FORM = {
  name: '',
  description: '',
  effect: 'allow',
  resource: 'datasets',
  action: 'read',
  conditions: '{\n  "subject": {},\n  "resource": {}\n}',
  row_filter: '',
  enabled: true,
};

const DEFAULT_EVAL_FORM = {
  resource: 'datasets',
  action: 'read',
  resource_attributes:
    '{\n  "organization_id": null,\n  "effective_marking": "public",\n  "consumer_surface": "workshop"\n}',
};

export function PoliciesSection({ setNotice, setError }: PoliciesSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const result = useQuery({ ...policiesQuery, enabled: perms.canReadPolicies });
  const policies = result.data ?? [];

  const [form, setForm] = useState(DEFAULT_POLICY_FORM);
  const [evalForm, setEvalForm] = useState(DEFAULT_EVAL_FORM);
  const [evaluation, setEvaluation] = useState<PolicyEvaluationResult | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () => {
      let conditions: Record<string, unknown>;
      try {
        conditions = parseJson(form.conditions);
      } catch (err) {
        return Promise.reject(
          new Error(err instanceof Error ? `Invalid conditions JSON: ${err.message}` : 'Invalid conditions JSON'),
        );
      }
      return createPolicy({
        name: form.name,
        description: toOptionalString(form.description),
        effect: form.effect,
        resource: form.resource,
        action: form.action,
        conditions,
        row_filter: toOptionalString(form.row_filter),
        enabled: form.enabled,
      });
    },
    onSuccess: async () => {
      setForm(DEFAULT_POLICY_FORM);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.policies });
      setNotice('Policy created.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create policy'),
  });

  const deleteMutation = useMutation({
    mutationFn: (policyId: string) => deletePolicy(policyId),
    onMutate: (policyId) => setDeletingId(policyId),
    onSettled: () => setDeletingId(null),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.policies });
      setNotice('Policy deleted.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to delete policy'),
  });

  const evaluateMutation = useMutation({
    mutationFn: () => {
      let resourceAttributes: Record<string, unknown>;
      try {
        resourceAttributes = parseJson(evalForm.resource_attributes);
      } catch (err) {
        return Promise.reject(
          new Error(
            err instanceof Error ? `Invalid resource attributes JSON: ${err.message}` : 'Invalid resource attributes JSON',
          ),
        );
      }
      return evaluatePolicy({
        resource: evalForm.resource,
        action: evalForm.action,
        resource_attributes: resourceAttributes,
      });
    },
    onSuccess: (data) => {
      setEvaluation(data);
      setNotice('Policy evaluation completed.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Evaluation failed'),
  });

  if (!perms.canReadPolicies) return null;

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">ABAC</p>
      <h2 className="of-heading-lg">Policies and evaluation</h2>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.15fr) minmax(0, 0.85fr)', gap: 24, marginTop: 24 }}>
        <div>
          <div style={{ display: 'grid', gap: 12 }}>
            {policies.map((policy) => (
              <article key={policy.id} className="of-panel-muted" style={{ padding: 16 }}>
                <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
                  <div>
                    <h3 className="of-heading-sm">{policy.name}</h3>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {policy.resource}:{policy.action} • {policy.effect}
                    </p>
                  </div>
                  <span className={`of-chip ${policy.enabled ? 'of-status-success' : ''}`}>
                    {policy.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </header>
                <pre
                  className="of-scrollbar"
                  style={{
                    margin: '12px 0 0',
                    overflow: 'auto',
                    padding: 12,
                    background: '#fff',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: 'var(--radius-sm)',
                    fontSize: 12,
                  }}
                >
                  {JSON.stringify(policy.conditions, null, 2)}
                </pre>
                {policy.row_filter && (
                  <div
                    style={{
                      marginTop: 12,
                      padding: '8px 12px',
                      background: '#fff',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: 'var(--radius-sm)',
                      fontSize: 12,
                    }}
                  >
                    {policy.row_filter}
                  </div>
                )}
                {perms.canManagePolicies && (
                  <button
                    type="button"
                    className="of-btn of-btn-danger"
                    style={{ marginTop: 12 }}
                    onClick={() => deleteMutation.mutate(policy.id)}
                    disabled={deletingId === policy.id}
                  >
                    {deletingId === policy.id ? 'Deleting…' : 'Delete'}
                  </button>
                )}
              </article>
            ))}
            {result.isLoading && <p className="of-text-muted">Loading policies…</p>}
            {!result.isLoading && policies.length === 0 && (
              <p className="of-text-muted">No policies registered.</p>
            )}
          </div>
        </div>

        <div style={{ display: 'grid', gap: 16 }}>
          {perms.canManagePolicies && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                createMutation.mutate();
              }}
              style={{
                display: 'grid',
                gap: 10,
                padding: 20,
                border: '1px dashed var(--border-default)',
                borderRadius: 'var(--radius-md)',
              }}
            >
              <div className="of-eyebrow">Create policy</div>
              <input
                className="of-input"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="Policy name"
                required
              />
              <textarea
                className="of-textarea"
                value={form.description}
                onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                rows={2}
                placeholder="Description"
              />
              <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
                <input
                  className="of-input"
                  value={form.resource}
                  onChange={(e) => setForm((f) => ({ ...f, resource: e.target.value }))}
                  placeholder="Resource"
                  required
                />
                <input
                  className="of-input"
                  value={form.action}
                  onChange={(e) => setForm((f) => ({ ...f, action: e.target.value }))}
                  placeholder="Action"
                  required
                />
              </div>
              <select
                className="of-select"
                value={form.effect}
                onChange={(e) => setForm((f) => ({ ...f, effect: e.target.value }))}
              >
                <option value="allow">Allow</option>
                <option value="deny">Deny</option>
              </select>
              <textarea
                className="of-textarea"
                value={form.conditions}
                onChange={(e) => setForm((f) => ({ ...f, conditions: e.target.value }))}
                rows={7}
                style={{ fontFamily: 'var(--font-mono)' }}
              />
              <input
                className="of-input"
                value={form.row_filter}
                onChange={(e) => setForm((f) => ({ ...f, row_filter: e.target.value }))}
                placeholder="Optional row filter template"
              />
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
                />
                Enabled
              </label>
              <button type="submit" className="of-btn of-btn-primary" disabled={createMutation.isPending}>
                {createMutation.isPending ? 'Saving…' : 'Create policy'}
              </button>
            </form>
          )}

          <form
            onSubmit={(e) => {
              e.preventDefault();
              evaluateMutation.mutate();
            }}
            style={{
              display: 'grid',
              gap: 10,
              padding: 20,
              border: '1px solid var(--border-default)',
              borderRadius: 'var(--radius-md)',
            }}
          >
            <div className="of-eyebrow">Evaluate access</div>
            <input
              className="of-input"
              value={evalForm.resource}
              onChange={(e) => setEvalForm((f) => ({ ...f, resource: e.target.value }))}
              placeholder="Resource"
              required
            />
            <input
              className="of-input"
              value={evalForm.action}
              onChange={(e) => setEvalForm((f) => ({ ...f, action: e.target.value }))}
              placeholder="Action"
              required
            />
            <textarea
              className="of-textarea"
              value={evalForm.resource_attributes}
              onChange={(e) => setEvalForm((f) => ({ ...f, resource_attributes: e.target.value }))}
              rows={6}
              style={{ fontFamily: 'var(--font-mono)' }}
            />
            <button type="submit" className="of-btn" disabled={evaluateMutation.isPending}>
              {evaluateMutation.isPending ? 'Evaluating…' : 'Evaluate'}
            </button>

            {evaluation && (
              <div className="of-panel-muted" style={{ padding: 12, fontSize: 13 }}>
                <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>
                  {evaluation.allowed ? 'Allowed' : 'Denied'}
                </div>
                <div className="of-text-muted" style={{ marginTop: 6 }}>
                  Matched: {evaluation.matched_policy_ids.length}
                  {' · '}Restricted views: {evaluation.matched_restricted_view_ids.length}
                  {' · '}Deny hits: {evaluation.deny_policy_ids.length}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 10 }}>
                  {evaluation.allowed_markings.map((marking) => (
                    <span key={marking} className="of-chip of-chip-active">
                      {marking}
                    </span>
                  ))}
                  {evaluation.hidden_columns.map((column) => (
                    <span key={column} className="of-chip of-status-danger">
                      Hide {column}
                    </span>
                  ))}
                  {evaluation.consumer_mode && (
                    <span className="of-chip of-status-warning">Consumer mode</span>
                  )}
                </div>
                {evaluation.deny_reasons.length > 0 && (
                  <div
                    style={{
                      marginTop: 10,
                      padding: '8px 12px',
                      background: 'var(--status-danger-bg)',
                      color: 'var(--status-danger)',
                      borderRadius: 'var(--radius-sm)',
                      fontSize: 12,
                    }}
                  >
                    {evaluation.deny_reasons.join(' · ')}
                  </div>
                )}
                <pre
                  className="of-scrollbar"
                  style={{
                    margin: '10px 0 0',
                    overflow: 'auto',
                    padding: 12,
                    background: '#fff',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: 'var(--radius-sm)',
                    fontSize: 11,
                  }}
                >
                  {JSON.stringify(evaluation, null, 2)}
                </pre>
              </div>
            )}
          </form>
        </div>
      </div>
    </section>
  );
}
