/**
 * `virtual-table-service` HTTP client.
 *
 * Mirrors the Rust DTOs in `services/virtual-table-service/src/models/virtual_table.rs`
 * + the proto in `proto/virtual_tables/virtual_tables.proto`. Routes are
 * wired in `services/virtual-table-service/src/main.rs::build_http_router`
 * and live under `/api/v1/...` once the platform reverse proxy mounts
 * the service. Source of truth for the contract:
 *   docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/
 *   Core concepts/Virtual tables.md
 */

import api from './client';

// ---------------------------------------------------------------------------
// Provider / table-type / capability vocabulary
// ---------------------------------------------------------------------------

/**
 * Foundry source providers that can back virtual tables. 1:1 with the
 * Rust `SourceProvider` enum in
 * `services/virtual-table-service/src/domain/capability_matrix.rs`.
 */
export type VirtualTableProvider =
  | 'AMAZON_S3'
  | 'AZURE_ABFS'
  | 'BIGQUERY'
  | 'DATABRICKS'
  | 'FOUNDRY_ICEBERG'
  | 'GCS'
  | 'SNOWFLAKE';

/** Providers documented to support the "Virtual tables" tab in the UI. */
export const VIRTUAL_TABLE_PROVIDERS: ReadonlyArray<VirtualTableProvider> = [
  'AMAZON_S3',
  'AZURE_ABFS',
  'BIGQUERY',
  'DATABRICKS',
  'FOUNDRY_ICEBERG',
  'GCS',
  'SNOWFLAKE',
];

/**
 * Subset of providers that support bulk registration per the Foundry doc
 * § "Bulk registration" — tabular sources only.
 */
export const BULK_REGISTER_PROVIDERS: ReadonlyArray<VirtualTableProvider> = [
  'BIGQUERY',
  'DATABRICKS',
  'SNOWFLAKE',
];

export type TableType =
  | 'TABLE'
  | 'VIEW'
  | 'MATERIALIZED_VIEW'
  | 'EXTERNAL_DELTA'
  | 'MANAGED_DELTA'
  | 'MANAGED_ICEBERG'
  | 'PARQUET_FILES'
  | 'AVRO_FILES'
  | 'CSV_FILES'
  | 'OTHER';

export type ComputePushdownEngine = 'ibis' | 'pyspark' | 'snowpark';

/** Where Foundry can run **its** compute against the table. */
export interface FoundryCompute {
  python_single_node: boolean;
  python_spark: boolean;
  pipeline_builder_single_node: boolean;
  pipeline_builder_spark: boolean;
}

export interface Capabilities {
  read: boolean;
  write: boolean;
  incremental: boolean;
  versioning: boolean;
  compute_pushdown: ComputePushdownEngine | null;
  snapshot_supported: boolean;
  append_only_supported: boolean;
  foundry_compute: FoundryCompute;
}

// ---------------------------------------------------------------------------
// Locator + virtual-table shapes
// ---------------------------------------------------------------------------

export type Locator =
  | { kind: 'tabular'; database: string; schema: string; table: string }
  | { kind: 'file'; bucket: string; prefix: string; format: string }
  | { kind: 'iceberg'; catalog: string; namespace: string; table: string };

/**
 * Inferred schema column. Mirrors the Rust `InferredColumn` shape in
 * `services/virtual-table-service/src/domain/schema_inference.rs`.
 */
export interface SchemaColumn {
  name: string;
  source_type: string;
  inferred_type: string;
  nullable: boolean;
}

/**
 * Update detection state (Foundry doc § "Update detection for virtual
 * table inputs"). The poller body lives behind a flag; the UI only
 * surfaces the toggle + last_polled_at + last_observed_version.
 */
export interface UpdateDetection {
  enabled: boolean;
  interval_seconds: number | null;
  last_observed_version: string | null;
  last_polled_at: string | null;
}

/**
 * Persisted virtual table row. Mirrors `VirtualTableRow` in
 * `services/virtual-table-service/src/models/virtual_table.rs`.
 */
export interface VirtualTable {
  id: string;
  rid: string;
  source_rid: string;
  project_rid: string;
  name: string;
  parent_folder_rid: string | null;
  locator: Record<string, unknown>;
  table_type: TableType;
  schema_inferred: SchemaColumn[];
  capabilities: Capabilities;
  update_detection_enabled: boolean;
  update_detection_interval_seconds: number | null;
  last_observed_version: string | null;
  last_polled_at: string | null;
  markings: string[];
  properties: Record<string, unknown>;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface VirtualTableSourceLink {
  source_rid: string;
  provider: VirtualTableProvider;
  virtual_tables_enabled: boolean;
  code_imports_enabled: boolean;
  export_controls: Record<string, unknown>;
  auto_register_project_rid: string | null;
  auto_register_enabled: boolean;
  auto_register_interval_seconds: number | null;
  auto_register_tag_filters: unknown[];
  iceberg_catalog_kind: IcebergCatalogKind | null;
  iceberg_catalog_config: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
}

export type IcebergCatalogKind =
  | 'AWS_GLUE'
  | 'HORIZON'
  | 'OBJECT_STORAGE'
  | 'POLARIS'
  | 'UNITY_CATALOG';

export interface DiscoveredEntry {
  display_name: string;
  path: string;
  kind:
    | 'database'
    | 'schema'
    | 'table'
    | 'view'
    | 'materialized_view'
    | 'iceberg_namespace'
    | 'iceberg_table'
    | 'file_prefix'
    | 'other';
  registrable: boolean;
  inferred_table_type: TableType | null;
}

export interface RegisterVirtualTableRequest {
  project_rid: string;
  name?: string;
  parent_folder_rid?: string;
  locator: Locator;
  table_type: TableType;
  markings?: string[];
}

export interface BulkRegisterRequest {
  project_rid: string;
  entries: RegisterVirtualTableRequest[];
}

export interface BulkRegisterResponse {
  registered: VirtualTable[];
  errors: Array<{ name: string; error: string }>;
}

/**
 * P4 — auto-registration enable body. Mirrors
 * `EnableAutoRegistrationRequest` in
 * `services/virtual-table-service/src/domain/auto_registration.rs`.
 */
export type FolderMirrorKind = 'FLAT' | 'NESTED';

export interface EnableAutoRegistrationRequest {
  project_name: string;
  folder_mirror_kind: FolderMirrorKind;
  /** Databricks-only — `INFORMATION_SCHEMA.TABLE_TAGS` filter list. */
  table_tag_filters?: string[];
  poll_interval_seconds: number;
}

export interface ListVirtualTablesQuery {
  project?: string;
  source?: string;
  name?: string;
  type?: TableType;
  limit?: number;
  cursor?: string;
}

export interface ListVirtualTablesResponse {
  items: VirtualTable[];
  next_cursor: string | null;
}

/**
 * Structured 412 payload returned when the source fails the doc §
 * "Limitations" rules. Surfaced verbatim to the user so they can act
 * on the remediation hint.
 */
export interface IncompatibleSourceError {
  error: 'VIRTUAL_TABLES_INCOMPATIBLE_SOURCE_CONFIG';
  code:
    | 'AGENT_WORKER_NOT_SUPPORTED'
    | 'AGENT_PROXY_EGRESS_NOT_SUPPORTED'
    | 'BUCKET_ENDPOINT_EGRESS_NOT_SUPPORTED'
    | 'SELF_SERVICE_PRIVATE_LINK_NOT_SUPPORTED'
    | 'SOURCE_NOT_FOUND'
    | 'UPSTREAM_LOOKUP_FAILED';
  reason: { code: string; observed?: string; remediation?: string };
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

const BASE = '/v1';

export const virtualTables = {
  /** POST /v1/sources/{source_rid}/virtual-tables/enable — idempotent. */
  enableOnSource(
    sourceRid: string,
    body: {
      provider: VirtualTableProvider;
      iceberg_catalog_kind?: IcebergCatalogKind;
      iceberg_catalog_config?: Record<string, unknown>;
    },
  ): Promise<VirtualTableSourceLink> {
    return api.post<VirtualTableSourceLink>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/virtual-tables/enable`,
      body,
    );
  },

  /** DELETE /v1/sources/{source_rid}/virtual-tables/enable — disable + return 204. */
  disableOnSource(sourceRid: string): Promise<void> {
    return api.delete<void>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/virtual-tables/enable`,
    );
  },

  /**
   * GET /v1/sources/{source_rid}/virtual-tables/discover?path=
   *
   * Browse the remote catalog. Path encodes the drill-down hierarchy
   * (database → schema → table for warehouses; bucket → prefix →
   * file for object stores).
   */
  discoverRemoteCatalog(
    sourceRid: string,
    path?: string,
  ): Promise<{ data: DiscoveredEntry[] }> {
    const query = path ? `?path=${encodeURIComponent(path)}` : '';
    return api.get<{ data: DiscoveredEntry[] }>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/virtual-tables/discover${query}`,
    );
  },

  /** POST /v1/sources/{source_rid}/virtual-tables/register — single entry. */
  registerVirtualTable(
    sourceRid: string,
    body: RegisterVirtualTableRequest,
  ): Promise<VirtualTable> {
    return api.post<VirtualTable>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/virtual-tables/register`,
      body,
    );
  },

  /** POST /v1/sources/{source_rid}/virtual-tables/bulk-register. */
  bulkRegister(
    sourceRid: string,
    body: BulkRegisterRequest,
  ): Promise<BulkRegisterResponse> {
    return api.post<BulkRegisterResponse>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/virtual-tables/bulk-register`,
      body,
    );
  },

  /**
   * P6 — PATCH /v1/sources/{rid}/code-imports. Toggle whether the
   * source can be imported into a code repository. Foundry doc §
   * "Virtual tables in Code Repositories".
   */
  setCodeImportsEnabled(
    sourceRid: string,
    enabled: boolean,
  ): Promise<VirtualTableSourceLink> {
    return api.patch<VirtualTableSourceLink>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/code-imports`,
      { enabled },
    );
  },

  /**
   * P6 — POST /v1/sources/{rid}/export-controls. Caps the markings /
   * organisations a virtual-table input can carry into a Python
   * Transform run. Empty lists mean "no constraint".
   */
  setExportControls(
    sourceRid: string,
    body: { allowed_markings: string[]; allowed_organizations: string[] },
  ): Promise<VirtualTableSourceLink> {
    return api.post<VirtualTableSourceLink>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/export-controls`,
      body,
    );
  },

  /** POST /v1/sources/{source_rid}/iceberg-catalog. */
  setIcebergCatalog(
    sourceRid: string,
    body: { kind: IcebergCatalogKind; config: Record<string, unknown> },
  ): Promise<VirtualTableSourceLink> {
    return api.post<VirtualTableSourceLink>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/iceberg-catalog`,
      body,
    );
  },

  /** GET /v1/virtual-tables — paginated, filtered list. */
  listVirtualTables(query: ListVirtualTablesQuery = {}): Promise<ListVirtualTablesResponse> {
    const params = new URLSearchParams();
    if (query.project) params.set('project', query.project);
    if (query.source) params.set('source', query.source);
    if (query.name) params.set('name', query.name);
    if (query.type) params.set('type', query.type);
    if (query.limit) params.set('limit', String(query.limit));
    if (query.cursor) params.set('cursor', query.cursor);
    const suffix = params.toString() ? `?${params.toString()}` : '';
    return api.get<ListVirtualTablesResponse>(`${BASE}/virtual-tables${suffix}`);
  },

  /** GET /v1/virtual-tables/{rid}. */
  getVirtualTable(rid: string): Promise<VirtualTable> {
    return api.get<VirtualTable>(`${BASE}/virtual-tables/${encodeURIComponent(rid)}`);
  },

  /** DELETE /v1/virtual-tables/{rid}. */
  deleteVirtualTable(rid: string): Promise<void> {
    return api.delete<void>(`${BASE}/virtual-tables/${encodeURIComponent(rid)}`);
  },

  /** PATCH /v1/virtual-tables/{rid}/markings. */
  updateMarkings(rid: string, markings: string[]): Promise<VirtualTable> {
    return api.patch<VirtualTable>(
      `${BASE}/virtual-tables/${encodeURIComponent(rid)}/markings`,
      { markings },
    );
  },

  /** POST /v1/virtual-tables/{rid}/refresh-schema. */
  refreshSchema(rid: string): Promise<VirtualTable> {
    return api.post<VirtualTable>(
      `${BASE}/virtual-tables/${encodeURIComponent(rid)}/refresh-schema`,
      {},
    );
  },

  /**
   * P4 — POST /v1/sources/{rid}/auto-registration. Provisions the
   * Foundry-managed project + persists the auto-register toggles.
   */
  enableAutoRegistration(
    sourceRid: string,
    body: EnableAutoRegistrationRequest,
  ): Promise<VirtualTableSourceLink> {
    return api.post<VirtualTableSourceLink>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/auto-registration`,
      body,
    );
  },

  /** DELETE /v1/sources/{rid}/auto-registration — flips toggle off (no delete). */
  disableAutoRegistration(sourceRid: string): Promise<void> {
    return api.delete<void>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/auto-registration`,
    );
  },

  /** POST /v1/sources/{rid}/auto-registration:scan-now. */
  scanAutoRegistrationNow(
    sourceRid: string,
  ): Promise<{ added: number; updated: number; orphaned: number }> {
    return api.post<{ added: number; updated: number; orphaned: number }>(
      `${BASE}/sources/${encodeURIComponent(sourceRid)}/auto-registration:scan-now`,
      {},
    );
  },

  /**
   * Legacy P3 stub kept for backwards compat. Prefer
   * [`setUpdateDetection`] below — the real backend endpoint shipped
   * in P5.
   */
  enableUpdateDetection(
    _rid: string,
    body: { interval_seconds: number },
  ): Promise<{ enabled: boolean; interval_seconds: number; deferred: true }> {
    return Promise.resolve({
      enabled: true,
      interval_seconds: body.interval_seconds,
      deferred: true,
    });
  },

  /**
   * P5 — PATCH /v1/virtual-tables/{rid}/update-detection. Foundry
   * doc § "Update detection for virtual table inputs".
   */
  setUpdateDetection(
    rid: string,
    body: UpdateDetectionToggle,
  ): Promise<VirtualTable> {
    return api.patch<VirtualTable>(
      `${BASE}/virtual-tables/${encodeURIComponent(rid)}/update-detection`,
      body,
    );
  },

  /** POST /v1/virtual-tables/{rid}/update-detection:poll-now. */
  pollUpdateDetectionNow(rid: string): Promise<UpdateDetectionPollResult> {
    return api.post<UpdateDetectionPollResult>(
      `${BASE}/virtual-tables/${encodeURIComponent(rid)}/update-detection:poll-now`,
      {},
    );
  },

  /**
   * GET /v1/virtual-tables/{rid}/update-detection/history?limit=
   *
   * Returns the most recent N polls. Renders the same row shape the
   * Dataset Preview Details panel uses to graph error rates by
   * source.
   */
  updateDetectionHistory(
    rid: string,
    limit = 50,
  ): Promise<{ data: UpdateDetectionPollHistoryRow[] }> {
    return api.get<{ data: UpdateDetectionPollHistoryRow[] }>(
      `${BASE}/virtual-tables/${encodeURIComponent(rid)}/update-detection/history?limit=${limit}`,
    );
  },
};

// ---------------------------------------------------------------------------
// P5 — update-detection types.
// ---------------------------------------------------------------------------

export interface UpdateDetectionToggle {
  enabled: boolean;
  interval_seconds: number;
}

export type UpdateDetectionPollOutcome =
  | 'initial'
  | 'changed'
  | 'unchanged'
  | 'potential_update'
  | 'failed';

export interface UpdateDetectionPollResult {
  virtual_table_rid: string;
  outcome: UpdateDetectionPollOutcome;
  observed_version: string | null;
  previous_version: string | null;
  latency_ms: number;
  change_detected: boolean;
  event_emitted: boolean;
}

export interface UpdateDetectionPollHistoryRow {
  id: string;
  virtual_table_id: string;
  polled_at: string;
  observed_version: string | null;
  change_detected: boolean;
  latency_ms: number;
  error_message: string | null;
}

// ---------------------------------------------------------------------------
// Pure helpers (UI-facing labels / heuristics).
// ---------------------------------------------------------------------------

export function providerLabel(provider: VirtualTableProvider): string {
  switch (provider) {
    case 'AMAZON_S3':
      return 'Amazon S3';
    case 'AZURE_ABFS':
      return 'Azure ADLS / OneLake';
    case 'BIGQUERY':
      return 'BigQuery';
    case 'DATABRICKS':
      return 'Databricks';
    case 'FOUNDRY_ICEBERG':
      return 'Foundry Iceberg';
    case 'GCS':
      return 'Google Cloud Storage';
    case 'SNOWFLAKE':
      return 'Snowflake';
  }
}

export function tableTypeLabel(type: TableType): string {
  switch (type) {
    case 'TABLE':
      return 'Table';
    case 'VIEW':
      return 'View';
    case 'MATERIALIZED_VIEW':
      return 'Materialized view';
    case 'EXTERNAL_DELTA':
      return 'External Delta';
    case 'MANAGED_DELTA':
      return 'Managed Delta';
    case 'MANAGED_ICEBERG':
      return 'Managed Iceberg';
    case 'PARQUET_FILES':
      return 'Parquet files';
    case 'AVRO_FILES':
      return 'Avro files';
    case 'CSV_FILES':
      return 'CSV files';
    case 'OTHER':
      return 'Other';
  }
}

/**
 * `provider × tableType` ⇒ does the source bulk-register UI apply?
 * The Foundry doc only blesses bulk for tabular providers, but the
 * UI also gates on the table type because S3/GCS/ABFS file-based
 * tables register one prefix at a time.
 */
export function supportsBulkRegister(provider: VirtualTableProvider): boolean {
  return BULK_REGISTER_PROVIDERS.includes(provider);
}

/**
 * Map a provider to the tab-eligibility flag the source detail page uses.
 * Mirrors the doc § "Set up a connection for a virtual table" — only
 * the seven supported providers expose the tab.
 */
export function providerSupportsVirtualTables(
  provider: string | null | undefined,
): provider is VirtualTableProvider {
  return !!provider && (VIRTUAL_TABLE_PROVIDERS as readonly string[]).includes(provider);
}

/**
 * Foundry compatibility-matrix lookup, mirrored from the Rust
 * `capability_matrix::compatibility` so the UI can render
 * `Capabilities` badges before the row hits the backend.
 */
export function defaultCapabilities(
  provider: VirtualTableProvider,
  tableType: TableType,
): Capabilities {
  const allFoundry: FoundryCompute = {
    python_single_node: true,
    python_spark: true,
    pipeline_builder_single_node: false,
    pipeline_builder_spark: true,
  };
  const sparkOnly: FoundryCompute = {
    python_single_node: false,
    python_spark: true,
    pipeline_builder_single_node: false,
    pipeline_builder_spark: true,
  };
  const noFoundry: FoundryCompute = {
    python_single_node: false,
    python_spark: false,
    pipeline_builder_single_node: false,
    pipeline_builder_spark: false,
  };
  if (provider === 'BIGQUERY') {
    return {
      read: true,
      write: true,
      incremental: true,
      versioning: false,
      compute_pushdown: 'ibis',
      snapshot_supported: true,
      append_only_supported: true,
      foundry_compute: allFoundry,
    };
  }
  if (provider === 'DATABRICKS') {
    if (tableType === 'EXTERNAL_DELTA' || tableType === 'MANAGED_ICEBERG') {
      return {
        read: true,
        write: true,
        incremental: true,
        versioning: true,
        compute_pushdown: 'pyspark',
        snapshot_supported: true,
        append_only_supported: true,
        foundry_compute: allFoundry,
      };
    }
    return {
      read: true,
      write: false,
      incremental: tableType === 'MANAGED_DELTA',
      versioning: tableType === 'MANAGED_DELTA',
      compute_pushdown: 'pyspark',
      snapshot_supported: true,
      append_only_supported: tableType === 'MANAGED_DELTA',
      foundry_compute: allFoundry,
    };
  }
  if (provider === 'SNOWFLAKE') {
    if (tableType === 'MANAGED_ICEBERG') {
      return {
        read: true,
        write: false,
        incremental: true,
        versioning: true,
        compute_pushdown: 'snowpark',
        snapshot_supported: true,
        append_only_supported: true,
        foundry_compute: sparkOnly,
      };
    }
    return {
      read: true,
      write: true,
      incremental: true,
      versioning: false,
      compute_pushdown: 'snowpark',
      snapshot_supported: true,
      append_only_supported: true,
      foundry_compute: allFoundry,
    };
  }
  // Object stores: read+write but no Foundry compute / pushdown.
  return {
    read: true,
    write: true,
    incremental: false,
    versioning: false,
    compute_pushdown: null,
    snapshot_supported: false,
    append_only_supported: false,
    foundry_compute: noFoundry,
  };
}

/**
 * Stable list of capability "chips" the UI renders next to a virtual
 * table. Order is doc-aligned so visual scanning is consistent across
 * tables.
 */
export function capabilityChips(caps: Capabilities): string[] {
  const out: string[] = [];
  if (caps.read) out.push('Read');
  if (caps.write) out.push('Write');
  if (caps.incremental) out.push('Incremental');
  if (caps.versioning) out.push('Versioning');
  if (caps.compute_pushdown) {
    out.push(`Pushdown: ${caps.compute_pushdown}`);
  }
  return out;
}
