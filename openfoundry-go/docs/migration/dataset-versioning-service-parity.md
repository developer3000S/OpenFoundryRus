# dataset-versioning-service Rust → Go parity inventory

Date: 2026-05-07

This inventory tracks the 1:1 migration of `services/dataset-versioning-service` into `openfoundry-go/services/dataset-versioning-service`. It is documentation-only: no runtime behavior is changed here.

## Inputs and audit command

Generated baseline:

```sh
cd openfoundry-go && go run ./tools/route-audit -services dataset-versioning-service
```

Manual validation sources:

- Rust Foundry/versioning router: `services/dataset-versioning-service/src/lib.rs`
- Rust data asset catalog router: `services/dataset-versioning-service/src/data_asset_catalog/mod.rs`
- Rust quality router: `services/dataset-versioning-service/src/dataset_quality/mod.rs`
- Go chi router: `openfoundry-go/services/dataset-versioning-service/internal/server/server.go`

Route-audit summary after the Data Asset Catalog handler port on 2026-05-07: Rust routes: 70; Go routes: 87; state counts: `implemented: 61`, `config-gated: 8`, `501: 18`, `missing: 0`. The absorbed dataset model/metadata/markings/permissions/lineage/file-index and advanced branch lifecycle, views/schema/preview, and files/backing filesystem routes now execute real Go handlers; the remaining work is explicit handler implementation behind mounted routes, not missing HTTP paths.

Status meanings used below:

- `implemented`: Go has the exact Rust path and a non-placeholder handler.
- `partial`: Go has a related handler/model but not the full Rust contract.
- `missing`: no Go route/handler equivalent is mounted; this should remain zero after this pass.
- `stub`: Go route exists and intentionally returns HTTP `501 Not Implemented` with `TODO(dataset-versioning parity): ...`.
- `config-gated`: Go route exists and depends on optional runtime/config wiring such as backing filesystem presign support.

## Current Go-only mounted surface

These Go routes remain mounted for the existing Go subset and compatibility, in addition to the exact Rust `/v1`, `/api/v1`, `/internal`, and public paths mounted in this pass.

| Method | Go path | Go handler | Notes |
| --- | --- | --- | --- |
| GET | `/healthz` | inline health handler | Exact public healthz equivalent. |
| GET | `/metrics` | `m.Handler()` | Exact public metrics equivalent. |
| GET | `/api/v1/datasets` | `h.ListDatasets` | Related to Rust `GET /v1/datasets`; prefix/contract mismatch. |
| POST | `/api/v1/datasets` | `h.CreateDataset` | Related to Rust `POST /v1/datasets`; prefix/contract mismatch. |
| GET | `/api/v1/datasets/{id}` | `h.GetDataset` | Related to Rust `GET /v1/datasets/{rid}`; UUID `{id}` vs RID-aware Rust. |
| PATCH | `/api/v1/datasets/{id}` | `h.UpdateDataset` | Related to Rust `PATCH /v1/datasets/{rid}`. |
| DELETE | `/api/v1/datasets/{id}` | `h.DeleteDataset` | Related to Rust `DELETE /v1/datasets/{rid}`. |
| GET | `/api/v1/datasets/{id}/versions` | `h.ListVersions` | Related to Rust `GET /v1/datasets/{rid}/versions`. |
| POST | `/api/v1/datasets/{id}/versions` | `h.CreateVersion` | Go-only legacy-style create version; no Rust route in audited routers. |
| GET | `/api/v1/datasets/{id}/versions/{version}` | `h.GetVersion` | Go-only route; Rust audited router only lists versions. |
| GET | `/api/v1/datasets/{id}/branches` | `h.ListBranches` | Related to Rust `GET /v1/datasets/{rid}/branches`. |
| POST | `/api/v1/datasets/{id}/branches` | `h.CreateBranch` | Related to Rust `POST /v1/datasets/{rid}/branches`. |
| GET | `/api/v1/datasets/{id}/branches/{branch}` | `h.GetBranch` | Related to Rust `GET /v1/datasets/{rid}/branches/{branch}`. |
| GET | `/api/v1/datasets/{id}/files` | `h.ListFiles` | Related to Rust file listings; prefix/contract mismatch. |
| GET | `/api/v1/datasets/{id}/files/{file_id}/download` | `h.DownloadFile` | Related to Rust download route; `config-gated`. |
| POST | `/api/v1/datasets/{id}/transactions/{txn}/files` | `h.CreateFileUploadURL` | Related to Rust upload-url route; `config-gated`. |

## Route parity by domain

### health / metrics / internal local-fs

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/healthz` | `healthz` / `handlers::health::healthz` | `/healthz` | `func(w` | implemented | `smoke.rs`, `retention_aborted_txns.rs` |
| GET | `/health` | `healthz` / `handlers::health::healthz` | `/health` | `func(w` | implemented | none found |
| GET | `/metrics` | `metrics_endpoint` / `handlers::health::metrics` | `/metrics` | `m.Handler(` | implemented | `smoke.rs` |
| GET | `/internal/datasets/{rid}/metadata` | `handlers::internal::get_dataset_metadata` | `/internal/datasets/{rid}/metadata` | `h.GetDatasetMetadata` | stub | none found |
| GET | `/v1/_internal/local-fs/{*key}` | `handlers::files::local_presign_proxy` | `/v1/_internal/local-fs/{key:.+}` | `h.LocalPresignProxy` | config-gated | `backing_fs_local_round_trip.rs`, `presigned_download_url_audited.rs` |

### dataset CRUD / model / metadata

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/catalog/facets` | `handlers::catalog::get_catalog_facets` | `/v1/catalog/facets` | `h.GetCatalogFacets` | stub | none found |
| GET | `/v1/datasets` | `handlers::crud::list_datasets` | `/v1/datasets` | `h.ListDatasets` | implemented | none found |
| POST | `/v1/datasets` | `handlers::crud::create_dataset` | `/v1/datasets` | `h.CreateDataset` | implemented | none found |
| GET | `/v1/datasets/{rid}` | `handlers::crud::get_dataset` | `/v1/datasets/{rid}` | `h.GetDataset` | implemented | none found |
| PATCH | `/v1/datasets/{rid}` | `handlers::crud::update_dataset` | `/v1/datasets/{rid}` | `h.UpdateDataset` | implemented | none found |
| DELETE | `/v1/datasets/{rid}` | `handlers::crud::delete_dataset` | `/v1/datasets/{rid}` | `h.DeleteDataset` | implemented | none found |
| GET | `/v1/datasets/{rid}/model` | `handlers::dataset_model::get_dataset_model` | `/v1/datasets/{rid}/model` | `h.GetDatasetModel` | implemented | `dataset_foundry_model.rs`, `cross_service_via_lineage.rs` |
| PATCH | `/v1/datasets/{rid}/metadata` | `handlers::dataset_model::patch_dataset_metadata` | `/v1/datasets/{rid}/metadata` | `h.PatchDatasetMetadata` | implemented | `dataset_foundry_model.rs` |

### markings / permissions / lineage-links / file index

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/datasets/{rid}/markings` | `handlers::dataset_model::list_dataset_markings` | `/v1/datasets/{rid}/markings` | `h.ListDatasetMarkings` | implemented | `markings_inheritance.rs`, `branch_markings_snapshot_at_creation.rs` |
| PUT | `/v1/datasets/{rid}/markings` | `handlers::dataset_model::put_dataset_markings` | `/v1/datasets/{rid}/markings` | `h.PutDatasetMarkings` | implemented | `markings_inheritance.rs`, `branch_markings_snapshot_at_creation.rs` |
| GET | `/v1/datasets/{rid}/permissions` | `handlers::dataset_model::list_dataset_permissions` | `/v1/datasets/{rid}/permissions` | `h.ListDatasetPermissions` | implemented | `dataset_foundry_model.rs` |
| PUT | `/v1/datasets/{rid}/permissions` | `handlers::dataset_model::put_dataset_permissions` | `/v1/datasets/{rid}/permissions` | `h.PutDatasetPermissions` | implemented | `dataset_foundry_model.rs` |
| GET | `/v1/datasets/{rid}/lineage-links` | `handlers::dataset_model::list_dataset_lineage_links` | `/v1/datasets/{rid}/lineage-links` | `h.ListDatasetLineageLinks` | implemented | `cross_service_via_lineage.rs` |
| PUT | `/v1/datasets/{rid}/lineage-links` | `handlers::dataset_model::put_dataset_lineage_links` | `/v1/datasets/{rid}/lineage-links` | `h.PutDatasetLineageLinks` | implemented | `cross_service_via_lineage.rs` |
| GET | `/v1/datasets/{rid}/files/index` | `handlers::dataset_model::list_dataset_file_index` | `/v1/datasets/{rid}/files/index` | `h.ListDatasetFileIndex` | implemented | `files_endpoint_respects_view_algorithm.rs` |
| PUT | `/v1/datasets/{rid}/files/index` | `handlers::dataset_model::put_dataset_file_index` | `/v1/datasets/{rid}/files/index` | `h.PutDatasetFileIndex` | implemented | `files_endpoint_respects_view_algorithm.rs` |

### versions

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/datasets/{rid}/versions` | `handlers::versions::list_versions` | `/v1/datasets/{rid}/versions` | `h.ListVersions` | implemented | `api_conformance_pagination.rs`, `transactions_lifecycle.rs` |

### branches / branch actions / ancestry / fallback / compare / retention / markings / restore / rollback

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/datasets/{rid}/branches` | `handlers::foundry::list_branches` | `/v1/datasets/{rid}/branches` | `h.ListBranches` | implemented | `branches_create_and_reparent.rs`, `branch_lifecycle_full_journey.rs`, `api_conformance_pagination.rs` |
| POST | `/v1/datasets/{rid}/branches` | `handlers::foundry::create_branch` | `/v1/datasets/{rid}/branches` | `h.CreateBranch` | implemented | `branches_create_and_reparent.rs`, `branch_create_from_transaction.rs`, `branch_create_from_transaction_rejects_open.rs`, `branch_create_from_transaction_rejects_aborted.rs`, `branch_event_emitted_on_create_via_outbox.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}` | `handlers::foundry::get_branch` | `/v1/datasets/{rid}/branches/{branch}` | `h.GetBranch` | implemented | `api_conformance_etag.rs`, `foundry_semantics_edge_cases.rs` |
| DELETE | `/v1/datasets/{rid}/branches/{branch}` | `handlers::foundry::delete_branch` | `/v1/datasets/{rid}/branches/{branch}` | `h.DeleteBranch` | implemented | `branch_delete_reparents_children_idempotent.rs`, `branch_lifecycle_full_journey.rs` |
| POST | `/v1/datasets/{rid}/branches/{branch}` | `handlers::foundry::branch_action` | `/v1/datasets/{rid}/branches/{branch}` | `h.BranchAction` | implemented | `branches_create_and_reparent.rs` |
| POST | `/v1/datasets/{rid}/branches/{branch}/checkout` | `handlers::branches::checkout_branch` | `/v1/datasets/{rid}/branches/{branch}/checkout` | `h.CheckoutBranch` | implemented | `branch_lifecycle_full_journey.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}/ancestry` | `handlers::foundry::branch_ancestry` | `/v1/datasets/{rid}/branches/{branch}/ancestry` | `h.BranchAncestry` | implemented | `branch_ancestry_endpoint.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}/preview-delete` | `handlers::foundry::preview_delete_branch` | `/v1/datasets/{rid}/branches/{branch}/preview-delete` | `h.PreviewDeleteBranch` | implemented | `branch_delete_reparents_children_idempotent.rs` |
| PATCH | `/v1/datasets/{rid}/branches/{branch}/retention` | `handlers::retention::update_retention` | `/v1/datasets/{rid}/branches/{branch}/retention` | `h.UpdateRetention` | implemented | `branch_retention_inherited_resolves_to_parent.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}/markings` | `handlers::retention::get_branch_markings` | `/v1/datasets/{rid}/branches/{branch}/markings` | `h.GetBranchMarkings` | implemented | `branch_markings_snapshot_at_creation.rs`, `markings_inheritance.rs` |
| POST | `/v1/datasets/{rid}/branches/{branch}:restore` | `handlers::retention::restore_branch` | `/v1/datasets/{rid}/branches/{branch}:restore` | `h.RestoreBranch` | implemented | `branch_lifecycle_full_journey.rs` |
| GET | `/v1/datasets/{rid}/branches/compare` | `handlers::compare::compare_branches` | `/v1/datasets/{rid}/branches/compare` | `h.CompareBranches` | implemented | `branch_compare_detects_conflicts.rs`, `branch_compare_lca_correct.rs` |
| POST | `/v1/datasets/{rid}/branches/{branch}/rollback` | `handlers::foundry::rollback_branch` | `/v1/datasets/{rid}/branches/{branch}/rollback` | `h.RollbackBranch` | implemented | `branch_lifecycle_full_journey.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}/fallbacks` | `handlers::foundry::list_fallbacks` | `/v1/datasets/{rid}/branches/{branch}/fallbacks` | `h.ListFallbacks` | implemented | `branch_fallback_resolution.rs` |
| PUT | `/v1/datasets/{rid}/branches/{branch}/fallbacks` | `handlers::foundry::put_fallbacks` | `/v1/datasets/{rid}/branches/{branch}/fallbacks` | `h.PutFallbacks` | implemented | `branch_fallback_resolution.rs` |
| GET | `/v1/datasets/{rid}/compare` | `handlers::foundry::compare_views` | `/v1/datasets/{rid}/compare` | `h.CompareViews` | stub | `branch_compare_detects_conflicts.rs`, `files_endpoint_respects_view_algorithm.rs` |

### transactions / batchGet / commit / abort

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| POST | `/v1/datasets/{rid}/branches/{branch}/transactions` | `handlers::foundry::start_transaction` | `/v1/datasets/{rid}/branches/{branch}/transactions` | `h.StartTransaction` | stub | `transactions_lifecycle.rs`, `transaction_types_matrix.rs`, `concurrent_transactions.rs`, `branch_open_tx_blocks_new_tx.rs`, `branch_open_tx_allows_child_branch.rs` |
| GET | `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | `handlers::foundry::get_transaction` | `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | `h.GetTransaction` | stub | `transactions_lifecycle.rs`, `api_conformance_batch_207.rs` |
| POST | `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | `handlers::foundry::transaction_action` | `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | `h.TransactionAction` | stub | `transactions_lifecycle.rs`, `transaction_types_matrix.rs`, `delete_transaction_does_not_purge_storage.rs`, `retention_aborted_txns.rs`, `concurrent_transactions.rs` |
| GET | `/v1/datasets/{rid}/transactions` | `handlers::foundry::list_transactions` | `/v1/datasets/{rid}/transactions` | `h.ListTransactions` | stub | `api_conformance_pagination.rs`, `transactions_lifecycle.rs` |
| POST | `/v1/datasets/{rid}/transactions:batchGet` | `handlers::foundry::batch_get_transactions` | `/v1/datasets/{rid}/transactions:batchGet` | `h.BatchGetTransactions` | stub | `api_conformance_batch_207.rs` |

### views / current / at / files / preview / data / schema

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/datasets/{rid}/views` | `handlers::views::list_views` | `/v1/datasets/{rid}/views` | `h.ListViews` | implemented | `union_view_aggregates_all_deployment_outputs.rs`, `view_at_timestamp.rs` |
| POST | `/v1/datasets/{rid}/views` | `handlers::views::create_view` | `/v1/datasets/{rid}/views` | `h.CreateView` | implemented | `union_view_aggregates_all_deployment_outputs.rs` |
| GET | `/v1/datasets/{rid}/views/{view_or_action}` | `handlers::views::get_view` | `/v1/datasets/{rid}/views/{view_or_action}` | `h.GetView` | implemented | `union_view_aggregates_all_deployment_outputs.rs` |
| POST | `/v1/datasets/{rid}/views/{view_or_action}` | `view_action_dispatch` | `/v1/datasets/{rid}/views/{view_or_action}` | `h.ViewAction` | implemented | `union_view_aggregates_all_deployment_outputs.rs` |
| GET | `/v1/datasets/{rid}/views/{view_id}/preview` | `handlers::views::preview_view` | `/v1/datasets/{rid}/views/{view_id}/preview` | `h.PreviewMaterializedView` | implemented | `preview_text_json_lines.rs`, `preview_csv_with_custom_delimiter.rs`, `preview_avro.rs` |
| GET | `/v1/datasets/{rid}/views/current` | `handlers::foundry::get_current_view` | `/v1/datasets/{rid}/views/current` | `h.GetCurrentView` | implemented | `files_endpoint_respects_view_algorithm.rs`, `schema_per_view_versions_independently.rs` |
| GET | `/v1/datasets/{rid}/views/at` | `handlers::foundry::get_view_at` | `/v1/datasets/{rid}/views/at` | `h.GetViewAt` | implemented | `view_at_timestamp.rs` |
| GET | `/v1/datasets/{rid}/views/{view_id}/files` | `handlers::foundry::list_view_files` | `/v1/datasets/{rid}/views/{view_id}/files` | `h.ListViewFiles` | implemented | `files_endpoint_respects_view_algorithm.rs` |
| GET | `/v1/datasets/{rid}/views/{view_id}/data` | `handlers::preview::preview_view` | `/v1/datasets/{rid}/views/{view_id}/data` | `h.PreviewViewData` | implemented | `preview_text_json_lines.rs`, `preview_csv_with_custom_delimiter.rs`, `preview_avro.rs` |
| GET | `/v1/datasets/{rid}/views/{view_id}/schema` | `handlers::schema::get_view_schema` | `/v1/datasets/{rid}/views/{view_id}/schema` | `h.GetViewSchema` | implemented | `schema_field_types_roundtrip.rs`, `schema_csv_options_round_trip.rs`, `schema_per_view_versions_independently.rs` |
| POST | `/v1/datasets/{rid}/views/{view_id}/schema` | `handlers::schema::put_view_schema` | `/v1/datasets/{rid}/views/{view_id}/schema` | `h.PutViewSchema` | implemented | `schema_field_types_roundtrip.rs`, `schema_csv_options_round_trip.rs`, `schema_per_view_versions_independently.rs` |
| GET | `/v1/datasets/{rid}/preview` | `handlers::preview::preview_data` | `/v1/datasets/{rid}/preview` | `h.PreviewDataset` | implemented | `preview_text_json_lines.rs`, `preview_csv_with_custom_delimiter.rs`, `preview_avro.rs` |
| GET | `/v1/datasets/{rid}/schema` | `handlers::preview::get_schema` / `handlers::schema::get_current_schema` | `/v1/datasets/{rid}/schema` | `h.GetCurrentSchema` | implemented | `schema_field_types_roundtrip.rs`, `schema_per_view_versions_independently.rs` |

### files / upload-url / storage-details / upload multipart

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/v1/datasets/{rid}/files` | `handlers::export::list_files` / `handlers::files::list_files` | `/v1/datasets/{rid}/files` | `h.ListFiles` | implemented | `files_endpoint_respects_view_algorithm.rs` |
| GET | `/v1/datasets/{rid}/files/{file_id}/download` | `handlers::files::download_file` | `/v1/datasets/{rid}/files/{file_id}/download` | `h.DownloadFile` | config-gated | `presigned_download_url_audited.rs`, `backing_fs_local_round_trip.rs` |
| POST | `/v1/datasets/{rid}/transactions/{txn_id}/files` | `handlers::files::upload_url` | `/v1/datasets/{rid}/transactions/{txn_id}/files` | `h.CreateFileUploadURL` | config-gated | `presigned_download_url_audited.rs`, `backing_fs_local_round_trip.rs` |
| GET | `/v1/datasets/{rid}/storage-details` | `handlers::files::storage_details` | `/v1/datasets/{rid}/storage-details` | `h.StorageDetails` | config-gated | `backing_fs_logical_to_physical_mapping_stable.rs` |
| POST | `/v1/datasets/{rid}/upload` | `handlers::upload::upload_data` | `/v1/datasets/{rid}/upload` | `h.UploadData` | config-gated | none found |

### schema validation

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| POST | `/v1/datasets/{rid}/schema:validate` | `handlers::schema_validate::validate_schema` | `/v1/datasets/{rid}/schema:validate` | `h.ValidateSchema` | implemented | `schema_validation_rejects_invalid.rs` |

### quality / lint / health

| Method | Rust path | Rust handler | Go path | Go handler | Status | Rust tests |
| --- | --- | --- | --- | --- | --- | --- |
| GET | `/api/v1/datasets/{id}/quality` | `handlers::quality::get_dataset_quality` | `/api/v1/datasets/{id}/quality` | `h.GetDatasetQuality` | stub | none found |
| POST | `/api/v1/datasets/{id}/quality/profile` | `handlers::quality::refresh_dataset_quality` | `/api/v1/datasets/{id}/quality/profile` | `h.RefreshDatasetQuality` | stub | none found |
| POST | `/api/v1/datasets/{id}/quality/rules` | `handlers::quality::create_quality_rule` | `/api/v1/datasets/{id}/quality/rules` | `h.CreateQualityRule` | stub | none found |
| PATCH | `/api/v1/datasets/{id}/quality/rules/{rule_id}` | `handlers::quality::update_quality_rule` | `/api/v1/datasets/{id}/quality/rules/{rule_id}` | `h.UpdateQualityRule` | stub | none found |
| DELETE | `/api/v1/datasets/{id}/quality/rules/{rule_id}` | `handlers::quality::delete_quality_rule` | `/api/v1/datasets/{id}/quality/rules/{rule_id}` | `h.DeleteQualityRule` | stub | none found |
| GET | `/api/v1/datasets/{id}/lint` | `handlers::lint::get_dataset_lint` | `/api/v1/datasets/{id}/lint` | `h.GetDatasetLint` | stub | none found |
| GET | `/v1/datasets/{rid}/health` | `handlers::health::get_dataset_health` | `/v1/datasets/{rid}/health` | `h.GetDatasetHealth` | stub | none found |
| GET | `/api/v1/datasets/{rid}/health` | `handlers::health::get_dataset_health` | `/api/v1/datasets/{rid}/health` | `h.GetDatasetHealth` | stub | none found |

### retention worker

No HTTP route is mounted for the retention worker. Rust has domain/runtime coverage in `src/domain/retention_worker.rs` and test coverage in `branch_retention_archives_after_ttl.rs` plus retention-related route tests (`branch_retention_inherited_resolves_to_parent.rs`, `retention_aborted_txns.rs`). Go has no equivalent worker package in `openfoundry-go/services/dataset-versioning-service` yet.

### Iceberg / catalog / backing filesystem

No Iceberg/catalog/backing-filesystem implementation package exists in the current Go target beyond optional `storageabstraction.BackingFS` use by file download/upload-url handlers. Rust coverage spans `src/storage/iceberg.rs`, `src/storage/backing_fs_factory.rs`, `src/storage/factory.rs`, `src/storage/runtime.rs`, and `src/data_asset_catalog/domain/*`. Related Rust tests include `backing_fs_local_round_trip.rs`, `backing_fs_logical_to_physical_mapping_stable.rs`, `presigned_download_url_audited.rs`, and `union_view_aggregates_all_deployment_outputs.rs`.

## Models parity inventory

Go now carries Rust-compatible wire structs in `openfoundry-go/services/dataset-versioning-service/internal/models/models.go`, including exact JSON tags for dataset, catalog/model metadata, branches, transactions, views/files/schema, quality/lint/health, retention-worker and Iceberg/write payload surfaces. JSONB / `serde_json::Value` fields are represented as `json.RawMessage` via `JSONValue`; fixed tokens are represented by typed string constants.

| Rust source | Rust model/domain types | Go equivalent | Status |
| --- | --- | --- | --- |
| `src/models/dataset.rs`, `data_asset_catalog/models/dataset.rs` | `Dataset`, `CreateDatasetRequest`, `UpdateDatasetRequest`, `ListDatasetsQuery` | `Dataset`, `CreateDatasetRequest`, `UpdateDatasetRequest`, `ListResponse`, `Page` | implemented for wire shape; query structs remain handler-local where applicable. |
| `src/models/version.rs`, `data_asset_catalog/models/version.rs` | `DatasetVersion` | `DatasetVersion`, `CreateDatasetVersionRequest` | implemented |
| `src/models/branch.rs`, `storage/runtime.rs`, `handlers/foundry.rs` | `DatasetBranch`, `RuntimeBranch`, `CreateBranchBody`, `BranchSource`, `ReparentBody`, fallback/ancestry structs | `DatasetBranch`, `RuntimeBranch`, `CreateBranchBody`, `BranchSource`, `ReparentBody`, `RuntimeFallbackEntry`, `BranchAncestryResponse` | implemented |
| `src/domain/retention.rs`, `handlers/retention.rs` | `RetentionPolicy`, `RetentionRow`, `EffectiveRetention`, `UpdateRetentionBody` | `RetentionPolicy` constants, `RetentionRow`, `EffectiveRetention`, `UpdateRetentionBody` | implemented |
| `src/domain/branch_markings.rs` | `MarkingSource`, `BranchMarking`, `BranchMarkingsView` | `MarkingSource` constants, `BranchMarking`, `BranchMarkingsView` | implemented |
| `src/handlers/compare.rs`, `handlers/foundry.rs` | `TransactionSummary`, `ConflictingFile`, `BranchCompareResponse`, `CompareOut`, `FileDiff`, `FileChange` | same Go wire structs | implemented |
| `src/models/transaction.rs`, `storage/runtime.rs`, `handlers/conformance.rs` | `DatasetTransaction`, `RuntimeTransaction`, transaction action/list bodies, batch 207 envelopes | `DatasetTransaction`, `RuntimeTransaction`, `StartTransactionBody`, `BatchGetTransactionsRequest`, `BatchItemResult`, `ErrorEnvelope` | implemented |
| `src/models/schema.rs`, `handlers/schema.rs`, `data_asset_catalog/handlers/schema_validate.rs` | Foundry schema model, schema response/upsert, validation reports | `DatasetSchema`, `Field`, `CsvOptions`, `CustomMetadata`, `SchemaResponse`, `PutSchemaBody`, `ValidateRequest`, `ValidateResponse` | implemented |
| `src/domain/views.rs`, `data_asset_catalog/models/view.rs`, `handlers/preview.rs` | view rows, computed views, file ops, preview/data query shapes | `DatasetView`, `ViewOut`, `RuntimeViewFile`, `PreviewQuery`, `PreviewDataResponse`, `RuntimeTransactionFile` | implemented |
| `src/handlers/files.rs`, `data_asset_catalog/models/dataset_model.rs` | file list/download/upload-url/storage-details, permissions, lineage, file index, rich model | `DatasetFileOut`, `ListFilesOut`, `UploadUrlBody`, `UploadUrlOut`, `StorageDetailsOut`, `DatasetRichModel`, `DatasetPermissionEdge`, `DatasetLineageLink`, `DatasetFileIndexEntry` | implemented |
| `src/dataset_quality/models/*` | quality, lint and health models | `DatasetQuality*`, `DatasetLint*`, `DatasetHealth*` structs | implemented |
| `src/domain/branch_events.rs`, `domain/retention_worker.rs` | branch events and retention worker results | `BranchEnvelope`, `RetentionWorkerResult` | implemented for wire/event payloads |
| `src/domain/parameterized_view.rs`, storage/Iceberg write payloads | union view and backing filesystem/write payloads | `UnionViewSpec`, `IcebergTableRef`, `IcebergWritePayload`, `ResolvedDatasetSource`, staged/runtime file payloads | implemented for wire payloads |

Compatibility fixtures live under `internal/models/testdata/` and are round-tripped by `internal/models/*_test.go`.

## Migration parity inventory

Go has a copied/ported migration set under `openfoundry-go/services/dataset-versioning-service/internal/repo/migrations`. It includes every Rust migration listed below plus one Go-only compatibility migration (`20260501120000_dataset_rid_compat.sql`).

| Rust migration | Go equivalent | Status |
| --- | --- | --- |
| `20260419100001_initial_datasets.sql` | same filename | implemented |
| `20260419100002_reserved_compatibility_slot.sql` | same filename | implemented |
| `20260421103000_data_catalog_quality.sql` | same filename | implemented |
| `20260421174000_dataset_branches.sql` | same filename | implemented |
| `20260424213000_dataset_branch_merge_metadata.sql` | same filename | implemented |
| `20260425173000_dataset_views_transactions.sql` | same filename | implemented |
| `20260501000001_versioning_init.sql` | same filename | implemented |
| `20260501000002_transaction_invariants.sql` | same filename | implemented |
| `20260502000001_dataset_branches_v2.sql` | same filename | implemented |
| `20260502120000_dataset_markings.sql` | same filename | implemented |
| `20260503000001_schema_per_view.sql` | same filename | implemented |
| `20260503000002_dataset_files.sql` | same filename | implemented |
| `20260503000004_dataset_health.sql` | same filename | implemented |
| `20260503130000_dataset_foundry_model.sql` | same filename | implemented |
| `20260504000010_branches_unify.sql` | same filename | implemented |
| `20260504000030_branch_retention.sql` | same filename | implemented |
| `20260504000091_parameterized_view.sql` | same filename | implemented |
| Go-only | `20260501120000_dataset_rid_compat.sql` | partial; compatibility shim to review during RID parity. |

## Rust test inventory by migration slice

| Test file | Main coverage area | Go equivalent status |
| --- | --- | --- |
| `smoke.rs` | public health/metrics smoke | partial |
| `dataset_foundry_model.rs`, `cross_service_via_lineage.rs` | dataset model, metadata, permissions, lineage | missing |
| `markings_inheritance.rs`, `branch_markings_snapshot_at_creation.rs` | dataset/branch markings | missing |
| `branches_create_and_reparent.rs`, `branch_lifecycle_full_journey.rs`, `branch_create_from_transaction*.rs`, `branch_event_emitted_on_create_via_outbox.rs` | branch creation/actions/lifecycle/outbox | partial to missing |
| `api_conformance_etag.rs`, `api_conformance_pagination.rs`, `api_conformance_batch_207.rs`, `foundry_semantics_edge_cases.rs` | protocol conformance | partial to missing |
| `branch_ancestry_endpoint.rs`, `branch_fallback_resolution.rs`, `branch_delete_reparents_children_idempotent.rs` | ancestry/fallback/delete planning | missing |
| `branch_compare_detects_conflicts.rs`, `branch_compare_lca_correct.rs` | branch compare/LCA/conflicts | missing |
| `branch_retention_archives_after_ttl.rs`, `branch_retention_inherited_resolves_to_parent.rs`, `retention_aborted_txns.rs` | retention worker/routes | missing |
| `transactions_lifecycle.rs`, `transaction_types_matrix.rs`, `concurrent_transactions.rs`, `branch_open_tx_blocks_new_tx.rs`, `branch_open_tx_allows_child_branch.rs`, `delete_transaction_does_not_purge_storage.rs` | transaction start/get/commit/abort invariants | missing |
| `union_view_aggregates_all_deployment_outputs.rs`, `view_at_timestamp.rs`, `files_endpoint_respects_view_algorithm.rs` | views/current/at/files algorithms | implemented |
| `preview_text_json_lines.rs`, `preview_csv_with_custom_delimiter.rs`, `preview_avro.rs` | preview/data readers | partial (format dispatch and limit envelope ported; full file readers pending) |
| `schema_field_types_roundtrip.rs`, `schema_csv_options_round_trip.rs`, `schema_per_view_versions_independently.rs`, `schema_validation_rejects_invalid.rs` | schema storage/validation | implemented |
| `backing_fs_local_round_trip.rs`, `backing_fs_logical_to_physical_mapping_stable.rs`, `presigned_download_url_audited.rs` | backing filesystem and presign audit | partial/config-gated |

## Prioritized PR / slice plan

1. **Router shell parity with no-op/stub handlers**: mount exact Rust `/v1`, `/api/v1`, public, and internal paths in Go with typed handler names and explicit `501`/stub responses where logic is not migrated. This unblocks route-audit from prefix false negatives.
2. **RID-aware dataset CRUD + model metadata slice**: reconcile `/v1/datasets*`, `/v1/catalog/facets`, model, metadata, markings, permissions, lineage-links, and file-index routes with Go repository methods and copied fixtures.
3. **Foundry branches slice**: port branch list/create/get/delete/action, checkout, reparent, ancestry, fallback, compare, rollback, retention, markings, and restore contracts.
4. **Transactions slice**: port transaction start/get/list/batchGet/action dispatch for `:commit` and `:abort`, including invariants and pagination/conformance tests.
5. **Views/files/schema slice**: port current/at/list/create/action/preview/data/files/schema routes and schema-per-view models.
6. **Backing filesystem and upload slice**: finish LocalBackingFs/Iceberg/backing filesystem abstractions, presigned download/upload-url, storage-details, and multipart upload parity.
7. **Quality/lint/health slice**: port `/api/v1/datasets/*/quality`, lint, and dataset health routes plus quality metrics/models.
8. **Retention worker slice**: add Go worker package and scheduling/config equivalents for branch/transaction retention.
9. **Rust test migration matrix**: port the Rust integration tests listed above as Go tests in the same slice order, keeping route-audit and contract fixtures as required checks.
