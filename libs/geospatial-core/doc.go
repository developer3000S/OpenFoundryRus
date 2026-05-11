// Package geospatialcore defines the shared geospatial logical-type
// contracts used by Pipeline Builder, Map, and Ontology-facing services.
//
// The current productive scope is intentionally conservative:
// WGS84/EPSG:4326 GeoPoints, standard GeoJSON geometries, GeoJSON bounding
// boxes, string-backed H3 indices, explicit CRS metadata, coordinate-order
// policy, and no-op EPSG:4326 reprojection. These types are designed to be
// JSON-stable, validatable before execution, and mappable to existing Ontology
// property base types.
package geospatialcore
