# Inventory — tenancy-organizations-service

Snapshot of the Rust crate `services/tenancy-organizations-service` taken
during the Go port. Total Rust source: ~3 950 LOC across handlers + models
+ domain (excluding migrations).

## Schema (Postgres)

| Migration                                        | Owns                                  |
|--------------------------------------------------|---------------------------------------|
| `20260423091500_nexus_foundation.sql`            | early `nexus_*` tables (legacy)       |
| `20260425223000_spaces_and_admin_lifecycle.sql`  | spaces + admin lifecycle              |
| `20260427000100_tenancy_organizations_foundation.sql` | **organizations + enrollments** (Slice 1) |
| `20260501000300_user_favorites.sql`              | favorites                             |
| `20260501000400_resource_access_log.sql`         | recents (resource access log)         |
| `20260501000500_resource_shares.sql`             | sharing rules                         |

## Handler surface (Rust → planned slices)

| Rust file                          | LOC | Slice  | Status        |
|------------------------------------|-----|--------|---------------|
| `handlers/organizations.rs`        | 114 | **1**  | ✅ ported     |
| `handlers/enrollments.rs`          | 108 | **1**  | ✅ ported (CRUD) |
| `handlers/spaces.rs`               | 179 | 2      | pending       |
| `handlers/projects.rs`             | 786 | 3      | pending (large) |
| `handlers/sharing.rs`              | 326 | 4      | pending       |
| `handlers/trash.rs`                | 340 | 5      | pending       |
| `handlers/favorites.rs`            | 157 | 6      | pending       |
| `handlers/recents.rs`              | 131 | 6      | pending       |
| `handlers/workspace.rs`            | 106 | 2      | pending       |
| `handlers/tenant_resolution.rs`    |  30 | 7      | pending       |
| `handlers/resource_resolve.rs`     | 169 | 7      | pending       |
| `handlers/resource_ops.rs`         | 509 | 7      | pending       |

## Domain logic

- `domain/tenant_resolution.rs` (124 LOC) — RID → org/space/project lookup.
- `domain/project_access.rs` (334 LOC) — project access decisions.

Both will land alongside the slices that consume them (resource_resolve →
slice 7; projects → slice 3).

## Model crate split

- `models/organization.rs` (36) — ✅ ported.
- `models/enrollment.rs` (32) — ✅ ported.
- `models/space.rs` (84) — slice 2.
- `models/project.rs` (131) — slice 3.
- `models/control_plane.rs` (42) — slice 7.
- `models/peer.rs` (104) — slice 7.

## Wire-format invariants (locked)

- Snake-case JSON for every body: `display_name`, `organization_type`,
  `default_workspace`, `tenant_tier`, `created_at`, `updated_at`,
  `organization_id`, `user_id`, `workspace_slug`, `role_slug`.
- List envelope: `{"items": [...]}` (NEVER `data` or `results`).
- IDs are RFC-4122 v4 UUIDs.
- Timestamps are ISO-8601 with timezone (UTC).
- Status enums: `active`, `disabled`, `archived`.

These are pinned by `internal/handlers/handlers_test.go` in the Go port.

## Sliced port plan

1. **Foundation** (this commit) — organizations + enrollments CRUD.
2. **Spaces + workspace** — `spaces` table, workspace handlers.
3. **Projects** — `projects` table, project access domain (large; may split).
4. **Sharing** — `resource_shares` + share rule evaluation.
5. **Trash** — soft-delete + restore lifecycle.
6. **Favorites + recents** — UX surfaces over `user_favorites` +
   `resource_access_log`.
7. **Resource resolution** — `tenant_resolution`, `resource_resolve`,
   `resource_ops` (cross-service RID lookup helpers).

Each slice is independently committable and preserves wire-format from
the Rust crate.

## Configuration parity

Same env names as the Rust crate:

- `DATABASE_URL` (required)
- `OPENFOUNDRY_JWT_SECRET` / `JWT_SECRET` (required)
- `HOST`, `PORT` (default `0.0.0.0:50113`)

Port `50113` matches the Rust foundation listener.
