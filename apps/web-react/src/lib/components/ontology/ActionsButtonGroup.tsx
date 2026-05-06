import { useState } from 'react';

import { executeAction, getActionType, type ActionType } from '@/lib/api/ontology';
import { ActionExecutor } from './ActionExecutor';

export interface ActionButtonConfig {
  action_id: string;
  label?: string;
  color?: 'emerald' | 'sky' | 'rose' | 'amber' | 'slate';
  default_values?: Record<string, unknown>;
  hidden_params?: string[];
  immediate?: boolean;
}

interface ActionsButtonGroupProps {
  typeId: string;
  objectId?: string;
  buttons: ActionButtonConfig[];
  onExecuted?: () => void;
}

const COLOR: Record<NonNullable<ActionButtonConfig['color']>, { background: string; color: string }> = {
  emerald: { background: '#059669', color: '#fff' },
  sky: { background: '#0284c7', color: '#fff' },
  rose: { background: '#e11d48', color: '#fff' },
  amber: { background: '#f59e0b', color: '#0f172a' },
  slate: { background: '#1e293b', color: '#cbd5e1' },
};

export function ActionsButtonGroup({ typeId, objectId, buttons, onExecuted }: ActionsButtonGroupProps) {
  const [openButton, setOpenButton] = useState<ActionButtonConfig | null>(null);
  const [openAction, setOpenAction] = useState<ActionType | null>(null);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');

  async function activate(button: ActionButtonConfig) {
    setError('');
    setBusy(button.action_id);
    try {
      const action = await getActionType(button.action_id);
      const visibleRequired = action.input_schema.filter(
        (field) =>
          field.required &&
          !(button.hidden_params ?? []).includes(field.name) &&
          (button.default_values?.[field.name] ?? field.default_value) === undefined,
      );
      if (button.immediate && visibleRequired.length === 0) {
        await executeAction(button.action_id, {
          target_object_id: objectId,
          parameters: button.default_values ?? {},
        });
        onExecuted?.();
        return;
      }
      setOpenAction(action);
      setOpenButton(button);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy('');
    }
  }

  function close() {
    setOpenAction(null);
    setOpenButton(null);
  }

  function executed() {
    close();
    onExecuted?.();
  }

  return (
    <>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
        {buttons.map((b) => {
          const tone = COLOR[b.color ?? 'sky'];
          return (
            <button
              key={b.action_id + (b.label ?? '')}
              type="button"
              onClick={() => void activate(b)}
              disabled={busy === b.action_id}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 8,
                padding: '6px 12px',
                borderRadius: 12,
                border: 'none',
                fontWeight: 600,
                fontSize: 13,
                cursor: busy === b.action_id ? 'not-allowed' : 'pointer',
                ...tone,
              }}
            >
              {b.label ?? 'Run action'}
            </button>
          );
        })}
      </div>
      {error && <p style={{ marginTop: 8, fontSize: 11, color: '#fca5a5' }}>{error}</p>}
      {openAction && openButton && typeId && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16, zIndex: 100 }}>
          <div style={{ maxHeight: '90vh', width: '100%', maxWidth: 720, overflow: 'auto', background: '#0f172a', color: '#e2e8f0', borderRadius: 16, padding: 24, border: '1px solid #1e293b' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>{openButton.label ?? openAction.display_name ?? openAction.name}</h2>
              <button type="button" onClick={close} className="of-button" style={{ fontSize: 12 }}>Close</button>
            </div>
            <ActionExecutor action={openAction} initialParameters={{ ...(openButton.default_values ?? {}) }} hiddenParams={openButton.hidden_params ?? []} targetObjectId={objectId ?? null} onExecuted={executed} />
            {(openButton.hidden_params ?? []).length > 0 && (
              <p style={{ marginTop: 12, fontSize: 11, color: '#94a3b8' }}>
                Hidden parameters: {(openButton.hidden_params ?? []).join(', ')} are pre-filled by this button and not displayed in the form.
              </p>
            )}
          </div>
        </div>
      )}
    </>
  );
}
