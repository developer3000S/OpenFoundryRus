import { useSyncExternalStore } from 'react';

export interface Toast {
  id: string;
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
}

let snapshot: Toast[] = [];
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return snapshot;
}

function setSnapshot(next: Toast[]) {
  snapshot = next;
  listeners.forEach((l) => l());
}

function add(type: Toast['type'], message: string, duration = 5000) {
  const id = crypto.randomUUID();
  setSnapshot([...snapshot, { id, type, message }]);
  if (duration > 0) {
    setTimeout(() => dismiss(id), duration);
  }
}

function dismiss(id: string) {
  setSnapshot(snapshot.filter((toast) => toast.id !== id));
}

export const notifications = {
  subscribe,
  getSnapshot,
  success: (msg: string) => add('success', msg),
  error: (msg: string) => add('error', msg),
  info: (msg: string) => add('info', msg),
  warning: (msg: string) => add('warning', msg),
  dismiss,
};

export function useToasts() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
