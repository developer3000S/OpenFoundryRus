# ADR-0023: Cross-region disaster recovery for the Iceberg lakehouse

- **Status:** Accepted
- **Date:** 2026-05-02
- **Deciders:** OpenFoundry platform architecture group
- **Supersedes / supplements:**
  - The "single region" implicit assumption in
    [docs/architecture/adr/ADR-0008-iceberg-rest-catalog-lakekeeper.md](./ADR-0008-iceberg-rest-catalog-lakekeeper.md).
- **Related ADRs:**
  - [ADR-0008](./ADR-0008-iceberg-rest-catalog-lakekeeper.md) —
    Lakekeeper as the Iceberg REST catalog of record.
  - [ADR-0009](./ADR-0009-internal-query-fabric-datafusion-flightsql.md) —
    DataFusion as the engine of record for Iceberg reads.
  - [ADR-0020](./ADR-0020-cassandra-as-operational-store.md) — Cassandra
    multi-DC topology that defines the "regions" referenced here.
  - [ADR-0024](./ADR-0024-postgres-consolidation.md) — `pg-lakekeeper`
    is one of the 4 consolidated CNPG clusters and is the catalog
    metadata store referenced below.

## Context

The lakehouse is the canonical, WORM, analytical store of OpenFoundry:

- **Object storage:** Iceberg table files (data + metadata) live on
  Rook Ceph S3 (`ceph-objectstore`) in the **primary region**, bucket
  `lakehouse`.
- **Catalog:** Lakekeeper (Iceberg REST) backed by Postgres
  `pg-lakekeeper`. Catalog rows track namespaces, tables, and the
  pointer to the current `metadata.json` for each table.

A region-level failure today (Ceph cluster loss, Postgres cluster loss,
or full DC outage) means the lakehouse is unavailable. RPO is
"whatever the last off-site backup captured" — currently nothing
formal. RTO is "rebuild from cold backup" — hours to days.

The platform's tier-1 SLOs require **RPO ≤ 15 minutes** and
**RTO ≤ 1 hour** for the lakehouse, matching the operational
plane.

We need a cross-region DR design that is:

- Compatible with the Iceberg specification (no proprietary forks of
  the metadata).
- Compatible with Lakekeeper without forking it.
- Cheap to operate and easy to test (a tabletop exercise must produce
  evidence at least quarterly).
- Capable of being promoted to primary without a data rewrite.

## Options considered

### Option A — S3 CRR + Lakekeeper read-replica (chosen)

- The data plane (object bytes) is replicated by **S3 Cross-Region
  Replication** at the bucket level.
  - On Rook Ceph this is `radosgw` zone replication
    (`MultisiteZoneGroup` with one `master` zone and one or more
    `secondary` zones); on AWS it is native S3 CRR; on GCP / Azure
    the equivalent native primitive.
  - Replication is **strict-key, asynchronous, ordered per object**.
    The standard lag is sub-minute under normal load.
- The catalog (`pg-lakekeeper`) is replicated by **Postgres physical
  streaming replication** to a hot-standby cluster in the secondary
  region. CNPG already supports this directly via the
  `replica.source` field referencing a remote primary.
- Lakekeeper itself runs in the secondary region as a **read-only**
  instance pointed at the standby; it serves catalog reads to any DR
  workload that needs them while the primary is healthy.
- Promotion procedure on regional failure:
  1. Failover the Postgres replica (`cnpg promote`).
  2. Promote the Lakekeeper deployment to read-write (config flag).
  3. Update the platform DNS to point at the secondary region.
  4. Resume writes.

Why this works: the **only** integrity invariant Iceberg needs is
that, for every `metadata.json` reachable from the catalog, every file
it references (manifest list, manifests, data files, delete files,
positional deletes, statistics) is present in object storage. Because
the catalog is committed *after* the metadata file is uploaded
(Iceberg's commit protocol), and because S3 CRR replicates objects
in arrival order with ack-on-write semantics on the source, the
secondary region has the same invariant as long as the catalog row
that points at a `metadata.json` is replicated **after** the underlying
object has been replicated.

The rare race is: the Postgres row is replicated before S3 CRR
finishes replicating the object it points at. We close this race with
a **commit-time delay** in the Lakekeeper standby read path: a DR
read of a table waits for the referenced `metadata.json` to be
visible in the secondary bucket, with a configurable timeout. The
window is bounded by S3 CRR lag (sub-minute in practice) and the
operational decision is to accept it as part of RPO accounting.

### Option B — Iceberg snapshot mirror via a custom job (rejected)

- A scheduled job reads each table from the primary, writes a new
  snapshot to a parallel table in the secondary catalog. Each table
  in the secondary lives at a different location and has different
  snapshot ids.
- Pros: fewer moving infrastructure parts.
- Cons:
  - Snapshot ids and file paths diverge — failover is **not
    transparent** to consumers; every reference to a snapshot id
    breaks.
  - Time-travel queries on the secondary cannot reach pre-mirror
    snapshots.
  - Mirror lag is whatever the job cadence is (hours, not minutes).
  - We own and operate the job, including back-pressure, retry,
    parallelism, partial-failure recovery, and the table-level
    consistency contract.
  - Statistics files, positional delete files and partition stats
    must be re-derived in the mirror, a non-trivial code path that
    duplicates Iceberg internals.
- This is the path taken by some legacy Hadoop shops with tools like
  `iceberg-mirror`. The maintenance cost is well documented and is
  exactly the kind of bespoke replication code we eliminated by
  picking Lakekeeper in the first place.

### Option C — Multi-region active-active catalog (rejected for now)

- Two Lakekeeper instances, both writable, with a CRDT or
  consensus-coordinated catalog.
- Iceberg's commit protocol requires a single linearisable point of
  truth per table (atomic compare-and-swap on `metadata_location`).
  Active-active either serialises every write through one region
  (then it is not active-active) or partitions the catalog by table
  ownership (then it is active-passive at the table level). Neither
  buys anything over Option A.
- Lakekeeper does not implement this today and we are not in a
  position to fork it.

### Option D — Off-site cold backups only (rejected)

- `aws s3 sync` / `rclone` snapshots of the bucket plus
  `pg_basebackup` of the catalog, restored on demand.
- RPO is the backup interval (hours), RTO is the restore time
  (hours). Both miss the tier-1 SLO by more than an order of
  magnitude. Suitable as a third tier on top of Option A, not as the
  primary DR mechanism.

## Decision

We adopt **Option A**: **S3 Cross-Region Replication for the
lakehouse bucket** plus a **Lakekeeper read-replica** in the secondary
region backed by a CNPG hot-standby of `pg-lakekeeper`.

Option D (off-site cold backups) is retained as a **third-tier
safety net** against catastrophic loss of both regions or against
logical corruption that propagates through replication.

## Topology

```
       Region A (primary)                            Region B (secondary)

  ┌──────────────────────────┐                ┌──────────────────────────┐
  │ Lakekeeper (RW) × 3      │                │ Lakekeeper (RO) × 2      │
  └──────────┬───────────────┘                └──────────┬───────────────┘
             │                                            │
             │ writes / reads                             │ reads only
             ▼                                            ▼
  ┌──────────────────────────┐    streaming    ┌──────────────────────────┐
  │ pg-lakekeeper (primary)  │  ───────────►   │ pg-lakekeeper (standby)  │
  │  CNPG Cluster            │                 │  CNPG Cluster            │
  └──────────────────────────┘                 └──────────────────────────┘

  ┌──────────────────────────┐    S3 CRR       ┌──────────────────────────┐
  │ Ceph zone "primary"      │  ───────────►   │ Ceph zone "secondary"    │
  │   bucket: lakehouse      │   async, per    │   bucket: lakehouse      │
  └──────────────────────────┘   object        └──────────────────────────┘
```

### Object replication

- **Source of truth:** Rook Ceph `objectstore` "primary" with
  `MultisiteRealm` / `MultisiteZoneGroup` configured.
- **Bucket policy:** every prefix under `lakehouse/` is replicated.
  No selective replication; the lakehouse is one consistent unit.
- **Throttle:** `radosgw` data sync threads sized for 4× peak write
  throughput so that a brief surge does not increase lag past the
  RPO budget.
- **Monitoring:** Prometheus alert on
  `radosgw_sync_lag_seconds > 60` and on
  `radosgw_sync_failed_objects_total > 0`.

### Catalog replication

- `pg-lakekeeper` standby is a CNPG `Cluster` with
  `bootstrap.recovery` from the primary cluster's continuous
  base backup, then `replica.enabled: true` with `replica.source`
  pointing at the primary's WAL endpoint.
- Replication is synchronous **inside** Region A (between the
  primary and a local synchronous standby) and asynchronous to
  Region B. This protects local writes against single-node
  failure while keeping cross-region overhead off the write path.
- Lakekeeper Region B reads from the standby via the standard
  Postgres read URL.

### Promotion runbook

A new runbook `infra/runbooks/lakehouse-dr.md` documents:

1. Confirm the primary region is unrecoverable for the time
   horizon under consideration.
2. Stop all writers in Region A (if reachable).
3. Failover Postgres: `kubectl cnpg promote pg-lakekeeper -n cnpg`
   in Region B.
4. Update Lakekeeper Region B from `--read-only` to
   `--read-write` (Helm value flip + restart).
5. Update the platform's DNS / service mesh to direct
   `lakekeeper.svc.openfoundry.local` to Region B.
6. Run the **post-promotion verification job** that walks every
   table's `metadata_location`, reads it from Region B object
   storage, and asserts every referenced file is present.
7. Re-enable writers.

The runbook is exercised quarterly in a tabletop and at least
once a year in a live game day.

## Configuration knobs

- `lakekeeper.dr.read_metadata_max_wait_ms` (default `60000`) —
  in the secondary region, how long a catalog read will wait for a
  referenced `metadata.json` to appear in the local bucket before
  failing with `503`. Bounds the race window.
- `cnpg.replication.mode` for `pg-lakekeeper`:
  `synchronous` inside Region A, `async` to Region B.
- `radosgw.sync_threads`: sized to keep peak lag < 60 s at 4× normal
  load.

## Operational consequences

- New CNPG `Cluster` manifest for the standby in
  `infra/k8s/platform/manifests/cnpg/clusters/pg-lakekeeper-replica.yaml`.
- New Lakekeeper Helm release in Region B with `readOnly: true`.
- New Ceph `MultisiteRealm` / `MultisiteZoneGroup` /
  `MultisiteZone` CRs in `infra/k8s/platform/manifests/rook-ceph/multisite/`.
- New runbook `infra/runbooks/lakehouse-dr.md`.
- New Prometheus alerts:
  - `radosgw_sync_lag_seconds > 60` (P3, page on > 300 s).
  - `radosgw_sync_failed_objects_total > 0` (P2).
  - `pg_replication_lag_bytes{cluster="pg-lakekeeper"} > 100 MiB`
    (P3).
- New CI check ensuring every Iceberg `metadata.json` written by
  Lakekeeper is followed by a catalog commit (Lakekeeper already
  guarantees this; the check is asserting we do not bypass
  Lakekeeper from any code path).

## Cost consequences

- Egress between Region A and Region B for object replication. On
  managed clouds this is the dominant DR cost line; on on-prem Ceph
  it is bandwidth between sites.
- Standby Postgres cluster cost (one CNPG `Cluster` of the same
  size as the primary minus the synchronous standby pod count).
- Standby Lakekeeper cost (2 pods, idle CPU, modest memory).

## Consequences

### Positive

- Failover is **transparent at the Iceberg semantic level**: snapshot
  ids, file paths and time-travel queries continue to work after
  promotion because the secondary is a byte-identical copy.
- RPO bounded by S3 CRR lag (sub-minute in practice; alert at 60 s).
- RTO bounded by promotion wall-clock (target ≤ 30 minutes for
  Postgres failover + Lakekeeper config flip + DNS propagation).
- No bespoke replication code; the moving parts are operated
  primitives (Ceph multisite, CNPG replication, Helm).

### Negative

- Cross-region object egress cost.
- A small race window exists in which a catalog row in Region B can
  reference a `metadata.json` that has not yet replicated. Closed by
  the read-side wait described above; observed window in practice
  is single-digit seconds.
- Standby capacity is paid for but mostly idle outside DR drills.

### Neutral

- The DR design follows the same "primary region + read-replica"
  shape we use for Cassandra
  ([ADR-0020](./ADR-0020-cassandra-as-operational-store.md)) and for
  the consolidated Postgres clusters
  ([ADR-0024](./ADR-0024-postgres-consolidation.md)). Operationally
  it is one more instance of a pattern operators already know.

## Follow-ups

- Implement migration plan task **S5.x** (Lakehouse DR roll-out:
  Ceph multisite, CNPG standby, Region B Lakekeeper).
- Author `infra/runbooks/lakehouse-dr.md` and run the first
  tabletop exercise within the same release that ships the standby.
- Add the post-promotion verification job as a Helm Job under the
  Region B Lakekeeper release, runnable on demand
  (`helm test lakekeeper-region-b`).
- Re-evaluate this ADR if Lakekeeper publishes first-class
  multi-region support.
