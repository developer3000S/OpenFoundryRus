# ABAC and CBAC model

> **Sensitive admin surface.** Attribute-based decisions depend on the
> identity source of truth. See the
> [Security overview](../security-overview.md), the
> [Shared responsibility model](../shared-responsibility-model.md), and
> the [Foundry public-docs parity policy](../../reference/foundry-public-docs-parity-policy.md).

OpenFoundry already shows signs of moving beyond RBAC-only authorization.

## Repository signals

`authorization-policy-service` contains explicit modules for:

- RBAC role bindings (also exposed via `identity-federation-service`'s administrative surface)
- ABAC evaluation built on top of `libs/authz-cedar-go` (Cedar entity/attribute model)
- restricted views (row/column-level filtering)

Migration history also references markings, CBAC-style controls, and restricted views.

## Why this matters

For an ontology-driven and data-sensitive platform, role checks alone are usually not enough.

Attribute- and context-aware access control becomes important for:

- tenant-aware experiences (`tenancy-organizations-service` provides the principal's organization context)
- sensitive operational data (markings → see [Data-connectivity: Markings](../../data-connectivity/datasets/markings.md))
- object- and property-level restrictions (enforced by `ontology-query-service` + `object-database-service`)
- conditional action execution (`ontology-actions-service` consults policy before dispatching writes)
