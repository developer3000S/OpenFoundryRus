# Policy evaluation flows

> **Sensitive admin surface.** Evaluation flows wire the seven control
> layers together. See the [Security overview](../security-overview.md),
> the [Shared responsibility model](../shared-responsibility-model.md),
> and the [Foundry public-docs parity policy](../../reference/foundry-public-docs-parity-policy.md).

Policy evaluation is where abstract authorization models become operational decisions.

## Repository signals

`authorization-policy-service` exposes explicit evaluation endpoints:

- `POST /api/v1/policies/evaluate`
- `POST /api/v2/admin/policies/evaluate`

Those endpoints are wired in `services/authorization-policy-service/cmd/authorization-policy-service/main.go` and `internal/server/`, and backed by policy handlers under `services/authorization-policy-service/internal/handlers/`. The decision engine itself lives in `libs/authz-cedar-go` (Cedar bindings).

In the data plane, hot-path decisions are usually evaluated in-process by services that link `libs/auth-middleware` and consume a policy bundle pushed over NATS JetStream — see [Policy bundles in-process](../policy-bundles.md). This is by design: there is no central PDP on the hot path.

## Why this matters

Documenting the evaluation flow helps explain:

- where decisions are made (admin/control plane endpoints vs in-process evaluation in data services)
- which user, role, group, and attribute inputs matter
- how restricted views and ontology operations can consume policy outcomes
