import { useSyncExternalStore } from 'react';

interface OntologySearchState {
  open: boolean;
  initialQuery: string;
}

let snapshot: OntologySearchState = { open: false, initialQuery: '' };
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return snapshot;
}

function setSnapshot(next: OntologySearchState) {
  snapshot = next;
  listeners.forEach((l) => l());
}

export const ontologySearch = {
  open(initialQuery = '') { setSnapshot({ open: true, initialQuery }); },
  close() { setSnapshot({ ...snapshot, open: false }); },
  toggle() { setSnapshot({ ...snapshot, open: !snapshot.open, initialQuery: '' }); },
};

export function useOntologySearch(): OntologySearchState {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
