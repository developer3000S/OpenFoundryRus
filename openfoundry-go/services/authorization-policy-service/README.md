# authorization-policy-service (Go port)

Go port of the Rust `services/authorization-policy-service` crate (per
ADR-0027). The Rust binary is currently `fn main() {}` (S8 / B14
consolidation pending), so the Go port is the **canonical
implementation**.

## Port status (2026-05-07)

The Go binary is no longer just the Cedar-policy foundation slice. It
wires the consolidated `/api/v1` authorization surface: Cedar policy
CRUD with strict validation, ABAC policies and `/policy-evaluations`,
RBAC roles/groups/permissions, governance-template applications,
project constraints, structural-security rules, checkpoints/purpose
records, cipher channels/licenses, and network-boundary resources.

The latest stub scan found **no productive `StatusNotImplemented` or
placeholder handler matches** in this service. Test-only matches live in
`*_test.go` and are excluded from the production scan.

Plus `/healthz` + `/metrics`.

## Configuration

| Env var                                  | Required | Default        |
|------------------------------------------|----------|----------------|
| `DATABASE_URL`                           | yes      | —              |
| `OPENFOUNDRY_JWT_SECRET` / `JWT_SECRET`  | yes      | —              |
| `NATS_URL`                               | no       | unset (no publish) |
| `HOST`                                   | no       | `0.0.0.0`      |
| `PORT`                                   | no       | `50115`        |
| `METRICS_ADDR`                           | no       | `0.0.0.0:9090` |
| `SERVICE_VERSION`                        | no       | `dev`          |

When `NATS_URL` is set, every successful CRUD write publishes to
`authz.policy.changed` (the subject `libs/authz-cedar-go`'s
`PolicyReloadSubscriber` listens on by default).

## Schema

`internal/repo/migrations/0001_cedar_policies.sql` creates the
`cedar_policies` table read by `libs/authz-cedar-go/pg.go` (per
ADR-0027). Column set is locked to that loader's `SELECT` — changing
either side breaks the contract.

```sql
cedar_policies (
    id          TEXT     PRIMARY KEY,
    version     INTEGER  NOT NULL,
    source      TEXT     NOT NULL,
    description TEXT,
    active      BOOLEAN  NOT NULL DEFAULT TRUE,
    created_by  UUID     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
```

## Wire format (locked)

`CedarPolicy`:

```json
{
  "id": "permit-cleared-readers",
  "version": 1,
  "source": "permit(principal, action == Action::\"read\", resource is Dataset);",
  "description": "view dataset when clearance covers markings",
  "active": true,
  "created_by": "11111111-1111-1111-1111-111111111111",
  "created_at": "2026-05-06T00:00:00Z",
  "updated_at": "2026-05-06T00:00:00Z"
}
```

List envelope: `{"items": [...]}` (matches the foundation-slice
convention used by organizations + enrollments).

## Validation contract

Every write (POST + PATCH-with-`source`) runs through a fresh in-memory
`PolicyStore` from `libs/authz-cedar-go` before the SQL INSERT/UPDATE.
A bad source therefore fails with `400 Bad Request` and the row is
never persisted. The active validator state is hermetic per request
so concurrent malformed writes can't poison one another.

## Remaining migration notes

Per the [stub audit](../../STUB-AUDIT.md), this service currently has no
productive stub matches in production Go files. Keep future work focused
on conformance depth (Cedar/ABAC/RBAC fixtures and cross-service policy
reload behavior), not on replacing placeholder route handlers.

## Build / test

```sh
cd openfoundry-go
go build ./services/authorization-policy-service/...
go test -race ./services/authorization-policy-service/...
```

## Smoke

```sh
go run ./services/authorization-policy-service/cmd/authorization-policy-service
# → ERROR config load failed error="required environment variable DATABASE_URL is not set"
```
