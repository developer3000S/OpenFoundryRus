# `ai-sink`

Kafka → Iceberg sink for AI-platform events.

* **Source topic:** `ai.events.v1` (producer: `agent-runtime-service`,
  which absorbed the legacy `prompt-workflow-service` producer per
  ADR-0030).
* **Target catalog:** `lakekeeper`.
* **Target namespace:** `of_ai`.
* **Target tables:** `prompts`, `responses`, `evaluations`, `traces` —
  routed per envelope `kind`.
* **Partition spec:** `day(at)` on every table.
* **Sort order:** `at ASC`.
* **Batch policy:** flush at 100 000 records OR 60 s elapsed
  (`BatchPolicy::PLAN_DEFAULT`, identical to `audit-sink`).
* **Retention:** 1-year snapshot expiration on the four tables (data
  files themselves persist; partition files are kept by Iceberg until
  an explicit `expire_snapshots` is run). Audit-class WORM (no expiry
  ever) is reserved for `of_audit.events`; AI logs are not regulatory
  evidence.

## Crate shape

* `[lib]` substrate (`ai_sink`) — pure constants + decoder + routing,
  zero I/O. Unit-testable.
* `[[bin]] ai-sink` gated behind feature `runtime` (pulls
  `event-bus-data` and `prometheus`). Binary is currently a stub
  identical to `audit-sink`'s — the Kafka consumer + Iceberg writer
  land in a follow-up PR per S5.3.b.

## Why not embed in `audit-sink`?

The two sinks share *shape* (Kafka-batched-to-Iceberg) but differ in
retention policy (audit = WORM forever, AI = 1y rollover) and target
namespace ACLs (audit is read-restricted to security; AI is readable
by ML eval pipelines). Keeping them as separate consumers also lets
them scale independently — agent-eval loads can spike orders of
magnitude above audit volume.

## Required Kafka ACL

Read on `ai.events.v1` and group `ai-sink` — declared in
[`infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml).
