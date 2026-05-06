-- Tarea D1.1.9 P1 — first-class virtual table catalog (3/5 → 4/5).
--
-- Foundry baseline:
--   docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/
--   Core concepts/Virtual tables.md
--
-- A virtual table is "a pointer to a table in a source system outside of
-- Foundry" (doc § Core concepts). The pointer is the (source, locator)
-- pair. Locator shape varies by source:
--   * tabular   — { database, schema, table }   (BigQuery, Snowflake, Databricks)
--   * file      — { bucket, prefix/key, format } (S3, GCS, Azure ADLS)
--   * iceberg   — { catalog, namespace, table }  (Iceberg-backed sources)
--
-- The link table (`virtual_table_sources_link`) holds the per-source
-- toggles that the doc surfaces in the Data Connection app: enabling
-- virtual tables on the source, the auto-registration project, the
-- iceberg catalog kind, and the Code Repositories code-imports /
-- export-controls toggles. P2 will populate the read/write side of
-- those toggles from the connector-management-service `connections`
-- row; P1 only wires the schema and the manual-registration path.

CREATE TABLE IF NOT EXISTS virtual_table_sources_link (
    -- Foundry-style RID minted by `connector-management-service`.
    -- Logical FK only — the row lives in another bounded context, so
    -- we cannot enforce it at the DB level.
    source_rid TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN (
        'AMAZON_S3',
        'AZURE_ABFS',
        'BIGQUERY',
        'DATABRICKS',
        'FOUNDRY_ICEBERG',
        'GCS',
        'SNOWFLAKE'
    )),
    virtual_tables_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    code_imports_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    export_controls JSONB NOT NULL DEFAULT '{}'::jsonb,
    auto_register_project_rid TEXT NULL,
    auto_register_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    auto_register_interval_seconds INT NULL,
    auto_register_tag_filters JSONB NOT NULL DEFAULT '[]'::jsonb,
    iceberg_catalog_kind TEXT NULL CHECK (iceberg_catalog_kind IN (
        'AWS_GLUE',
        'HORIZON',
        'OBJECT_STORAGE',
        'POLARIS',
        'UNITY_CATALOG'
    )),
    iceberg_catalog_config JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_virtual_table_sources_link_provider
    ON virtual_table_sources_link (provider);

CREATE TABLE IF NOT EXISTS virtual_tables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rid TEXT UNIQUE GENERATED ALWAYS AS
        ('ri.foundry.main.virtual-table.' || id::text) STORED,
    source_rid TEXT NOT NULL
        REFERENCES virtual_table_sources_link(source_rid) ON DELETE CASCADE,
    project_rid TEXT NOT NULL,
    name TEXT NOT NULL,
    parent_folder_rid TEXT NULL,
    -- Locator schema is one of: tabular {database, schema, table},
    -- file {bucket, key, format} or iceberg {catalog, namespace, table}.
    locator JSONB NOT NULL,
    table_type TEXT NOT NULL CHECK (table_type IN (
        'TABLE',
        'VIEW',
        'MATERIALIZED_VIEW',
        'EXTERNAL_DELTA',
        'MANAGED_DELTA',
        'MANAGED_ICEBERG',
        'PARQUET_FILES',
        'AVRO_FILES',
        'CSV_FILES',
        'OTHER'
    )),
    schema_inferred JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- {read, write, incremental, versioning, compute_pushdown,
    --  snapshot_supported, append_only_supported, foundry_compute}
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    update_detection_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    update_detection_interval_seconds INT NULL,
    -- Snapshot id (Iceberg) or ETag (object stores). Used by the update
    -- detection poller to decide whether downstream builds must run.
    last_observed_version TEXT NULL,
    last_polled_at TIMESTAMPTZ NULL,
    markings TEXT[] NOT NULL DEFAULT '{}',
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A given tabular locator (db, schema, table) is one virtual table
    -- per source. JSONB equality is text-canonical so the JSON shape
    -- must be stable across writes (we sort keys before binding).
    CONSTRAINT virtual_tables_unique_locator UNIQUE (source_rid, locator),
    CONSTRAINT virtual_tables_unique_name
        UNIQUE (project_rid, parent_folder_rid, name)
);

CREATE INDEX IF NOT EXISTS idx_virtual_tables_project
    ON virtual_tables (project_rid);
CREATE INDEX IF NOT EXISTS idx_virtual_tables_source
    ON virtual_tables (source_rid);
CREATE INDEX IF NOT EXISTS idx_virtual_tables_table_type
    ON virtual_tables (table_type);
CREATE INDEX IF NOT EXISTS idx_virtual_tables_update_detection_enabled
    ON virtual_tables (update_detection_enabled)
    WHERE update_detection_enabled;

CREATE TABLE IF NOT EXISTS virtual_table_imports (
    virtual_table_id UUID NOT NULL
        REFERENCES virtual_tables(id) ON DELETE CASCADE,
    project_rid TEXT NOT NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    imported_by TEXT NULL,
    PRIMARY KEY (virtual_table_id, project_rid)
);

CREATE INDEX IF NOT EXISTS idx_virtual_table_imports_project
    ON virtual_table_imports (project_rid);

CREATE TABLE IF NOT EXISTS virtual_table_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    virtual_table_id UUID NULL,
    source_rid TEXT NULL,
    action TEXT NOT NULL,
    actor_id TEXT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    details JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_virtual_table_audit_virtual_table
    ON virtual_table_audit (virtual_table_id);
CREATE INDEX IF NOT EXISTS idx_virtual_table_audit_source
    ON virtual_table_audit (source_rid);
CREATE INDEX IF NOT EXISTS idx_virtual_table_audit_action_time
    ON virtual_table_audit (action, occurred_at DESC);
