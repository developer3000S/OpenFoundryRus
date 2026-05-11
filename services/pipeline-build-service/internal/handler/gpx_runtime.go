package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func (rt *lightweightTableRuntime) runGPXParse(node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	if inline := strings.TrimSpace(firstNonEmpty(cfg.GPXContent, cfg.GPXXML, cfg.GPX)); inline != "" {
		trail, err := geospatialcore.ParseGPXTrail([]byte(inline), geospatialcore.GPXParseOptions{
			TrailID:    cfg.TrailID,
			TrailName:  cfg.TrailName,
			SourceName: cfg.SourceName,
		})
		if err != nil {
			return nil, fmt.Errorf("lightweight_gpx_parse: %w", err)
		}
		row, err := gpxTrailToRuntimeRow(trail)
		if err != nil {
			return nil, err
		}
		return []pipelineexpression.Row{row}, nil
	}

	inputRows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	out := make([]pipelineexpression.Row, 0, len(inputRows))
	for index, row := range inputRows {
		gpxColumn := firstNonEmpty(cfg.GPXColumn, cfg.SourceColumn, cfg.ContentColumn)
		if strings.TrimSpace(gpxColumn) == "" {
			gpxColumn = detectGPXSourceColumn(row)
		}
		if strings.TrimSpace(gpxColumn) == "" {
			return nil, fmt.Errorf("lightweight_gpx_parse_missing_source_column: row %d has no GPX content column", index)
		}
		raw, ok := row[gpxColumn]
		if !ok {
			return nil, fmt.Errorf("lightweight_gpx_parse_missing_source_column: column %q does not exist", gpxColumn)
		}
		trail, err := geospatialcore.ParseGPXTrail([]byte(runtimeScalarString(raw)), geospatialcore.GPXParseOptions{
			TrailID:    runtimeConfigOrColumnValue(cfg.TrailID, cfg.TrailIDColumn, row),
			TrailName:  runtimeConfigOrColumnValue(cfg.TrailName, cfg.TrailNameColumn, row),
			SourceName: runtimeConfigOrColumnValue(cfg.SourceName, firstNonEmpty(cfg.SourceNameColumn, cfg.FileNameColumn), row),
		})
		if err != nil {
			return nil, fmt.Errorf("lightweight_gpx_parse row %d: %w", index, err)
		}
		parsed, err := gpxTrailToRuntimeRow(trail)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

func detectGPXSourceColumn(row pipelineexpression.Row) string {
	for _, candidate := range []string{"gpx", "gpx_xml", "gpx_content", "raw_gpx", "content", "file_content", "body"} {
		if _, ok := row[candidate]; ok {
			return candidate
		}
	}
	for column, raw := range row {
		if strings.Contains(strings.ToLower(runtimeScalarString(raw)), "<gpx") {
			return column
		}
	}
	return ""
}

func runtimeConfigOrColumnValue(configValue, column string, row pipelineexpression.Row) string {
	if strings.TrimSpace(configValue) != "" {
		return strings.TrimSpace(configValue)
	}
	if strings.TrimSpace(column) == "" {
		return ""
	}
	raw, ok := row[column]
	if !ok {
		return ""
	}
	return runtimeScalarString(raw)
}

func gpxTrailToRuntimeRow(trail geospatialcore.GPXTrail) (pipelineexpression.Row, error) {
	route, err := trail.RouteGeoJSONString()
	if err != nil {
		return nil, fmt.Errorf("serialize GPX route GeoJSON: %w", err)
	}
	bbox, err := trail.RouteBBoxJSONString()
	if err != nil {
		return nil, fmt.Errorf("serialize GPX route bbox: %w", err)
	}
	trailhead, err := trail.TrailheadOntologyString()
	if err != nil {
		return nil, fmt.Errorf("serialize GPX trailhead: %w", err)
	}
	return pipelineexpression.Row{
		"trail_id":               mustRuntimeJSON(trail.TrailID),
		"trail_name":             mustRuntimeJSON(trail.TrailName),
		"point_count":            mustRuntimeJSON(trail.PointCount),
		"distance_meters":        mustRuntimeJSON(trail.DistanceMeters),
		"distance_miles":         mustRuntimeJSON(trail.DistanceMiles),
		"elevation_gain_meters":  mustRuntimeJSON(trail.ElevationGainMeters),
		"elevation_gain_ft":      mustRuntimeJSON(trail.ElevationGainFeet),
		"min_elevation_meters":   nullableRuntimeFloat(trail.MinElevationMeters),
		"max_elevation_meters":   nullableRuntimeFloat(trail.MaxElevationMeters),
		"min_elevation_ft":       nullableRuntimeFloat(trail.MinElevationFeet),
		"max_elevation_ft":       nullableRuntimeFloat(trail.MaxElevationFeet),
		"start_lat":              mustRuntimeJSON(trail.StartPoint.Lat),
		"start_lon":              mustRuntimeJSON(trail.StartPoint.Lon),
		"end_lat":                mustRuntimeJSON(trail.EndPoint.Lat),
		"end_lon":                mustRuntimeJSON(trail.EndPoint.Lon),
		"trailhead_geo_point":    mustRuntimeJSON(trailhead),
		"route_geojson":          mustRuntimeJSON(route),
		"route_bbox":             mustRuntimeJSON(bbox),
		"route_geometry_type":    mustRuntimeJSON(string(trail.RouteGeoJSON.Type)),
		"route_coordinate_count": mustRuntimeJSON(trail.PointCount),
	}, nil
}

func nullableRuntimeFloat(value *float64) json.RawMessage {
	if value == nil {
		return json.RawMessage("null")
	}
	return mustRuntimeJSON(*value)
}
