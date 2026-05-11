package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
)

func validateGPXParseNode(nodeID string, cfg tableRuntimeConfig, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	out := gpxTrailStrictSchema()
	validateSchemaInternals(nodeID, out, report)
	if strings.TrimSpace(firstNonEmpty(cfg.GPXContent, cfg.GPXXML, cfg.GPX)) != "" {
		return out
	}

	base := requireOneInput(nodeID, "gpx_parse", deps, report)
	source := strings.TrimSpace(firstNonEmpty(cfg.GPXColumn, cfg.SourceColumn, cfg.ContentColumn))
	if source == "" || !base.Known {
		return out
	}
	field, ok := base.field(source)
	if !ok {
		report.addError(nodeID, strPtr(source), "missing_column", fmt.Sprintf("GPX source column %q does not exist in upstream schema", source))
		return out
	}
	if !strings.EqualFold(field.FieldType, "STRING") {
		report.addError(nodeID, strPtr(source), "incompatible_gpx_source_column", fmt.Sprintf("GPX source column %q must be STRING, got %s", source, field.FieldType))
	}
	return out
}

func gpxTrailStrictSchema() pipelineStrictSchema {
	return pipelineStrictSchema{Known: true, Fields: []pipelineStrictValidationField{
		{Name: "trail_id", FieldType: "STRING", Nullable: false},
		{Name: "trail_name", FieldType: "STRING", Nullable: false},
		{Name: "point_count", FieldType: "INTEGER", Nullable: false},
		{Name: "distance_meters", FieldType: "DOUBLE", Nullable: false},
		{Name: "distance_miles", FieldType: "DOUBLE", Nullable: false},
		{Name: "elevation_gain_meters", FieldType: "DOUBLE", Nullable: false},
		{Name: "elevation_gain_ft", FieldType: "DOUBLE", Nullable: false},
		{Name: "min_elevation_meters", FieldType: "DOUBLE", Nullable: true},
		{Name: "max_elevation_meters", FieldType: "DOUBLE", Nullable: true},
		{Name: "min_elevation_ft", FieldType: "DOUBLE", Nullable: true},
		{Name: "max_elevation_ft", FieldType: "DOUBLE", Nullable: true},
		{Name: "start_lat", FieldType: "DOUBLE", Nullable: false},
		{Name: "start_lon", FieldType: "DOUBLE", Nullable: false},
		{Name: "end_lat", FieldType: "DOUBLE", Nullable: false},
		{Name: "end_lon", FieldType: "DOUBLE", Nullable: false},
		{Name: "trailhead_geo_point", FieldType: "STRING", Nullable: false, Metadata: gpxGeospatialMetadata(geospatialcore.LogicalTypeGeoPoint, geospatialcore.CoordinateOrderLatLon)},
		{Name: "route_geojson", FieldType: "STRING", Nullable: false, Metadata: gpxGeospatialMetadata(geospatialcore.LogicalTypeGeoJSON, geospatialcore.CoordinateOrderLonLat)},
		{Name: "route_bbox", FieldType: "STRING", Nullable: false, Metadata: gpxGeospatialMetadata(geospatialcore.LogicalTypeBoundingBox, geospatialcore.CoordinateOrderLonLat)},
		{Name: "route_geometry_type", FieldType: "STRING", Nullable: false},
		{Name: "route_coordinate_count", FieldType: "INTEGER", Nullable: false},
	}}
}

func gpxGeospatialMetadata(logicalType geospatialcore.GeospatialLogicalType, order geospatialcore.CoordinateOrder) map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"logical_type":     strictRawString(string(logicalType)),
		"crs":              strictRawString("EPSG:4326"),
		"coordinate_order": strictRawString(string(order)),
	}
}

func strictRawString(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
