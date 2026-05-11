# ADR-0024: Postgres consolidation — 71 CNPG clusters down to 4

- **Status:** Accepted
- **Date:** 2026-05-02
- **Deciders:** OpenFoundry platform architecture group
- **Supersedes / supplements:**
  - The 71-cluster topology currently described under
    [infra/k8s/platform/manifests/cnpg/clusters/](../../../infra/k8s/platform/manifests/cnpg/clusters/) and
    in
    [docs/architecture/audit-and-reference-no-spof.md](../audit-and-reference-no-spof.md).
- **Related ADRs:**
  - [ADR-0010](./ADR-0010-cnpg-postgres-operator.md) — CNPG operating
    posture (HA, backups, monitoring). This ADR keeps that posture and
    only changes the cardinality and naming of clusters.
  - [ADR-0020](./ADR-0020-cassandra-as-operational-store.md) — moves
    hot operational state out of Postgres into Cassandra; the four
    remaining clusters host only what genuinely belongs in Postgres.
  - [ADR-0022](./ADR-0022-transactional-outbox-postgres-debezium.md) —
    `pg-policy` is the host of the transactional outbox.
  - [ADR-0023](./ADR-0023-iceberg-cross-region-dr.md) — `pg-lakekeeper`
    has a cross-region standby for DR.
  - [ADR-0027](./ADR-0027-cedar-policy-engine.md) — Cedar policies live
    in `pg-policy.cedar_policies`.

## Context

Today the platform deploys **71 CNPG `Cluster` CRs**, one per service,
under [infra/k8s/platform/manifests/cnpg/clusters/](../../../infra/k8s/platform/manifests/cnpg/clusters/).
Each cluster is HA (3 instances), each one runs continuous backups to
object storage, each one is monitored, each one is alerted on, and
each one runs a connection pooler. The pattern was inherited from the
"one database per service" interpretation of microservices and was
applied uniformly without regard to traffic shape, data volume or
domain boundaries.

The audit in
[docs/architecture/audit-and-reference-no-spof.md](../audit-and-reference-no-spof.md)
quantifies the cost:

- **213 Postgres pods** running continuously to host data that adds
  up to a few GB total, almost entirely declarative configuration and
  metadata.
- **71 backup chains, 71 monitoring scopes, 71 secret rotations,
  71 PITR procedures** for the operations team.
- Most of these clusters are **near-idle** by transaction rate: more
  than half receive fewer than 1 write per second on average; many
  receive fewer than 1 write per minute.
- The "isolation" that one-cluster-per-service was supposed to
  provide is largely illusory: the clusters share the same nodes, the
  same Ceph backend and the same operator. A failure in CNPG itself
  affects all of them simultaneously.

[ADR-0020](./ADR-0020-cassandra-as-operational-store.md) further
reduces what Postgres needs to host, because hot operational state
(objects, links, sessions, action logs, time-series) moves to
Cassandra.

We need to decide the **target cardinality**, the **layout of
schemas**, the **role and access model**, and the **migration path**
from 71 to that target.

## Options considered

### Option A — Keep one cluster per service (status quo, rejected)

- Maintains current operational cost and SPOF surface (one CNPG
  operator failure cascades across 71 clusters).
- After [ADR-0020](./ADR-0020-cassandra-as-operational-store.md) most
  clusters would shrink to a handful of declarative tables, making
  the cost-per-byte ratio even worse.

### Option B — One single cluster for the whole platform (rejected)

- Operationally simplest, but creates a single global blast radius
  for any logical corruption, any noisy-neighbour DDL, any long-
  running migration, and any major-version upgrade.
- Forces unrelated workloads with very different RPO/RTO and very
  different change cadences (the Iceberg catalog vs declarative
  config) to share a maintenance window.

### Option C — Consolidate by **change cadence and blast radius**, into 4 clusters (chosen)

The decision criterion is the **rate of change of the data**, the
**dependency graph for upgrades**, and the **blast radius of a logical
failure**:

- Some data is **declarative** (definitions, schemas, configurations):
  changes infrequently, consumed by many services, no transactional
  side effects.
- Some data is **policy / outbox / tenancy**: low write rate but
  on the critical path for authorisation and event publication;
  needs `wal_level=logical` for Debezium
  ([ADR-0022](./ADR-0022-transactional-outbox-postgres-debezium.md)).
- Some data is the **Iceberg catalog**: managed by Lakekeeper, has
  its own DR posture
  ([ADR-0023](./ADR-0023-iceberg-cross-region-dr.md)), should not
  share an upgrade window with anything else.
- Some data is **runtime configuration / catalog content** for
  marketplace / app-builder / connectors / model registry: declarative
  but evolves more frequently than schemas, low write traffic.

Four clusters cleanly partition these axes and keep blast radius
bounded.

### Option D — Per-domain clusters (8–12) (rejected)

- Splitting "ontology" / "datasets" / "auth" into separate clusters
  re-introduces operational cost without a corresponding benefit:
  these schemas are all declarative and all benefit from being
  upgradeable together. Domain isolation is enforced at the **schema
  and role** level inside Option C; that is sufficient.

## Decision

We adopt **Option C**: **4 CNPG clusters**, named, scoped and laid
out as follows.

### `pg-schemas`

**Purpose:** declarative schemas — the things that define what the
platform knows about, that change at a slow cadence and that are
read by many services.

| Schema             | Owner / writers                       | Contents                                                                                                          |
| ------------------ | -------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `ontology_schema`  | `ontology-management-service`          | Object types, link types, action definitions, branch definitions, marking definitions.                            |
| `dataset_schema`   | `dataset-platform-service`             | Dataset metadata, dataset versions (declarative), schema evolutions.                                              |
| `auth_schema`      | `identity-federation-service`          | OIDC clients, JWKS keys (encrypted at rest, key custody in Vault), SCIM mappings, role definitions.               |
| `app_schema`       | `app-builder-service`, `nexus-service` | App templates, page definitions, widget definitions, navigation trees.                                            |
| `pipeline_schema`  | `pipeline-orchestrator-service`        | Pipeline definitions, transformation graphs, schedule definitions (Temporal owns the runtime; this is the spec).  |

**Sizing:** 3 instances, modest CPU/RAM, modest storage. Hot path is
**read**, dominated by service startup and by occasional editor flows.

### `pg-policy`

**Purpose:** policies, outbox, tenancy, audit metadata. **Critical
path** for authorisation and event publication. Requires logical
decoding for Debezium
([ADR-0022](./ADR-0022-transactional-outbox-postgres-debezium.md)).

| Schema            | Owner / writers                                   | Contents                                                                                                       |
| ----------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `outbox`          | every service that emits events (via `libs/outbox`) | `outbox.events` table; deleted by Debezium after Kafka commit.                                                 |
| `cedar_policies`  | `policy-decision-service` (writer), all services (readers) | Cedar policy documents, versioned; hot-reloaded via NATS event `authz.policy.changed` ([ADR-0027](./ADR-0027-cedar-policy-engine.md)). |
| `audit_metadata`  | `audit-trail-service`                                | Indexable metadata about audit events; the events themselves go to Iceberg.                                    |
| `tenancy`         | `organization-service`, `identity-federation-service` | Organisations, projects, marking inheritance, sharing rules.                                                   |

**Sizing:** 3 instances, more CPU and faster disks than `pg-schemas`
because of the WAL volume from outbox writes and from logical
decoding by Debezium.

**Configuration deltas vs the platform default:**

```yaml
postgresql:
  parameters:
    wal_level: logical
    max_wal_senders: "10"
    max_replication_slots: "8"
    max_logical_replication_workers: "4"
    max_slot_wal_keep_size: "8 GB"
```

### `pg-lakekeeper`

**Purpose:** Iceberg catalog metadata for Lakekeeper
([ADR-0008](./ADR-0008-iceberg-rest-catalog-lakekeeper.md)). Has a
cross-region standby
([ADR-0023](./ADR-0023-iceberg-cross-region-dr.md)).

| Schema       | Owner / writers | Contents                                                                                                          |
| ------------ | ---------------- | ----------------------------------------------------------------------------------------------------------------- |
| `lakekeeper` | Lakekeeper       | Namespaces, tables, current `metadata_location`, snapshot history, transactional commit log. Schema is owned by Lakekeeper migrations and must not be modified by hand. |

**Sizing:** 3 instances locally; cross-region standby per
[ADR-0023](./ADR-0023-iceberg-cross-region-dr.md). Storage grows with
catalog history (table count × snapshot retention), modest write
rate.

**Constraint:** schema migrations are driven exclusively by the
Lakekeeper image upgrade. Application code from OpenFoundry must not
introduce DDL into this cluster.

### `pg-runtime-config`

**Purpose:** runtime configuration and catalog content for
marketplace / app-builder / connectors / model registry / developer
console — declarative-ish content that evolves more frequently than
"schemas" but slower than operational state.

| Schema                   | Owner / writers                  | Contents                                                                                       |
| ------------------------ | -------------------------------- | ---------------------------------------------------------------------------------------------- |
| `marketplace`            | `marketplace-service`            | Public listings, versions, install configurations.                                             |
| `app_builder`            | `app-builder-service`            | User-authored app metadata, drafts, publication state.                                         |
| `connector_definitions`  | `connector-registry-service`     | Source / sink connector definitions, parameters, capability flags.                             |
| `model_registry`         | `model-registry-service`         | Model cards, model versions, lineage pointers, evaluation metadata.                            |
| `developer_console`      | `developer-console-service`      | Console preferences, saved queries, layout state.                                               |

**Sizing:** 3 instances, modest. Read-heavy with bursty writes from
publication flows.

## Schema and role model (applies to all four clusters)

For every cluster:

- **Database** is named after the cluster (`of_schemas`, `of_policy`,
  `of_lakekeeper`, `of_runtime_config`).
- **One schema per logical owner**, named per the table above.
- **One role per schema:**
  - `<schema>_owner` — owns the schema, may DDL. Used by the migration
    runner only.
  - `<schema>_app` — `USAGE` on the schema, `SELECT/INSERT/UPDATE/DELETE`
    on its tables. Used by the owning service.
  - `<schema>_reader` — `USAGE` + `SELECT` only. Used by services that
    cross-read another domain's schema (kept to a minimum, listed
    explicitly per cluster).
- **Cross-schema reads are explicitly enumerated** in the consolidated
  `pg-*-roles.sql` migration; any new cross-schema read requires a PR
  that updates that file.
- **Cross-schema writes are forbidden.** A service writes only to its
  own schema. The outbox (`pg-policy.outbox`) is the channel for
  cross-domain side effects.
- **Migrations** run via `sqlx-cli` from a per-schema migrations
  directory under each owning service repo, executed by an init
  container with the `<schema>_owner` role.

## CNPG configuration baseline (applies to all four clusters)

Inherits [ADR-0010](./ADR-0010-cnpg-postgres-operator.md) and adds:

- **3 instances** per cluster (1 primary, 2 sync standbys when
  capacity allows; otherwise 1 sync, 1 async).
- **PITR** via Barman to Ceph S3, retention 30 days; full backup
  daily, WAL archive continuous.
- **Connection pooling** via PgBouncer in transaction mode, sized per
  cluster (`pg-policy` gets the largest pool because of outbox
  burstiness).
- **Monitoring:** the standard CNPG metrics; alerts on replication
  lag, WAL volume, connection saturation, slot growth (`pg-policy`
  only — see [ADR-0022](./ADR-0022-transactional-outbox-postgres-debezium.md)).
- **Major version:** Postgres 16 across all four; upgrades are
  scheduled per cluster, not globally.

## Migration path (71 → 4)

The migration is organised by source cluster, not by target schema,
to keep blast radius bounded:

1. **Stand up the four target clusters empty**, with the role model
   in place. Confirm Postgres 16 and the configuration deltas.
2. **For each source cluster**, in dependency order (schemas first,
   then policies, then runtime-config, then lakekeeper):
   1. Tag the owning service with `--read-only` (Helm value).
   2. Snapshot the source data (`pg_dump -Fc`).
   3. Restore into the target schema in the target cluster.
   4. Repoint the service's `DATABASE_URL` to the target via Helm.
   5. Roll the service to read-write.
   6. Verify counts and a domain-specific smoke check.
   7. Mark the source cluster as `decommissioned: true` (label) but do
      not delete it for one release cycle.
3. **After one release cycle** with no traffic to the source, delete
   the source `Cluster` CR.
4. **Catalog migration for `pg-lakekeeper`** uses Lakekeeper's own
   migration tool (`lakekeeper migrate`) against the new database
   first, then a stop-the-world cutover (Lakekeeper writes are paused
   for a few minutes while `pg_dump`/`pg_restore` runs).

The platform is pre-production
([ADR-0020](./ADR-0020-cassandra-as-operational-store.md) records
this), so the migration may break interfaces but must not lose data
that the user has authored. The runbook
`infra/runbooks/postgres-consolidation.md` walks through each cluster
and includes rollback steps (Helm value flip back to the source
cluster, valid until step 3 of the same cluster).

## Operational consequences

- 71 `Cluster` CRs in
  [infra/k8s/platform/manifests/cnpg/clusters/](../../../infra/k8s/platform/manifests/cnpg/clusters/) →
  4 (`pg-schemas`, `pg-policy`, `pg-lakekeeper`, `pg-runtime-config`).
- Backup chains: 71 → 4. Each retains the same RPO/RTO posture as
  before; the **operational surface** drops by ~94%.
- Monitoring scopes: 71 → 4 dashboards.
- Secret rotations: 71 → 4 cluster secrets, plus per-schema role
  secrets managed under External Secrets.
- New Helm value `pg.cluster` per service, used to repoint
  `DATABASE_URL`.
- New runbook `infra/runbooks/postgres-consolidation.md`.
- New CI checks:
  - Every `Cluster` CR under `infra/k8s/platform/manifests/cnpg/clusters/` matches one
    of the four allowed names.
  - Every service's `DATABASE_URL` Helm value points at one of those
    four clusters.
  - No service writes to a schema it does not own (enforced via the
    role model in Postgres; the CI check is a sanity assertion that
    the role assigned to the service is `<owned_schema>_app`).

## Consequences

### Positive

- Operational surface drops from 213 Postgres pods to ~12, plus
  PgBouncer.
- Backup, monitoring, alerting, secret rotation and upgrade windows
  all scale by the new cardinality.
- Blast radius is bounded by **change cadence and criticality**,
  not by service ownership: a long-running migration on
  `pg-runtime-config` cannot affect `pg-policy`'s outbox latency,
  and a Lakekeeper schema upgrade cannot affect anything outside
  the catalog.
- Schemas keep domain ownership and isolation through the role model;
  the "one DB per service" intent survives where it matters
  (writability) and is dropped where it costs without paying back
  (HA cost).

### Negative

- A logical corruption in one cluster affects every schema in that
  cluster. Mitigated by per-schema PITR (CNPG supports
  `targetDatabase` / `targetTable` recovery for the owning schema),
  by short backup intervals on `pg-policy`, and by the
  cross-schema-write prohibition that limits how corruption can
  spread.
- Services share connection pool capacity within a cluster.
  Mitigated by sizing pools per cluster with headroom and by
  separating the bursty `pg-policy` cluster from the other three.

### Neutral

- The "one DB per service" microservices orthodoxy is partially
  abandoned in favour of "one DB per change-cadence + blast-radius
  class". This is consistent with the platform's broader pre-production
  pivot to a smaller number of well-operated stateful systems
  ([ADR-0020](./ADR-0020-cassandra-as-operational-store.md)).

## Follow-ups

- Implement migration plan tasks in **S0.6** (consolidated CNPG
  manifests) and the per-cluster migration steps in **S1.x**, **S3.x**,
  **S4.x** that repoint each service.
- Author `infra/k8s/platform/manifests/cnpg/clusters/pg-schemas.yaml`,
  `pg-policy.yaml`, `pg-lakekeeper.yaml`, `pg-runtime-config.yaml`.
- Author the per-cluster `pg-*-roles.sql` migration that creates the
  owner / app / reader roles and grants.
- Author `infra/runbooks/postgres-consolidation.md`.
- Add the CI checks listed under "Operational consequences".
- Once migration is complete, delete every legacy `Cluster` CR under
  `infra/k8s/platform/manifests/cnpg/clusters/` that is not one of the four allowed
  names.
- Do **not** mark the plan milestone "Postgres residual" as closed
  until the final gates `G-S1`, `G-S3`, `G-S5` and `G-S6` in
  [`migration-plan-cassandra-foundry-parity.md`](../migration-plan-cassandra-foundry-parity.md)
  are all green.
