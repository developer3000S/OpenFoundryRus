# tenancy-organizations-service (Go port)

Go port of the Rust `services/tenancy-organizations-service` crate. Owns
organizations, workspace enrollments, and (in follow-up slices) workspaces,
projects, sharing, trash, favorites, and the resource-resolve / resource-ops
helpers.

## Foundation slice (this commit)

Endpoints (all under `/api/v1`, JWT-protected):

- `GET    /organizations`            — list (200 most-recent)
- `POST   /organizations`            — create
- `GET    /organizations/{id}`       — fetch
- `PATCH  /organizations/{id}`       — partial update
- `DELETE /organizations/{id}`       — delete
- `GET    /organizations/{id}/enrollments` — list enrollments for an org
- `POST   /enrollments`              — create
- `DELETE /enrollments/{id}`         — delete

Plus the standard `/healthz` + `/metrics` foundation surface.

The migration (`internal/repo/migrations/0001_tenancy_organizations_foundation.sql`)
is copied verbatim from the Rust crate to keep schema parity.

## Configuration

| Env var                   | Required | Default  |
|---------------------------|----------|----------|
| `DATABASE_URL`            | yes      | —        |
| `OPENFOUNDRY_JWT_SECRET` / `JWT_SECRET` | yes      | —        |
| `HOST`                    | no       | `0.0.0.0`|
| `PORT`                    | no       | `50113`  |
| `METRICS_ADDR`            | no       | `0.0.0.0:9090` |
| `SERVICE_VERSION`         | no       | `dev`    |

## Follow-up slices (deferred)

- Spaces (`tenancy_workspaces` table) — Rust migration `0002`
- Projects (`tenancy_projects` table)
- Sharing rules + invitations
- Trash + favorites + recents
- `resource_resolve` / `resource_ops` helpers (cross-service RID lookup)

These are tracked under todos and the inventory in
`openfoundry-go/INVENTORY-tenancy-organizations-service.md` (forthcoming).

## Build / test

```sh
cd openfoundry-go
go build ./services/tenancy-organizations-service/...
go test -race ./services/tenancy-organizations-service/...
```
