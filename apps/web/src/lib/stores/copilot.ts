import { useSyncExternalStore } from 'react';

interface CopilotState {
  open: boolean;
  seedQuestion: string;
}

let snapshot: CopilotState = { open: false, seedQuestion: '' };
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return snapshot;
}

function setSnapshot(next: CopilotState) {
  snapshot = next;
  listeners.forEach((l) => l());
}

export const copilot = {
  open(seedQuestion?: string) {
    setSnapshot({ open: true, seedQuestion: seedQuestion ?? snapshot.seedQuestion });
  },
  close() {
    setSnapshot({ ...snapshot, open: false });
  },
  setSeed(question: string) {
    setSnapshot({ ...snapshot, seedQuestion: question });
  },
};

export function useCopilot(): CopilotState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
