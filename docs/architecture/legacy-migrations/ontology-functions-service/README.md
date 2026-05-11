# `ontology-functions-service` — S1.7 split (no archived migrations)

Runtime for ontology functions (validate, simulate, runs, metrics).
Currently has **no sqlx migrations** — all storage today comes via
`ontology-service`'s legacy schema (tables
`ontology_function_packages`, `function_package_versions`,
`function_package_run_metrics`, archived under
[`legacy-migrations/ontology-definition-service/`](../ontology-definition-service/README.md)).

## S1.7 split

* **Declarative → `pg-schemas.ontology_schema`**:
  - `ontology_function_packages`, `function_package_versions`
    (definitions of function packages and their semver-pinned
    versions; written rarely, read with every dispatch — perfect
    candidate for the consolidated schema cluster).
* **Hot → Cassandra `ontology_indexes` keyspace**:
  - `function_package_run_metrics` → time-series CF
    `function_runs_by_package` keyed by
    `(tenant_id, package_id), bucket_hour, run_id` with TTL aligned
    to the metrics retention policy. Follow-up adds the CQL DDL.
* **Tool dispatch contract**: function packages exposed to the LLM
  agent runtime go through `services/agent-runtime-service`; the
  retired `tool-registry-service` contract was absorbed there in S8.1.b.
  This service stays as the ontology runtime owner.

Substrate-only shell; per-handler migration is a follow-up.
