import { useEffect, useRef, useState } from 'react';

import { revertActionExecution, type RevertActionExecutionResponse } from '@/lib/api/ontology';
import { Toast } from '@/lib/components/ui/Toast';

interface ActionRevertToastProps {
  executionId: string | null | undefined;
  actionLabel?: string;
  windowMs?: number;
  onReverted?: (response: RevertActionExecutionResponse) => void;
  onError?: (message: string) => void;
  onDismiss?: () => void;
}

export function ActionRevertToast({
  executionId,
  actionLabel = 'action',
  windowMs = 30_000,
  onReverted,
  onError,
  onDismiss,
}: ActionRevertToastProps) {
  const [remainingMs, setRemainingMs] = useState(windowMs);
  const [busy, setBusy] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!executionId) {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
      setDismissed(false);
      return;
    }
    setDismissed(false);
    setBusy(false);
    setRemainingMs(windowMs);
    if (timerRef.current) clearInterval(timerRef.current);
    timerRef.current = setInterval(() => {
      setRemainingMs((prev) => {
        const next = prev - 1000;
        if (next <= 0) {
          if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
          setDismissed(true);
          onDismiss?.();
          return 0;
        }
        return next;
      });
    }, 1000);
    return () => {
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
    };
  }, [executionId, windowMs]);

  function dismiss() {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
    setDismissed(true);
    onDismiss?.();
  }

  async function undo() {
    if (!executionId || busy) return;
    setBusy(true);
    try {
      const response = await revertActionExecution(executionId);
      if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
      setDismissed(true);
      onReverted?.(response);
    } catch (cause) {
      onError?.(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  }

  const open = Boolean(executionId) && !dismissed;
  const seconds = Math.max(0, Math.ceil(remainingMs / 1000));

  return (
    <Toast open={open} tone="info">
      <span style={{ fontSize: 13 }}>
        {actionLabel} applied. Undo available for <strong>{seconds}s</strong>.
      </span>
      <button type="button" onClick={() => void undo()} disabled={busy || !executionId} className="of-button" style={{ fontSize: 11 }}>
        {busy ? 'Reverting…' : 'Undo'}
      </button>
      <button type="button" onClick={dismiss} aria-label="Dismiss" style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer', fontSize: 14, padding: 0 }}>
        ✕
      </button>
    </Toast>
  );
}
