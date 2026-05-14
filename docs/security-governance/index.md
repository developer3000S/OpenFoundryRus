# Security & governance

This section covers identity, permissions, policy, traceability, and controlled operational change.

> **New here?** Start with [Security overview](./security-overview.md) for the seven control layers and how they compose, and with [Shared responsibility model](./shared-responsibility-model.md) for who owns which control. The full parity backlog lives in the [security & governance 1:1 checklist](../migration/foundry-security-governance-1to1-checklist.md).

## OpenFoundry mapping

- `services/identity-federation-service` — identity, MFA, WebAuthn, OIDC, SAML, RBAC, SCIM, JWKS rotation
- `services/authorization-policy-service` — Cedar-backed authorization decision point (ABAC + RBAC + restricted views)
- `services/audit-compliance-service` — platform audit ledger, retention policies, lineage deletion subsystem
- `services/audit-sink` — Kafka → Iceberg consumer for the `audit.events.v1` stream
- `services/tenancy-organizations-service` — organizations, workspace enrollments, multi-tenancy
- `services/telemetry-governance-service` — telemetry permissions, export policies, monitoring rules
- `libs/auth-middleware` — HTTP auth middleware (claims, session scope, JWKS verification)
- `libs/authz-cedar-go` — Cedar policy engine bindings used by `authorization-policy-service`
- `libs/audit-trail` — structured audit events for the compliance collectors
- `libs/media-scanner` — Sensitive Data Scanner integration (PII detection)
- `apps/web/src/routes/audit`, `/auth`, `/control-panel` — security-oriented UIs
- `proto/auth/*`, `proto/audit/*` — wire contracts (wire-format breakage hits every consumer)

> The `identity-federation-service`, `authorization-policy-service` and `libs/auth-middleware` are designated **security-critical zones** in [`CLAUDE.md`](../../CLAUDE.md): changes there require extra care and explicit human review.

## Key concerns

- authentication and federated identity (OIDC, SAML, MFA, WebAuthn, SCIM)
- role-based, attribute-based and policy-based authorization (Cedar)
- auditability (immutable ledger + Kafka sink to Iceberg)
- semantic and row/column-level access control via restricted views
- environment and deployment governance (tenancy + telemetry policies)

## Section map

- [Security overview](/security-governance/security-overview) — landing page for the seven control layers
- [Shared responsibility model](/security-governance/shared-responsibility-model) — operator vs tenant ownership
- [Identity and access](/security-governance/identity-and-access)
- [Policies and authorization](/security-governance/policies-and-authorization)
- [Policy bundles in-process](/security-governance/policy-bundles)
- [Restricted views and data controls](/security-governance/restricted-views-and-data-controls)
- [Audit and traceability](/security-governance/audit-and-traceability)
- [ABAC and CBAC model](/security-governance/abac-and-cbac-model/)
- [Policy evaluation flows](/security-governance/policy-evaluation-flows/)
- [Audit model](/security-governance/audit-model/)
