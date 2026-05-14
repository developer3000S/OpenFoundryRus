# Foundry Data Foundation 1:1 parity checklist

Date: 2026-05-11
Scope: public-docs-based parity plan for OpenFoundry's dataset foundation:
datasets, files, branches, transactions, views, schemas, build orchestration,
schedules, Data Lineage, Data Health, data expectations, retention, Data
Lifetime, observability handoffs, and API contracts.

This document is intentionally implementation-oriented. It does not attempt to
clone Palantir branding, private source code, proprietary assets, or any
non-public behavior. The target is **functional parity based on public Palantir
Foundry documentation**: same product concepts, comparable builder and operator
workflows, compatible resource models where useful, and OpenFoundry-native
implementation details that can be tested locally.

## Parity scope boundary

All checklist work is governed by the
[Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).
OpenFoundry may implement behavior described in public Palantir documentation,
but contributors must not copy private source, decompile bundles, import
tenant-specific exports, use Palantir branding, or reuse proprietary assets.
The product target is functional parity in an OpenFoundry-native implementation,
not a pixel-perfect clone.

This checklist covers the data foundation below Pipeline Builder, Workshop,
Ontology, Functions, and Map. It should integrate with those surfaces, but it
should not duplicate the specialized parity checklists for visual pipeline
authoring, Workshop app building, Ontology action execution, geospatial maps,
or Data Connection connector catalogs.

## Status vocabulary

| Status | Meaning |
| --- | --- |
| `todo` | Not implemented or not yet verified in OpenFoundry. |
| `partial` | Some surface exists, but behavior is incomplete or not wired end-to-end. |
| `blocked` | Requires a platform dependency, public documentation, or product decision. |
| `done` | Implemented, tested, documented, and verified through UI or API smoke tests. |

## Priority vocabulary

| Priority | Meaning |
| --- | --- |
| `P0` | Required for credible dataset, build, and schedule semantics used by the Trail Running demo and basic production pipelines. |
| `P1` | Required for Foundry-style data platform parity beyond a single demo. |
| `P2` | Advanced, governance-heavy, or scale-oriented parity. |

## Official Palantir documentation library

These public docs should be treated as the external behavioral contract while
implementing this checklist.

### Product and API overview

- [Foundry platform summary for LLMs](https://www.palantir.com/docs/foundry/getting-started/foundry-platform-summary-llm)
- [Foundry API overview](https://www.palantir.com/docs/foundry/api/)
- [Data integration overview](https://www.palantir.com/docs/foundry/data-integration/overview/)
- [Connecting to data](https://www.palantir.com/docs/foundry/data-integration/connecting-to-data/)

### Datasets, files, transactions, branches, schemas, and views

- [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets)
- [Dataset API: dataset basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/dataset-basics)
- [Dataset API: create dataset](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/create-dataset)
- [Dataset API: get dataset](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/get-dataset)
- [Dataset API: read table](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/read-table-dataset)
- [Dataset API: get dataset schema](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/get-dataset-schema)
- [Dataset API: get schemas batch](https://www.palantir.com/docs/foundry/api/v2/datasets-v2-resources/datasets/get-schema-datasets-batch)
- [Dataset API: put dataset schema](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/put-dataset-schema)
- [Dataset API: list transactions](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/list-transactions-of-dataset)
- [Dataset API: branch basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/branch-basics)
- [Dataset API: get branch](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/get-branch)
- [Dataset API: branch transaction history](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/get-branch-transaction-history)
- [Dataset API: transaction basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/transaction-basics)
- [Dataset API: create transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/create-transaction)
- [Dataset API: commit transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/commit-transaction)
- [Dataset API: abort transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/abort-transaction)
- [Dataset API: file basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/file-basics)
- [Dataset API: list files](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/list-files)
- [Dataset API: upload file](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/upload-file)
- [Dataset API: get file content](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/get-file-content)
- [Dataset API: view basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/views)
- [CSV parsing in Dataset Preview](https://www.palantir.com/docs/foundry/dataset-preview/csv-parsing/)
- [Infer a schema for CSV or JSON files](https://www.palantir.com/docs/foundry/building-pipelines/infer-schema/)
- [Iceberg tables core concepts](https://www.palantir.com/docs/foundry/data-integration/iceberg-tables/)

### Builds and schedules

- [Builds core concepts](https://www.palantir.com/docs/foundry/data-integration/builds/)
- [Scheduling overview](https://www.palantir.com/docs/foundry/building-pipelines/scheduling-overview)
- [Schedules core concepts](https://www.palantir.com/docs/foundry/data-integration/schedules/)
- [Create a schedule](https://www.palantir.com/docs/foundry/building-pipelines/create-schedule/)
- [Scheduling best practices](https://www.palantir.com/docs/foundry/building-pipelines/scheduling-best-practices/)

### Data Lineage and rollback

- [Data Lineage overview](https://www.palantir.com/docs/foundry/data-lineage/overview/)
- [Build datasets from Data Lineage](https://www.palantir.com/docs/foundry/data-lineage/build-datasets/)
- [Manage schedules in Data Lineage](https://www.palantir.com/docs/foundry/data-lineage/manage-schedules/)
- [Node coloring](https://www.palantir.com/docs/foundry/data-lineage/node-coloring/)
- [Roll back a dataset](https://www.palantir.com/docs/foundry/data-lineage/dataset-rollback)
- [Roll back a pipeline](https://www.palantir.com/docs/foundry/data-lineage/pipeline-rollback)

### Data Health, expectations, and observability

- [Observability overview](https://www.palantir.com/docs/foundry/observability/overview)
- [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/)
- [Data Health check types](https://www.palantir.com/docs/foundry/data-health/check-types/)
- [Data Health checks reference](https://www.palantir.com/docs/foundry/data-health/checks-reference/)
- [Configure data health checks from Pipeline Builder](https://www.palantir.com/docs/foundry/pipeline-builder/dataexpectations-configure-health-check)
- [Define data expectations](https://www.palantir.com/docs/foundry/maintaining-pipelines/define-data-expectations/)
- [Workflow Lineage overview](https://www.palantir.com/docs/foundry/workflow-builder/overview)

### Retention and lifecycle

- [Retention overview](https://www.palantir.com/docs/foundry/retention/overview/)
- [Manage retention policies](https://www.palantir.com/docs/foundry/retention/manage-retention-policies)
- [Retention dataset selectors](https://www.palantir.com/docs/foundry/retention/dataset-selectors/)
- [Data Lifetime core concepts](https://www.palantir.com/docs/foundry/data-lifetime/core-concepts-data-lifetime)

## Target OpenFoundry resource model

The implementation should define stable, OpenFoundry-owned resources that can
map to public Foundry concepts without requiring Palantir RID formats.
Compatibility aliases may be accepted at API boundaries, but persisted state
should use OpenFoundry canonical IDs.

| Public Foundry concept | OpenFoundry resource target | Required notes |
| --- | --- | --- |
| Dataset | `dataset` | Wrapper around file-backed data with permissions, schema, branches, transactions, and lineage metadata. |
| Dataset file | `dataset_file` | Logical path plus storage pointer, size, content type, checksum, and transaction membership. |
| Transaction | `dataset_transaction` | Atomic dataset mutation with `OPEN`, `COMMITTED`, and `ABORTED` states. |
| Transaction type | `SNAPSHOT`, `APPEND`, `UPDATE`, `DELETE` | Must affect dataset view calculation exactly as documented in public Foundry concepts. |
| Branch | `dataset_branch` | Named pointer to latest open or committed transaction, with fallback behavior documented locally. |
| Dataset view | `dataset_view` | Effective file set for a branch and transaction/time/version point. |
| Schema | `dataset_schema` | Versioned schema metadata stored on dataset views, including nested and complex field types. |
| View resource | `logical_view` | Schema-backed virtual row view over one or more datasets, not a transform output target. |
| JobSpec | `job_spec` | Immutable build logic definition published by Pipeline Builder or code transforms. |
| Build | `build` | One-time computation of target datasets, composed of ordered jobs. |
| Job | `build_job` | Unit of work from shared logic producing one or more output datasets. |
| Schedule | `build_schedule` | Recurring build trigger plus target strategy and run history. |
| Lineage edge | `lineage_edge` | Input/output/resource dependency edge with branch, version, and logic metadata. |
| Health check | `health_check` | Resource-level validation rule with severity, reports, subscriptions, and alert destinations. |
| Monitoring view | `monitoring_view` | Scope-based monitoring definition over projects, folders, resources, or resource types. |
| Data expectation | `data_expectation` | Build-time assertion that can abort builds and publish Data Health results. |
| Retention policy | `retention_policy` | Dataset/transaction selector plus deletion behavior and audit trail. |
| Data Lifetime policy | `data_lifetime_policy` | Lineage-aware deletion-date assignment for transactions at namespace or folder scope. |

## Milestone A: minimum viable data foundation parity

### Dataset resources and browsing

- [x] `DF.1` Dataset resource CRUD (`P0`, `done`)
  - Create, get, update metadata, move/rename, soft-delete, restore, and hard-delete datasets.
  - Store stable ID, display name, path/folder/project, description, owner, created/updated timestamps, and resource visibility.
  - Expose dataset links from Pipeline Builder outputs, Data Lineage nodes, and Dataset Preview.
  - Implementation note: Dataset rows now persist Foundry-style resource identity and placement fields (`rid`, display name, folder/project/path, visibility, soft-delete timestamp) with active-resource uniqueness. Dataset CRUD resolves UUIDs or RIDs, defaults DELETE to soft-delete, exposes restore plus explicit hard-delete, and returns stable preview/lineage links. Pipeline Builder output controls, build output tables, Data Lineage preview nodes, and Dataset Preview now link back to the dataset resource.
  - Docs: [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets), [Create dataset API](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/create-dataset), [Get dataset API](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/get-dataset).

- [x] `DF.2` Dataset file browser and logical path model (`P0`, `done`)
  - Track logical file paths separately from backing object storage paths.
  - Support list files, get metadata, download content, upload content, and delete file within an open transaction.
  - Capture size, media type, checksum, row-count hint when available, transaction RID/ID, and storage location.
  - Implementation note: `dataset_files` now carries the Foundry-visible logical path, stable transaction ID/RID, backing physical URI, media type/content type, SHA-256 checksum, row-count hint, and structured storage location. DVS exposes list, get-by-ID/get-by-path metadata, presigned content download, presigned upload URL, raw content upload into an open transaction, and transaction-scoped logical delete. Dataset Preview's Files tab now renders the real backing file contract and links active files to the download endpoint.
  - Docs: [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets), [File basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/file-basics), [List files](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/list-files), [Upload file](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/upload-file), [Get file content](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/files/get-file-content).

- [x] `DF.3` Dataset Preview application shell (`P0`, `done`)
  - Provide tabs for Preview, Files, Details, Schema, History, Jobs, Schedules, Health, Lineage, and Retention.
  - Include branch selector, latest-view indicator, transaction/version selector, and API/copy-link affordances.
  - Show permission-aware empty/error states for missing dataset, missing branch, missing transaction, and no schema.
  - Implementation note: Dataset Preview now has the full shell with Preview, Files, Details, Schema, History, Jobs, Schedules, Health, Lineage, and Retention tabs. The header exposes branch, version, and transaction selectors with latest/historical state; Details exposes copyable API paths; Preview/Files/Schema/History are branch-aware; Jobs, Schedules, Health, Lineage, and Retention reuse the existing build, schedule, Data Health, lineage, and retention policy surfaces. Missing datasets, branches, transactions, schemas, and permission failures now render scoped empty/error states instead of collapsing the whole page.
  - Docs: [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets), [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/), [Data Lineage overview](https://www.palantir.com/docs/foundry/data-lineage/overview/).

### Transactions, branches, and views

- [x] `DF.4` Transaction lifecycle (`P0`, `done`)
  - Implement `OPEN -> COMMITTED` and `OPEN -> ABORTED` transitions.
  - Reject commits for non-open transactions and unknown datasets.
  - Preserve written files only after commit and ignore aborted files in latest views.
  - Return transaction type, status, created time, closed time, and IDs in API responses.
  - Implementation note: DVS now exposes the branch transaction lifecycle on both `/v1` and `/api/v1`, accepts the Foundry `transactionType` request field, and returns transaction RID/ID, dataset/branch IDs, transaction type, status, created time, and closed time. Commit and abort both reject non-`OPEN` transactions with `TRANSACTION_NOT_OPEN`, unknown datasets still resolve to 404 before mutation, and the materialization triggers only copy staged files/schemas for `OPEN -> COMMITTED` so aborted writes remain out of latest views.
  - Docs: [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets), [Transaction basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/transaction-basics), [Create transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/create-transaction), [Commit transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/commit-transaction), [Abort transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/abort-transaction).

- [x] `DF.5` Transaction type semantics (`P0`, `done`)
  - Support `SNAPSHOT`, `APPEND`, `UPDATE`, and `DELETE` transaction types.
  - `SNAPSHOT` replaces the effective current view.
  - `APPEND` adds files and rejects overwrites of current-view files.
  - `UPDATE` adds files and may replace existing file references.
  - `DELETE` removes files from the current view without immediately deleting backing storage.
  - Implementation note: DVS commit validation and replay now match the Foundry view rules: `SNAPSHOT` starts a new effective view, `APPEND` only adds non-overlapping logical paths, `UPDATE` adds or replaces logical references without accepting remove ops, and `DELETE` removes logical paths while leaving backing storage records available for retention. Current-view, file browser, and preview fallbacks now read from replayed committed transactions instead of raw undeleted physical rows, so replaced, deleted, and superseded files no longer leak into latest views.
  - Docs: [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets), [Commit transaction](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/transactions/commit-transaction).

- [x] `DF.6` Branch CRUD and branch pointer model (`P0`, `done`)
  - Support create, get, list, delete, and transaction-history APIs for named branches.
  - Track each branch pointer to the most recent open or committed transaction.
  - Prevent branch deletion when it would orphan protected production data.
  - Provide default branch configuration without hard-coding Palantir-only names.
  - Implementation note: Branch CRUD now has Foundry-shaped `/api/v2/datasets/{datasetRid}/branches` aliases with `{name, transactionRid}` responses plus branch transaction history at `/branches/{branchName}/transactions`. Runtime branch pointers move to newly opened transactions, stay on committed transactions, and are restored to the latest non-aborted transaction when an open transaction is aborted; an idempotent migration backfills existing branch heads. Delete now rejects root/default, active, `FOREVER` retention, and open-transaction branches before reparenting children. Dataset creation and default-branch bootstrapping use configurable `active_branch`/`default_branch` request values and the dataset's active branch rather than relying on a Palantir-only branch name.
  - Docs: [Datasets branches](https://www.palantir.com/docs/foundry/data-integration/datasets), [Branch basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/branch-basics), [Get branch](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/get-branch), [Branch transaction history](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/branches/get-branch-transaction-history).

- [x] `DF.7` Dataset view calculation (`P0`, `done`)
  - Compute effective file sets for a branch at latest, transaction-specific, and time/version-specific points.
  - Start views at the latest prior `SNAPSHOT`, or earliest transaction if no snapshot exists.
  - Apply subsequent `APPEND`, `UPDATE`, and `DELETE` operations deterministically.
  - Cache view manifests but always be able to reconstruct from transaction history.
  - Implementation note: Dataset view reads now reconstruct the effective file set from committed transaction history for latest, timestamp, transaction, and dataset-version cutoffs. Transaction cutoffs are applied by ordered transaction identity rather than timestamp alone, so ties do not leak later commits into historical views. Branch views include inherited parent history when a branch was forked from a committed transaction, and the computed manifest is cached into `dataset_views`/`dataset_view_files` by `(dataset, branch, head transaction)` while remaining rebuildable from source transactions.
  - Docs: [Dataset views](https://www.palantir.com/docs/foundry/data-integration/datasets), [View basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/views).

### Schemas and table reads

- [x] `DF.8` Versioned dataset schemas (`P0`, `done`)
  - Store schema metadata on dataset views, not just on datasets globally.
  - Support primitive, decimal, map, array, struct, binary, date, timestamp, nullability, and custom metadata.
  - Show schema evolution across transaction history.
  - Implementation note: Dataset schemas are now persisted against view manifests with branch, end-transaction, content hash, schema version ID, dataframe reader, and updated timestamp metadata. The Foundry-style `getSchema`, `putSchema`, and batch schema APIs normalize `fieldSchemaList` into internal view schema metadata while returning the Foundry wire shape. Recursive schema validation covers primitive, decimal, map, array, struct, binary, date, timestamp, nullability, and field-level custom metadata, and schema history reports per-view evolution with change flags across transaction history.
  - Docs: [Datasets schemas](https://www.palantir.com/docs/foundry/data-integration/datasets), [Get dataset schema](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/get-dataset-schema), [Put dataset schema](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/put-dataset-schema), [Get schemas batch](https://www.palantir.com/docs/foundry/api/v2/datasets-v2-resources/datasets/get-schema-datasets-batch).

- [x] `DF.9` Schema inference and edit flow for CSV/JSON (`P0`, `done`)
  - Offer “apply schema” for CSV and JSON files based on samples.
  - Allow manual column type changes, parser options, delimiter/quote/escape configuration, jagged-row behavior, parse-error behavior, encoding, skip-lines, file path/imported-at/row-number helper columns, and dynamic-inference warnings.
  - Implementation note: Dataset Preview now has an editable Schema tab that can infer CSV or JSON schemas from selected logical files, review sample warnings, manually adjust column names/types/nullability/descriptions, persist parser options, and apply or save the schema to the active branch. The backend exposes `schema:infer` on the Foundry-compatible and OpenFoundry v1 surfaces, samples local backing files or inline JSON/CSV samples, records delimiter/quote/escape/header/null/encoding/skip-line/jagged-row/parse-error/helper-column settings, emits dynamic-inference warnings, and stores the resulting schema as view-scoped metadata through the versioned schema APIs.
  - Docs: [Infer schema](https://www.palantir.com/docs/foundry/building-pipelines/infer-schema/), [CSV parsing](https://www.palantir.com/docs/foundry/dataset-preview/csv-parsing/).

- [x] `DF.10` Table read and preview API (`P0`, `done`)
  - Read rows from the selected branch/view using schema metadata.
  - Provide limit, pagination, column selection, filter, sort, and sample controls.
  - Return typed parse errors with file path and row/column context where possible.
  - Implementation note: Dataset Preview now reads table rows from the selected branch, transaction/version, or view manifest using view-scoped schema metadata and local backing-file contents, with repo metadata preview retained as a fallback. The preview API supports limit/offset pagination, column projection, simple filter expressions, multi-column sort tokens, deterministic sampling, and typed parse errors that include logical file path, row number, column number, field, kind, message, and raw value. A Foundry-style `readTable` endpoint is exposed for CSV output with `branchName`, `endTransactionRid`, `columns`, and `rowLimit` query parameters.
  - Docs: [Read table dataset](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/datasets/read-table-dataset), [Datasets schemas](https://www.palantir.com/docs/foundry/data-integration/datasets).

### Build basics

- [x] `DF.11` Build and job resource model (`P0`, `done`)
  - Model builds as one-time computations over target datasets.
  - Model jobs as units of work generated from immutable JobSpecs and shared logic.
  - Support jobs with one or multiple output datasets, with all outputs updating together.
  - Implementation note: Pipeline Build now persists `target_dataset_rids` on build resources and snapshots each job's immutable JobSpec context (`logic_kind`, content hash, input dataset RIDs, output dataset RIDs, and opened output transactions). Published JobSpecs are idempotent immutable resources with shared multi-output logic; lookup by any output dataset resolves to the same JobSpec RID. The v1 API now exposes build jobs at `/v1/builds/{rid}/jobs`, job resources at `/v1/jobs/{rid}`, and job output atomicity metadata at `/v1/jobs/{rid}/outputs`, including total/committed/aborted counts plus `atomic_commit_status` so multi-output jobs can be verified as updating together.
  - Docs: [Builds core concepts](https://www.palantir.com/docs/foundry/data-integration/builds/).

- [x] `DF.12` Build staleness resolution (`P0`, `done`)
  - Determine whether output datasets are fresh by comparing input data and JobSpec logic against previous builds.
  - Skip up-to-date outputs by default.
  - Support force builds that recompute even fresh targets.
  - Expose “ignored because fresh” status in build and schedule run history.
  - Implementation note: Build resolution now stores a stable input signature from resolved input branch heads, schema metadata, fallback/view options, and internal producer logic, plus a canonical JobSpec logic hash on each job. Non-force builds compare those signatures with prior committed outputs and mark fresh jobs as `stale_skipped`; dependent jobs only skip when their dependency chain is also fresh, while force builds always recompute. The executor now completes fresh jobs without invoking runtime code or committing outputs, aborts the unused open transactions, records `ignored because fresh`, and exposes ignored counts/statuses in build execution responses, job history, pipeline run node results, output resources (`unchanged`), and schedule-triggered run history (`ignored` when all nodes were fresh).
  - Docs: [Builds core concepts](https://www.palantir.com/docs/foundry/data-integration/builds/), [Schedules core concepts](https://www.palantir.com/docs/foundry/data-integration/schedules/).

- [x] `DF.13` Build execution status, logs, and history (`P0`, `done`)
  - Track queued, running, succeeded, failed, cancelled, skipped, and ignored statuses.
  - Show job DAG, attempts, worker/runtime, start/end time, duration, row/file counts, output transactions, and failure causes.
  - Provide live logs while jobs run and persisted logs after completion.
  - Implementation note: Build/job resources now expose normalized execution statuses (`queued`, `running`, `succeeded`, `failed`, `cancelled`, `skipped`, and `ignored`) alongside the canonical Foundry state strings. Jobs carry dependency edges, start/end timestamps, duration, runtime, worker ID, row/file counts, compact output metadata, output transaction status, attempts, stale/ignored flags, hashes, and failure reason; build envelopes include `job_dag`, status counts, and build duration. The executor audit path now advances persisted build state to running/terminal states, releases build locks on terminal events, writes job start/finish timestamps, records result metrics on output commits, and emits transition/output log entries to the configured persistent log store plus live subscriber so `/jobs/{rid}/logs`, `/logs/stream`, and `/logs/ws` can replay history and follow active jobs.
  - Docs: [Builds core concepts](https://www.palantir.com/docs/foundry/data-integration/builds/), [Observability overview](https://www.palantir.com/docs/foundry/observability/overview).

### Schedule basics

- [x] `DF.14` Schedule CRUD and sidebar (`P0`, `done`)
  - Create, edit, pause, resume, delete, and view schedules from Dataset Preview and Data Lineage.
  - Track name, owner, project/folder, targets, trigger, build strategy, branch, run-as identity, last updated user, and pause state.
  - Implementation note: Pipeline Build now exposes a Foundry-shaped schedule resource API at `/api/v1/data-integration/v1/schedules` with create, get, list, patch, pause, resume, delete, run-now, auto-pause exemption, run history, version history, version diff, and project-scope conversion handlers. Schedule persistence now tracks folder, resource RIDs, branch, build strategy, run-as identity, last editor, pause reason/time, owner, trigger, target, and project scope metadata. Dataset Preview's Schedules tab can create dataset-seeded schedules, Build schedules can create/edit/pause/resume/delete, and Data Lineage's right-rail schedule drawer lists and creates schedules for the selected dataset.
  - Docs: [Schedules core concepts](https://www.palantir.com/docs/foundry/data-integration/schedules/), [Create a schedule](https://www.palantir.com/docs/foundry/building-pipelines/create-schedule/), [Manage schedules](https://www.palantir.com/docs/foundry/data-lineage/manage-schedules/).

- [x] `DF.15` Schedule triggers and run history (`P0`, `done`)
  - Support time-based triggers, data-updated triggers, logic-updated triggers, and combined trigger conditions.
  - If a trigger fires while a previous run is active, queue or preserve the pending trigger and run after the previous run completes.
  - Record succeeded, ignored, and failed schedule runs with build IDs and diagnostics.
  - Implementation note: Schedule persistence now carries pending trigger snapshots, last-triggered timestamps, trigger type, and JSON diagnostics on every run. The dispatcher evaluates Foundry-shaped time, event, and compound triggers, stores event observations for multi-condition triggers, exposes scheduler endpoints for due ticks and data/logic events, and enqueues schedule builds with linked build RIDs. If a trigger fires while a prior build is active, the run history records an `IGNORED` coalescing row and preserves the pending trigger snapshot; later scheduler ticks finalize terminal builds, map all-fresh builds to `IGNORED`, and dispatch the preserved pending run.
  - Docs: [Scheduling overview](https://www.palantir.com/docs/foundry/building-pipelines/scheduling-overview), [Schedules core concepts](https://www.palantir.com/docs/foundry/data-integration/schedules/).

## Milestone B: credible Foundry-style data platform parity

### Advanced datasets and views

- [x] `DF.16` Dataset API compatibility surface (`P1`, `done`)
  - Provide OpenFoundry-native endpoints equivalent to public dataset, branch, transaction, file, schema, and view operations.
  - Return stable error codes for not found, permission denied, invalid argument, branch not found, transaction not open, and schema parse errors.
  - Include OAuth/scope or local-token checks equivalent to read/write operation classes.
  - Implementation note: Dataset Versioning Service now mounts a cohesive `/api/v2/datasets` compatibility surface for dataset CRUD/restore, branch CRUD/history, transaction create/get/list/commit/abort/batch-get, logical file list/metadata/download/upload/delete, schema get/put/batch/infer/history/validate, table preview/read, and view list/create/current/at/files/schema/data/refresh operations. Error responses now keep the legacy `error` field while adding stable `code`/`error_code` values for `NOT_FOUND`, `PERMISSION_DENIED`, `INVALID_ARGUMENT`, `BRANCH_NOT_FOUND`, `TRANSACTION_NOT_OPEN`, and `SCHEMA_PARSE_ERROR`. The `/api/v2` surface enforces dataset read/write scopes (`datasets:read`, `datasets:write`, plus existing `dataset.*`/admin aliases) and local-token method/path scopes before dispatching to handlers.
  - Docs: [Foundry API overview](https://www.palantir.com/docs/foundry/api/), [Dataset API docs](https://www.palantir.com/docs/foundry/api/).

- [x] `DF.17` Logical views over backing datasets (`P1`, `done`)
  - Create schema-backed view resources that point to one or more backing datasets and do not store files.
  - Read a view as the union of backing datasets.
  - Support optional primary-key deduplication and automatic rebuild when backing datasets change.
  - Enforce that views can be transform inputs but not transform outputs.
  - Docs: [View basics](https://www.palantir.com/docs/foundry/api/datasets-v2-resources/views).

- [x] `DF.18` Incremental pipeline readiness (`P1`, `done`)
  - Surface whether each dataset is append-only, snapshot-based, update-bearing, delete-bearing, or mixed.
  - Warn when `UPDATE` or `DELETE` transactions break append-only incremental assumptions.
  - Show first-snapshot state and incremental view boundaries.
  - Docs: [Datasets transaction types](https://www.palantir.com/docs/foundry/data-integration/datasets), [Create incremental syncs](https://www.palantir.com/docs/foundry/building-pipelines/create-incremental-syncs/).

- [ ] `DF.19` Iceberg table metadata bridge (`P1`, `todo`)
  - Represent Iceberg table snapshots distinctly from Foundry-style `SNAPSHOT` transactions.
  - Track current schema, branch schema behavior, replace-snapshot/compaction operations, and table metadata pointers.
  - Expose limitations and feature gaps in Dataset Preview.
  - Docs: [Iceberg tables core concepts](https://www.palantir.com/docs/foundry/data-integration/iceberg-tables/).

### Data Lineage graph

- [ ] `DF.20` Data Lineage graph explorer (`P1`, `todo`)
  - Build an interactive graph from datasets, transforms, builds, schedules, Ontology outputs, and workflow handoffs.
  - Support search by dataset, path, project, folder, resource type, repository, schedule, and branch.
  - Provide node details for schema, preview, history, jobs, schedules, health, permissions, and code/source references.
  - Docs: [Data Lineage overview](https://www.palantir.com/docs/foundry/data-lineage/overview/), [Workflow Lineage overview](https://www.palantir.com/docs/foundry/workflow-builder/overview).

- [ ] `DF.21` Data Lineage build helper (`P1`, `todo`)
  - From selected lineage nodes, preview and run build strategies:
    - all ancestor datasets;
    - all transforms between selected datasets;
    - selected datasets only.
  - Apply branch and fallback-branch context when resolving build targets.
  - Allow force build for up-to-date datasets.
  - Docs: [Build datasets from Data Lineage](https://www.palantir.com/docs/foundry/data-lineage/build-datasets/).

- [ ] `DF.22` Data Lineage node coloring and filters (`P1`, `todo`)
  - Provide built-in coloring for resource type, project, folder, repository, build status, Data Health, out-of-date, branch, code status, storage, compute, transaction type, permissions, custom groups, user views, and issues.
  - Allow filters and legends to be saved with graph snapshots.
  - Docs: [Node coloring](https://www.palantir.com/docs/foundry/data-lineage/node-coloring/).

- [ ] `DF.23` Saved lineage graphs and snapshots (`P1`, `todo`)
  - Save graph state, selected nodes, expanded ancestors/descendants, colors, filters, branch, and camera/layout.
  - Support copy link, duplicate graph, export metadata, and presentation-friendly read-only mode.
  - Docs: [Data Lineage overview](https://www.palantir.com/docs/foundry/data-lineage/overview/).

### Build and schedule operations

- [ ] `DF.24` Schedule target strategies (`P1`, `todo`)
  - Configure scheduled builds for one dataset, one dataset plus dependencies, all descendants of a dataset, all datasets connecting two datasets, and mixed target sets.
  - Preview exact build targets before saving.
  - Docs: [Scheduling overview](https://www.palantir.com/docs/foundry/building-pipelines/scheduling-overview), [Create a schedule](https://www.palantir.com/docs/foundry/building-pipelines/create-schedule/).

- [ ] `DF.25` Schedule discovery application (`P1`, `todo`)
  - List schedules by file/dataset/resource, project, owner/updater, name substring, pause state, latest run status, latest run time, and branch.
  - Support saved queries such as paused schedules, schedules scoped to a project, and schedules touching a dataset.
  - Docs: [Schedules core concepts](https://www.palantir.com/docs/foundry/data-integration/schedules/).

- [ ] `DF.26` Schedule best-practice guardrails (`P1`, `todo`)
  - Warn on over-broad targets, schedule overlap, redundant downstream builds, missing health checks, missing owner, and expensive force-build settings.
  - Suggest schedule-status checks for production schedules.
  - Docs: [Scheduling best practices](https://www.palantir.com/docs/foundry/building-pipelines/scheduling-best-practices/), [Data Health check types](https://www.palantir.com/docs/foundry/data-health/check-types/).

### Data Health and expectations

- [ ] `DF.27` Data Health monitoring views (`P1`, `todo`)
  - Create scope-based monitoring views for projects, folders, single resources, and resource types.
  - Include datasets, schedules, streaming datasets, agents, object types, functions, actions, automations, and pipeline resources as monitorable resource classes where local services exist.
  - Support watched checks and aggregate status rollups.
  - Docs: [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/), [Observability overview](https://www.palantir.com/docs/foundry/observability/overview).

- [ ] `DF.28` Resource-level health checks (`P1`, `todo`)
  - Configure health checks from Dataset Preview, Data Lineage, and Pipeline Builder preview panels.
  - Include status, duration, freshness, content, size, schema, sync, build, job, and schedule checks where data is available.
  - Store severity, escalation after consecutive failures, group/monitoring view, notes, and issue-creation prompt.
  - Docs: [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/), [Check types](https://www.palantir.com/docs/foundry/data-health/check-types/), [Checks reference](https://www.palantir.com/docs/foundry/data-health/checks-reference/), [Configure health checks](https://www.palantir.com/docs/foundry/pipeline-builder/dataexpectations-configure-health-check).

- [ ] `DF.29` Health reports, alerts, and subscriptions (`P1`, `todo`)
  - Generate latest and historical check reports.
  - Notify through in-platform notifications and email digests.
  - Provide extension points for Slack, PagerDuty, and arbitrary REST/webhook destinations without hard-coding external credentials.
  - Show health status in Dataset Preview, Data Lineage, schedule details, and project dashboards.
  - Docs: [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/), [Foundry API overview](https://www.palantir.com/docs/foundry/api/).

- [ ] `DF.30` Data expectations as build-time gates (`P1`, `todo`)
  - Define input and output expectations alongside transform code or Pipeline Builder nodes.
  - Abort builds on failed expectations when configured.
  - Publish expectation results into Data Health reports.
  - Require branch/review workflow for expectation changes when protected branches are enabled.
  - Docs: [Define data expectations](https://www.palantir.com/docs/foundry/maintaining-pipelines/define-data-expectations/), [Check types](https://www.palantir.com/docs/foundry/data-health/check-types/).

### Rollback and recovery

- [ ] `DF.31` Dataset rollback (`P1`, `todo`)
  - Roll a transactional dataset back to a successful earlier transaction.
  - Support force-snapshot-on-next-build for incremental recovery.
  - Require editor permission, branch selection, confirmation, and rollback audit records.
  - Show crossed-out rolled-back transactions in History.
  - Docs: [Roll back a dataset](https://www.palantir.com/docs/foundry/data-lineage/dataset-rollback).

- [ ] `DF.32` Pipeline rollback (`P1`, `todo`)
  - Roll back a selected upstream dataset and downstream transactional datasets with preview before confirmation.
  - Allow excluding downstream datasets.
  - Show unsupported resources such as streaming datasets, media sets, virtual tables, and restricted views.
  - Preserve incrementality where possible and warn when logic changed after the selected transaction.
  - Docs: [Roll back a pipeline](https://www.palantir.com/docs/foundry/data-lineage/pipeline-rollback).

## Milestone C: advanced parity, governance, and scale

### Retention and Data Lifetime

- [ ] `DF.33` Retention policy application (`P2`, `todo`)
  - Provide space- or namespace-scoped recommended, custom, and legacy-policy views.
  - Manage policy list, details, filters, and execution history.
  - Treat legacy YAML-style policies as import/read-only or migration inputs, not as the primary authoring surface.
  - Docs: [Retention overview](https://www.palantir.com/docs/foundry/retention/overview/), [Retention navigation](https://www.palantir.com/docs/foundry/retention/navigation/).

- [ ] `DF.34` Retention dataset selectors (`P2`, `todo`)
  - Select/exclude datasets by explicit dataset IDs, folders/projects, derived dataset status, worker type, and future datasets in selected folders.
  - Preview selected datasets before enabling a policy.
  - Docs: [Dataset selectors](https://www.palantir.com/docs/foundry/retention/dataset-selectors/), [Manage retention policies](https://www.palantir.com/docs/foundry/retention/manage-retention-policies).

- [ ] `DF.35` Retention transaction selectors and deletion behavior (`P2`, `todo`)
  - Select transactions by status, type, age, count, branch, closed/open state, and latest-view behavior.
  - Ignore open transactions by default.
  - Support dangerous latest-view deletion only behind explicit admin confirmation and audit trail.
  - Create `DELETE` transactions when current view data is removed by policy.
  - Docs: [Manage retention policies](https://www.palantir.com/docs/foundry/retention/manage-retention-policies), [Datasets retention](https://www.palantir.com/docs/foundry/data-integration/datasets).

- [ ] `DF.36` Data Lifetime lineage-aware policies (`P2`, `todo`)
  - Define namespace/folder policies that assign deletion dates to dataset transactions.
  - Support fixed deletion date and latest-view-only policy modes.
  - Resolve interactions with retention policies and display effective deletion date per transaction.
  - Docs: [Data Lifetime core concepts](https://www.palantir.com/docs/foundry/data-lifetime/core-concepts-data-lifetime).

### Observability and workflow handoffs

- [ ] `DF.37` Workflow Lineage handoff from Data Lineage (`P2`, `todo`)
  - From a dataset or Ontology-backed dataset node, open related workflow graph resources: object types, actions, functions, LLM calls, Workshop applications, and downstream property usage.
  - Show where dataset columns/properties are used in application workflows.
  - Docs: [Workflow Lineage overview](https://www.palantir.com/docs/foundry/workflow-builder/overview), [Observability overview](https://www.palantir.com/docs/foundry/observability/overview).

- [ ] `DF.38` Logs, metrics, traces export (`P2`, `todo`)
  - Export build logs, schedule metrics, health reports, and execution traces to a streaming dataset or local telemetry sink for custom dashboards.
  - Provide filters by status, user, duration, version, source executor, and log search text.
  - Docs: [Observability overview](https://www.palantir.com/docs/foundry/observability/overview).

- [ ] `DF.39` Cross-resource metrics panels (`P2`, `todo`)
  - Show execution counts, failure rates, P95 duration, freshness, last successful build, last schedule run, and alert volume over rolling time windows.
  - Embed metrics in Data Health, Dataset Preview, Data Lineage, project overview, and schedule details.
  - Docs: [Observability overview](https://www.palantir.com/docs/foundry/observability/overview), [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/).

### Permissions, governance, and scale

- [ ] `DF.40` Permission-aware lineage and preview (`P2`, `todo`)
  - Enforce dataset/resource permissions on preview rows, schemas, files, lineage expansion, build actions, rollback, and retention views.
  - In Data Lineage, support permission coloring for current user and selected user when authorized.
  - Docs: [Node coloring](https://www.palantir.com/docs/foundry/data-lineage/node-coloring/), [Datasets core concepts](https://www.palantir.com/docs/foundry/data-integration/datasets).

- [ ] `DF.41` Marking and access propagation hooks (`P2`, `blocked`)
  - Integrate with OpenFoundry security/governance checklist once markings and resource roles exist.
  - Propagate access requirements through derived datasets and lineage.
  - Mark as blocked until security/governance resource semantics are defined in OpenFoundry.
  - Docs: [Foundry API overview](https://www.palantir.com/docs/foundry/api/), [Node coloring](https://www.palantir.com/docs/foundry/data-lineage/node-coloring/).

- [ ] `DF.42` Large-scale graph and metadata indexing (`P2`, `todo`)
  - Incrementally index dataset metadata, branches, transactions, schemas, file manifests, jobs, builds, schedules, and health reports.
  - Support pagination, search, batched schema reads, batched resource lookup, and graph expansion without loading the full universe.
  - Docs: [Get schemas batch](https://www.palantir.com/docs/foundry/api/v2/datasets-v2-resources/datasets/get-schema-datasets-batch), [Data Lineage overview](https://www.palantir.com/docs/foundry/data-lineage/overview/).

## Implementation inventory to collect before coding

- [ ] `INV.1` Identify existing OpenFoundry services that own dataset metadata, dataset files, and dataset versioning.
- [ ] `INV.2` Identify all API routes already exposing dataset, file, transaction, branch, schema, build, schedule, and lineage concepts.
- [ ] `INV.3` Identify the storage backend currently used for files and whether it supports transactional staging.
- [ ] `INV.4` Identify existing build lifecycle tables and whether they can reference dataset transactions.
- [ ] `INV.5` Identify existing schedule/cron/orchestration primitives.
- [ ] `INV.6` Identify existing health-check, alert, issue, notification, and audit-log primitives.
- [ ] `INV.7` Identify existing permission/resource-role primitives that can gate preview, file, build, rollback, and retention operations.
- [ ] `INV.8` Identify existing frontend routes for Dataset Preview, Data Lineage, build details, schedule details, and Data Health.
- [ ] `INV.9` Produce a machine-readable parity matrix sibling JSON after the first implementation inventory, following the pattern of [foundry-feature-parity-matrix.json](./foundry-feature-parity-matrix.json).

## Suggested service boundaries

> **Reader note (2026-05-14)** — The services in the table below are
> *target* decomposition proposals, not a current inventory of
> binaries. Some have been built under consolidated names after S8
> (`marketplace-service` → `federation-product-exchange-service`;
> `approvals-service` → `workflow-automation-service/internal/approvals`;
> `ontology-security-service` → `authorization-policy-service`;
> `ai-service` → `agent-runtime-service` + `llm-catalog-service`).
> Others are not yet implemented. For the canonical list of binaries
> on disk today, see
> [`docs/architecture/services-and-ports.md`](../architecture/services-and-ports.md).

| Surface | Responsibilities |
| --- | --- |
| `dataset-versioning-service` | Dataset CRUD, transaction lifecycle, branch pointers, view manifests, schema versions, file manifests, table reads. |
| `pipeline-build-service` | JobSpec publication, build resolution, staleness, job execution, output transaction commits, build logs. |
| `schedule/orchestration service` | Recurring triggers, schedule run history, pending trigger handling, schedule search. |
| `lineage service` | Dependency graph indexing, graph search, graph snapshots, node coloring facts, rollback planning. |
| `data-health service` | Health checks, monitoring views, reports, alert subscriptions, expectation result ingestion. |
| `retention service` | Retention policy selectors, transaction deletion plans, deletion execution, audit records. |
| `apps/web` | Dataset Preview, Builds app, Schedule sidebar/app, Data Lineage graph, Data Health UI, Retention UI. |

## Acceptance criteria for first complete data foundation milestone

- [ ] A user can create a dataset, open a transaction, upload files, commit it, and see the latest dataset view.
- [ ] A user can create `SNAPSHOT`, `APPEND`, `UPDATE`, and `DELETE` transactions and observe correct file-view semantics.
- [ ] A user can create a branch, commit data on that branch, and inspect branch transaction history.
- [ ] A user can apply or edit a schema and read preview rows through the selected branch/view.
- [ ] A build can create or update output datasets by committing transactions atomically.
- [ ] Build history shows jobs, statuses, logs, output transactions, and staleness/force-build behavior.
- [ ] A schedule can run builds on time or data-update triggers and record succeeded, ignored, and failed runs.
- [ ] Data Lineage can show dataset dependencies and run a build-helper strategy from selected graph nodes.
- [ ] Data Health can define at least one dataset health check and surface its latest report in Dataset Preview and Data Lineage.
- [ ] A rollback smoke test can restore a dataset to an earlier committed transaction on a branch.
- [ ] Retention is either implemented for non-current historical transactions or explicitly blocked behind an admin/product decision with tests protecting current-view data.
- [ ] All OpenFoundry runtime UI is OpenFoundry-native and does not use Palantir branding, screenshots, icons, fonts, or proprietary assets.

## Test plan expectations

- Unit tests for transaction state transitions, transaction type file-view semantics, branch pointers, schema versioning, staleness resolution, schedule trigger evaluation, health-check evaluation, and retention selectors.
- API tests for dataset CRUD, file upload/download/list/delete, transaction create/commit/abort, branch CRUD/history, schema get/put/batch, table preview, build run/history/logs, schedule CRUD/run history, Data Health checks/reports, and rollback planning/execution.
- Integration tests covering Pipeline Builder output commit to dataset transactions, schedule-triggered builds, Data Lineage build helper, Data Health report display, and rollback after incremental commits.
- E2E tests for Dataset Preview, Data Lineage graph, build details, schedule sidebar, and Data Health configuration.
- Regression tests proving aborted transactions and retention-marked historical transactions cannot leak into latest dataset views.
