# Geospatial CRS Policy

OpenFoundry follows the public Foundry CRS guidance for geospatial parity: source data can arrive in many coordinate reference systems, but maps expect WGS 84 and render using Web Mercator for display. The OpenFoundry runtime therefore keeps one conservative contract until reprojection support is implemented.

## Contract

- Canonical data CRS: WGS 84 / EPSG:4326.
- Map display projection: Web Mercator is a rendering concern only; persisted rows and API payloads stay in EPSG:4326.
- Reprojection: only EPSG:4326 to EPSG:4326 is supported as a no-op. Any other CRS is rejected with a structured validation error.
- Coordinate bounds: latitude must be finite and within `[-90, 90]`; longitude must be finite and within `[-180, 180]`.

## Coordinate Order

OpenFoundry uses explicit coordinate-order metadata so Map, Pipeline Builder, and Ontology agree on pair semantics.

| Surface | Order | Example |
| --- | --- | --- |
| Internal `GeoPoint`, Map `Coordinate`, and Ontology GeoPoint string | `lat_lon` | `{ "lat": 40.4168, "lon": -3.7038 }`, `40.4168,-3.7038` |
| GeoJSON positions and GeoJSON `bbox` | `lon_lat` | `[ -3.7038, 40.4168 ]`, `[minLon, minLat, maxLon, maxLat]` |
| Pipeline field metadata | `coordinate_order` should match the logical type default | `geo_point -> lat_lon`, `geojson -> lon_lat`, `bounding_box -> lon_lat` |

## Pipeline Metadata

Pipeline Builder schema fields can carry:

```json
{
  "logical_type": "geojson",
  "crs": "EPSG:4326",
  "coordinate_order": "lon_lat"
}
```

The strict schema validator rejects unsupported CRS values such as `EPSG:3857`, invalid coordinate-order names, and coordinate-order metadata that conflicts with the logical type.

## Implementation Pointers

- Shared policy and no-op reprojection live in [`libs/geospatial-core/crs_policy.go`](../../libs/geospatial-core/crs_policy.go).
- Logical geospatial types live in [`libs/geospatial-core/types.go`](../../libs/geospatial-core/types.go).
- Pipeline Builder schema validation consumes the policy in [`schema_validation.go`](../../services/pipeline-build-service/internal/handler/schema_validation.go).

Official reference: [Palantir Foundry coordinate reference systems and projections](https://www.palantir.com/docs/foundry/geospatial/coordinate_reference_systems_and_projections/).
