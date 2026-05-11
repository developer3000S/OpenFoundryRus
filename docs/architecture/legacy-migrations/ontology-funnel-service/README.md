# `ontology-funnel-service` — S1.7 split (no archived migrations)

This service ingests datasets and streams into ontology objects
(funnel, sync, indexing, storage insights). It currently has **no
sqlx migrations** — all storage today comes via
`ontology-service`'s legacy schema (tables `ontology_funnel_sources`,
`ontology_funnel_runs`, archived under
[`legacy-migrations/ontology-definition-service/`](../ontology-definition-service/README.md)).

## S1.7 split

* **Declarative → `pg-schemas.ontology_schema`**:
  - `ontology_funnel_sources` (source binding configs).
* **Hot → Cassandra `ontology_indexes` keyspace**:
  - `ontology_funnel_runs` → run-history CF
    `funnel_runs_by_source` keyed by
    `(tenant_id, source_id), started_at DESC, run_id`. Append-only,
    bounded retention. Follow-up adds the CQL DDL.
* **Search projections → `libs/search-abstraction`**:
  - The funnel's index-output documents already flow through the
    search abstraction (ADR-0024); no schema lives here.

The substrate ([`src/lib.rs`](../../../../services/ontology-funnel-service/src/lib.rs))
is shell-only; the per-handler refactor to `Arc<dyn ObjectStore>` and
the run-history CF land in per-PR follow-ups (same pragmatic
deferral as S1.4.b / S1.5.f).
