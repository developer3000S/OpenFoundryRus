# ADR-0030 — Service consolidation to ownership boundaries

| Field | Value |
| --- | --- |
| Status | Accepted, amended 2026-05-03 after live repo audit |
| Date | 2026-05-02 |
| Stream | S8.1 (cleanup & hardening) |
| Supersedes / amends | The 85-service taxonomy in [`microservicios-derivados-desde-foundry-docs.md`](../../archive/microservicios-derivados-desde-foundry-docs.md) and the prompt program in [`prompts-migracion-hasta-85-microservicios.md`](../../archive/prompts-migracion-hasta-85-microservicios.md). |
| Related ADRs | [ADR-0011](ADR-0011-control-vs-data-bus-contract.md), [ADR-0020](ADR-0020-cassandra-as-operational-store.md), [ADR-0024](ADR-0024-postgres-consolidation.md). |

## Context

The 85-bounded-context map produced one Rust crate per documented
domain. Older S8 drafts treated this as a physical reduction target
from 97 crates to no more than 30 binaries. A live audit on
2026-05-03 found **95 directories** under `services/`: the retired
`health-check-service`, `tool-registry-service` and
`widget-registry-service` are gone, while `media-sets-service` is a
real current service and must be tracked. The audit still found this
granularity:

* **inflates Cargo build time** (each service is a workspace member
  with its own deps tree),
* **duplicates infrastructure boilerplate** (DI, telemetry, health
  endpoints repeated across 95 current directories),
* **fragments oncall** (no human can hold context on 95 service
  directories),
* **and offers little operational independence** in practice — most
  services share the same deploy cadence and the same blast radius
  (single Cassandra keyspace, single Postgres pool).

The 85-services map was always an *ownership-clarification* artefact,
not a production runtime topology. ADR-0011 already cuts the system
into a control plane and a data plane; this ADR merges siblings within
each plane that share storage, transactional boundary and SLO into
single deployables.

## Decision

Consolidate ownership and release operations, not the source tree
itself, into **33 ownership boundaries + 3 sinks** organised in **5
Helm releases** (see ADR-0031). The repository may keep more service
directories while a merge is pending; those directories are tracked in
[`service-consolidation-map.md`](../service-consolidation-map.md) as
`merge → X` or `delete`. Each ownership boundary is the runtime owner
of one or more bounded contexts that share:

1. the same primary storage technology and keyspace/database,
2. the same transactional boundary (one DB transaction per request),
3. the same scaling and SLO profile.

Bounded contexts that fit this rule are merged into a parent service
at the **module** level (one Rust module per bounded context inside
the same crate), preserving ownership clarity in the source tree
without the runtime cost of a separate process.

## Target topology (33 ownership boundaries + 3 sinks)

The full mapping lives in
[`docs/architecture/service-consolidation-map.md`](../service-consolidation-map.md);
the headline groupings are:

### Identity & policy plane (3 services)

| Target service | Absorbs |
| --- | --- |
| `identity-federation-service` | `oauth-integration-service`, `session-governance-service` |
| `authorization-policy-service` | `cipher-service`, `network-boundary-service`, `checkpoints-purpose-service`, `security-governance-service` |
| `tenancy-organizations-service` | (unchanged) |

### Audit / governance plane (2 services)

| Target service | Absorbs |
| --- | --- |
| `audit-compliance-service` | `sds-service`, `retention-policy-service`, `lineage-deletion-service` |
| `telemetry-governance-service` | `monitoring-rules-service`, `health-check-service` (S8.1.a), `execution-observability-service` |

### Edge plane (1 service)

| Target service | Absorbs |
| --- | --- |
| `edge-gateway-service` | (unchanged; route fan-in) |

### Data engineering plane (6 services)

| Target service | Absorbs |
| --- | --- |
| `connector-management-service` | `oauth-integration-service` (data side), `virtual-table-service` |
| `ingestion-replication-service` | `cdc-metadata-service`, `event-streaming-service` |
| `dataset-versioning-service` | `data-asset-catalog-service`, `dataset-quality-service` |
| `lineage-service` | `workflow-trace-service` |
| `media-sets-service` | (unchanged; media set transactions and object-store access) |
| `pipeline-build-service` | `pipeline-authoring-service`, `pipeline-schedule-service`, `compute-modules-control-plane-service`, `compute-modules-runtime-service` |

Within this merge boundary, `dataset-versioning-service` is the only
runtime owner of `dataset_versions`, `dataset_branches` and
`dataset_transactions`. `data-asset-catalog-service` may retain
read-side / declarative metadata temporarily, but snapshots and dataset
data state are owned by Iceberg and must not be dual-written into
Postgres runtime tables.

### Ontology plane (4 services + 1 sink)

| Target service | Absorbs |
| --- | --- |
| `ontology-definition-service` | (unchanged; Postgres) |
| `ontology-actions-service` | `ontology-funnel-service`, `ontology-functions-service`, `ontology-security-service` |
| `ontology-query-service` | (unchanged; Cassandra + Vespa cache) |
| `object-database-service` | (unchanged; Cassandra) |
| `ontology-indexer` | (sink; unchanged) |

### Models & ML plane (2 services)

| Target service | Absorbs |
| --- | --- |
| `model-catalog-service` | `model-adapter-service`, `ml-experiments-service`, `model-lifecycle-service` |
| `model-deployment-service` | `model-serving-service`, `model-evaluation-service`, `model-inference-history-service` |

### AIP plane (4 services)

| Target service | Absorbs |
| --- | --- |
| `agent-runtime-service` | `tool-registry-service` (S8.1.b), `conversation-state-service`, `prompt-workflow-service` |
| `llm-catalog-service` | (unchanged) |
| `retrieval-context-service` | `knowledge-index-service`, `document-intelligence-service` |
| `ai-evaluation-service` | `ai-application-generation-service`, `mcp-orchestration-service` |

### Analytics & apps plane (5 services)

| Target service | Absorbs |
| --- | --- |
| `application-composition-service` | `application-curation-service`, `widget-registry-service` (S8.1.b), `developer-console-service`, `custom-endpoints-service`, `managed-workspace-service` |
| `notebook-runtime-service` | `document-reporting-service`, `spreadsheet-computation-service` |
| `sql-bi-gateway-service` | `sql-warehousing-service`, `tabular-analysis-service`, `analytical-logic-service` |
| `ontology-exploratory-analysis-service` | `ontology-timeseries-analytics-service`, `time-series-data-service`, `geospatial-intelligence-service`, `scenario-simulation-service` |
| `solution-design-service` | (unchanged; static knowledge) |

### Workflow plane (2 services)

| Target service | Absorbs |
| --- | --- |
| `workflow-automation-service` | `automation-operations-service`, `approvals-service` |
| `notification-alerting-service` | (unchanged; lives outside Temporal) |

### Federation & marketplace (1 service)

| Target service | Absorbs |
| --- | --- |
| `federation-product-exchange-service` | `marketplace-service`, `marketplace-catalog-service`, `product-distribution-service` |

### Code & SDK (3 services)

| Target service | Absorbs |
| --- | --- |
| `code-repository-review-service` | `global-branch-service`, `code-security-scanning-service` |
| `sdk-generation-service` | (unchanged) |
| `entity-resolution-service` | (unchanged; specialised algorithms) |

### Sinks (3 binaries — kept independent)

`audit-sink`, `ai-sink`, `ontology-indexer` — these are not service
crates with their own ownership boundary, they are **Kafka consumers**
with one job each. They are kept as separate binaries so they can be
scaled, paused and restarted without touching the owning service.
There is no `outbox-relay`: the transactional outbox is relayed by
Debezium Kafka Connect per ADR-0022.

### Result: 33 ownership boundaries + 3 sinks across five Helm releases.

The old physical-service target phrase is superseded. The live metric is:
95 service directories in the repo, 33 target ownership boundaries, and
3 Kafka sink binaries. Physical crate deletion remains an execution
detail for the `merge → X` rows in the consolidation map.

## S8.1.a — Why `health-check-service` goes away

Rolled into `telemetry-governance-service`. Health checks are a
read-side concern of the same observability domain; running them as a
separate service forced inter-service hops on every dashboard refresh
and duplicated the Prometheus scrape target.

## S8.1.b — Why `widget-registry-service` and `tool-registry-service` go away

* `widget-registry-service` → merged into `application-composition-service`.
  Widget catalog and host bridge are useless without the composition
  runtime that loads them; the registry was a 1-table CRUD service
  consumed only by composition.
* `tool-registry-service` → merged into `agent-runtime-service`. Tool
  catalog and dispatch are tightly coupled with the agent loop; the
  registry was a CRUD wrapper over a `tools` table only ever read by
  the runtime.

## Consequences

### Positive

* **Build-time clarity**: ownership boundaries are explicit, and
  future physical crate merges can reduce duplicate dependency trees
  without changing the architecture again.
* **Operational footprint**: five release families replace the old
  one-bucket topology. Pod count only drops when individual `merge → X`
  rows are disabled or physically merged.
* **Helm release surface**: 5 releases (ADR-0031) instead of one
  monolith with per-service release sprawl.
* **Oncall load**: 5 release-aligned rotations, with 33 documented
  ownership boundaries instead of an unbounded per-directory model.

### Negative

* **Larger blast radius per service**: a panic in
  `application-composition-service` now also takes down widget
  registry. Mitigated by:
  * keeping bounded-context modules **internal** so a refactor inside
    one module does not require redeploying others (Rust enforces
    this at the type-system level),
  * preserving the Helm Deployment-per-service model (multiple
    replicas, PDB, HPA),
  * the chaos suite (S8.4) explicitly tests "kill one Pod of `merged service`"
    and validates SLOs hold.
* **Loss of microservice purity**: an architectural constituency may
  argue this is "deconstructing the monolith back into a monolith".
  Counter-argument: ADR-0011 still partitions control vs data, and
  the 33-boundary target is not a single binary — it is the Goldilocks
  point between deploy independence and oncall sustainability.

## Execution

Per-service deletion is sequenced by the existing
[`prompts-migracion-hasta-85-microservicios.md`](../../archive/prompts-migracion-hasta-85-microservicios.md)
program (Phase 9 R-prompts), with this ADR replacing the 85-service
target with the 33-boundary target. Each merger is a separate PR with:

1. The bounded-context Rust module moved into the parent crate.
2. Routes re-registered in the parent's `axum::Router`.
3. The legacy crate removed from `Cargo.toml` workspace members.
4. The legacy Helm sub-chart removed.
5. Smoke tests confirming the parent serves the moved routes.

`docs/architecture/service-consolidation-map.md` tracks per-service
status (pending / in-progress / merged).

## References

* [ADR-0011 — Control vs data bus contract](ADR-0011-control-vs-data-bus-contract.md)
* [ADR-0020 — Cassandra as operational store](ADR-0020-cassandra-as-operational-store.md)
* [ADR-0024 — Postgres consolidation](ADR-0024-postgres-consolidation.md)
* [ADR-0031 — Helm chart split into 5 releases](ADR-0031-helm-chart-split-five-releases.md)
* [docs/architecture/service-consolidation-map.md](../service-consolidation-map.md)
