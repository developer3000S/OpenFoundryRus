package handler

import (
	"fmt"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
)

func applyHaversineSchema(nodeID string, schema pipelineStrictSchema, block runtimeTransformBlock, report *pipelineStrictValidationReport) pipelineStrictSchema {
	columns := haversineColumnsFromBlock(block)
	for label, column := range map[string]string{
		"start_lat_column": columns.StartLat,
		"start_lon_column": columns.StartLon,
		"end_lat_column":   columns.EndLat,
		"end_lon_column":   columns.EndLon,
	} {
		if strings.TrimSpace(column) == "" {
			report.addError(nodeID, nil, "haversine_missing_coordinate_column", fmt.Sprintf("haversine distance requires %s", label))
		}
	}
	unit := haversineUnitFromBlock(block)
	if _, err := geospatialcore.ParseDistanceUnit(unit); err != nil {
		report.addError(nodeID, nil, "haversine_invalid_unit", err.Error())
	}
	target := haversineTargetColumn(block.TargetColumn, unit)
	if strings.TrimSpace(target) == "" {
		report.addError(nodeID, nil, "haversine_missing_target_column", "haversine distance requires a target column")
		return schema
	}
	if !schema.Known {
		return schema
	}
	for _, column := range []string{columns.StartLat, columns.StartLon, columns.EndLat, columns.EndLon} {
		if strings.TrimSpace(column) == "" {
			continue
		}
		field, ok := schema.field(column)
		if !ok {
			report.addError(nodeID, strPtr(column), "missing_column", fmt.Sprintf("coordinate column %q does not exist in upstream schema", column))
			continue
		}
		if !fieldType(field).IsNumeric() {
			report.addError(nodeID, strPtr(column), "incompatible_haversine_column", fmt.Sprintf("coordinate column %q must be numeric, got %s", column, field.FieldType))
		}
	}
	fields := cloneStrictFields(schema.Fields)
	replaced := false
	for i, field := range fields {
		if field.Name == target {
			fields[i] = pipelineStrictValidationField{Name: target, FieldType: strictTypeName(pipelineexpression.DoubleType()), Nullable: true}
			replaced = true
			break
		}
	}
	if !replaced {
		fields = append(fields, pipelineStrictValidationField{Name: target, FieldType: strictTypeName(pipelineexpression.DoubleType()), Nullable: true})
	}
	checkSchemaDuplicateFields(nodeID, pipelineStrictSchema{Known: true, Fields: fields}, report)
	return pipelineStrictSchema{Known: true, Fields: fields}
}
