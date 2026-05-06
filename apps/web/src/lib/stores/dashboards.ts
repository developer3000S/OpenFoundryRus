import { useSyncExternalStore } from 'react';

import {
  createDashboard,
  createStarterDashboards,
  duplicateDashboardDefinition,
  type DashboardDefinition,
} from '../utils/dashboards';

const STORAGE_KEY = 'of_dashboards';

let snapshot: DashboardDefinition[] = [];
let restored = false;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return snapshot;
}

function setSnapshot(next: DashboardDefinition[]) {
  snapshot = next;
  listeners.forEach((l) => l());
}

function persist(next: DashboardDefinition[]) {
  setSnapshot(next);
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  }
  return next;
}

function restore() {
  if (restored) return;
  restored = true;

  if (typeof localStorage === 'undefined') return;

  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    persist(createStarterDashboards());
    return;
  }

  try {
    const parsed = JSON.parse(raw) as DashboardDefinition[];
    if (!Array.isArray(parsed) || parsed.length === 0) {
      persist(createStarterDashboards());
      return;
    }
    setSnapshot(parsed);
  } catch {
    persist(createStarterDashboards());
  }
}

function create(name?: string) {
  const dashboard = createDashboard(name);
  persist([...snapshot, dashboard]);
  return dashboard;
}

function save(dashboard: DashboardDefinition) {
  const next: DashboardDefinition = {
    ...dashboard,
    updatedAt: new Date().toISOString(),
  };
  const has = snapshot.some((entry) => entry.id === dashboard.id);
  if (has) {
    persist(snapshot.map((entry) => (entry.id === dashboard.id ? next : entry)));
  } else {
    persist([...snapshot, next]);
  }
  return next;
}

function remove(id: string) {
  persist(snapshot.filter((entry) => entry.id !== id));
}

function duplicate(id: string) {
  const source = snapshot.find((entry) => entry.id === id);
  if (!source) return null;
  const copy = duplicateDashboardDefinition(source);
  persist([...snapshot, copy]);
  return copy;
}

function getById(id: string) {
  return snapshot.find((entry) => entry.id === id) ?? null;
}

export const dashboards = {
  subscribe,
  getSnapshot,
  restore,
  create,
  save,
  remove,
  duplicate,
  getById,
};

export function useDashboards() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
