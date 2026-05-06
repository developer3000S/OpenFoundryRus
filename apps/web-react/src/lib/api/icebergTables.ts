/**
 * `iceberg-catalog-service` REST client.
 *
 * Mirrors the admin surface (`/api/v1/iceberg-tables/*`) that backs the
 * `/iceberg-tables` page. The Iceberg REST Catalog spec endpoints
 * (`/iceberg/v1/...`) live on a separate prefix and are consumed by
 * external clients (PyIceberg, Spark, Trino), not by the Foundry UI.
 *
 * Doc reference: `Iceberg tables.md` (§ "Foundry Iceberg catalog").
 */

import api from './client';

export interface IcebergTableSummary {
  id: string;
  rid: string;
  project_rid: string;
  namespace: string[];
  name: string;
  format_version: 1 | 2 | 3;
  location: string;
  markings: string[];
  last_snapshot_at: string | null;
  row_count_estimate: number | null;
  created_at: string;
}

export interface IcebergTableListResponse {
  tables: IcebergTableSummary[];
}

export interface IcebergTableDetail {
  summary: IcebergTableSummary;
  schema: unknown;
  properties: Record<string, unknown>;
  partition_spec: unknown;
  sort_order: unknown;
  current_metadata_location: string | null;
  current_snapshot_id: number | null;
  last_sequence_number: number;
}

export interface IcebergSnapshotEntry {
  snapshot_id: number;
  parent_snapshot_id: number | null;
  operation: 'append' | 'overwrite' | 'delete' | 'replace';
  timestamp: string | null;
  sequence_number: number;
  manifest_list: string;
  schema_id: number;
  summary: Record<string, string>;
}

export interface IcebergSnapshotsResponse {
  snapshots: IcebergSnapshotEntry[];
}

export interface IcebergMetadataResponse {
  metadata: unknown;
  metadata_location: string;
  history: Array<{ version: number; path: string; created_at: string }>;
}

export interface IcebergBranchesResponse {
  branches: Array<{ name: string; kind: 'branch' | 'tag'; snapshot_id: number }>;
}

export interface IcebergCreateApiTokenResponse {
  id: string;
  name: string;
  token_hint: string;
  scopes: string[];
  expires_at: string | null;
  created_at: string;
  raw_token: string;
}

export interface IcebergListFilters {
  project_rid?: string;
  namespace?: string;
  name?: string;
  sort?: 'name' | 'created_at';
}

export async function listIcebergTables(
  filters: IcebergListFilters = {},
): Promise<IcebergTableListResponse> {
  const search = new URLSearchParams();
  if (filters.project_rid) search.set('project_rid', filters.project_rid);
  if (filters.namespace) search.set('namespace', filters.namespace);
  if (filters.name) search.set('name', filters.name);
  if (filters.sort) search.set('sort', filters.sort);
  const qs = search.toString();
  return api.get<IcebergTableListResponse>(
    `/iceberg-tables${qs ? `?${qs}` : ''}`,
  );
}

export async function getIcebergTable(id: string): Promise<IcebergTableDetail> {
  return api.get<IcebergTableDetail>(`/iceberg-tables/${id}`);
}

export async function listIcebergSnapshots(id: string): Promise<IcebergSnapshotsResponse> {
  return api.get<IcebergSnapshotsResponse>(`/iceberg-tables/${id}/snapshots`);
}

export async function getIcebergMetadata(id: string): Promise<IcebergMetadataResponse> {
  return api.get<IcebergMetadataResponse>(`/iceberg-tables/${id}/metadata`);
}

export async function listIcebergBranches(id: string): Promise<IcebergBranchesResponse> {
  return api.get<IcebergBranchesResponse>(`/iceberg-tables/${id}/branches`);
}

export async function createIcebergApiToken(
  name: string,
  scopes: string[] = ['api:iceberg-read', 'api:iceberg-write'],
  ttl_secs?: number,
): Promise<IcebergCreateApiTokenResponse> {
  return api.fetch<IcebergCreateApiTokenResponse>('/iceberg-clients/api-tokens', {
    method: 'POST',
    body: { name, scopes, ttl_secs },
  });
}

// ─── P3 markings + diagnose ────────────────────────────────────────────

export interface IcebergMarkingProjection {
  marking_id: string;
  name: string;
  description: string;
}

export interface IcebergTableMarkings {
  effective: IcebergMarkingProjection[];
  explicit: IcebergMarkingProjection[];
  inherited_from_namespace: IcebergMarkingProjection[];
}

export interface IcebergNamespaceMarkings {
  effective: IcebergMarkingProjection[];
  explicit: IcebergMarkingProjection[];
}

async function rawCatalogFetch<T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  // Iceberg catalog endpoints live outside the platform's `/api/v1`
  // prefix because external clients (PyIceberg, Spark, …) expect the
  // exact path the spec defines. The UI talks to the same surface so
  // we sidestep the api client's base URL.
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...api.authorizationHeaders(),
  };
  const response = await fetch(path, {
    method: options.method ?? 'GET',
    headers,
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  if (!response.ok) {
    const detail = await response.text().catch(() => '');
    throw new Error(`${response.status}: ${detail}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}

export async function getIcebergTableMarkingsByPath(
  namespace: string,
  table: string,
): Promise<IcebergTableMarkings> {
  return rawCatalogFetch<IcebergTableMarkings>(
    `/iceberg/v1/namespaces/${encodeURIComponent(namespace)}/tables/${encodeURIComponent(table)}/markings`,
  );
}

export async function updateIcebergTableMarkings(
  namespace: string,
  table: string,
  markings: string[],
): Promise<IcebergTableMarkings> {
  return rawCatalogFetch<IcebergTableMarkings>(
    `/iceberg/v1/namespaces/${encodeURIComponent(namespace)}/tables/${encodeURIComponent(table)}/markings`,
    { method: 'PATCH', body: { markings } },
  );
}

export interface IcebergDiagnoseStep {
  name: string;
  ok: boolean;
  latency_ms: number;
  detail: string | null;
}

export interface IcebergDiagnoseResponse {
  client: string;
  success: boolean;
  steps: IcebergDiagnoseStep[];
  total_latency_ms: number;
}

export async function runIcebergDiagnose(
  client: string,
  project_rid?: string,
): Promise<IcebergDiagnoseResponse> {
  return rawCatalogFetch<IcebergDiagnoseResponse>('/iceberg/v1/diagnose', {
    method: 'POST',
    body: { client, project_rid },
  });
}

/**
 * Format the Foundry-style operation badge color used in the UI.
 */
export function operationBadgeClass(op: IcebergSnapshotEntry['operation']): string {
  switch (op) {
    case 'append':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300';
    case 'overwrite':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-300';
    case 'delete':
      return 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300';
    case 'replace':
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300';
  }
}
