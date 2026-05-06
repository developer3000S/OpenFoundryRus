import { useEffect, useMemo, useState } from 'react';

import {
  executeAction,
  executeActionBatch,
  validateAction,
  type ActionInputField,
  type ActionType,
  type ExecuteActionResponse,
  type ExecuteBatchActionResponse,
  type ValidateActionResponse,
} from '@/lib/api/ontology';

interface ActionExecutorProps {
  action: ActionType | null;
  initialParameters?: Record<string, unknown>;
  hiddenParams?: string[];
  targetObjectId?: string | null;
  batchTargetObjectIds?: string[];
  emptyMessage?: string;
  onExecuted?: (response: ExecuteActionResponse | ExecuteBatchActionResponse) => void;
  onValidated?: (response: ValidateActionResponse) => void;
}

function inputFor(field: ActionInputField, value: unknown, set: (v: unknown) => void, disabled: boolean) {
  switch (field.property_type) {
    case 'boolean':
      return (
        <select
          value={value === undefined ? '' : String(value)}
          onChange={(e) => set(e.target.value === '' ? undefined : e.target.value === 'true')}
          disabled={disabled}
          className="of-input"
        >
          <option value="">—</option>
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      );
    case 'integer':
    case 'float':
      return (
        <input
          type="number"
          step={field.property_type === 'float' ? 'any' : '1'}
          value={value === undefined || value === null ? '' : String(value)}
          onChange={(e) => set(e.target.value === '' ? undefined : Number(e.target.value))}
          disabled={disabled}
          className="of-input"
        />
      );
    case 'date':
      return (
        <input type="date" value={typeof value === 'string' ? value : ''} onChange={(e) => set(e.target.value || undefined)} disabled={disabled} className="of-input" />
      );
    case 'json':
    case 'array':
    case 'struct':
      return (
        <textarea
          rows={4}
          value={value === undefined ? '' : JSON.stringify(value, null, 2)}
          onChange={(e) => {
            const raw = e.target.value;
            if (!raw.trim()) { set(undefined); return; }
            try { set(JSON.parse(raw)); } catch { set(raw); }
          }}
          disabled={disabled}
          className="of-input"
          style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}
        />
      );
    default:
      return (
        <input
          type="text"
          value={typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value)}
          onChange={(e) => set(e.target.value || undefined)}
          disabled={disabled}
          className="of-input"
        />
      );
  }
}

export function ActionExecutor({
  action,
  initialParameters = {},
  hiddenParams = [],
  targetObjectId = null,
  batchTargetObjectIds = [],
  emptyMessage = 'This action does not require user-entered parameters.',
  onExecuted,
  onValidated,
}: ActionExecutorProps) {
  const [parameters, setParameters] = useState<Record<string, unknown>>({ ...initialParameters });
  const [justification, setJustification] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [validating, setValidating] = useState(false);
  const [validation, setValidation] = useState<ValidateActionResponse | null>(null);
  const [error, setError] = useState('');
  const [confirm, setConfirm] = useState(false);

  useEffect(() => {
    setParameters({ ...initialParameters });
    setValidation(null);
    setError('');
  }, [action?.id]);

  const visibleFields = useMemo(
    () => (action?.input_schema ?? []).filter((f) => !hiddenParams.includes(f.name)),
    [action, hiddenParams],
  );

  function setField(name: string, value: unknown) {
    setParameters((prev) => {
      const next = { ...prev };
      if (value === undefined) delete next[name];
      else next[name] = value;
      return next;
    });
  }

  async function runValidate() {
    if (!action) return;
    setValidating(true);
    setError('');
    try {
      const r = await validateAction(action.id, { target_object_id: targetObjectId ?? undefined, parameters });
      setValidation(r);
      onValidated?.(r);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setValidating(false);
    }
  }

  async function runExecute() {
    if (!action) return;
    if (action.confirmation_required && !confirm) {
      setConfirm(true);
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      let response: ExecuteActionResponse | ExecuteBatchActionResponse;
      if (batchTargetObjectIds.length > 0) {
        response = await executeActionBatch(action.id, {
          target_object_ids: batchTargetObjectIds,
          parameters,
          justification: justification || undefined,
        });
      } else {
        response = await executeAction(action.id, {
          target_object_id: targetObjectId ?? undefined,
          parameters,
          justification: justification || undefined,
        });
      }
      onExecuted?.(response);
      setConfirm(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSubmitting(false);
    }
  }

  if (!action) {
    return <p className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 13 }}>No action selected.</p>;
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <header>
        <h3 style={{ margin: 0, fontSize: 14 }}>{action.display_name || action.name}</h3>
        {action.description && <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>{action.description}</p>}
      </header>

      {visibleFields.length === 0 ? (
        <p className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 13 }}>{emptyMessage}</p>
      ) : (
        <div style={{ display: 'grid', gap: 8 }}>
          {visibleFields.map((field) => (
            <label key={field.name} style={{ display: 'block', fontSize: 13 }}>
              {field.display_name || field.name}
              {field.required && <span style={{ color: '#fca5a5' }}>*</span>}
              {field.description && <span className="of-text-muted" style={{ display: 'block', fontSize: 11 }}>{field.description}</span>}
              <div style={{ marginTop: 4 }}>
                {inputFor(field, parameters[field.name] ?? field.default_value, (v) => setField(field.name, v), submitting)}
              </div>
            </label>
          ))}
        </div>
      )}

      <label style={{ fontSize: 13 }}>
        Justification (optional)
        <input value={justification} onChange={(e) => setJustification(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
      </label>

      {validation && (
        <div style={{ padding: 8, background: validation.valid ? '#022c22' : '#7f1d1d', color: validation.valid ? '#86efac' : '#fecaca', borderRadius: 6, fontSize: 12 }}>
          {validation.valid ? '✓ Valid' : '✗ Invalid'}
          {validation.errors.length > 0 && (
            <ul style={{ marginTop: 4, paddingLeft: 18 }}>
              {validation.errors.map((e, i) => <li key={i}>{e}</li>)}
            </ul>
          )}
        </div>
      )}

      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}

      {confirm && (
        <div style={{ padding: 12, background: '#78350f', color: '#fde68a', borderRadius: 6, fontSize: 13 }}>
          Confirmation required. Press Execute again to proceed.
        </div>
      )}

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        <button type="button" onClick={() => void runValidate()} disabled={validating} className="of-button" style={{ fontSize: 12 }}>
          {validating ? 'Validating…' : 'Validate'}
        </button>
        <button type="button" onClick={() => void runExecute()} disabled={submitting} className="of-button of-button--primary" style={{ fontSize: 12 }}>
          {submitting ? 'Executing…' : batchTargetObjectIds.length > 0 ? `Execute on ${batchTargetObjectIds.length} objects` : confirm ? 'Confirm execute' : 'Execute'}
        </button>
        {confirm && <button type="button" onClick={() => setConfirm(false)} className="of-button" style={{ fontSize: 12 }}>Cancel</button>}
      </div>
    </section>
  );
}
