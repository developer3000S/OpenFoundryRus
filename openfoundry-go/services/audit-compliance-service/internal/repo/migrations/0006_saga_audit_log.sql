-- FASE 6 / Tarea 6.2 — system-wide saga audit log.
--
-- Append-only ledger of every `saga.*.v1` Kafka event observed
-- across every bounded context that runs a `libs/saga::SagaRunner`.
-- Fed by a Kafka consumer (FASE 6.5+ deliverable; this migration
-- provisions the schema only).
--
-- Why a separate audit table:
--   * The operational `saga_state` row lives in the OWNING service's
--     bounded-context schema (e.g.
--     `pg-runtime-config.automation_operations.saga_state`,
--     `pg-runtime-config.workflow_automation.saga_state`). That row
--     is mutable by `SagaRunner` and gets garbage-collected with the
--     service's data lifecycle.
--   * The audit story needs an **append-only**, system-wide,
--     long-retention view of every saga event for compliance,
--     forensics and cross-bounded-context join queries (e.g. "did a
--     compensation in saga A fire because saga B failed?"). That
--     view lives here, in `pg-policy.audit_compliance` — the same
--     cluster the rest of the audit trail uses.
--   * Idempotency is critical: Kafka's at-least-once delivery means
--     the consumer will see duplicates. The `event_id` PRIMARY KEY
--     collapses them at the row level (`INSERT ... ON CONFLICT DO
--     NOTHING`). This is the same record-before-process pattern
--     `libs/idempotency` formalises, but co-located with the audit
--     ledger so a single INSERT both records the dedup key and the
--     audit row.
--
-- The `audit_compliance` schema and the `svc_audit_compliance` role
-- are pre-created by the pg-policy bootstrap
-- (`infra/helm/infra/postgres-clusters/templates/clusters/`
-- `pg-policy-bootstrap-sql.yaml`). When the service DSN points at
-- the legacy per-service cluster the schema is created on the fly
-- here so the migration is portable across both topologies.

CREATE SCHEMA IF NOT EXISTS audit_compliance;

CREATE TABLE IF NOT EXISTS audit_compliance.saga_audit_log (
    -- Deterministic `event_id` set by the producing service via
    -- `outbox::enqueue` (UUIDv5 of
    -- `(saga_id, step_name, kind)` per `libs/saga`). Idempotency at
    -- the row level: a Kafka redelivery short-circuits via
    -- `INSERT ... ON CONFLICT (event_id) DO NOTHING`.
    event_id        uuid        PRIMARY KEY,
    -- Saga aggregate id — same value as
    -- `<bounded_context>.saga_state.saga_id`. Indexed below for
    -- per-saga timeline queries.
    saga_id         uuid        NOT NULL,
    -- Saga type, e.g. `retention.sweep`. Mirrors
    -- `saga_state.name`.
    saga_name       text        NOT NULL,
    -- Bounded-context that emitted the event (so the audit table
    -- can be partitioned / filtered by service of origin).
    -- Value comes from the producer's `OutboxEvent::aggregate`
    -- field (today: hard-coded `"saga"` by `libs/saga::SagaRunner`).
    -- Future enrichment: enrich the consumer to set this from the
    -- Kafka `ol-producer` header.
    source_service  text        NOT NULL,
    -- One of:
    --   `step.requested`    — inbound (producer action)
    --   `step.completed`    — runner-emitted on step success
    --   `step.failed`       — runner-emitted on step failure
    --   `step.compensated`  — runner-emitted per LIFO compensation
    --   `compensate`        — inbound (cross-saga rollback signal)
    --   `saga.completed`    — terminal happy-path
    --   `saga.aborted`      — terminal abort
    -- Stored as a free-form `text` because the audit row's lifetime
    -- outlasts the application enum; the `saga.*` topics already
    -- enforce these names server-side via the Kafka topic catalog.
    kind            text        NOT NULL,
    -- Step name when applicable (NULL for terminal saga events
    -- and for the inbound `compensate` request).
    step_name       text,
    -- Full payload of the Kafka record, verbatim. Forensic queries
    -- can crack this open without joining back to the operational
    -- saga state, which may have GC'd by then.
    payload         jsonb       NOT NULL,
    -- End-to-end correlation id propagated from the producer's
    -- `x-audit-correlation-id` header — the same UUID flows
    -- through every step's effect call so cross-service traces
    -- stitch together.
    correlation_id  uuid,
    -- Tenant the saga belongs to. Indexed for tenant-scoped audit
    -- exports (GDPR / DPIA).
    tenant_id       text,
    -- Wall-clock time the event was first seen by this consumer.
    -- The producer's "step finished at" timestamp is inside
    -- `payload`; this column is the consumer's observation point.
    observed_at     timestamptz NOT NULL DEFAULT now()
);

-- Per-saga timeline: "show me everything that happened to saga
-- 9f8e..." — the most common operator query.
CREATE INDEX IF NOT EXISTS saga_audit_log_saga_id_idx
    ON audit_compliance.saga_audit_log (saga_id, observed_at);

-- Tenant-scoped audit export.
CREATE INDEX IF NOT EXISTS saga_audit_log_tenant_idx
    ON audit_compliance.saga_audit_log (tenant_id, observed_at)
    WHERE tenant_id IS NOT NULL;

-- Cross-saga forensics ("which sagas failed in the last hour?").
CREATE INDEX IF NOT EXISTS saga_audit_log_kind_observed_idx
    ON audit_compliance.saga_audit_log (kind, observed_at)
    WHERE kind IN ('step.failed', 'saga.aborted');

-- Correlation-id stitching across services.
CREATE INDEX IF NOT EXISTS saga_audit_log_correlation_idx
    ON audit_compliance.saga_audit_log (correlation_id)
    WHERE correlation_id IS NOT NULL;
