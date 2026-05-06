/**
 * `media-sets-service` REST client.
 *
 * Mirrors the Foundry-style contract documented in
 * `proto/media_set/media_set_service.proto` and exposed by
 * `services/media-sets-service` through `edge-gateway-service`. The
 * proto pipeline does not currently emit TypeScript bindings (the
 * `connect-es` stage in `proto/buf.gen.yaml` writes to a path that
 * isn't materialised in this repo yet); the types below are the manual
 * mirror, kept in lock-step with `media_set.proto` field names and
 * enum values.
 *
 * Convention:
 * * Snake-case field names match the JSON the Rust `serde` impls emit.
 * * Enums (`MediaSetSchema`, `TransactionPolicy`, `TransactionState`)
 *   are the upper-snake-case strings the proto enum values resolve to
 *   without the `MEDIA_SET_SCHEMA_` / `TRANSACTION_POLICY_` /
 *   `TRANSACTION_STATE_` prefix.
 */

import api from './client';

// ---------------------------------------------------------------------------
// Types (mirror of proto/media_set/media_set.proto)
// ---------------------------------------------------------------------------

export type MediaSetSchema =
  | 'IMAGE'
  | 'AUDIO'
  | 'VIDEO'
  | 'DOCUMENT'
  | 'SPREADSHEET'
  | 'EMAIL';

export const MEDIA_SET_SCHEMAS: readonly MediaSetSchema[] = [
  'IMAGE',
  'AUDIO',
  'VIDEO',
  'DOCUMENT',
  'SPREADSHEET',
  'EMAIL',
] as const;

export type TransactionPolicy = 'TRANSACTIONLESS' | 'TRANSACTIONAL';

export type TransactionState = 'OPEN' | 'COMMITTED' | 'ABORTED';

/** Foundry write modes (`Incremental media sets.md`). `REPLACE` is
 *  rejected on transactionless sets server-side with HTTP 422. */
export type WriteMode = 'MODIFY' | 'REPLACE';

/** Per-path resolution policy for `POST .../branches/{name}/merge`. */
export type MergeResolution = 'LATEST_WINS' | 'FAIL_ON_CONFLICT';

export interface MediaSetBranch {
  media_set_rid: string;
  branch_name: string;
  /** Stable Foundry RID derived server-side from `(set_rid, name)`. */
  branch_rid: string;
  parent_branch_rid: string | null;
  head_transaction_rid: string | null;
  created_at: string;
  created_by: string;
}

/** History row surfaced by `GET /media-sets/{rid}/transactions`. */
export interface MediaSetTransactionHistoryEntry {
  rid: string;
  media_set_rid: string;
  branch: string;
  state: TransactionState;
  write_mode: WriteMode;
  opened_at: string;
  closed_at: string | null;
  opened_by: string;
  items_added: number;
  items_modified: number;
  items_deleted: number;
}

export interface MergeBranchResponse {
  source_branch: string;
  target_branch: string;
  resolution: MergeResolution;
  paths_copied: number;
  paths_overwritten: number;
  paths_skipped: number;
}

export interface ResetBranchResponse {
  branch: MediaSetBranch;
  items_soft_deleted: number;
}

export interface MediaSet {
  rid: string;
  project_rid: string;
  name: string;
  schema: MediaSetSchema;
  allowed_mime_types: string[];
  transaction_policy: TransactionPolicy;
  /** Retention window in seconds. `0` = retain forever. */
  retention_seconds: number;
  /** True when the set is virtual (bytes live in `source_rid`). */
  virtual: boolean;
  source_rid: string | null;
  /** Foundry markings (security classifications) attached to the set. */
  markings: string[];
  created_at: string;
  created_by: string;
}

export interface MediaItem {
  rid: string;
  media_set_rid: string;
  branch: string;
  transaction_rid: string;
  path: string;
  mime_type: string;
  size_bytes: number;
  sha256: string;
  metadata: Record<string, unknown>;
  storage_uri: string;
  /** RID of the prior item this one replaced via path dedup. */
  deduplicated_from: string | null;
  deleted_at: string | null;
  created_at: string;
  /**
   * Per-item markings (granular Cedar override; H3). Empty = inherit
   * the parent set's markings 1:1.
   */
  markings?: string[];
}

export interface CreateMediaSetParams {
  name: string;
  project_rid: string;
  schema: MediaSetSchema;
  allowed_mime_types?: string[];
  transaction_policy?: TransactionPolicy;
  /** 0 (default) = retain forever. */
  retention_seconds?: number;
  virtual_?: boolean;
  source_rid?: string | null;
  markings?: string[];
}

export interface ListMediaSetsParams {
  project_rid?: string;
  limit?: number;
  offset?: number;
}

export interface ListItemsParams {
  branch?: string;
  prefix?: string;
  limit?: number;
  cursor?: string;
}

export interface PresignedUploadRequest {
  path: string;
  mime_type: string;
  branch?: string;
  transaction_rid?: string;
  sha256?: string;
  size_bytes?: number;
  expires_in_seconds?: number;
}

export interface PresignedUrlBody {
  url: string;
  expires_at: string;
  headers: Record<string, string>;
  /** Set on upload responses; the freshly-minted item the URL is scoped to. */
  item?: MediaItem;
}

// ---------------------------------------------------------------------------
// Default MIME types per schema (used to prefill the create modal +
// validate uploads when `allowed_mime_types` is empty).
// ---------------------------------------------------------------------------

export const DEFAULT_MIME_TYPES: Record<MediaSetSchema, string[]> = {
  IMAGE: ['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/tiff'],
  AUDIO: ['audio/mpeg', 'audio/wav', 'audio/flac', 'audio/ogg'],
  VIDEO: ['video/mp4', 'video/webm', 'video/quicktime'],
  DOCUMENT: ['application/pdf', 'text/plain', 'text/markdown'],
  SPREADSHEET: [
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'text/csv',
  ],
  EMAIL: ['message/rfc822', 'application/vnd.ms-outlook'],
};

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

export function createMediaSet(params: CreateMediaSetParams) {
  // The Rust DTO accepts `virtual_` to side-step the Rust keyword; the
  // wire format uses the same field name (`#[serde(rename)]`-free) so
  // we pass it through untouched.
  return api.post<MediaSet>('/media-sets', {
    name: params.name,
    project_rid: params.project_rid,
    schema: params.schema,
    allowed_mime_types: params.allowed_mime_types ?? [],
    transaction_policy: params.transaction_policy ?? 'TRANSACTIONLESS',
    retention_seconds: params.retention_seconds ?? 0,
    virtual_: params.virtual_ ?? false,
    source_rid: params.source_rid ?? null,
    markings: params.markings ?? [],
  });
}

export function listMediaSets(params: ListMediaSetsParams = {}) {
  const query = new URLSearchParams();
  if (params.project_rid) query.set('project_rid', params.project_rid);
  if (params.limit) query.set('limit', String(params.limit));
  if (params.offset) query.set('offset', String(params.offset));
  const qs = query.toString();
  return api.get<MediaSet[]>(`/media-sets${qs ? `?${qs}` : ''}`);
}

export function getMediaSet(rid: string) {
  return api.get<MediaSet>(`/media-sets/${encodeURIComponent(rid)}`);
}

export function deleteMediaSet(rid: string) {
  return api.delete<void>(`/media-sets/${encodeURIComponent(rid)}`);
}

// ---------------------------------------------------------------------------
// Markings (H3) — replace + dry-run preview.
// ---------------------------------------------------------------------------

export interface MarkingsPreviewResponse {
  /** Normalised replacement set (lower-case, deduped, sorted). */
  markings: string[];
  /** Markings currently in effect on the set. */
  current_markings: string[];
  /** New markings the operator would add. */
  added: string[];
  /** Markings the operator would remove. */
  removed: string[];
  /** Number of users that will lose access. Wired in H4. */
  users_losing_access: number;
}

export function patchSetMarkings(rid: string, markings: string[]) {
  return api.patch<MediaSet>(`/media-sets/${encodeURIComponent(rid)}/markings`, {
    markings
  });
}

export function previewSetMarkings(rid: string, markings: string[]) {
  return api.post<MarkingsPreviewResponse>(
    `/media-sets/${encodeURIComponent(rid)}/markings/preview`,
    { markings }
  );
}

export function patchItemMarkings(itemRid: string, markings: string[]) {
  return api.patch<MediaItem>(`/items/${encodeURIComponent(itemRid)}/markings`, {
    markings
  });
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

export function listItems(rid: string, params: ListItemsParams = {}) {
  const query = new URLSearchParams();
  if (params.branch) query.set('branch', params.branch);
  if (params.prefix) query.set('prefix', params.prefix);
  if (params.limit) query.set('limit', String(params.limit));
  if (params.cursor) query.set('cursor', params.cursor);
  const qs = query.toString();
  return api.get<MediaItem[]>(
    `/media-sets/${encodeURIComponent(rid)}/items${qs ? `?${qs}` : ''}`,
  );
}

export function getItem(itemRid: string) {
  return api.get<MediaItem>(`/items/${encodeURIComponent(itemRid)}`);
}

export function deleteItem(itemRid: string) {
  return api.delete<void>(`/items/${encodeURIComponent(itemRid)}`);
}

export function getDownloadUrl(
  itemRid: string,
  params: { expires_in_seconds?: number } = {},
) {
  const query = new URLSearchParams();
  if (params.expires_in_seconds) {
    query.set('expires_in_seconds', String(params.expires_in_seconds));
  }
  const qs = query.toString();
  return api.get<PresignedUrlBody>(
    `/items/${encodeURIComponent(itemRid)}/download-url${qs ? `?${qs}` : ''}`,
  );
}

// ---------------------------------------------------------------------------
// Upload orchestrator
// ---------------------------------------------------------------------------

export interface UploadItemOptions {
  branch?: string;
  transaction_rid?: string;
  /**
   * Optional SHA-256 hex digest of the file. The backend defaults to a
   * placeholder derived from the new item RID when absent.
   */
  sha256?: string;
  /** Progress callback in `[0, 1]`. */
  onProgress?: (fraction: number) => void;
}

export interface UploadItemResult {
  item: MediaItem;
  url: string;
}

/**
 * Three-step upload:
 *   1. Ask the backend for a presigned PUT URL — this also persists the
 *      `media_items` row (and applies path dedup if a live item already
 *      exists at the same path).
 *   2. PUT the bytes against the presigned URL.
 *   3. Re-fetch the item so the caller sees the freshly-confirmed row
 *      (storage_uri, deduplicated_from, …) the same way an ingestion
 *      executor would observe it after the upload completes.
 */
export async function uploadItem(
  mediaSetRid: string,
  file: File,
  options: UploadItemOptions = {},
): Promise<UploadItemResult> {
  const presigned = await api.post<PresignedUrlBody>(
    `/media-sets/${encodeURIComponent(mediaSetRid)}/items/upload-url`,
    {
      path: file.name,
      mime_type: file.type || 'application/octet-stream',
      branch: options.branch ?? 'main',
      transaction_rid: options.transaction_rid,
      sha256: options.sha256,
      size_bytes: file.size,
    } satisfies PresignedUploadRequest,
  );

  await putWithProgress(presigned.url, file, presigned.headers, options.onProgress);

  // Re-fetch so callers see the row the way the backend persisted it
  // (path-dedup link + storage_uri).
  const item = presigned.item
    ? await getItem(presigned.item.rid).catch(() => presigned.item!)
    : (() => {
        throw new Error('upload-url response did not include the registered item');
      })();

  return { item, url: presigned.url };
}

/**
 * `fetch` does not surface upload progress today; XHR is the only
 * cross-browser option for a real progress event. We still go through
 * `fetch` when no progress callback is requested so the test harness
 * (Playwright `page.route`) can intercept the PUT cleanly.
 */
function putWithProgress(
  url: string,
  file: File,
  headers: Record<string, string>,
  onProgress?: (fraction: number) => void,
): Promise<void> {
  if (!onProgress) {
    return fetch(url, {
      method: 'PUT',
      headers: {
        'Content-Type': file.type || 'application/octet-stream',
        ...headers,
      },
      body: file,
    }).then((response) => {
      if (!response.ok) {
        throw new Error(`storage PUT failed: ${response.status}`);
      }
    });
  }

  return new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('PUT', url);
    xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream');
    for (const [key, value] of Object.entries(headers)) {
      xhr.setRequestHeader(key, value);
    }
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        onProgress(event.loaded / event.total);
      }
    };
    xhr.onerror = () => reject(new Error('storage PUT transport failure'));
    xhr.onabort = () => reject(new Error('storage PUT aborted'));
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress(1);
        resolve();
      } else {
        reject(new Error(`storage PUT failed: ${xhr.status}`));
      }
    };
    xhr.send(file);
  });
}


// ---------------------------------------------------------------------------
// Branches (H4)
// ---------------------------------------------------------------------------

export function listMediaSetBranches(rid: string) {
  return api.get<MediaSetBranch[]>(`/media-sets/${encodeURIComponent(rid)}/branches`);
}

export function createMediaSetBranch(
  rid: string,
  body: { name: string; from_branch?: string; from_transaction_rid?: string },
) {
  return api.post<MediaSetBranch>(
    `/media-sets/${encodeURIComponent(rid)}/branches`,
    body,
  );
}

export function deleteMediaSetBranch(rid: string, branchName: string) {
  return api.delete<void>(
    `/media-sets/${encodeURIComponent(rid)}/branches/${encodeURIComponent(branchName)}`,
  );
}

/** TRANSACTIONAL only — server returns 422
 *  `MEDIA_SET_TRANSACTIONLESS_REJECTS_RESET` otherwise. */
export function resetMediaSetBranch(rid: string, branchName: string) {
  return api.post<ResetBranchResponse>(
    `/media-sets/${encodeURIComponent(rid)}/branches/${encodeURIComponent(branchName)}/reset`,
    {},
  );
}

export function mergeMediaSetBranch(
  rid: string,
  sourceBranch: string,
  body: { target_branch: string; resolution?: MergeResolution },
) {
  return api.post<MergeBranchResponse>(
    `/media-sets/${encodeURIComponent(rid)}/branches/${encodeURIComponent(sourceBranch)}/merge`,
    body,
  );
}

// ---------------------------------------------------------------------------
// Transactions history (H4 — feeds the History tab)
// ---------------------------------------------------------------------------

export function listMediaSetTransactions(rid: string) {
  return api.get<MediaSetTransactionHistoryEntry[]>(
    `/media-sets/${encodeURIComponent(rid)}/transactions`,
  );
}



// ---------------------------------------------------------------------------
// Access patterns + Usage (H5)
// ---------------------------------------------------------------------------

export type PersistencePolicy = 'RECOMPUTE' | 'PERSIST' | 'CACHE_TTL';

export interface AccessPattern {
  id: string;
  media_set_rid: string;
  kind: string;
  params: Record<string, unknown>;
  persistence: PersistencePolicy;
  ttl_seconds: number;
  created_at: string;
  created_by: string;
}

export interface AccessPatternRunResponse {
  pattern_id: string;
  kind: string;
  item_rid: string;
  persistence: PersistencePolicy;
  cache_hit: boolean;
  compute_seconds: number;
  output_mime_type: string;
  output_storage_uri?: string;
  output_bytes_base64?: string;
  not_implemented_reason?: string;
}

export function listAccessPatterns(rid: string) {
  return api.get<AccessPattern[]>(
    `/media-sets/${encodeURIComponent(rid)}/access-patterns`,
  );
}

export function registerAccessPattern(
  rid: string,
  body: {
    kind: string;
    params?: Record<string, unknown>;
    persistence?: PersistencePolicy;
    ttl_seconds?: number;
  },
) {
  return api.post<AccessPattern>(
    `/media-sets/${encodeURIComponent(rid)}/access-patterns`,
    body,
  );
}

export function runAccessPattern(patternId: string, itemRid: string) {
  return api.get<AccessPatternRunResponse>(
    `/access-patterns/${encodeURIComponent(patternId)}/run?item_rid=${encodeURIComponent(itemRid)}`,
  );
}

// Usage / cost-meter feed for the Usage tab.
export interface UsageBucketByKind {
  kind: string;
  invocations: number;
  cache_hits: number;
  compute_seconds: number;
  input_bytes: number;
}

export interface UsageDailyPoint {
  day: string; // ISO yyyy-mm-dd
  kind: string;
  compute_seconds: number;
  input_bytes: number;
}

export interface UsageResponse {
  since: string;
  until: string;
  total_compute_seconds: number;
  total_input_bytes: number;
  by_kind: UsageBucketByKind[];
  by_day_kind: UsageDailyPoint[];
}

export function getMediaSetUsage(
  rid: string,
  params: { since?: string; until?: string } = {},
) {
  const query = new URLSearchParams();
  if (params.since) query.set('since', params.since);
  if (params.until) query.set('until', params.until);
  const qs = query.toString();
  return api.get<UsageResponse>(
    `/media-sets/${encodeURIComponent(rid)}/usage${qs ? `?${qs}` : ''}`,
  );
}
