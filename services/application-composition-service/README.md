# `application-composition-service` (Go)

Runtime owner for OpenFoundry's Workshop-style application composition surface.
The service persists apps, pages/widgets inside the app definition, versions,
publish snapshots, and public runtime loading by slug.

## Compatibility naming

Application Builder public payloads should follow the frozen terminology in
[`docs/reference/foundry-compatibility-glossary.md`](../../docs/reference/foundry-compatibility-glossary.md):
use `app` for the authored/published application, reserve `module` for a
future first-class Workshop module boundary, use `page` for route-level app
surfaces, `widget` for renderable building blocks, and `variable` for values
that move data between widgets, pages, modules, actions, and functions.

Current app definitions are normalized to the `2026-05-11.ws.1` Workshop app
contract before create, update, and publish. That contract exposes the key wire
fields:

- `app.id`, `app.slug`, `pages`, `settings`, `theme`
- `page.id`, `page.path`, `page.layout`, `page.widgets`, `page.sections`,
  `page.overlays`
- `section.id`, `section.layout`, `section.widgets`, `section.sections`
- `widget.id`, `widget.widget_type`, `widget.props`, `widget.config`,
  `widget.binding`, `widget.bindings`, `widget.events`, `widget.actions`,
  `widget.children`
- compatibility object-set variables under `settings.object_set_variables`
  and Workshop variables under `settings.workshop_variables`
- runtime metadata under `settings.runtime_metadata`, including
  `schema_version` and `public_slug`

The web runtime evaluates `settings.workshop_variables` through the WS.4
variable engine. Variable definitions may reference `source_variable_id`,
`filter_variable_id`, `source_widget_id`, static filters, default values, and
metadata for URL/runtime parameters or aggregations.

Use `id` for internal UUIDs or local child identifiers, and introduce `rid`
only when a resource needs a stable external identity.

## Workshop editor endpoints

The service owns the backend calls expected by `apps/web/src/lib/api/apps.ts`:

- `GET /api/v1/widgets/catalog` serves the embedded, versioned
  `internal/catalog/widget_catalog.v1.json` contract and returns
  `X-OpenFoundry-Widget-Catalog-Version` / `X-OpenFoundry-Widget-Catalog-Schema`
  headers.
- `GET /api/v1/apps/templates`
- `POST /api/v1/apps/from-template`
- `POST /api/v1/apps/{id}/pages`
- `PATCH /api/v1/apps/{id}/pages/{pageId}`
- `DELETE /api/v1/apps/{id}/pages/{pageId}`
- `GET /api/v1/apps/{id}/preview`
- `GET|POST /api/v1/apps/{id}/slate-package`
- `GET /api/v1/apps/public/{slug}` and `/embed`

## Build & run

```sh
go build -o bin/application-composition-service ./services/application-composition-service/cmd/application-composition-service
go test ./services/application-composition-service/...
```
