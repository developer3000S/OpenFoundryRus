# Restricted views and data controls

> **Sensitive admin surface.** Restricted views and row-level policies are
> mandatory access controls — misconfiguration leaks data. Read the
> [Security overview](./security-overview.md) for how restricted views
> compose with markings and scoped sessions, and the
> [Shared responsibility model](./shared-responsibility-model.md) for
> who is allowed to define and approve policies. Anything modeled on a
> Foundry concept must follow the
> [Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).

Data protection in OpenFoundry is not only about authentication. It also needs controlled projections of data for different audiences and contexts.

## Repository signals

`authorization-policy-service` includes explicit support for restricted views through:

- `internal/handlers/restricted_views.go` — CRUD over view definitions
- `internal/models/restricted_view.go` — view schema + scope rules
- migration history that mentions restricted views and CBAC-style controls

These signals suggest that OpenFoundry is already moving toward a layered data-protection model rather than a simple binary allow/deny gate.

At the data plane the decision typically runs in-process via `libs/auth-middleware`, which evaluates the view's filter expression against the caller's claims — see [Policy bundles in-process](./policy-bundles.md) for how the bundles reach each service.

## Why this matters

Restricted views are especially useful for:

- regulated or marked datasets
- tenant-aware operational data
- partial exposure to external partners
- object-level and field-level semantics in ontology-driven apps
