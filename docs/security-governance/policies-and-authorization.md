# Policies and authorization

> **Sensitive admin surface.** Policy and role administration is the layer
> that turns identity into authorization decisions. Read the
> [Security overview](./security-overview.md) for how this layer composes
> with the other six, and the
> [Shared responsibility model](./shared-responsibility-model.md) for which
> roles can configure what. Anything modeled on a Foundry concept must
> follow the [Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).

Authorization in OpenFoundry is broader than role checks alone.

## Repository signals

Authorization responsibilities are split between two services:

- `services/identity-federation-service` — user identity, role and permission administration; **issues** the principal that decisions are taken about.
- `services/authorization-policy-service` — the **decision point**. Cedar-backed engine that evaluates ABAC + RBAC + restricted-view policies. Lib bindings live in `libs/authz-cedar-go`.

The implementation entry points live in:

- `services/authorization-policy-service/cmd/authorization-policy-service/main.go` + `internal/server/` — chi router, policy CRUD and evaluation endpoints
- `services/authorization-policy-service/internal/handlers/` — policy management, permission management, role binding handlers
- `services/identity-federation-service/internal/handlers/` — RBAC role administration on the identity side
- `libs/auth-middleware` — claims extraction + scope checks invoked from every protected route

## Why this matters

Operational platforms usually need a layered model:

- role-based access for broad capability boundaries (RBAC)
- policy-based evaluation for fine-grained control (Cedar)
- attribute-aware decisions for sensitive data and object operations (ABAC)
- restricted views for row/column-level filtering

The current repo already contains the primitives for that model. The pattern for distributing policies to the data-plane in-process is documented in [Policy bundles in-process](./policy-bundles.md).
