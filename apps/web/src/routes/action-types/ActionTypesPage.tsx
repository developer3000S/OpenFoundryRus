import { useEffect, useMemo, useState } from 'react';

import {
  createActionType,
  createActionWhatIfBranch,
  deleteActionType,
  deleteActionWhatIfBranch,
  executeAction,
  executeActionBatch,
  getActionMetrics,
  listActionTypes,
  listActionWhatIfBranches,
  listObjectTypes,
  updateActionType,
  validateAction,
  type ActionMetricsResponse,
  type ActionOperationKind,
  type ActionType,
  type ActionWhatIfBranch,
  type ObjectType,
} from '@/lib/api/ontology';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { Tabs } from '@/lib/components/Tabs';

type Tab = 'authoring' | 'operate' | 'inline-edits' | 'monitoring';

const OPERATION_KINDS: ActionOperationKind[] = [
  'update_object',
  'create_link',
  'delete_object',
  'invoke_function',
  'invoke_webhook',
  'create_interface',
  'modify_interface',
  'delete_interface',
  'create_interface_link',
  'delete_interface_link',
];

interface Draft {
  id?: string;
  name: string;
  display_name: string;
  description: string;
  object_type_id: string;
  operation_kind: ActionOperationKind;
  confirmation_required: boolean;
  permission_key: string;
  input_schema_json: string;
  form_schema_json: string;
  config_json: string;
  authorization_policy_json: string;
}

function emptyDraft(): Draft {
  return {
    name: 'my_action',
    display_name: 'My action',
    description: '',
    object_type_id: '',
    operation_kind: 'update_object',
    confirmation_required: false,
    permission_key: '',
    input_schema_json: JSON.stringify(
      [{ name: 'target_id', property_type: 'reference', required: true }],
      null,
      2,
    ),
    form_schema_json: JSON.stringify({ sections: [] }, null, 2),
    config_json: JSON.stringify(
      { operation: { kind: 'update_object', mappings: [] }, notification_side_effects: [] },
      null,
      2,
    ),
    authorization_policy_json: JSON.stringify({}, null, 2),
  };
}

export function ActionTypesPage() {
  const [tab, setTab] = useState<Tab>('authoring');
  const [actions, setActions] = useState<ActionType[]>([]);
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // operate
  const [executeTargetId, setExecuteTargetId] = useState('');
  const [executeParamsJson, setExecuteParamsJson] = useState('{}');
  const [executeJustification, setExecuteJustification] = useState('');
  const [executeResult, setExecuteResult] = useState<unknown>(null);
  const [validateResult, setValidateResult] = useState<unknown>(null);
  const [batchTargetsText, setBatchTargetsText] = useState('');
  const [batchResult, setBatchResult] = useState<unknown>(null);

  // what-if
  const [whatIfBranches, setWhatIfBranches] = useState<ActionWhatIfBranch[]>([]);
  const [whatIfDraftJson, setWhatIfDraftJson] = useState(
    JSON.stringify({ target_object_id: '', parameters: {}, name: 'Branch 1', description: '' }, null, 2),
  );

  // monitoring
  const [metrics, setMetrics] = useState<ActionMetricsResponse | null>(null);
  const [metricsWindow, setMetricsWindow] = useState('30d');

  async function refresh() {
    setError('');
    try {
      const [acts, types] = await Promise.all([
        listActionTypes({ per_page: 200 }),
        listObjectTypes({ per_page: 200 }),
      ]);
      setActions(acts.data);
      setObjectTypes(types.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  const selectedAction = useMemo(() => actions.find((a) => a.id === draft.id) ?? null, [actions, draft.id]);

  function loadAction(a: ActionType) {
    setDraft({
      id: a.id,
      name: a.name,
      display_name: a.display_name,
      description: a.description,
      object_type_id: a.object_type_id,
      operation_kind: a.operation_kind,
      confirmation_required: a.confirmation_required,
      permission_key: a.permission_key ?? '',
      input_schema_json: JSON.stringify(a.input_schema, null, 2),
      form_schema_json: JSON.stringify(a.form_schema, null, 2),
      config_json: JSON.stringify(a.config, null, 2),
      authorization_policy_json: JSON.stringify(a.authorization_policy, null, 2),
    });
  }

  async function save() {
    setBusy(true);
    setError('');
    try {
      const input_schema = JSON.parse(draft.input_schema_json);
      const form_schema = JSON.parse(draft.form_schema_json);
      const config = JSON.parse(draft.config_json);
      const authorization_policy = JSON.parse(draft.authorization_policy_json);
      if (draft.id) {
        await updateActionType(draft.id, {
          display_name: draft.display_name,
          description: draft.description,
          operation_kind: draft.operation_kind,
          input_schema,
          form_schema,
          config,
          confirmation_required: draft.confirmation_required,
          permission_key: draft.permission_key || undefined,
          authorization_policy,
        });
      } else {
        await createActionType({
          name: draft.name,
          display_name: draft.display_name,
          description: draft.description,
          object_type_id: draft.object_type_id,
          operation_kind: draft.operation_kind,
          input_schema,
          form_schema,
          config,
          confirmation_required: draft.confirmation_required,
          permission_key: draft.permission_key || undefined,
          authorization_policy,
        });
      }
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!draft.id) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete action type?')) return;
    setBusy(true);
    try {
      await deleteActionType(draft.id);
      setDraft(emptyDraft());
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function runValidate() {
    if (!draft.id) return;
    setBusy(true);
    setError('');
    try {
      setValidateResult(
        await validateAction(draft.id, {
          target_object_id: executeTargetId || undefined,
          parameters: JSON.parse(executeParamsJson || '{}'),
        }),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Validate failed');
    } finally {
      setBusy(false);
    }
  }

  async function runExecute() {
    if (!draft.id) return;
    setBusy(true);
    setError('');
    try {
      setExecuteResult(
        await executeAction(draft.id, {
          target_object_id: executeTargetId || undefined,
          parameters: JSON.parse(executeParamsJson || '{}'),
          justification: executeJustification || undefined,
        }),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Execute failed');
    } finally {
      setBusy(false);
    }
  }

  async function runBatch() {
    if (!draft.id) return;
    setBusy(true);
    setError('');
    try {
      const target_object_ids = batchTargetsText
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean);
      setBatchResult(
        await executeActionBatch(draft.id, {
          target_object_ids,
          parameters: JSON.parse(executeParamsJson || '{}'),
          justification: executeJustification || undefined,
        }),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Batch execute failed');
    } finally {
      setBusy(false);
    }
  }

  async function loadWhatIf() {
    if (!draft.id) return;
    setBusy(true);
    try {
      const res = await listActionWhatIfBranches(draft.id, { per_page: 50 });
      setWhatIfBranches(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load what-if branches');
    } finally {
      setBusy(false);
    }
  }

  async function createWhatIf() {
    if (!draft.id) return;
    setBusy(true);
    try {
      await createActionWhatIfBranch(draft.id, JSON.parse(whatIfDraftJson));
      await loadWhatIf();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create what-if failed');
    } finally {
      setBusy(false);
    }
  }

  async function deleteWhatIf(branchId: string) {
    if (!draft.id) return;
    setBusy(true);
    try {
      await deleteActionWhatIfBranch(draft.id, branchId);
      await loadWhatIf();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete what-if failed');
    } finally {
      setBusy(false);
    }
  }

  async function loadMetrics() {
    if (!draft.id) return;
    setBusy(true);
    try {
      setMetrics(await getActionMetrics(draft.id, { window: metricsWindow }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Metrics failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Action types</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Author actions on object types, validate + execute against targets, manage what-if branches, monitor metrics.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs tabs={['authoring', 'operate', 'inline-edits', 'monitoring'] as const} active={tab} onChange={setTab} />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)' }}>
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Action types ({actions.length})</p>
          <button type="button" onClick={() => setDraft(emptyDraft())} className="of-button" style={{ marginTop: 8, fontSize: 12 }}>
            New action
          </button>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {actions.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  onClick={() => loadAction(a)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 10,
                    borderRadius: 8,
                    border: `1px solid ${draft.id === a.id ? '#1d4ed8' : 'var(--border-default)'}`,
                    background: draft.id === a.id ? '#eff6ff' : 'transparent',
                    cursor: 'pointer',
                    marginBottom: 4,
                  }}
                >
                  <strong>{a.display_name}</strong> · {a.operation_kind}
                  <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>{a.name}</p>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          {tab === 'authoring' && (
            <>
              <p className="of-eyebrow">{draft.id ? 'Edit action' : 'New action'}</p>
              <label style={{ fontSize: 13 }}>
                Name
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
                Object type
                <select
                  value={draft.object_type_id}
                  disabled={Boolean(draft.id)}
                  onChange={(e) => setDraft((d) => ({ ...d, object_type_id: e.target.value }))}
                  className="of-input"
                  style={{ marginTop: 4 }}
                >
                  <option value="">— pick —</option>
                  {objectTypes.map((t) => (
                    <option key={t.id} value={t.id}>{t.display_name} ({t.name})</option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Operation kind
                <select
                  value={draft.operation_kind}
                  onChange={(e) => setDraft((d) => ({ ...d, operation_kind: e.target.value as ActionOperationKind }))}
                  className="of-input"
                  style={{ marginTop: 4 }}
                >
                  {OPERATION_KINDS.map((k) => (
                    <option key={k} value={k}>{k}</option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
                <input
                  type="checkbox"
                  checked={draft.confirmation_required}
                  onChange={(e) => setDraft((d) => ({ ...d, confirmation_required: e.target.checked }))}
                />
                Confirmation required
              </label>
              <label style={{ fontSize: 13 }}>
                Permission key
                <input value={draft.permission_key} onChange={(e) => setDraft((d) => ({ ...d, permission_key: e.target.value }))} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <JsonEditor
                label="Input schema JSON"
                value={draft.input_schema_json}
                onChange={(v) => setDraft((d) => ({ ...d, input_schema_json: v }))}
                minHeight={120}
              />
              <JsonEditor
                label="Form schema JSON"
                value={draft.form_schema_json}
                onChange={(v) => setDraft((d) => ({ ...d, form_schema_json: v }))}
                minHeight={80}
              />
              <JsonEditor
                label="Config JSON"
                value={draft.config_json}
                onChange={(v) => setDraft((d) => ({ ...d, config_json: v }))}
                minHeight={140}
              />
              <JsonEditor
                label="Authorization policy JSON"
                value={draft.authorization_policy_json}
                onChange={(v) => setDraft((d) => ({ ...d, authorization_policy_json: v }))}
                minHeight={80}
              />
              <div style={{ display: 'flex', gap: 6 }}>
                <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">
                  {draft.id ? 'Update' : 'Create'}
                </button>
                {draft.id && (
                  <button type="button" onClick={() => void remove()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
                    Delete
                  </button>
                )}
              </div>
            </>
          )}

          {tab === 'operate' && (
            <>
              <p className="of-eyebrow">{selectedAction ? `Operate · ${selectedAction.display_name}` : 'Pick an action first'}</p>
              {selectedAction && (
                <>
                  <label style={{ fontSize: 13 }}>
                    Target object id
                    <input value={executeTargetId} onChange={(e) => setExecuteTargetId(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
                  </label>
                  <JsonEditor
                    label="Parameters JSON"
                    value={executeParamsJson}
                    onChange={setExecuteParamsJson}
                    minHeight={100}
                  />
                  <label style={{ fontSize: 13 }}>
                    Justification
                    <input value={executeJustification} onChange={(e) => setExecuteJustification(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
                  </label>
                  <div style={{ display: 'flex', gap: 6 }}>
                    <button type="button" onClick={() => void runValidate()} disabled={busy} className="of-button">Validate</button>
                    <button type="button" onClick={() => void runExecute()} disabled={busy} className="of-button of-button--primary">Execute</button>
                  </div>
                  {!!validateResult && (
                    <pre style={{ marginTop: 8, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 200 }}>
                      validate: {JSON.stringify(validateResult, null, 2)}
                    </pre>
                  )}
                  {!!executeResult && (
                    <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 240 }}>
                      execute: {JSON.stringify(executeResult, null, 2)}
                    </pre>
                  )}

                  <p className="of-eyebrow" style={{ marginTop: 14 }}>Batch execute</p>
                  <textarea
                    value={batchTargetsText}
                    onChange={(e) => setBatchTargetsText(e.target.value)}
                    placeholder="One target id per line"
                    className="of-input"
                    style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 80 }}
                  />
                  <button type="button" onClick={() => void runBatch()} disabled={busy} className="of-button" style={{ marginTop: 6 }}>Execute batch</button>
                  {!!batchResult && (
                    <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 200 }}>
                      batch: {JSON.stringify(batchResult, null, 2)}
                    </pre>
                  )}
                </>
              )}
            </>
          )}

          {tab === 'inline-edits' && (
            <>
              <p className="of-eyebrow">What-if branches {selectedAction ? `· ${selectedAction.display_name}` : ''}</p>
              {selectedAction && (
                <>
                  <button type="button" onClick={() => void loadWhatIf()} disabled={busy} className="of-button" style={{ fontSize: 12 }}>
                    Load branches
                  </button>
                  <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
                    {whatIfBranches.map((b) => (
                      <li key={b.id}>
                        <strong>{b.name}</strong> · {b.target_object_id ?? '—'}
                        <button type="button" onClick={() => void deleteWhatIf(b.id)} disabled={busy} className="of-button" style={{ marginLeft: 6, fontSize: 10, color: '#b91c1c', borderColor: '#fecaca' }}>
                          delete
                        </button>
                      </li>
                    ))}
                  </ul>
                  <JsonEditor value={whatIfDraftJson} onChange={setWhatIfDraftJson} minHeight={100} />
                  <button type="button" onClick={() => void createWhatIf()} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 6 }}>
                    Create branch
                  </button>
                </>
              )}
            </>
          )}

          {tab === 'monitoring' && (
            <>
              <p className="of-eyebrow">Metrics {selectedAction ? `· ${selectedAction.display_name}` : ''}</p>
              {selectedAction && (
                <>
                  <label style={{ fontSize: 13 }}>
                    Window (e.g. 30d, 12h, 45m)
                    <input value={metricsWindow} onChange={(e) => setMetricsWindow(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
                  </label>
                  <button type="button" onClick={() => void loadMetrics()} disabled={busy} className="of-button" style={{ marginTop: 6 }}>
                    Load metrics
                  </button>
                  {metrics && (
                    <pre style={{ marginTop: 8, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
                      {JSON.stringify(metrics, null, 2)}
                    </pre>
                  )}
                </>
              )}
            </>
          )}
        </section>
      </div>
    </section>
  );
}
