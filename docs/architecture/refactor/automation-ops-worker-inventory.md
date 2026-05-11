# `automation-ops` worker functional inventory

> **Migration context.** FASE 6 / Tarea 6.1 of
> [`docs/architecture/migration-plan-foundry-pattern-orchestration.md`](../migration-plan-foundry-pattern-orchestration.md).
> The plan describes this worker as the saga substrate for
> "automation-operations": cleanup, retention, dependency resolution
> and compensations. This inventory is the **read-only baseline**:
> the worker, its single workflow, its single activity, and — most
> importantly — the gap between the plan's saga ambitions and what
> the codebase actually expresses today. **No code is being moved
> here**; the migration itself lands in subsequent tasks (6.2–6.5).

---

## 1. Worker process topology

| Field | Value | Source |
|---|---|---|
| Worker binary | `workers-go/automation-ops/main.go` | [main.go](../../../workers-go/automation-ops/main.go) |
| Temporal task queue | `openfoundry.automation-ops` (constant `contract.TaskQueue`) | [contract.go:4](../../../workers-go/automation-ops/internal/contract/contract.go) |
| Registered workflows | **`AutomationOpsTask`** (only one — registered without `RegisterWorkflowWithOptions`, so the runtime name is the Go function symbol; the constant is exposed by the contract for cross-language use) | [main.go:38](../../../workers-go/automation-ops/main.go) |
| Registered activities | `(*Activities).ExecuteTask` (only one) | [activities.go:83](../../../workers-go/automation-ops/activities/activities.go) |
| Health / metrics | `:9090/healthz`, `:9090/metrics` (placeholder; mirrors the other three workers) | [main.go:54-66](../../../workers-go/automation-ops/main.go) |
| Cross-language contract | Mirrors `libs/temporal-client::AutomationOpsInput` and `task_queues::AUTOMATION_OPS` / `workflow_types::AUTOMATION_OPS_TASK` | [contract.go:4-14](../../../workers-go/automation-ops/internal/contract/contract.go), [temporal-client lib.rs:1630,1640,1996](../../../libs/temporal-client/src/lib.rs) |
| Canonical workflow id | `automation-ops:{task_id}` | [temporal-client lib.rs:1984](../../../libs/temporal-client/src/lib.rs) |

Configuration env (worker → Rust services):

| Env var | Default | Purpose |
|---|---|---|
| `TEMPORAL_ADDRESS` / `TEMPORAL_HOST_PORT` | `127.0.0.1:7233` | Temporal frontend gRPC |
| `TEMPORAL_NAMESPACE` | `default` | Namespace |
| `TEMPORAL_TASK_QUEUE` | `openfoundry.automation-ops` | Task queue polled |
| `OF_AUTOMATION_OPS_URL` (or `AUTOMATION_OPERATIONS_SERVICE_URL`, legacy `OF_AUTOMATION_OPS_GRPC_ADDR`) | `automation-operations-service:50116` | `ExecuteTask` HTTP base |
| `OF_AUTOMATION_OPS_BEARER_TOKEN` (or `AUTOMATION_OPS_BEARER_TOKEN`) | _(empty)_ | Service bearer token |
| `OF_LOG_LEVEL` | `info` | slog level |
| `METRICS_ADDR` | `:9090` | Prometheus exporter address |

---

## 2. Workflow inventory

There is exactly **one workflow type** registered on this task queue.

### `AutomationOpsTask` — single-activity task recorder

Source: [`workflows/automation_ops_task.go`](../../../workers-go/automation-ops/workflows/automation_ops_task.go)

```text
AutomationOpsTask(input) → result
   │
   ├── ActivityOptions{ StartToClose=5m, Retry: init=10s, max=2m, attempts=5 }
   │
   └── ExecuteActivity(ExecuteTask, input) → RunResult{task_id, run_id?, status, run?}
       └── On error: return AutomationOpsResult{status="failed", error}
       └── On success: return AutomationOpsResult{status=run.status, run_id=run.run_id}
```

| Field | Value |
|---|---|
| Inputs (`AutomationOpsInput`) | `task_id`, `tenant_id`, `task_type`, `payload` (free-form `map[string]any`) |
| Outputs (`AutomationOpsResult`) | `task_id`, `status` (verbatim from upstream `RunResult.Status`, `"failed"` on activity error), `run_id?`, `error?` |
| Search attributes | `audit_correlation_id` (= a fresh UUIDv7 generated in the Rust adapter when the caller does not supply one — see [`temporal_adapter.rs:64-67`](../../../services/automation-operations-service/src/domain/temporal_adapter.rs)) |
| Signals | **None.** No `workflow.GetSignalChannel` calls. |
| Queries | **None.** No `workflow.SetQueryHandler` calls. |
| Child workflows | **None.** Single activity, single result. |
| Compensation | **None.** Failure is reported via the result; **no rollback step exists in code today** despite the migration plan describing this worker as the saga / compensation substrate. |
| Determinism hazards | None observed (no `time.Now`, `rand`, native goroutines in workflow code). |

The single most important fact of this inventory is the **scope
mismatch between the plan and the code**. The plan (FASE 6 §6.2)
talks about `saga.step.requested.v1` / `saga.step.completed.v1` /
`saga.step.failed.v1` / `saga.compensate.v1` events, a
`saga_state` table for audit + dedup, multi-step orchestration with
LIFO compensation, etc. The Go worker today is **a thin wrapper that
records a single HTTP POST** and ships back its status. The unit
test confirms it (`TestAutomationOpsTaskRecordsRun`,
[automation_ops_task_test.go](../../../workers-go/automation-ops/workflows/automation_ops_task_test.go)).

This is the same shape we found for `workflow-automation-worker` in
FASE 5 / Tarea 5.1: **the saga complexity does not exist in code; it
is an aspirational shape carried forward from the legacy in-process
control plane**.

---

## 3. Activity inventory

There is exactly **one activity** registered on this task queue.

### 3.1 `ExecuteTask` → `automation-operations-service`

Source: [`activities/activities.go:83-101`](../../../workers-go/automation-ops/activities/activities.go)

| Aspect | Detail |
|---|---|
| Target service | `automation-operations-service` (`OF_AUTOMATION_OPS_URL`, default `automation-operations-service:50116`) |
| Endpoint called | `POST /api/v1/automations/{task_id}/runs` |
| Headers | `authorization: Bearer …` (optional), `x-audit-correlation-id: <task_id>`, `content-type: application/json` |
| Input | `AutomationOpsInput{task_id, tenant_id, task_type, payload}` |
| Request body | `{payload:{task_id, task_type, tenant_id, input, audit_correlation_id, worker:"automation-ops"}}` |
| Output | `RunResult{task_id, run_id?, status:"completed", run: <decoded JSON body>}` (status is hard-coded `"completed"` on 2xx; the upstream's status field is buried inside `run`) |
| Failure modes | `nonRetryable("invalid_automation_ops_input"` / `"automation_ops_config"` / `"automation_ops_request"` / `"automation_ops_client_error")` for missing fields and 4xx (except 429); transient retry for 5xx and 429 |
| Side effects | **None on the worker side.** The downstream Rust handler `create_secondary` ([handlers.rs:85-106](../../../services/automation-operations-service/src/handlers.rs)) is itself a stub — it returns a synthesized `WorkflowRun` JSON without writing to Postgres (the legacy `automation_queue_runs` table is dropped per migration `20260503100000_drop_automation_queues.sql`). |

There are **no other activities** registered on this task queue.
Anything the migration plan envisions for retention sweeps, cleanup
fan-out, dependency resolution, etc. is unimplemented.

---

## 4. How `AutomationOpsTask` is triggered

Producers in code today:

### 4.1 The Rust adapter (only producer in tree)

Path: external HTTP caller → `automation-operations-service::POST
/api/v1/automations` ([handlers.rs:26-58](../../../services/automation-operations-service/src/handlers.rs))
→ `AutomationOpsAdapter::enqueue` ([temporal_adapter.rs:64-68](../../../services/automation-operations-service/src/domain/temporal_adapter.rs))
→ Temporal start workflow on task queue `openfoundry.automation-ops`
→ `AutomationOpsTask(task_id, tenant_id, task_type, payload)`.

The handler accepts a free-form JSON body and extracts:

* `task_id` — caller-supplied UUID, or a fresh UUIDv7 if absent.
* `tenant_id` — caller-supplied string, or `"default"`.
* `task_type` — **REQUIRED**, free-form string (no enum, no schema).
* `audit_correlation_id` — caller-supplied UUID, or a fresh UUIDv7.
* `payload` — the entire inbound JSON body, verbatim.

There is **no other producer in tree** that starts this workflow.
A `grep` for `AUTOMATION_OPS_TASK` / `AutomationOpsClient::start_task`
across `services/`, `libs/` and `workers-go/` returns only the
adapter and its tests.

### 4.2 The HTTP read-side handlers

Routes registered in [`main.rs:42-47`](../../../services/automation-operations-service/src/main.rs):

| Route | Status today |
|---|---|
| `GET /api/v1/automations` | Returns `{"data": [], "source": "temporal"}` — non-authoritative stub. |
| `POST /api/v1/automations` | The producer path described in §4.1. |
| `GET /api/v1/automations/:id` | Returns `{"id": id, "source": "temporal", "temporal":{"workflow_id":"automation-ops:{id}", "authoritative": true}}` — non-authoritative stub. |
| `GET /api/v1/automations/:parent_id/runs` | Returns `{"data": [], "source": "temporal"}` — non-authoritative stub. |
| `POST /api/v1/automations/:parent_id/runs` | The endpoint **the worker activity calls back into**. Returns a synthesised JSON `{id, parent_id, payload, ...}` without persisting anything. |

So the round-trip is: external caller → handler → Temporal workflow
→ activity → handler → response. Neither side persists; the only
durability is Temporal's own event history.

---

## 5. Catalog of `task_type` values (the saga "step types")

The whole point of FASE 6 is to map task types to saga step graphs.
**There is no enum, no registry, no schema for task_type in code.**
The only concrete value present in the entire repository is
`"retention.sweep"`, used as a test fixture in three places:

* [`temporal_adapter.rs:178,219`](../../../services/automation-operations-service/src/domain/temporal_adapter.rs)
* [`handlers.rs:292,299,314,342`](../../../services/automation-operations-service/src/handlers.rs)

The test for the activity itself uses `"retention_sweep"` (no dot)
[`activities_test.go:54,67`](../../../workers-go/automation-ops/activities/activities_test.go)
— suggesting the dot-vs-underscore form is also unenforced.

The migration plan §6.1 calls out a notional catalog ("cleanup,
retention, etc.") but nothing in code actually issues anything other
than ad-hoc test calls. Two implications for FASE 6:

* The "saga step graph per task type" mapping in Tarea 6.2 / 6.3 has
  **no production data to migrate**. The graphs are designed
  forward; they do not need to mirror anything historical.
* A discoverable catalog must be introduced as part of FASE 6 (or
  FASE 9 service consolidation): the new substrate needs to know
  what step graph to dispatch when it sees `task_type = "X"`. Today
  the worker's response to *any* task_type is "POST it back to the
  service and report whatever the service says".

---

## 6. Where state lives today

| Surface | Today |
|---|---|
| Workflow definition catalog | **None.** `task_type` is an opaque string with no backing table. |
| Per-task run history | **None.** The `automation_queues` and `automation_queue_runs` tables were dropped (migration [`20260503100000_drop_automation_queues.sql`](../../../services/automation-operations-service/migrations/20260503100000_drop_automation_queues.sql)). The `migrations/README.md` confirms: "No active SQL migrations remain in this directory." |
| Authoritative live state | Temporal workflow event history + visibility. |
| Read-side projection | **None implemented.** `list_items` / `list_secondary` return empty arrays with a `"source": "temporal"` note. |
| Audit trail | None at the service boundary; Temporal stamps `audit_correlation_id` as a search attribute, but there is no projector to `audit-compliance-service`. |

So the live truth is in Temporal and only there. **A FASE 6
migration that retires Temporal must establish the entire state
plane from scratch** — there is no `workflow_runs` / `processed_events`
table to extend (in contrast to FASE 5, where the
`workflow_run_projections` schema at least existed).

The legacy archive
[`docs/architecture/legacy-migrations/automation-operations-service/`](../legacy-migrations/automation-operations-service/README.md)
documents what *used to* live in Postgres before the Temporal
cutover (S2.7 of the Cassandra-parity plan): a generic
`automation_queues(id, payload jsonb, created_at, updated_at)` plus
`automation_queue_runs(parent_id, payload jsonb, created_at)`. No
status enum, no compensation columns, no dependencies. The legacy
shape is **not a saga state machine** either — it was just an
op-log.

---

## 7. Migration table — Temporal pieces → Saga choreography targets

Aligned with FASE 6 of the migration plan and ADR-0022 (outbox) +
ADR-0037 (Foundry-pattern orchestration). Each row is a discrete
piece of behaviour observable in `workers-go/automation-ops/` (or in
the adjacent Rust adapter that funnels into it) today; the right-
hand column is the post-migration owner.

| # | Today (Temporal) | Tomorrow (Saga choreography) | Notes / target topic |
|---|---|---|---|
| 1 | `AutomationOpsTask` workflow registered on `openfoundry.automation-ops` | **Removed.** Each `task_type` becomes a sequence of `libs/saga::SagaStep` impls (or a single-step saga, for the trivial `task_type`s) driven by a Kafka consumer in `services/automation-operations-service`. | `libs/saga` already exists (Tarea 1.2) with the runner + the `saga.state` schema. |
| 2 | `ExecuteTask` activity → `POST /automations/{id}/runs` | **Removed.** No more self-call; per-`task_type` step bodies call the appropriate domain services directly (e.g. `audit-compliance-service` for retention sweeps, `dataset-versioning-service` for cleanup). | Effect calls vary per step. |
| 3 | Single-step DAG inside the workflow | **Multi-step saga per `task_type`** — the Tarea 6.3 deliverable. Single-step task_types still go through `SagaRunner` so the audit + idempotency story is uniform. | Even single-step usage benefits from the outbox + step events. |
| 4 | Activity retries (`InitialInterval=10s`, backoff 2.0, `Max=2m`, `MaximumAttempts=5`) | Per-step retry policy on the consumer side, written explicitly with `tokio::time::sleep` envelopes. The 4xx-non-retryable rule from the legacy activity carries over. | Same envelope, different substrate. |
| 5 | `StartToCloseTimeout=5m` per activity | Per-step deadline tracked by the `saga.state.current_step` column + a timeout sweep CronJob. | Mirror of FASE 7 / Tarea 7.4. |
| 6 | The Rust adapter `enqueue` → Temporal start | Same adapter API; instead of starting a Temporal workflow it (a) inserts a `saga.state` row via `SagaRunner::start`, (b) enqueues `saga.step.requested.v1` outbox event for the first step, (c) returns 202. | Change is internal to the adapter. |
| 7 | `task_type` is a free-form string the worker ignores | `task_type` becomes the **dispatch key** the saga registry uses to pick the step graph. Introduce a typed enum (or registry trait) on the consumer side; rejected unknown task types land the saga in `failed` immediately. | Backwards-incompatible if any external producer passes an unknown type — but inventory §5 confirms no real producers exist. |
| 8 | Workflow id `automation-ops:{task_id}` | `saga.state.saga_id = task_id` (also UUIDv7 in the typical case). The `WorkflowId` is no longer addressable; consumers look up rows by `saga_id`. | Same uniqueness guarantee. |
| 9 | `audit_correlation_id` propagation via search attribute + activity HTTP header | Same UUID flows on the `saga.step.requested.v1` event header and on each effect call's `x-audit-correlation-id` header. | Cross-cutting. |
| 10 | No compensation in code today | Each `SagaStep` impl has its own `compensate` — runner replays them in LIFO order on failure (`saga.step.compensated` events). | The compensation chain only does work for steps that already completed. |
| 11 | No outbox / no Kafka — runtime is RPC-only | New events on outbox: `saga.step.completed`, `saga.step.failed`, `saga.step.compensated`, `saga.completed`, `saga.aborted`. Topic plumbing already declared (`saga.step.v1` + `__dlq.saga.step.v1` in `infra/helm/infra/kafka-cluster/values.yaml`). | The `libs/saga::SagaEventKind` enum maps each kind to its topic. |
| 12 | Per-task projection in Postgres: **none** | `automation_operations.saga_state` is the source of truth (`completed_steps text[]`, `step_outputs jsonb`, `failed_step text`, `status enum`). The HTTP read-side queries this row directly. | Provides the Temporal-visibility replacement. |
| 13 | Audit only via Temporal visibility | Every `saga.step.*` outbox event flows through Debezium → `audit.events.v1` consumer in `audit-compliance-service`. | Removes the only Temporal-only observability surface. |

---

## 8. Target service structure (`automation-operations-service` 2.0)

Per Tareas 6.2 + 6.3, the service grows from "thin REST→Temporal
adapter" to a self-contained saga runtime. Two concurrent tokio tasks
under the same `main.rs`.

```text
                       ┌─────────────────────────────────────────────────────┐
                       │           automation-operations-service              │
                       │  (one Rust binary, two concurrent tokio tasks)      │
                       │                                                       │
   user / svc-to-svc ─►│  (a) HTTP API (axum)                                  │
                       │      - POST /automations          (start saga)        │
                       │      - GET  /automations/{id}     (read saga.state)   │
                       │      - POST /automations/{id}/cancel (abort)          │
                       │      └─► writes saga.state row + outbox row in TX  ──┐│
                       │                                                       ││
   saga.step.requested │  (b) Saga consumer (rdkafka)                          ││
   .v1 (Kafka)   ─────►│      - dedup via libs/idempotency (event_id UUIDv5)   ││
                       │      - load saga.state by saga_id                     ││
                       │      - dispatch step body per `task_type` registry    ││
                       │      - outcome → SagaRunner::execute_step             ││
                       │        (writes saga.state + outbox, one TX)           ││
                       │      - on terminal failure → run compensations LIFO   ──┤
                       └────────────────────────────────────────────────────┬──┘│
                                                                            │   │
                              outbox.events  ──── Debezium ──► Kafka ◄──────┘   │
                                                                                │
                             ┌──────────────────────────────────────────────────┘
                             ▼
                    ┌───────────────────────────────────────────┐
                    │ saga.step.completed / .failed /           │
                    │ .compensated / saga.completed / .aborted  │
                    │ (one logical Kafka topic via EventRouter) │
                    └───────────────────────────────────────────┘
                              │
                              ▼
                    audit-compliance-service (audit projection)
                    notification-alerting-service (UI feed)
```

Component cardinality:

| Component | Substrate | Notes |
|---|---|---|
| HTTP API | axum, port `:50116` (unchanged) | Same routes today; semantics flip from "start Temporal workflow" to "start saga". |
| Saga consumer | rdkafka (`event-bus-data`); group `automation-operations-service`; topic `saga.step.requested.v1` (or, optionally, the broadcast `saga.step.v1` + filter on `aggregate=automation_run`) | Single consumer per service replica; partitioned by `saga_id` (= `task_id`). |
| Step registry | Compile-time registry: `pub fn step_runner_for(task_type: &str) -> Option<Box<dyn SagaStepBuilder>>` | New code — defines what step graph runs per `task_type`. |
| Step bodies | `libs/saga::SagaStep` impls living in `src/domain/steps/<task_type>.rs` | Each step calls the appropriate downstream service via `reqwest`. |
| Saga state store | `pg-???.automation_operations.saga_state` (per-service cluster, schema name TBD in Tarea 6.2) | Schema borrowed verbatim from `libs/saga/migrations/0001_saga_state.sql`. |
| Outbox / idempotency | `outbox.events` + `automation_operations.processed_events` | Same pattern as Tarea 5.3. |
| Timeout sweep | (deferred) k8s CronJob reading `saga.state` rows past their step deadline | Symmetric with FASE 5 / FASE 7. |

What goes away:

- `domain/temporal_adapter.rs` (Temporal start) — gone with FASE 8.
- `tests/temporal_e2e.rs`-style integration tests against the Go
  worker (none exist for this service today, so no work).
- Workspace dep on `temporal-client` (FASE 8 / Tarea 8.3).

What stays:

- HTTP routes + the JWT middleware on them.
- The `EnqueueTaskRequest` DTO (rename optional, but the wire shape
  is preserved).

---

## 9. Net call graph (today)

```text
                ┌──────────────────────────────────────┐
   user / svc ─►│ POST /api/v1/automations             │
                └──────────────────────────────────────┘
                                    │
                                    ▼
                ┌──────────────────────────────────────┐
                │ automation-operations-service        │
                │  enqueue_request_from_payload        │
                │  → AutomationOpsAdapter::enqueue     │
                └──────────────────────────────────────┘
                                    │ start_workflow_execution
                                    ▼
                ┌──────────────────────────────────────┐
                │ Temporal frontend (gRPC)             │
                │ ns=default, tq=of.automation-ops     │
                └──────────────────────────────────────┘
                                    │ poll
                                    ▼
                ┌──────────────────────────────────────┐
                │ workers-go/automation-ops worker     │
                │   AutomationOpsTask workflow         │
                │   └── ExecuteTask activity           │
                └──────────────────────────────────────┘
                                    │ HTTP POST /automations/{id}/runs
                                    ▼
                ┌──────────────────────────────────────┐
                │ automation-operations-service        │
                │ create_secondary handler             │
                │   (returns synthetic JSON; no DB)    │
                └──────────────────────────────────────┘
```

The self-call loop is the most striking aspect: the worker calls
back into the same service that started it, only to get a stub
response that is discarded. Removing the worker collapses this loop
into a single HTTP call.

## 10. Net call graph (post-migration target, for orientation only)

```text
   user / svc ─POST /api/v1/automations─► automation-operations-service
                                            │
                                            ▼
                              ┌──────────────────────────────┐
                              │ HTTP handler:                │
                              │  1. INSERT saga.state row    │
                              │     (status=running, name=   │
                              │      <task_type>, saga_id=   │
                              │      <task_id>)              │
                              │  2. INSERT outbox.events     │
                              │     (saga.step.requested.v1, │
                              │      first step in graph)    │
                              │  3. return 202               │
                              └──────────────────────────────┘
                                            │
                              Debezium      │
                                            ▼
                            ┌──────────────────────────┐
                            │ saga.step.requested.v1   │
                            └──────────────────────────┘
                                            │
                                            ▼
                              ┌──────────────────────────────┐
                              │ automation-operations-service │
                              │   saga consumer (same binary) │
                              │   1. dedup (libs/idempotency) │
                              │   2. load saga.state          │
                              │   3. dispatch step body via   │
                              │      task_type registry       │
                              │   4. SagaRunner::execute_step │
                              │      (UPDATE saga.state +     │
                              │       INSERT outbox.events,   │
                              │       one TX)                 │
                              │   5. on success: publish next │
                              │      step.requested OR        │
                              │      saga.completed           │
                              │   6. on failure: run LIFO     │
                              │      compensations + publish  │
                              │      saga.aborted             │
                              └──────────────────────────────┘
                                            │
                              Debezium      │
                                            ▼
                       saga.step.* / saga.completed / saga.aborted
                       → audit-compliance-service / UI feed
```

---

## 11. Risks / non-functional notes

* **Scope mismatch with the plan** — the migration plan describes a
  saga substrate that does not currently exist in code. Tareas
  6.2/6.3 are not "refactor" tasks — they are **net-new
  implementation**. Owners should plan accordingly.
* **No real producers means no backwards-compatibility constraint** —
  the wire contract on `POST /api/v1/automations` is technically
  free, but only test fixtures call it today. Tarea 6.3 can pick a
  cleaner shape (e.g., `{task_type: "...", input: {...}}` with the
  `task_type` as a discriminator).
* **`task_type` registry is the missing primitive** — without a
  concrete registry that maps strings to step graphs, the saga
  consumer cannot dispatch. This is the single most important
  decision for Tarea 6.3: where does the registry live (compile-time
  trait registration vs. data-driven config)? The reindex /
  workflow-automation precedents both used compile-time.
* **`libs/saga` already exists** with the runner + `saga.state`
  schema (Tarea 1.2). Tarea 6.3 should consume it as-is rather than
  re-implementing; the only adaptation needed is the bounded-context
  schema name (`automation_operations` instead of `saga`).
* **Topic naming inconsistency** — the helm catalog uses a single
  `saga.step.v1` topic; the migration plan §6.2 mentions four
  separate topics (`saga.step.requested.v1`, `.completed.v1`,
  `.failed.v1`, `saga.compensate.v1`). Tarea 6.2 must reconcile —
  either keep the single topic and use a `kind` field on the
  payload, or split into four. The `libs/saga` runner already
  picks the latter shape via `SagaEventKind::topic()`. The helm
  catalog needs at least three new topics
  (`saga.step.requested.v1`, `.completed.v1`, `.failed.v1`); the
  existing `saga.step.v1` covers the broadcast case.
* **No compensation chain to migrate** — the worker has no
  compensation logic today, so Tarea 6.4's "verify N-1, N-2
  compensations execute in inverse order on N's failure" test is
  about validating a brand-new code path, not regressing an existing
  one.
* **The HTTP read-side handlers are stubs** — `list_items` /
  `get_item` / `list_secondary` return placeholder JSON. Tarea 6.3
  should re-implement them on top of `saga.state` so the UI sees
  real data.
* **Self-call loop today is wasteful** — the worker activity POSTs
  back into the same service; both sides are stubs, so the only
  thing that ever happens is an HTTP round-trip that does nothing.
  Killing the worker (Tarea 6.5) eliminates the round-trip
  altogether.

---

## 12. Acceptance for Tarea 6.1

* This document exists at `docs/architecture/refactor/automation-ops-worker-inventory.md`.
* §2 documents the single workflow (`AutomationOpsTask`) with its
  full input / output / retry-policy / Temporal-feature footprint.
* §3 documents the single activity (`ExecuteTask`) with its
  full HTTP contract (URL, headers, body, response, error mapping).
* §4 lists every producer in the codebase (just the Rust adapter)
  and every read-side route (all stubs).
* §5 catalogues every `task_type` value present in code (only one
  test fixture: `retention.sweep` / `retention_sweep`) and flags the
  absence of a real catalog as the FASE 6 design problem.
* §7 maps each Temporal primitive in use today to its saga-pattern
  replacement (`libs/saga::SagaRunner` + outbox + `saga.step.*`
  topics + per-`task_type` step registry), ready to drive Tareas
  6.2 (saga schema), 6.3 (consumer + step registry), 6.4 (chaos
  test) and 6.5 (worker deletion).
* §8 sketches the post-migration `automation-operations-service` 2.0
  shape so Tarea 6.3 has a starting blueprint.
