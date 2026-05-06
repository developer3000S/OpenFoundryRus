import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createRestrictedView, deleteRestrictedView } from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { restrictedViewsQuery, settingsQueryKeys } from './queries';
import { parseJson, toList, toOptionalString } from './utils';

interface RestrictedViewsSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

const DEFAULT_FORM = {
  name: '',
  description: '',
  resource: 'datasets',
  action: 'read',
  conditions:
    '{\n  "subject": {},\n  "resource": {\n    "organization_id": null,\n    "effective_marking": "public"\n  }\n}',
  row_filter: '',
  hidden_columns: 'ssn, salary',
  allowed_org_ids: '',
  allowed_markings: 'public',
  consumer_mode_enabled: false,
  allow_guest_access: true,
  enabled: true,
};

export function RestrictedViewsSection({ setNotice, setError }: RestrictedViewsSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const result = useQuery({ ...restrictedViewsQuery, enabled: perms.canReadPolicies });
  const views = result.data ?? [];

  const [form, setForm] = useState(DEFAULT_FORM);
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
      return createRestrictedView({
        name: form.name,
        description: toOptionalString(form.description),
        resource: form.resource,
        action: form.action,
        conditions,
        row_filter: toOptionalString(form.row_filter),
        hidden_columns: toList(form.hidden_columns),
        allowed_org_ids: toList(form.allowed_org_ids),
        allowed_markings: toList(form.allowed_markings),
        consumer_mode_enabled: form.consumer_mode_enabled,
        allow_guest_access: form.allow_guest_access,
        enabled: form.enabled,
      });
    },
    onSuccess: async () => {
      setForm(DEFAULT_FORM);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.restrictedViews });
      setNotice('Restricted view created.');
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : 'Failed to create restricted view'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteRestrictedView(id),
    onMutate: (id) => setDeletingId(id),
    onSettled: () => setDeletingId(null),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.restrictedViews });
      setNotice('Restricted view deleted.');
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : 'Failed to delete restricted view'),
  });

  if (!perms.canReadPolicies) return null;

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">Restricted views</p>
      <h2 className="of-heading-lg">Row, column and consumer-mode boundaries</h2>
      <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 640 }}>
        Granular row and column cuts with explicit org, marking and consumer-mode boundaries.
      </p>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.15fr) minmax(0, 0.85fr)', gap: 24, marginTop: 24 }}>
        <div style={{ display: 'grid', gap: 12 }}>
          {views.map((view) => (
            <article key={view.id} className="of-panel-muted" style={{ padding: 16 }}>
              <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
                <div>
                  <h3 className="of-heading-sm">{view.name}</h3>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {view.resource}:{view.action}
                  </p>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, justifyContent: 'flex-end' }}>
                  <span className={`of-chip ${view.enabled ? 'of-status-success' : ''}`}>
                    {view.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                  {view.allow_guest_access && <span className="of-chip of-status-info">Guest</span>}
                  {view.consumer_mode_enabled && (
                    <span className="of-chip of-status-warning">Consumer</span>
                  )}
                </div>
              </header>
              {view.description && (
                <p className="of-text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                  {view.description}
                </p>
              )}
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                {view.allowed_markings.map((marking) => (
                  <span key={marking} className="of-chip of-chip-active">
                    {marking}
                  </span>
                ))}
                {view.hidden_columns.map((column) => (
                  <span key={column} className="of-chip of-status-danger">
                    Hide {column}
                  </span>
                ))}
              </div>
              {view.row_filter && (
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
                  {view.row_filter}
                </div>
              )}
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
                {JSON.stringify(view.conditions, null, 2)}
              </pre>
              {perms.canManagePolicies && (
                <button
                  type="button"
                  className="of-btn of-btn-danger"
                  style={{ marginTop: 12 }}
                  onClick={() => deleteMutation.mutate(view.id)}
                  disabled={deletingId === view.id}
                >
                  {deletingId === view.id ? 'Deleting…' : 'Delete restricted view'}
                </button>
              )}
            </article>
          ))}
          {result.isLoading && <p className="of-text-muted">Loading restricted views…</p>}
          {!result.isLoading && views.length === 0 && (
            <p className="of-text-muted">No restricted views registered.</p>
          )}
        </div>

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
              alignSelf: 'start',
            }}
          >
            <div className="of-eyebrow">Create restricted view</div>
            <input
              className="of-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="Restricted view name"
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
            <textarea
              className="of-textarea"
              value={form.conditions}
              onChange={(e) => setForm((f) => ({ ...f, conditions: e.target.value }))}
              rows={6}
              style={{ fontFamily: 'var(--font-mono)' }}
            />
            <input
              className="of-input"
              value={form.row_filter}
              onChange={(e) => setForm((f) => ({ ...f, row_filter: e.target.value }))}
              placeholder="Row filter template"
            />
            <input
              className="of-input"
              value={form.hidden_columns}
              onChange={(e) => setForm((f) => ({ ...f, hidden_columns: e.target.value }))}
              placeholder="Hidden columns, comma separated"
            />
            <input
              className="of-input"
              value={form.allowed_org_ids}
              onChange={(e) => setForm((f) => ({ ...f, allowed_org_ids: e.target.value }))}
              placeholder="Allowed org IDs, comma separated"
            />
            <input
              className="of-input"
              value={form.allowed_markings}
              onChange={(e) => setForm((f) => ({ ...f, allowed_markings: e.target.value }))}
              placeholder="Allowed markings, comma separated"
            />
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={form.allow_guest_access}
                onChange={(e) => setForm((f) => ({ ...f, allow_guest_access: e.target.checked }))}
              />
              Allow guest access
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={form.consumer_mode_enabled}
                onChange={(e) => setForm((f) => ({ ...f, consumer_mode_enabled: e.target.checked }))}
              />
              Consumer mode enabled
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
              />
              Enabled
            </label>
            <button type="submit" className="of-btn of-btn-primary" disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Saving…' : 'Create restricted view'}
            </button>
          </form>
        )}
      </div>
    </section>
  );
}
