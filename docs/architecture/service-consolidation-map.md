# Service consolidation map — 99 service directories → 36 ownership boundaries

> Companion to [ADR-0030](adr/ADR-0030-service-consolidation-30-targets.md)
> and [ADR-0042](adr/ADR-0042-service-consolidation-map-reconciliation.md)
> (reconciliation of 4 directories the original map did not enumerate).
> Tracks per-service status of the consolidation work declared in
> Stream S8.1 of the Cassandra/Foundry parity migration plan.
>
> Audit date: 2026-05-05 (S8 workflow-automation + retrieval-context +
> code-repository-review + workflow-trace + event-streaming +
> notebook-runtime + agent-runtime + model-deployment +
> dataset-versioning + pipeline-build + authorization-policy +
> audit-compliance consolidation). The live repository has **61
> directories** under `services/` (`ls services/ | wc -l`). S8 is
> now measured as
> ownership/deployment consolidation, not as physical reduction of the
> source tree to 30 directories. The three retired stubs
> `health-check-service`, `tool-registry-service` and
> `widget-registry-service` are documented below the current map and
> must not appear in Helm or compose runtime surfaces. The model-plane
> consolidation completed on 2026-05-05 retired `ml-experiments-service`,
> `model-adapter-service` and `model-lifecycle-service` into
> `model-catalog-service`. The same day's SQL/BI consolidation retired
> `sql-warehousing-service`, `tabular-analysis-service` and
> `analytical-logic-service` into `sql-bi-gateway-service`; the
> analytical-expressions surface collapsed further into the new
> internal `libs/analytical-logic` crate (no duplicated HTTP routes).
> Also on 2026-05-05, the retrieval consolidation retired
> `knowledge-index-service` and `document-intelligence-service` into
> `retrieval-context-service`, the workflow-automation consolidation
> retired `automation-operations-service` and `approvals-service` into
> `workflow-automation-service`, and the code-repository-review
> consolidation retired `global-branch-service` and
> `code-security-scanning-service` into
> `code-repository-review-service`.
>
> ADR-0030's original "95 dirs → 33 ownership boundaries + 3 sinks" framing
> is amended by ADR-0042 to "99 dirs → 36 ownership boundaries + 3 sinks +
> 1 non-Rust runtime image"; the four newly-enumerated directories
> (`iceberg-catalog-service`, `media-transform-runtime-service`,
> `pipeline-runner`, `reindex-coordinator-service`) were already accepted
> by ADR-0021, ADR-0036, ADR-0039 and ADR-0041 — only the map was stale.

## Status legend

| Symbol | Meaning |
| ------ | ------- |
| `keep` | Stays as a top-level ownership boundary in the current target. |
| `merge → X` | Pending: routes/storage/types still owned by the legacy crate; will be merged into `X`. |
| `merged → X` | Done: legacy crate removed; `X` is the runtime owner. |
| `delete` | Service domain is fully owned elsewhere; legacy crate scheduled for deletion. |
| `sink` | Kafka consumer / relay; counted separately from ownership boundaries. |
| `image` | Non-Rust container image directory (build artifact, not a workspace member); counted separately from ownership boundaries. |

## Map

| Current crate | Target | Status | Notes |
| ------------- | ------ | ------ | ----- |
| `agent-runtime-service` | `agent-runtime-service` | keep | absorbs `tool-registry-service`, `conversation-state-service`, `prompt-workflow-service` |
| `ai-application-generation-service` | `ai-evaluation-service` | merge → `ai-evaluation-service` | both share evaluation rig |
| `ai-evaluation-service` | `ai-evaluation-service` | keep | also absorbs `mcp-orchestration-service` |
| `ai-sink` | `ai-sink` | sink | Kafka → ML inference store |
| `analytical-logic-service` | `sql-bi-gateway-service` | merged → `sql-bi-gateway-service` | S8: directory removed; reusable expressions now live in the internal `libs/analytical-logic` crate (no duplicated HTTP routes). `analytical_expressions` schema folded into `services/sql-bi-gateway-service/migrations/`. |
| `app-builder-service` | (legacy) | delete | already retired in earlier R-prompts; verify Cargo workspace removal |
| `application-composition-service` | `application-composition-service` | keep | absorbs `application-curation-service`, `widget-registry-service` (S8.1.b), `developer-console-service`, `custom-endpoints-service`, `managed-workspace-service` |
| `application-curation-service` | `application-composition-service` | merge → `application-composition-service` | |
| `approvals-service` | `workflow-automation-service` | merged → `workflow-automation-service` | S8: directory removed; `audit_compliance.approval_requests` state machine + `approval.{requested,completed,expired,decided}.v1` outbox + `approvals-timeout-sweep` CronJob binary moved under `services/workflow-automation-service/src/approvals/` and `src/bin/approvals_timeout_sweep.rs`. Helm CronJob template moved from `of-platform` to `of-apps-ops`. |
| `audit-compliance-service` | `audit-compliance-service` | keep | absorbs `sds-service`, `retention-policy-service`, `lineage-deletion-service` |
| `audit-sink` | `audit-sink` | sink | Kafka → Iceberg |
| `authorization-policy-service` | `authorization-policy-service` | keep | absorbs `cipher-service`, `network-boundary-service`, `checkpoints-purpose-service`, `security-governance-service` |
| `automation-operations-service` | `workflow-automation-service` | merged → `workflow-automation-service` | S8: directory removed; saga substrate (`automation_operations` schema, `saga.state` table, `saga.step.requested.v1` consumer with the legacy `automation-operations-service` Kafka group id preserved) moved under `services/workflow-automation-service/src/automation_operations/`. |
| `cdc-metadata-service` | `ingestion-replication-service` | merged → `ingestion-replication-service` | S8-13A: code moved under `services/ingestion-replication-service/src/cdc_metadata/`; migrations kept in `migrations/cdc_metadata/` and still run against `cdc-metadata-pg` via `CDC_METADATA_DATABASE_URL`. |
| `checkpoints-purpose-service` | `authorization-policy-service` | merged → `authorization-policy-service` | S8: directory removed; 9 source files (config, domain, handlers/checkpoints, models) absorbed under `services/authorization-policy-service/src/checkpoints_purpose/`. Migration `20260427030100_checkpoints_purpose_foundation.sql` preserved on `pg-policy`. `CHECKPOINTS_PURPOSE_SERVICE_URL` callers retargeted at `authorization-policy-service:50093`. |
| `cipher-service` | `authorization-policy-service` | merged → `authorization-policy-service` | S8: directory removed; 7 source files (config, domain, handlers/cipher, models) absorbed under `services/authorization-policy-service/src/cipher/`. Migration `20260427050100_cipher_foundation.sql` preserved on `pg-policy`. `CIPHER_SERVICE_URL` callers retargeted at `authorization-policy-service:50093`. The runtime wiring is a follow-up. |
| `code-repository-review-service` | `code-repository-review-service` | keep | absorbs `global-branch-service`, `code-security-scanning-service` |
| `code-security-scanning-service` | `code-repository-review-service` | merged → `code-repository-review-service` | S8: directory removed; handlers/config/models folded into `services/code-repository-review-service/src/code_security.rs`. Migration `20260427070600_22_code_security_scans_foundation.sql` moved to `services/code-repository-review-service/migrations/`. Helm Deployment retired from `of-apps-ops`. |
| `compute-modules-control-plane-service` | `pipeline-build-service` | merged → `pipeline-build-service` | S8: directory removed; the source was a `tools/scaffold_p59_p85.py` placeholder (`fn main() {}` stub, generic CRUD over `compute_modules` / `compute_module_deployments`); migration preserved on `pg-pipeline` for future work. |
| `compute-modules-runtime-service` | `pipeline-build-service` | merged → `pipeline-build-service` | S8: directory removed; same scaffold pattern as the control-plane sibling (generic CRUD over `compute_module_runs` / `compute_module_metrics`); migration preserved on `pg-pipeline`. |
| `connector-management-service` | `connector-management-service` | keep | absorbs `virtual-table-service`, OAuth-data side of `oauth-integration-service` |
| `conversation-state-service` | `agent-runtime-service` | merged → `agent-runtime-service` | S8: directory removed; the source was a substrate-only crate (`fn main() {}` stub plus `domain.rs`/`handlers.rs`/`models.rs` shims that re-exported `libs/ai-kernel`). No migrations to move. Helm Deployment retired from `of-ml-aip`; `CONVERSATION_STATE_SERVICE_URL` callers retargeted at `agent-runtime-service:50127`. |
| `custom-endpoints-service` | `application-composition-service` | merge → `application-composition-service` | |
| `data-asset-catalog-service` | `dataset-versioning-service` | merged → `dataset-versioning-service` | S8: directory removed; 32 source files (config, domain with catalog/file_format/markings/runtime/storage/transactions/validation, handlers with catalog/crud/dataset_model/export/internal/preview/schema_validate/upload/views, models, security, metrics) absorbed under `services/dataset-versioning-service/src/data_asset_catalog/`. 8 source migrations preserved on `pg-schemas`. Edge gateway routing for `/api/v1/datasets` and `/api/v2/filesystem` retargeted at `dataset-versioning-service`. The streaming runtime wiring into the consolidated binary's main is a follow-up. |
| `dataset-quality-service` | `dataset-versioning-service` | merged → `dataset-versioning-service` | S8: directory removed; 19 source files (config, domain with health and quality/{alerts,profiler,rules,scorer}, handlers with health/lint/quality, models, metrics) absorbed under `services/dataset-versioning-service/src/dataset_quality/`. The single `dataset_health` migration preserved. Edge gateway routing for `/api/v1/datasets/.../quality` and `/lint` retargeted at `dataset-versioning-service`. Two integration tests (`health_freshness_sla.rs`, `schema_drift_detected.rs`) NOT moved — they need follow-up wiring of the dataset_quality handlers in target's main. |
| `dataset-versioning-service` | `dataset-versioning-service` | keep | sole runtime owner of `dataset_versions`, `dataset_branches`, `dataset_transactions`; Iceberg owns snapshots/data state |
| `developer-console-service` | `application-composition-service` | merge → `application-composition-service` | |
| `document-intelligence-service` | `retrieval-context-service` | merged → `retrieval-context-service` | S8: directory removed; sketch handlers/models preserved under `services/retrieval-context-service/src/document_intelligence/` and gated behind a new `parsers` Cargo feature so parser pipelines stay out of the default CI compile path. The `document_intelligence_jobs` / `_status_events` / `_extractions` migration is folded into `services/retrieval-context-service/migrations/0001_document_intelligence_foundation.sql`; tables stay on `pg-schemas`. |
| `document-reporting-service` | `notebook-runtime-service` | merged → `notebook-runtime-service` | S8: directory removed; the notepad domain (`domain/notepad.rs`, `handlers/notepad.rs`, `models/notepad.rs`) was already byte-identical between source and target before this merge — the source crate had degenerated to `fn main() {}` plus three `pub mod notepad;` shims. Migration `20260425193000_notepad_documents.sql` moved to `services/notebook-runtime-service/migrations/`. Edge gateway `/api/v1/notepad/*` retargeted at `notebook-runtime-service`. Helm Deployment retired from `of-apps-ops`. |
| `edge-gateway-service` | `edge-gateway-service` | keep | |
| `entity-resolution-service` | `entity-resolution-service` | keep | specialised matching |
| `event-streaming-service` | `ingestion-replication-service` | merged → `ingestion-replication-service` | S8: directory removed; ~100 source files (`backends/`, `domain/`, `grpc/`, `handlers/`, `models/`, `outbox`, `router/`, `runtime/`, `storage/`) absorbed under `services/ingestion-replication-service/src/event_streaming/` preserving the source `AppState`. Cargo features merged: `kafka-rdkafka`, `kafka-it`, `rocksdb-state`, `flink-runtime`. 20 SQL migrations moved to `services/ingestion-replication-service/migrations/streaming/` (schema namespace `event_streaming` on `pg-schemas` preserved). 18 integration tests moved to `services/ingestion-replication-service/tests/`. `proto/streaming/{router,streams}.proto` now compiled with both server and client stubs by the consolidated build.rs. Helm Deployment retired from `of-data-engine`; `EVENT_STREAMING_SERVICE_URL` repointed at `ingestion-replication-service:50121`. The streaming runtime wiring into the consolidated binary's main is a follow-up. |
| `execution-observability-service` | `telemetry-governance-service` | merge → `telemetry-governance-service` | |
| `federation-product-exchange-service` | `federation-product-exchange-service` | keep | absorbs `marketplace-service`, `marketplace-catalog-service`, `product-distribution-service` |
| `geospatial-intelligence-service` | `ontology-exploratory-analysis-service` | merge → `ontology-exploratory-analysis-service` | |
| `global-branch-service` | `code-repository-review-service` | merged → `code-repository-review-service` | S8: directory removed; sources moved under `services/code-repository-review-service/src/global_branch/` (handlers, store, subscriber, model). Migration `20260504000031_global_branches.sql` folded into `services/code-repository-review-service/migrations/`. Helm Deployment retired from `of-apps-ops`. |
| `iceberg-catalog-service` | `iceberg-catalog-service` | keep | Foundry-flavoured Iceberg REST Catalog (ADR-0041); supersedes Lakekeeper for the internal catalog surface, owns Foundry transaction/markings/schema-evolution semantics. |
| `identity-federation-service` | `identity-federation-service` | keep | absorbs `oauth-integration-service` (auth side), `session-governance-service` |
| `ingestion-replication-service` | `ingestion-replication-service` | keep | |
| `knowledge-index-service` | `retrieval-context-service` | merged → `retrieval-context-service` | S8: directory removed; the source crate was a stub re-exporting `libs/ai-kernel` modules, so no Rust code or migrations needed to move — `retrieval-context-service` already re-exports the same kernel modules. |
| `lineage-deletion-service` | `audit-compliance-service` | merged → `audit-compliance-service` | S8: directory removed; 11 source files (config, domain/deletion, handlers/deletion, models, retention runner) absorbed under `services/audit-compliance-service/src/lineage_deletion/`. Migration preserved on `pg-policy`. Edge gateway routing for `/api/v1/lineage-deletions` and `/api/v1/audit/gdpr/erase` retargeted at `audit-compliance-service`. |
| `lineage-service` | `lineage-service` | keep | absorbs `workflow-trace-service` |
| `llm-catalog-service` | `llm-catalog-service` | keep | |
| `managed-workspace-service` | `application-composition-service` | merge → `application-composition-service` | |
| `marketplace-catalog-service` | `federation-product-exchange-service` | merge → `federation-product-exchange-service` | |
| `marketplace-service` | `federation-product-exchange-service` | merge → `federation-product-exchange-service` | |
| `mcp-orchestration-service` | `ai-evaluation-service` | merge → `ai-evaluation-service` | |
| `media-sets-service` | `media-sets-service` | keep | Foundry media sets runtime; owns media set transactions, item metadata and presigned object-store access. |
| `media-transform-runtime-service` | `media-transform-runtime-service` | keep | Sibling runtime to `media-sets-service` per ADR-0039: executes the typed image / audio / video / document / spreadsheet access patterns, bills compute-seconds, emits the `media_set.access_pattern_invoked` audit envelope. Kept as its own ownership boundary so the metadata plane (`media-sets-service`) and the compute plane scale and ship independently. |
| `ml-experiments-service` | `model-catalog-service` | merged → `model-catalog-service` | S8: directory removed; experiments handler now re-exported by `model-catalog-service` from `libs/ml-kernel`. |
| `model-adapter-service` | `model-catalog-service` | merged → `model-catalog-service` | S8: directory removed; `model_adapters` / `inference_contracts` migrations folded into `services/model-catalog-service/migrations/`. No table-name collision with the target's `ml_*` tables. |
| `model-catalog-service` | `model-catalog-service` | keep | sole runtime owner of the model-catalog, model-adapter, model-lifecycle and ML-experiments domains within the `model_catalog` / `model_adapter` / `model_lifecycle` schemas of `pg-schemas` |
| `model-deployment-service` | `model-deployment-service` | keep | absorbs `model-serving-service`, `model-evaluation-service`, `model-inference-history-service` |
| `model-evaluation-service` | `model-deployment-service` | merged → `model-deployment-service` | S8: directory removed; the source was a substrate-only shim over `libs/ml-kernel` (`fn main() {}` stub, `domain/mod.rs` re-exporting `drift`, `handlers/mod.rs` re-exporting `deployments`). Edge gateway routing for `/api/v1/ml/deployments/{id}/drift` retargeted at `model-deployment-service`. |
| `model-inference-history-service` | `model-deployment-service` | merged → `model-deployment-service` | S8: directory removed; the source was a substrate-only shim over `libs/ml-kernel` (re-exported the same `predictions` modules as `model-serving-service`). Edge gateway routing for `/api/v1/ml/batch-predictions` retargeted at `model-deployment-service`. |
| `model-lifecycle-service` | `model-catalog-service` | merged → `model-catalog-service` | S8: directory removed; `modeling_objectives` / `model_submissions` / `model_lifecycle_events` migrations folded into `services/model-catalog-service/migrations/`. No table-name collision with the target's `ml_*` tables. |
| `model-serving-service` | `model-deployment-service` | merged → `model-deployment-service` | S8: directory removed; the source was a substrate-only shim over `libs/ml-kernel` (re-exported `predictions` modules; identical scaffold to `model-inference-history-service`). Edge gateway routing for `/api/v1/ml/deployments/{id}/predict` retargeted at `model-deployment-service`. |
| `monitoring-rules-service` | `telemetry-governance-service` | merge → `telemetry-governance-service` | |
| `network-boundary-service` | `authorization-policy-service` | merged → `authorization-policy-service` | S8: directory removed; 8 source files (config, domain/boundary, handlers/boundary, models) absorbed under `services/authorization-policy-service/src/network_boundary/`. Migration `20260427080100_network_boundary_foundation.sql` preserved on `pg-policy`. `NETWORK_BOUNDARY_SERVICE_URL` callers retargeted at `authorization-policy-service:50093`. |
| `nexus-service` | (legacy) | delete | retire after `tenancy-organizations-service` and `federation-product-exchange-service` confirmed |
| `notebook-runtime-service` | `notebook-runtime-service` | keep | absorbs `document-reporting-service`, `spreadsheet-computation-service` |
| `notification-alerting-service` | `notification-alerting-service` | keep | |
| `oauth-integration-service` | split → `identity-federation-service` (auth) + `connector-management-service` (data OAuth) | merge | |
| `object-database-service` | `object-database-service` | keep | |
| `ontology-actions-service` | `ontology-actions-service` | keep | sole runtime owner of the ontology action / funnel / function / rule HTTP surfaces; absorbed `ontology-funnel-service`, `ontology-functions-service`, `ontology-security-service` (S8.1) |
| `ontology-definition-service` | `ontology-definition-service` | keep | |
| `ontology-exploratory-analysis-service` | `ontology-exploratory-analysis-service` | keep | absorbs `ontology-timeseries-analytics-service`, `time-series-data-service`, `geospatial-intelligence-service`, `scenario-simulation-service` |
| `ontology-functions-service` | `ontology-actions-service` | merged → `ontology-actions-service` | crate removed (S8.1) |
| `ontology-funnel-service` | `ontology-actions-service` | merged → `ontology-actions-service` | crate removed (S8.1) |
| `ontology-indexer` | `ontology-indexer` | sink | |
| `ontology-query-service` | `ontology-query-service` | keep | |
| `ontology-security-service` | `ontology-actions-service` | merged → `ontology-actions-service` | crate removed (S8.1) |
| `ontology-timeseries-analytics-service` | `ontology-exploratory-analysis-service` | merge → `ontology-exploratory-analysis-service` | |
| `pipeline-authoring-service` | `pipeline-build-service` | merged → `pipeline-build-service` | S8: directory removed; 46 source files (config, domain with engine/executor/expressions/media_nodes/parameterized, handlers, models, plus the source's `lib.rs` re-rooting media-node + expression validators via `#[path]`) absorbed under `services/pipeline-build-service/src/pipeline_authoring/`. 6 source migrations preserved on `pg-pipeline`. Edge gateway routing for `/api/v1/pipelines` retargeted at `pipeline-build-service`. The pipeline-authoring runtime wiring into the consolidated binary's main is a follow-up. |
| `pipeline-build-service` | `pipeline-build-service` | keep | absorbs authoring, schedule, compute modules |
| `pipeline-runner` | `pipeline-runner` | image | Scala/SBT project (FASE 3 / Tarea 3.3) that builds the Spark/Iceberg image referenced by SparkApplication CRs launched by `pipeline-build-service`. **Not** a Rust workspace member, no service binary, no Helm Deployment of its own — it is a build artifact. Listed in `tools/regenerate_service_dockerfiles.py`'s `NON_RUST_SERVICES` skip set. |
| `pipeline-schedule-service` | `pipeline-build-service` | merged → `pipeline-build-service` | S8: directory removed; 51 source files (config, domain with aip/build_client/cron_dispatch/cron_registrar/dispatcher/event_listener/run_store/schedule_store/trigger/troubleshoot, handlers, models) absorbed under `services/pipeline-build-service/src/pipeline_schedule/`. 4 source migrations preserved on `pg-pipeline`. The legacy filesystem-path includes that pipeline-schedule used to reach pipeline-authoring/lineage-service/workflow-automation-service rewritten to match the new layout. Edge gateway routing for `/api/v1/pipelines/triggers/cron/*` retargeted at `pipeline-build-service`. |
| `product-distribution-service` | `federation-product-exchange-service` | merge → `federation-product-exchange-service` | |
| `prompt-workflow-service` | `agent-runtime-service` | merged → `agent-runtime-service` | S8: directory removed; the source was a substrate-only crate (`fn main() {}` stub, `lib.rs`, `domain.rs`/`handlers.rs`/`models.rs` shims over `libs/ai-kernel`, plus a producer-specific `ai_events.rs` mirror that has been retired in favour of agent-runtime's own — both producers now share the `agent-runtime-` Kafka transactional-id prefix). Helm Deployment retired from `of-ml-aip`; Strimzi KafkaUser + transactional-id ACL for `prompt-workflow-` retired; `PROMPT_WORKFLOW_SERVICE_URL` callers retargeted at `agent-runtime-service:50127`. |
| `reindex-coordinator-service` | `reindex-coordinator-service` | keep | Rust replacement (FASE 4 / Tarea 4.2) for the Go `workers-go/reindex` Temporal worker (ADR-0021). Owns the resume cursor in `pg-runtime-config.reindex_jobs`, drives Cassandra page-by-page scans via `cassandra-kernel`, and fans batches out to `services/ontology-indexer` over `ontology.reindex.v1`. Distinct ownership boundary (Postgres state + Temporal-replacement semantics) from the downstream `ontology-indexer` sink. |
| `report-service` | (legacy) | delete | already covered by `document-reporting-service` |
| `retention-policy-service` | `audit-compliance-service` | merged → `audit-compliance-service` | S8: directory removed; 14 source files (config, domain/retention, handlers/retention, models, metrics, retention runner) absorbed under `services/audit-compliance-service/src/retention_policy/`. 2 source migrations + 3 tests preserved on `pg-policy`. Edge gateway routing for `/api/v1/retention/*` and `/applicable-policies` / `/retention-preview` retargeted at `audit-compliance-service`. |
| `retrieval-context-service` | `retrieval-context-service` | keep | absorbs `knowledge-index-service`, `document-intelligence-service` |
| `scenario-simulation-service` | `ontology-exploratory-analysis-service` | merge → `ontology-exploratory-analysis-service` | |
| `sdk-generation-service` | `sdk-generation-service` | keep | |
| `sds-service` | `audit-compliance-service` | merged → `audit-compliance-service` | S8: directory removed; 8 source files (config, domain/sds, handlers/sds, models) absorbed under `services/audit-compliance-service/src/sds/`. Migration preserved on `pg-policy`. Edge gateway routing for `/api/v1/audit/sds` retargeted at `audit-compliance-service`. |
| `security-governance-service` | `authorization-policy-service` | merged → `authorization-policy-service` | S8: directory removed; 12 source files (config, domain, handlers/governance, models) absorbed under `services/authorization-policy-service/src/security_governance/`. Migration `20260427020100_security_governance_foundation.sql` preserved on `pg-policy`. The namespace's `AppState` carries `audit_db` and `policy_db` fields used by the governance reports + template handlers. `SECURITY_GOVERNANCE_SERVICE_URL` callers retargeted at `authorization-policy-service:50093`. |
| `session-governance-service` | `identity-federation-service` | merge → `identity-federation-service` | |
| `solution-design-service` | `solution-design-service` | keep | |
| `spreadsheet-computation-service` | `notebook-runtime-service` | merged → `notebook-runtime-service` | S8: directory removed; source was a `tools/scaffold_p59_p85.py` placeholder (`fn main() {}` stub, generic CRUD over `spreadsheet_sheets` / `spreadsheet_recalcs` with JSONB payloads, no production callers of `/api/v1/spreadsheets/*`). Migration `20260427070600_03_spreadsheet_sheets_foundation.sql` moved to `services/notebook-runtime-service/migrations/` so the schema remains on `notebook-pg`. Helm Deployment retired from `of-apps-ops`. |
| `sql-bi-gateway-service` | `sql-bi-gateway-service` | keep | absorbs warehousing, tabular, analytical-logic |
| `sql-warehousing-service` | `sql-bi-gateway-service` | merged → `sql-bi-gateway-service` | S8: directory removed; `warehouse_jobs` / `warehouse_transformations` / `warehouse_storage_artifacts` schemas folded into `services/sql-bi-gateway-service/migrations/`; CRUD served at `/api/v1/warehouse/*`. |
| `tabular-analysis-service` | `sql-bi-gateway-service` | merged → `sql-bi-gateway-service` | S8: directory removed; `tabular_analysis_jobs` / `tabular_analysis_results` schemas folded into `services/sql-bi-gateway-service/migrations/`; CRUD served at `/api/v1/tabular/*`. |
| `telemetry-governance-service` | `telemetry-governance-service` | keep | absorbs monitoring rules, health checks, execution observability |
| `tenancy-organizations-service` | `tenancy-organizations-service` | keep | |
| `time-series-data-service` | `ontology-exploratory-analysis-service` | merge → `ontology-exploratory-analysis-service` | |
| `virtual-table-service` | `connector-management-service` | merge → `connector-management-service` | |
| `workflow-automation-service` | `workflow-automation-service` | keep | absorbs automation-operations, approvals |
| `workflow-trace-service` | `lineage-service` | merged → `lineage-service` | S8: directory removed; source was a `tools/scaffold_p59_p85.py` placeholder (`fn main() {}` stub, generic CRUD handlers, no production callers of `/api/v1/workflow-traces/*`). Migration `20260427070600_07_workflow_trace_runs_foundation.sql` moved to `services/lineage-service/migrations/` so the `workflow_trace_runs` / `workflow_trace_events` schemas remain on `lineage-pg`. Helm Deployment retired from `of-apps-ops`. |

## Retired service directories

These services were present in older S8 planning artifacts but are no longer
directories under `services/` and must not be rendered by Helm or compose:

| Retired service | Runtime owner | Closure |
| --------------- | ------------- | ------- |
| `health-check-service` | `telemetry-governance-service` | S8.1.a / S8.6.c |
| `tool-registry-service` | `agent-runtime-service` | S8.1.b |
| `widget-registry-service` | `application-composition-service` | S8.1.b / S8.6.b |

## Summary by status

| Status | Count |
| ------ | ----- |
| keep / ownership boundary | 36 |
| merge → X (pending) | 18 |
| merged → X (completed) | 38 |
| delete scheduled for active legacy dirs | 3 |
| sink | 3 |
| image (non-Rust runtime image) | 1 |
| **Total current service directories** | **61** |
| **Retired service directories tracked for references** | **3** |
| **Current target metric** | **36 ownership boundaries + 3 sinks + 1 non-Rust runtime image across 5 Helm releases** |

## Execution sequence

Each merge is its own PR. The recommended ordering minimises rebase
churn:

1. **Sinks-and-leaves first** — services with zero downstream
   consumers drain to their parents. The three S8.1 retired stubs listed
   above have already drained and must stay absent from runtime manifests.
2. **Same-keyspace clusters** next — merges where source and target
   already share a Cassandra keyspace or Postgres schema (no data
   migration), e.g. ontology-actions absorbing
   ontology-functions/funnel/security.
3. **Cross-store merges** last — those that require schema work
   (e.g. consolidating audit-compliance with sds + retention +
   lineage-deletion all in `pg-policy`).
4. **Legacy macroservices** stay last (R-prompts in
   [`prompts-migracion-hasta-85-microservicios.md`](../../prompts-migracion-hasta-85-microservicios.md)).
