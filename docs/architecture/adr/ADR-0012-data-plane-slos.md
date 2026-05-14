# ADR-0012: Data-plane SLOs, SLIs and error budgets

- **Status:** Accepted
- **Date:** 2026-04-29
- **Deciders:** OpenFoundry platform architecture group
- **Related work:**
  - `docs/architecture/adr/ADR-0007-search-engine-choice.md` (Vespa as the
    sole production search engine).
  - `infra/helm/infra/charts/vespa/`, `infra/helm/infra/manifests/strimzi/`,
    `infra/helm/infra/manifests/flink/` — the Kubernetes packaging of the
    data-plane components covered by this ADR.
  - `infra/docker-compose.monitoring.yml` — Prometheus / Grafana / Loki /
    Tempo stack used to compute the SLIs below.

## Context

The OpenFoundry data plane spans several stateful systems (Flight SQL caches,
Iceberg + DataFusion, Kafka via Strimzi, Vespa, NATS for control
events). Each of them already exposes Prometheus metrics through the
monitoring stack defined in `infra/docker-compose.monitoring.yml` and the
Helm charts under `infra/helm/**`, but there is no single document that:

- states the **target latencies** per layer and per percentile (p50, p99,
  p99.9),
- names the **exact Prometheus metric / query** used to compute each SLI,
- defines the **monthly error budget** and the **freeze policy** when it is
  consumed,
- enumerates the **Grafana dashboards** that must exist to make the SLOs
  observable.

Without this, on-call engineers and capacity planners have to re-derive
acceptable latency from individual runbooks, which produces inconsistent
alert thresholds and makes regression detection unreliable.

## Decision

We adopt the following SLOs, SLIs, error budgets and dashboard inventory for
the data plane. They apply to all production deployments packaged from
`infra/helm/**`. Local Compose stacks
(`infra/compose/docker-compose.yml`, `infra/docker-compose.dev.yml`) are
**explicitly out of scope** — they are sized for DX, not for SLO compliance.

### 1. Latency SLOs per data-plane layer

All latencies are end-to-end at the component boundary unless stated
otherwise. "Hot dataset" means the working set fits in the component's
in-memory cache. "Intra-AZ" means producer and broker share an availability
zone. Measurement window for each percentile is **rolling 30 days**.

| # | Layer / operation | p50 | p99 | p99.9 |
|---|---|---|---|---|
| 1 | **Flight SQL point query** — cache hit, hot dataset | < 3 ms | **< 20 ms** | < 60 ms |
| 2 | **Iceberg scan, 1 GB, single-node DataFusion** | < 400 ms | **< 1.5 s** | < 4 s |
| 3 | **Kafka producer ack** — `acks=all`, intra-AZ | < 5 ms | **< 25 ms** | < 80 ms |
| 4 | **Vespa hybrid query, top-50** (BM25 + vector + filter) | < 15 ms | **< 80 ms** | < 250 ms |
| 5 | **NATS control event end-to-end** (publish → subscriber handler entry) | < 1 ms | **< 5 ms** | < 15 ms |

The **bold p99 targets are the load-bearing SLO**: alerting, error budgets
and freeze decisions are computed against them. p50 and p99.9 targets are
informational guard-rails used to detect "long tail growing" and
"bimodal regression" patterns respectively.

### 2. SLIs — exact Prometheus metrics and queries

Each SLI is expressed as a PromQL query. Histograms are assumed to use the
component's stock buckets. If a component does not yet expose the histogram
named below, exposing it is a prerequisite to claiming SLO compliance for
that layer (tracked as part of the rollout for this ADR).

#### 2.1 Flight SQL point query (cache hit)

- **Histogram:** `flight_sql_query_duration_seconds_bucket`
  (label `cache="hit"`, `query_kind="point"`).
- **p99 SLI:**
  ```promql
  histogram_quantile(
    0.99,
    sum by (le) (
      rate(flight_sql_query_duration_seconds_bucket{cache="hit",query_kind="point"}[5m])
    )
  )
  ```
- **Good/total ratio (for error budget):**
  ```promql
  sum(rate(flight_sql_query_duration_seconds_bucket{cache="hit",query_kind="point",le="0.020"}[30d]))
  /
  sum(rate(flight_sql_query_duration_seconds_count{cache="hit",query_kind="point"}[30d]))
  ```

#### 2.2 Iceberg scan, 1 GB, single-node DataFusion

- **Histogram:** `datafusion_iceberg_scan_duration_seconds_bucket`
  (label `bytes_bucket="1g"`, `nodes="1"`).
- **p99 SLI:**
  ```promql
  histogram_quantile(
    0.99,
    sum by (le) (
      rate(datafusion_iceberg_scan_duration_seconds_bucket{bytes_bucket="1g",nodes="1"}[5m])
    )
  )
  ```
- **Good/total ratio:** same shape as 2.1 with `le="1.5"`.

#### 2.3 Kafka producer ack (`acks=all`, intra-AZ)

- **Histogram (Strimzi / kafka-exporter):**
  `kafka_producer_request_latency_seconds_bucket`
  (label `acks="all"`, `topology="intra_az"`).
- **p99 SLI:**
  ```promql
  histogram_quantile(
    0.99,
    sum by (le) (
      rate(kafka_producer_request_latency_seconds_bucket{acks="all",topology="intra_az"}[5m])
    )
  )
  ```
- **Good/total ratio:** as 2.1 with `le="0.025"`.

#### 2.4 Vespa hybrid query, top-50

- **Histogram:** `vespa_query_latency_seconds_bucket`
  (label `query_profile="hybrid_top50"`).
- **p99 SLI:**
  ```promql
  histogram_quantile(
    0.99,
    sum by (le) (
      rate(vespa_query_latency_seconds_bucket{query_profile="hybrid_top50"}[5m])
    )
  )
  ```
- **Good/total ratio:** as 2.1 with `le="0.080"`.

#### 2.5 NATS control event end-to-end

- **Histogram:** `nats_control_event_e2e_seconds_bucket`
  (exposed by the OpenFoundry control-plane SDK at the subscriber side, with
  the publish timestamp propagated as a header). Label `subject_class="control"`.
- **p99 SLI:**
  ```promql
  histogram_quantile(
    0.99,
    sum by (le) (
      rate(nats_control_event_e2e_seconds_bucket{subject_class="control"}[5m])
    )
  )
  ```
- **Good/total ratio:** as 2.1 with `le="0.005"`.

### 3. Error budgets (monthly) and freeze policy

Each SLO is converted into a monthly error budget computed as
`1 - SLO_target`, where `SLO_target` is the fraction of requests in the
30-day window that must complete under the p99 latency bound listed in
section 1. The default availability target per SLI is **99.5 %** of
requests under the p99 latency, i.e. an error budget of **0.5 % of
requests per 30-day window**.

| Layer | SLO target (under p99 bound) | Monthly error budget |
|---|---|---|
| Flight SQL point query | 99.5 % under 20 ms | 0.5 % of point queries |
| Iceberg scan 1 GB | 99.0 % under 1.5 s | 1.0 % of scans |
| Kafka producer ack | 99.5 % under 25 ms | 0.5 % of produced records |
| Vespa hybrid top-50 | 99.5 % under 80 ms | 0.5 % of hybrid queries |
| NATS control event | 99.9 % under 5 ms | 0.1 % of control events |

Iceberg scans use a relaxed 99.0 % target because long-tail behaviour is
dominated by object-store cold reads, which are partially outside the
platform's control.

#### Freeze policy

The freeze policy is evaluated **per SLI**, not globally:

1. **Burn-rate alert (page):** if the 1-hour burn rate exceeds **14.4×**
   *and* the 6-hour burn rate exceeds **6×** the budget consumption rate,
   an on-call page fires for that layer.
2. **Soft freeze (>= 50 % of monthly budget consumed):** non-urgent
   changes that touch the affected layer (schema migrations, ranking
   profile changes, broker config changes, Vespa application package
   deploys, NATS subject restructuring) require
   an explicit reviewer from the platform team.
3. **Hard freeze (>= 90 % of monthly budget consumed):** all
   non-reliability changes to the affected layer are blocked until the
   end of the rolling 30-day window or until reliability work brings the
   burn rate back under 1× for at least 24 h. Reliability work, rollbacks
   and security fixes are always allowed.
4. **Budget exhausted (100 %):** in addition to the hard-freeze rules,
   the on-call team writes a post-incident note under
   `infra/runbooks/` and the next ADR review cycle must consider
   tightening or loosening the SLO based on observed reality.

The freeze policy is enforced procedurally (review + dashboards), not by
CI gating, to avoid coupling unrelated services to the budget of one
component.

### 4. Grafana dashboards to create

The following dashboards must exist and be provisioned alongside the
existing monitoring stack in `infra/docker-compose.monitoring.yml` and the
Helm releases under `infra/helm/**`. Each dashboard renders the p50 / p99 /
p99.9 panels and the error-budget burn-down for its SLI.

| Dashboard | UID (proposed) | Backing component / chart |
|---|---|---|
| Data Plane SLO Overview | `dp-slo-overview` | aggregates the five SLIs above |
| Flight SQL — point query SLO | `dp-slo-flightsql` | Flight SQL service metrics |
| DataFusion / Iceberg scan SLO | `dp-slo-datafusion` | DataFusion + Iceberg scan metrics |
| Kafka producer ack SLO | `dp-slo-kafka` | `infra/helm/infra/manifests/strimzi/` |
| Vespa hybrid query SLO | `dp-slo-vespa` | `infra/helm/infra/charts/vespa/` |
| NATS control event SLO | `dp-slo-nats` | NATS control plane |

The "Data Plane SLO Overview" dashboard is the single pane used by on-call
to decide whether a freeze is in effect.

## Consequences

### Positive

- One canonical source of truth for data-plane latency targets, replacing
  per-runbook ad-hoc thresholds.
- Each SLI is bound to a concrete Prometheus histogram, which makes
  regressions and missing instrumentation immediately visible (a missing
  histogram = an unmeasurable SLO).
- The freeze policy gives the platform team a predictable lever to slow
  down change velocity on a struggling component without stopping the
  whole org.

### Negative / trade-offs

- Components that do not yet expose the histograms named in section 2
  (notably the NATS end-to-end histogram) need additional instrumentation
  before their SLO can be claimed as met.
- A 30-day rolling window smooths out short incidents; teams must rely on
  the burn-rate alert (1 h / 6 h) for fast feedback.

### Migration / rollout

- Add the histograms listed in section 2 wherever they are missing; this
  is a prerequisite for declaring any SLO "live".
- Provision the six Grafana dashboards listed in section 4 next to the
  existing monitoring stack.
- Link this ADR from `docs/operations/index.md` so on-call engineers can
  find it from the operations entry point.

## Conditions under which this decision would be reopened

This ADR should be revisited if **any** of the following becomes true:

1. A component is replaced (e.g. a different search backend instead of
   Vespa, or a different streaming substrate instead of Kafka),
   invalidating the corresponding histogram and SLI.
2. Production traffic shape changes such that the chosen percentiles
   (p50 / p99 / p99.9) no longer represent user-visible behaviour
   (for example if batch traffic dominates and a different latency bound
   is needed per workload class).
3. The error budget for any layer is exhausted for **two consecutive
   months**, indicating the SLO is unrealistic for the current
   architecture and should be re-negotiated rather than chronically
   missed.

## References

- `infra/helm/infra/charts/vespa/`, `infra/helm/infra/manifests/strimzi/`,
  `infra/helm/infra/manifests/flink/` — data-plane component packaging.
- `infra/docker-compose.monitoring.yml` — Prometheus / Grafana / Loki /
  Tempo stack used to host the dashboards above.
- `docs/operations/index.md` — operations entry point, links to this ADR.
- `docs/architecture/adr/ADR-0007-search-engine-choice.md` — Vespa as the
  sole production search engine, on which SLI 2.4 depends.

---

## Addendum 2026-05 — Ontology hot path on Cassandra (S1.9.c)

The Stream S1 migration plan
(`docs/architecture/migration-plan-cassandra-foundry-parity.md`) added
Apache Cassandra 5.0.2 as the **primary store** for the ontology hot
path (objects, links, actions log, security visibility projection,
revisions). The bench harness defined in
[`benchmarks/ontology/`](../../../benchmarks/ontology/) (S1.8) is the
canonical source for the SLOs below; this addendum records the
**SLOs that S1 commits to** and the SLIs by which compliance is
measured.

### A.1 Latency SLOs — ontology hot path

Workload mix per S1.8.b (80 % read by-id / 15 % read by-type / 5 %
write), measured at the read service boundary on a 3-node Cassandra
5.0.2 cluster (RF=3, LOCAL_QUORUM for `strong` reads, LOCAL_ONE +
moka cache for `eventual` reads).

| # | Operation | p50 | p95 | p99 |
|---|---|---|---|---|
| A1 | `GET /api/v1/ontology/objects/{tenant}/{id}` (read by id, mixed strong/eventual) | < 5 ms | **< 15 ms** | < 35 ms |
| A2 | `GET /api/v1/ontology/objects/{tenant}/by-type/{type_id}?size=50` | < 5 ms | **< 20 ms** | < 50 ms |
| A3 | `POST /api/v1/ontology/actions/{id}/execute` (writeback w/ outbox) | < 20 ms | **< 80 ms** | < 200 ms |
| A4 | Sustained throughput on the mix above | 5 000 RPS sostenidos sin `dropped_iterations` |

The **bold p95 targets** are load-bearing for the freeze policy in
section 3 (an A1/A2 budget exhaustion behaves like the Flight SQL one;
an A3 budget exhaustion behaves like Kafka producer ack).

### A.2 SLIs — Prometheus queries

The read service (`ontology-query-service`, S1.5) and the actions
service (`ontology-actions-service`, S1.4) expose
`http_request_duration_seconds_bucket` with the labels
`{route, method, status, consistency}`. Cache hit rate is tracked via
`ontology_query_cache_hits_total` / `ontology_query_cache_misses_total`
(S1.5.a).

```promql
# A1 — read by id p95.
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket{
      service="ontology-query",
      route="/api/v1/ontology/objects/:tenant/:object_id"
    }[5m])
  )
)

# A1 cache hit ratio (rolling 1 h).
sum(rate(ontology_query_cache_hits_total[1h]))
/
(sum(rate(ontology_query_cache_hits_total[1h]))
 + sum(rate(ontology_query_cache_misses_total[1h])))

# A3 — writeback p95 (idempotent path included).
histogram_quantile(
  0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket{
      service="ontology-actions",
      route="/api/v1/ontology/actions/:id/execute"
    }[5m])
  )
)
```

### A.3 Storage-layer SLIs (Cassandra)

In addition to the application-level SLIs, Cassandra-side health is
tracked via the JMX exporter shipped by the k8ssandra-operator
sidecar (see `infra/helm/infra/manifests/cassandra/cluster-prod.yaml`):

| SLI | Metric | Threshold |
|---|---|---|
| Local read p99 | `org_apache_cassandra_metrics_table_readlatency{quantile="0.99",keyspace="ontology_objects"}` | < 5 ms |
| Local write p99 | `org_apache_cassandra_metrics_table_writelatency{quantile="0.99"}` | < 3 ms |
| Compacted partition max | `org_apache_cassandra_metrics_table_compactedpartitionmaximumbytes` | < 100 MiB |
| Tombstones per slice (avg) | `org_apache_cassandra_metrics_table_tombstonescannedhistogram{quantile="0.99"}` | < 100 |
| `ReadStage` dropped | `org_apache_cassandra_metrics_threadpools_droppedtasks{path="ReadStage"}` | == 0 |

Mantenimiento operativo de estos thresholds vive en el runbook
[`benchmarks/ontology/runbooks/hot-partitions.md`](../../../benchmarks/ontology/runbooks/hot-partitions.md).

### A.4 Evidence register and closure status

**Measurement attempt 2026-05-03:** blocked in the connected
Kubernetes context. The environment contained only a partial local
cluster (`kubectl config current-context` -> `default`), no
`CassandraDatacenter` resources, no Strimzi Kafka resources, no
Prometheus Operator resources, no SparkApplication resources, no
StatefulSets and no S1/S5 runtime dependencies. OpenFoundry identity
and gateway deployments were present but unavailable (`0/1`).

Primary evidence:
[`docs/architecture/slo-evidence/2026-05-03/summary.md`](../slo-evidence/2026-05-03/summary.md).

No exception is approved for ADR-0012. Therefore this ADR records the
mandatory metrics as **open** and does not claim SLO compliance for
S1/S3/S5 operational closure.

Evidence of the local attempt:

| Field | Value |
|---|---|
| Attempt date | 2026-05-03 |
| Commit base | `9d00721` |
| Workspace state | dirty; unrelated and in-progress changes present |
| Kubernetes context | `default` |
| Runtime discovery | no CassandraDatacenter, no Kafka CRDs, no Prometheus CRDs, no SparkApplication CRDs, no StatefulSets |
| OpenFoundry deployments | `authorization-policy-service`, `edge-gateway-service`, `identity-federation-service`, `tenancy-organizations-service` all `0/1`; `web` `1/1` |
| Dataset | not present in this context; the required `OF_BENCH_*` fixture cannot be resolved against live services |
| Load runner | `k6` not installed locally |
| Closure result | BLOCKED; no numeric SLO claim and no approved exception |

Mandatory metric status:

| Metric obligation | Evidence path | Status | Approval state |
|---|---|---|---|
| S1 ontology latency p50/p95/p99 | `benchmarks/results/ontology-mix-summary.json` + rendered ADR snippet | OPEN: blocked by missing Cassandra/services/k6 | no exception approved |
| S1 ontology throughput and dropped iterations | k6 summary from 5,000 RPS run | OPEN: blocked by missing runtime | no exception approved |
| S1 ontology error rate | k6 `http_req_failed` + service counters | OPEN: blocked by missing runtime | no exception approved |
| Kafka lag for S5 sinks | `docs/architecture/lakehouse-evidence/<date>/kafka-offsets.txt` + Grafana/Prometheus | OPEN: existing pack is BLOCKED | no exception approved |
| Temporal latency | Temporal frontend/SDK metrics or visibility timings | OPEN: Temporal runtime absent in current context | no exception approved |
| Iceberg write latency | sink append histograms + Iceberg row-count deltas | OPEN: Lakekeeper/Iceberg/S5 sinks absent | no exception approved |
| Trino query latency | gateway-routed Trino query output | OPEN: Trino/Lakekeeper/sql-bi-gateway runtime absent | no exception approved |
| S3 failover/DR | signed identity failover drill evidence | OPEN: identity stack unavailable and dependencies absent | no exception approved |

The harness remains the canonical path for the first accepted run:

```bash
export OF_BENCH_BASE_URL=https://<gateway-o-read-service>
export OF_BENCH_TOKEN=<jwt-con-scope-read-y-execute>
export OF_BENCH_TENANT=bench-tenant
export OF_BENCH_TYPE_ID=bench-type-T01
export OF_BENCH_OBJECT_IDS=benchmarks/ontology/k6/object-ids.txt
export OF_BENCH_ACTION_ID=<action-type-id>
export OF_BENCH_ENVIRONMENT=<cluster/region/node-shape>
export OF_BENCH_DATASET="50k objects; 10 type_ids; 5k objects/type"

benchmarks/ontology/scripts/run-s1-baseline.sh
```

The command above runs `k6`, writes
`benchmarks/results/ontology-mix-k6.json`,
`benchmarks/results/ontology-mix-summary.json`,
`benchmarks/results/ontology-mix-metadata.json`, and generates
`benchmarks/results/adr-0012-s1-baseline.md`, which contains the table
ready to paste here with real p50/p95/p99 values per operation.

Table to replace after the first accepted run:

| Operation | p50 result | p95 result | p99 result | Run id |
|---|---|---|---|---|
| A1 read by id (strong) | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | no accepted run |
| A1 read by id (eventual, cache hit) | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | no accepted run |
| A2 list by type | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | no accepted run |
| A3 action execute | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | BLOCKED: `EXC-S1-ADR0012-2026-05-03` not approved | no accepted run |
| Throughput (mix) | BLOCKED: no 5,000 RPS run | BLOCKED: no dropped-iteration evidence | BLOCKED: no error-rate evidence | no accepted run |

Every acceptable run must attach, in addition to the k6 JSON files, a
pre/post snapshot of `nodetool tablestats -F json` generated with
`benchmarks/ontology/scripts/capture-cassandra-baseline.sh`. Those
artifacts are the primary evidence; this section only summarizes them.

### A.5 Error budgets

We keep the pattern from section 3: **99.5%** of requests under the
p95 bound = 0.5% of monthly budget. For A4 (sustained throughput) the
budget is binary per run: any run that reports
`dropped_iterations > 0.01%` consumes budget equal to the duration of
the run.

### A.6 Conditions to revisit

In addition to the three general conditions of the original ADR, this
addendum is reopened if:

1. The PK of `objects_by_type` changes (hot-tenant mitigation, S1.8.e),
   because `route` and the partition cardinality change.
2. The moka cache of the read service is removed or replaced by a
   distributed side-cache — the strong / eventual separation is no
   longer observable through local hits/misses.
3. The NATS↔Kafka bridge of the invalidation path (S1.5.b) is replaced
   by a pure Kafka path, which changes the metrics that feed A1 cache
   hit ratio.
