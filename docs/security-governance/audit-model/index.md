# Audit model

> **Sensitive admin surface.** Audit data is itself sensitive and is
> gated by separate permissions. See the
> [Security overview](../security-overview.md), the
> [Shared responsibility model](../shared-responsibility-model.md), and
> the [Foundry public-docs parity policy](../../reference/foundry-public-docs-parity-policy.md).

OpenFoundry's audit model should explain not just that audit logs exist, but how audit is woven through platform capabilities.

## Repository signals

The current repo already includes:

- a dedicated `services/audit-compliance-service` (ledger + retention + lineage deletion)
- the `services/audit-sink` Kafka consumer that lands events into Iceberg for long-term storage
- a shared `libs/audit-trail` library every service uses to emit structured audit events
- gateway middleware with audit concerns (`libs/auth-middleware`)
- ontology actions and workflow paths that depend on traceability

## Why this matters

An audit model page is the right place to document:

- what gets recorded
- where audit events are emitted (via `libs/audit-trail` to Kafka topic `audit.events.v1`)
- how operational teams investigate changes (via the `/audit` UI route, backed by `audit-compliance-service`)
- how audit supports governance and incident review

## OpenFoundry current vs target

| Dimension | OpenFoundry current | OpenFoundry target |
| --- | --- | --- |
| audit backend | dedicated service (`audit-compliance-service`) + Iceberg sink (`audit-sink`) + shared library (`libs/audit-trail`) | platform-wide consistent event taxonomy |
| integration points | gateway and semantic workflows already imply audit hooks | every critical object, action, workflow, and policy event recorded |
| investigation | service and workflow level | cross-capability traceability from UI to backend event trail |
