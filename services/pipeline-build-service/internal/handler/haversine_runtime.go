package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
)

type haversineRuntimeColumns struct {
	StartLat string
	StartLon string
	EndLat   string
	EndLon   string
}

func applyRuntimeHaversineBlock(rows []pipelineexpression.Row, block runtimeTransformBlock) ([]pipelineexpression.Row, error) {
	columns := haversineColumnsFromBlock(block)
	if columns.StartLat == "" || columns.StartLon == "" || columns.EndLat == "" || columns.EndLon == "" {
		return nil, fmt.Errorf("lightweight_haversine_missing_coordinate_column")
	}
	unit := haversineUnitFromBlock(block)
	if _, err := geospatialcore.ParseDistanceUnit(unit); err != nil {
		return nil, fmt.Errorf("lightweight_haversine_invalid_unit:%w", err)
	}
	target := haversineTargetColumn(block.TargetColumn, unit)
	out := cloneRows(rows)
	for rowIndex, row := range out {
		lat1, null, err := runtimeNullableFloat(row, columns.StartLat)
		if err != nil {
			return nil, fmt.Errorf("lightweight_haversine row %d column %s: %w", rowIndex, columns.StartLat, err)
		}
		if null {
			row[target] = json.RawMessage("null")
			continue
		}
		lon1, null, err := runtimeNullableFloat(row, columns.StartLon)
		if err != nil {
			return nil, fmt.Errorf("lightweight_haversine row %d column %s: %w", rowIndex, columns.StartLon, err)
		}
		if null {
			row[target] = json.RawMessage("null")
			continue
		}
		lat2, null, err := runtimeNullableFloat(row, columns.EndLat)
		if err != nil {
			return nil, fmt.Errorf("lightweight_haversine row %d column %s: %w", rowIndex, columns.EndLat, err)
		}
		if null {
			row[target] = json.RawMessage("null")
			continue
		}
		lon2, null, err := runtimeNullableFloat(row, columns.EndLon)
		if err != nil {
			return nil, fmt.Errorf("lightweight_haversine row %d column %s: %w", rowIndex, columns.EndLon, err)
		}
		if null {
			row[target] = json.RawMessage("null")
			continue
		}
		distance, err := geospatialcore.HaversineDistance(lat1, lon1, lat2, lon2, unit)
		if err != nil {
			return nil, fmt.Errorf("lightweight_haversine row %d: %w", rowIndex, err)
		}
		row[target] = mustRuntimeJSON(distance)
	}
	return out, nil
}

func haversineColumnsFromBlock(block runtimeTransformBlock) haversineRuntimeColumns {
	return haversineRuntimeColumns{
		StartLat: strings.TrimSpace(firstNonEmpty(block.StartLatColumn, block.Lat1Column)),
		StartLon: strings.TrimSpace(firstNonEmpty(block.StartLonColumn, block.Lon1Column)),
		EndLat:   strings.TrimSpace(firstNonEmpty(block.EndLatColumn, block.Lat2Column)),
		EndLon:   strings.TrimSpace(firstNonEmpty(block.EndLonColumn, block.Lon2Column)),
	}
}

func haversineUnitFromBlock(block runtimeTransformBlock) string {
	return strings.TrimSpace(firstNonEmpty(block.Unit, "miles"))
}

func haversineTargetColumn(targetColumn, unit string) string {
	if strings.TrimSpace(targetColumn) != "" {
		return strings.TrimSpace(targetColumn)
	}
	parsed, err := geospatialcore.ParseDistanceUnit(unit)
	if err != nil {
		return "distance_miles"
	}
	switch parsed {
	case geospatialcore.DistanceUnitMeters:
		return "distance_meters"
	case geospatialcore.DistanceUnitKilometers:
		return "distance_km"
	default:
		return "distance_miles"
	}
}

func runtimeNullableFloat(row pipelineexpression.Row, column string) (float64, bool, error) {
	raw, ok := row[column]
	if !ok || isRuntimeNullish(raw, true) {
		return 0, true, nil
	}
	if value, ok := runtimeScalarFloat(raw); ok {
		return value, false, nil
	}
	return 0, false, fmt.Errorf("expected numeric coordinate, got %s", runtimeScalarString(raw))
}
