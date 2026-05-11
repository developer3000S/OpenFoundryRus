package handler

import (
	"net/http"
	"strings"
)

const pipelineTransformCatalogVersion = "pipeline_transform_catalog.v1"

type pipelineTransformCatalogResponse struct {
	SchemaVersion string                          `json:"schema_version"`
	Categories    []pipelineTransformCategory     `json:"categories"`
	Transforms    []pipelineTransformCatalogEntry `json:"transforms"`
}

type pipelineTransformCategory struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type pipelineTransformCatalogEntry struct {
	ID              string                            `json:"id"`
	Label           string                            `json:"label"`
	Description     string                            `json:"description"`
	Category        string                            `json:"category"`
	TransformType   string                            `json:"transform_type"`
	ConfigKind      string                            `json:"config_kind"`
	BuilderSurface  string                            `json:"builder_surface"`
	ExecutionStatus string                            `json:"execution_status"`
	Runtime         string                            `json:"runtime"`
	Icon            string                            `json:"icon"`
	Tags            []string                          `json:"tags"`
	Docs            []string                          `json:"docs"`
	Function        *pipelineTransformCatalogFunction `json:"function,omitempty"`
	DefaultConfig   map[string]any                    `json:"default_config"`
	Form            pipelineTransformCatalogForm      `json:"form"`
	OutputContract  pipelineTransformOutputContract   `json:"output_contract"`
}

type pipelineTransformCatalogFunction struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Version    string                      `json:"version"`
	Runtime    PipelineFunctionRuntime     `json:"runtime"`
	ResultType string                      `json:"result_type"`
	Parameters []PipelineFunctionParameter `json:"parameters"`
}

type pipelineTransformCatalogForm struct {
	Kind   string                          `json:"kind"`
	Fields []pipelineTransformCatalogField `json:"fields"`
}

type pipelineTransformCatalogField struct {
	Name        string                           `json:"name"`
	Label       string                           `json:"label"`
	FieldType   string                           `json:"field_type"`
	Required    bool                             `json:"required"`
	Repeated    bool                             `json:"repeated,omitempty"`
	Default     any                              `json:"default,omitempty"`
	Placeholder string                           `json:"placeholder,omitempty"`
	HelpText    string                           `json:"help_text,omitempty"`
	Options     []pipelineTransformCatalogOption `json:"options,omitempty"`
}

type pipelineTransformCatalogOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type pipelineTransformOutputContract struct {
	Mode        string `json:"mode"`
	Description string `json:"description"`
}

// ListPipelineTransformCatalog serves the versioned Pipeline Builder transform
// catalog consumed by the canvas editor. The base catalog is static; reusable
// functions are appended from the injected registry so versioned UDFs can
// participate without changing the stable catalog schema.
func ListPipelineTransformCatalog(w http.ResponseWriter, r *http.Request) {
	response := transformCatalogV1()
	if ports, ok := currentExecutionPorts(); ok && ports.Functions != nil {
		defs, err := ports.Functions.ListPipelineFunctions(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pipeline_function_catalog_failed", "detail": err.Error()})
			return
		}
		response = appendPipelineFunctionCatalog(response, defs)
	}
	writeJSON(w, http.StatusOK, response)
}

func transformCatalogV1() pipelineTransformCatalogResponse {
	docs := []string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/transforms-overview/",
		"https://www.palantir.com/docs/foundry/pipeline-builder/functions-index/",
	}
	return pipelineTransformCatalogResponse{
		SchemaVersion: pipelineTransformCatalogVersion,
		Categories: []pipelineTransformCategory{
			{ID: "column_shaping", Label: "Column shaping", Description: "Select, drop, rename, cast, derive, and normalize columns."},
			{ID: "row_filtering", Label: "Row filtering", Description: "Keep, drop, and order rows."},
			{ID: "relational", Label: "Relational", Description: "Join, union, aggregate, and explode tabular inputs."},
			{ID: "geospatial", Label: "Geospatial", Description: "Create trail geometries and distance measures from coordinate columns."},
			{ID: "parsing", Label: "Parsing", Description: "Extract structured columns from JSON, CSV, and GPX payloads."},
			{ID: "ai_assisted", Label: "AI assisted", Description: "LLM nodes and AIP generated transformation logic."},
			{ID: "reusable_functions", Label: "Reusable functions", Description: "Versioned expression and Python functions callable from pipeline nodes."},
			{ID: "outputs", Label: "Outputs", Description: "Map produced columns into dataset or ontology outputs."},
		},
		Transforms: []pipelineTransformCatalogEntry{
			transformCatalogSelect(docs),
			transformCatalogDrop(docs),
			transformCatalogRename(docs),
			transformCatalogCast(docs),
			transformCatalogFilter(docs),
			transformCatalogFormula(docs),
			transformCatalogDeriveColumn(docs),
			transformCatalogNormalizeColumns(docs),
			transformCatalogNormalizeUnits(docs),
			transformCatalogSort(docs),
			transformCatalogJoin(docs),
			transformCatalogUnion(docs),
			transformCatalogHaversineDistance(docs),
			transformCatalogGeoIntersectionJoin(docs),
			transformCatalogGeoDistanceJoin(docs),
			transformCatalogGeoNearestJoin(docs),
			transformCatalogAggregate(docs),
			transformCatalogExplode(docs),
			transformCatalogJSONExtract(docs),
			transformCatalogCSVParse(docs),
			transformCatalogGPXParse(docs),
			transformCatalogPythonTransform(docs),
			transformCatalogLLMNode(docs),
			transformCatalogOutputMapping(docs),
		},
	}
}

func appendPipelineFunctionCatalog(response pipelineTransformCatalogResponse, defs []PipelineFunctionDefinition) pipelineTransformCatalogResponse {
	docs := []string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/transforms-overview/",
		"https://www.palantir.com/docs/foundry/transforms-python/transforms-python-api",
	}
	for _, raw := range defs {
		def := normalisePipelineFunction(raw)
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		entry := baseTransformCatalogEntry(
			"function."+def.Name+"@"+def.Version,
			def.DisplayName,
			firstNonEmpty(def.Description, "Call the "+def.Name+" reusable function and write its result into a target column."),
			"reusable_functions",
			"function_call",
			"function_node",
			functionCatalogExecutionStatus(def),
			functionCatalogRuntime(def),
			"fx",
			append(docs, def.Docs...),
		)
		entry.TransformType = "function"
		entry.Tags = []string{"function", "udf", string(def.Runtime), "versioned"}
		entry.Function = &pipelineTransformCatalogFunction{
			ID:         def.ID,
			Name:       def.Name,
			Version:    def.Version,
			Runtime:    def.Runtime,
			ResultType: def.ResultType,
			Parameters: append([]PipelineFunctionParameter(nil), def.Parameters...),
		}
		entry.DefaultConfig = map[string]any{
			"function_id":           def.ID,
			"function_name":         def.Name,
			"function_version":      def.Version,
			"function_auto_upgrade": false,
			"target_column":         def.Name,
			"result_type":           def.ResultType,
			"arguments":             defaultFunctionArguments(def.Parameters),
		}
		entry.Form.Fields = functionCatalogFields(def)
		entry.OutputContract = pipelineTransformOutputContract{
			Mode:        "schema_add_column",
			Description: "Output schema adds or replaces the target column with the function result.",
		}
		response.Transforms = append(response.Transforms, entry)
	}
	return response
}

func functionCatalogExecutionStatus(def PipelineFunctionDefinition) string {
	if def.Runtime == PipelineFunctionRuntimePython {
		return "requires_python_sidecar"
	}
	return "available"
}

func functionCatalogRuntime(def PipelineFunctionDefinition) string {
	if def.Runtime == PipelineFunctionRuntimePython {
		return "python_sidecar"
	}
	return "pipeline-expression"
}

func defaultFunctionArguments(params []PipelineFunctionParameter) []map[string]string {
	out := make([]map[string]string, 0, len(params))
	for _, param := range params {
		if strings.TrimSpace(param.Name) == "" {
			continue
		}
		out = append(out, map[string]string{"name": param.Name, "expression": param.Name})
	}
	return out
}

func functionCatalogFields(def PipelineFunctionDefinition) []pipelineTransformCatalogField {
	fields := []pipelineTransformCatalogField{
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: true, Default: def.Name, HelpText: "Column written with the function result."},
		{Name: "function_auto_upgrade", Label: "Auto-upgrade version", FieldType: "boolean", Required: false, Default: false, HelpText: "When enabled, resolve the latest registered version at run time."},
	}
	for _, param := range def.Parameters {
		if strings.TrimSpace(param.Name) == "" {
			continue
		}
		fields = append(fields, pipelineTransformCatalogField{
			Name:        "args." + param.Name,
			Label:       param.Name,
			FieldType:   "expression",
			Required:    param.Required,
			Default:     param.Name,
			Placeholder: param.Name,
			HelpText:    firstNonEmpty(param.Description, "Expression evaluated for "+param.Name+" ("+param.Type+")."),
		})
	}
	return fields
}

func baseTransformCatalogEntry(id, label, description, category, configKind, surface, status, runtime, icon string, docs []string) pipelineTransformCatalogEntry {
	return pipelineTransformCatalogEntry{
		ID:              id,
		Label:           label,
		Description:     description,
		Category:        category,
		TransformType:   "sql",
		ConfigKind:      configKind,
		BuilderSurface:  surface,
		ExecutionStatus: status,
		Runtime:         runtime,
		Icon:            icon,
		Tags:            []string{},
		Docs:            append([]string(nil), docs...),
		DefaultConfig:   map[string]any{},
		Form:            pipelineTransformCatalogForm{Kind: "typed", Fields: []pipelineTransformCatalogField{}},
		OutputContract:  pipelineTransformOutputContract{Mode: "schema_preserving", Description: "Keeps the upstream schema unless configured otherwise."},
	}
}

func transformCatalogSelect(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("select", "Select columns", "Project an explicit ordered set of columns from the upstream table.", "column_shaping", "stack_block", "transform_stack", "available", "lightweight_table", "columns", docs)
	entry.Tags = []string{"projection", "schema"}
	entry.DefaultConfig = map[string]any{"kind": "select", "applied": false, "columns": []string{}}
	entry.Form.Fields = []pipelineTransformCatalogField{columnsField("columns", "Columns", true, "Columns to keep, in output order.")}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_projection", Description: "Output schema is exactly the selected columns, in order."}
	return entry
}

func transformCatalogDrop(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("drop", "Drop columns", "Remove one or more columns from the upstream table.", "column_shaping", "stack_block", "transform_stack", "available", "lightweight_table", "columns", docs)
	entry.Tags = []string{"projection", "schema"}
	entry.DefaultConfig = map[string]any{"kind": "drop", "applied": false, "columns": []string{}}
	entry.Form.Fields = []pipelineTransformCatalogField{columnsField("columns", "Columns", true, "Columns to remove from the output.")}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_minus_columns", Description: "Output schema removes the configured columns."}
	return entry
}

func transformCatalogRename(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("rename", "Rename columns", "Rename one or more upstream columns while preserving values and types.", "column_shaping", "stack_block", "transform_stack", "available", "lightweight_table", "rename", docs)
	entry.Tags = []string{"schema", "aliases"}
	entry.DefaultConfig = map[string]any{"kind": "rename", "applied": false, "renames": []map[string]string{{"from": "", "to": ""}}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "renames", Label: "Rename mappings", FieldType: "column_mapping", Required: true, Repeated: true, HelpText: "Each mapping requires an upstream column and a unique output name."},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_rename", Description: "Output schema renames configured fields and preserves unmodified fields."}
	return entry
}

func transformCatalogCast(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("cast", "Cast column", "Convert a column to another supported scalar type.", "column_shaping", "stack_block", "transform_stack", "available", "lightweight_table", "cast", docs)
	entry.Tags = []string{"types", "schema"}
	entry.DefaultConfig = map[string]any{"kind": "cast", "applied": false, "source_column": "", "target_type": "Timestamp", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("source_column", "Source column", true, "Column to convert."),
		{Name: "target_type", Label: "Target type", FieldType: "select", Required: true, Default: "Timestamp", Options: scalarTypeOptions()},
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: false, Placeholder: "Defaults to source column", HelpText: "Leave blank to replace the source column."},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_type_change", Description: "Output schema changes the target column type."}
	return entry
}

func transformCatalogFilter(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("filter", "Filter rows", "Keep or drop rows using one or more typed conditions.", "row_filtering", "stack_block", "transform_stack", "available", "lightweight_table", "filter", docs)
	entry.Tags = []string{"predicate", "rows"}
	entry.DefaultConfig = map[string]any{"kind": "filter", "applied": false, "mode": "keep", "match": "all", "conditions": []map[string]any{{"column": "", "operator": "is_not_null", "value": "", "treat_empty_as_null": true}}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "mode", Label: "Mode", FieldType: "select", Required: true, Default: "keep", Options: valueLabelOptions("keep", "Keep rows", "drop", "Drop rows")},
		{Name: "match", Label: "Condition match", FieldType: "select", Required: true, Default: "all", Options: valueLabelOptions("all", "All conditions", "any", "Any condition")},
		{Name: "conditions", Label: "Conditions", FieldType: "condition_group", Required: true, Repeated: true, HelpText: "Conditions are validated against upstream columns before build."},
	}
	return entry
}

func transformCatalogFormula(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("formula", "Formula", "Compute a value from a Pipeline Builder expression and write it into a target column.", "column_shaping", "stack_block", "transform_stack", "planned", "catalog_only", "fx", docs)
	entry.Tags = []string{"expression", "functions"}
	entry.DefaultConfig = map[string]any{"kind": "formula", "applied": false, "expression": "", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "expression", Label: "Expression", FieldType: "expression", Required: true, Placeholder: "distance_miles * 1.60934"},
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: true, Placeholder: "distance_km"},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_column", Description: "Output schema adds or replaces the target column."}
	return entry
}

func transformCatalogDeriveColumn(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("derive_column", "Derive column", "Create a new column from an expression without editing the source columns.", "column_shaping", "stack_block", "transform_stack", "planned", "catalog_only", "derive", docs)
	entry.Tags = []string{"expression", "schema"}
	entry.DefaultConfig = map[string]any{"kind": "derive_column", "applied": false, "expression": "", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "target_column", Label: "New column", FieldType: "text", Required: true, Placeholder: "effort_score"},
		{Name: "expression", Label: "Expression", FieldType: "expression", Required: true, Placeholder: "(distance_miles * 0.7) + (elevation_gain_ft / 500)"},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_column", Description: "Output schema appends the derived column."}
	return entry
}

func transformCatalogNormalizeColumns(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("normalize_columns", "Normalize column names", "Convert column names to a predictable snake_case style.", "column_shaping", "stack_block", "transform_stack", "available", "lightweight_table", "normalize", docs)
	entry.Tags = []string{"schema", "cleanup"}
	entry.DefaultConfig = map[string]any{"kind": "normalize", "applied": false, "remove_special_characters": false}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "remove_special_characters", Label: "Remove special characters", FieldType: "boolean", Required: false, Default: false},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_rename", Description: "Output schema renames every column according to the normalization rule."}
	return entry
}

func transformCatalogNormalizeUnits(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("normalize_units", "Normalize units", "Convert numeric measures between common unit systems.", "column_shaping", "stack_block", "transform_stack", "planned", "catalog_only", "units", docs)
	entry.Tags = []string{"units", "numeric"}
	entry.DefaultConfig = map[string]any{"kind": "normalize_units", "applied": false, "source_column": "", "source_unit": "", "target_unit": "", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("source_column", "Source column", true, "Numeric column to convert."),
		{Name: "source_unit", Label: "Source unit", FieldType: "text", Required: true, Placeholder: "meters"},
		{Name: "target_unit", Label: "Target unit", FieldType: "text", Required: true, Placeholder: "miles"},
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: false, Placeholder: "distance_miles"},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_or_replace_column", Description: "Output schema adds or replaces the target measure column."}
	return entry
}

func transformCatalogSort(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("sort", "Sort rows", "Order rows by one or more columns.", "row_filtering", "stack_block", "transform_stack", "planned", "catalog_only", "sort", docs)
	entry.Tags = []string{"order", "rows"}
	entry.DefaultConfig = map[string]any{"kind": "sort", "applied": false, "sort_keys": []map[string]string{{"column": "", "direction": "asc"}}}
	entry.Form.Fields = []pipelineTransformCatalogField{{Name: "sort_keys", Label: "Sort keys", FieldType: "sort_keys", Required: true, Repeated: true}}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_preserving", Description: "Output schema is unchanged; row order changes."}
	return entry
}

func transformCatalogJoin(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("join", "Join", "Combine two inputs using key matches or a cross join.", "relational", "join_draft", "join_editor", "available", "lightweight_table", "join", docs)
	entry.Tags = []string{"relational", "multi_input"}
	entry.DefaultConfig = map[string]any{"_join": map[string]any{"join_type": "left", "matches": []map[string]string{{"left_column": "", "right_column": ""}}, "auto_select_left": true, "auto_select_right": false}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "join_type", Label: "Join type", FieldType: "select", Required: true, Default: "left", Options: valueLabelOptions("left", "Left", "right", "Right", "inner", "Inner", "outer", "Full outer", "cross", "Cross")},
		{Name: "matches", Label: "Match conditions", FieldType: "join_matches", Required: true, Repeated: true},
		columnsField("left_columns", "Left columns", false, "Optional projection from the left input."),
		columnsField("right_columns", "Right columns", false, "Optional projection from the right input."),
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_join", Description: "Output schema combines selected left and right columns."}
	return entry
}

func transformCatalogUnion(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("union", "Union", "Append rows from two or more inputs by name or by position.", "relational", "union_draft", "union_editor", "available", "lightweight_table", "union", docs)
	entry.Tags = []string{"relational", "multi_input"}
	entry.DefaultConfig = map[string]any{"_union": map[string]any{"union_type": "by_name"}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "union_type", Label: "Union type", FieldType: "select", Required: true, Default: "by_name", Options: valueLabelOptions("by_name", "By column name", "by_position", "By position")},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_union", Description: "Output schema is reconciled from all union inputs."}
	return entry
}

func transformCatalogHaversineDistance(docs []string) pipelineTransformCatalogEntry {
	geoDocs := append([]string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/transforms-geospatial",
	}, docs...)
	entry := baseTransformCatalogEntry("haversine_distance", "Haversine distance", "Calculate great-circle distance between two latitude/longitude pairs and write it to a numeric column.", "geospatial", "stack_block", "transform_stack", "available", "lightweight_table", "route", geoDocs)
	entry.Tags = []string{"geospatial", "distance", "haversine", "lat_lon"}
	entry.DefaultConfig = map[string]any{
		"kind":             "haversine_distance",
		"applied":          false,
		"start_lat_column": "",
		"start_lon_column": "",
		"end_lat_column":   "",
		"end_lon_column":   "",
		"unit":             "miles",
		"target_column":    "distance_miles",
	}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("start_lat_column", "Start latitude", true, "Latitude column for the first point."),
		columnField("start_lon_column", "Start longitude", true, "Longitude column for the first point."),
		columnField("end_lat_column", "End latitude", true, "Latitude column for the second point."),
		columnField("end_lon_column", "End longitude", true, "Longitude column for the second point."),
		{Name: "unit", Label: "Distance unit", FieldType: "select", Required: true, Default: "miles", Options: valueLabelOptions("miles", "Miles", "km", "Kilometers", "meters", "Meters")},
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: true, Default: "distance_miles", Placeholder: "distance_miles"},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_column", Description: "Output schema adds or replaces the target distance column as nullable DOUBLE."}
	return entry
}

func transformCatalogGeoIntersectionJoin(docs []string) pipelineTransformCatalogEntry {
	entry := transformCatalogGeoJoinBase("geo_intersection_join", "Geometry intersection join", "Combine two inputs when their GeoJSON geometries intersect.", "intersection", docs)
	entry.Tags = []string{"geospatial", "join", "intersection", "geometry"}
	entry.DefaultConfig = map[string]any{
		"_geo_join": map[string]any{
			"mode":                  "intersection",
			"join_type":             "inner",
			"left_geometry_column":  "",
			"right_geometry_column": "",
			"right_prefix":          "right_",
			"auto_select_left":      true,
			"auto_select_right":     true,
			"max_left_rows":         defaultGeoJoinMaxLeftRows,
			"max_right_rows":        defaultGeoJoinMaxRightRows,
			"max_candidate_pairs":   defaultGeoJoinMaxCandidatePairs,
		},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_geo_join", Description: "Output schema combines selected left and right columns for intersecting geometries."}
	return entry
}

func transformCatalogGeoDistanceJoin(docs []string) pipelineTransformCatalogEntry {
	entry := transformCatalogGeoJoinBase("geo_distance_join", "Geometry distance join", "Combine two inputs when their geometries are within a configured distance threshold.", "distance", docs)
	entry.Tags = []string{"geospatial", "join", "distance", "geometry"}
	entry.DefaultConfig = map[string]any{
		"_geo_join": map[string]any{
			"mode":                  "distance",
			"join_type":             "inner",
			"left_geometry_column":  "",
			"right_geometry_column": "",
			"unit":                  "miles",
			"max_distance":          1.0,
			"distance_column":       "geo_distance_miles",
			"right_prefix":          "right_",
			"auto_select_left":      true,
			"auto_select_right":     true,
			"max_left_rows":         defaultGeoJoinMaxLeftRows,
			"max_right_rows":        defaultGeoJoinMaxRightRows,
			"max_candidate_pairs":   defaultGeoJoinMaxCandidatePairs,
		},
	}
	entry.Form.Fields = append(entry.Form.Fields,
		pipelineTransformCatalogField{Name: "max_distance", Label: "Max distance", FieldType: "number", Required: true, Default: 1.0, HelpText: "Only pairs at or below this distance are emitted."},
		pipelineTransformCatalogField{Name: "distance_column", Label: "Distance column", FieldType: "text", Required: true, Default: "geo_distance_miles"},
	)
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_geo_join", Description: "Output schema combines selected left and right columns and adds a nullable distance column."}
	return entry
}

func transformCatalogGeoNearestJoin(docs []string) pipelineTransformCatalogEntry {
	entry := transformCatalogGeoJoinBase("geo_nearest_neighbor_join", "Geometry nearest-neighbor join", "For each left row, attach the nearest K right-side geometries by great-circle distance.", "nearest", docs)
	entry.Tags = []string{"geospatial", "join", "nearest_neighbor", "knn", "geometry"}
	entry.DefaultConfig = map[string]any{
		"_geo_join": map[string]any{
			"mode":                  "nearest",
			"join_type":             "left",
			"left_geometry_column":  "",
			"right_geometry_column": "",
			"unit":                  "miles",
			"k":                     1,
			"distance_column":       "geo_distance_miles",
			"rank_column":           "geo_rank",
			"right_prefix":          "right_",
			"auto_select_left":      true,
			"auto_select_right":     true,
			"max_left_rows":         defaultGeoJoinMaxLeftRows,
			"max_right_rows":        defaultGeoJoinMaxRightRows,
			"max_candidate_pairs":   defaultGeoJoinMaxCandidatePairs,
		},
	}
	entry.Form.Fields = append(entry.Form.Fields,
		pipelineTransformCatalogField{Name: "k", Label: "Nearest neighbors", FieldType: "number", Required: true, Default: 1, HelpText: "Maximum neighbors per left row. Runtime limit is 50."},
		pipelineTransformCatalogField{Name: "max_distance", Label: "Optional max distance", FieldType: "number", Required: false, HelpText: "When set, neighbors farther away are ignored."},
		pipelineTransformCatalogField{Name: "distance_column", Label: "Distance column", FieldType: "text", Required: true, Default: "geo_distance_miles"},
		pipelineTransformCatalogField{Name: "rank_column", Label: "Rank column", FieldType: "text", Required: true, Default: "geo_rank"},
	)
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_geo_join", Description: "Output schema combines selected left/right columns and adds distance plus KNN rank columns."}
	return entry
}

func transformCatalogGeoJoinBase(id, label, description, mode string, docs []string) pipelineTransformCatalogEntry {
	geoDocs := append([]string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/transforms-geospatial",
	}, docs...)
	entry := baseTransformCatalogEntry(id, label, description, "geospatial", "geo_join", "geo_join_editor", "available", "lightweight_table", "geo", geoDocs)
	entry.TransformType = "geo_join"
	entry.DefaultConfig = map[string]any{"_geo_join": map[string]any{"mode": mode}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "mode", Label: "Geo join mode", FieldType: "select", Required: true, Default: mode, Options: valueLabelOptions("intersection", "Intersection", "distance", "Distance", "nearest", "Nearest neighbors")},
		columnField("left_geometry_column", "Left geometry", false, "GeoJSON geometry column from the left input. Use lat/lon fields when blank."),
		columnField("right_geometry_column", "Right geometry", false, "GeoJSON geometry column from the right input. Use lat/lon fields when blank."),
		columnField("left_lat_column", "Left latitude", false, "Latitude column fallback for point joins."),
		columnField("left_lon_column", "Left longitude", false, "Longitude column fallback for point joins."),
		columnField("right_lat_column", "Right latitude", false, "Latitude column fallback for point joins."),
		columnField("right_lon_column", "Right longitude", false, "Longitude column fallback for point joins."),
		{Name: "unit", Label: "Distance unit", FieldType: "select", Required: false, Default: "miles", Options: valueLabelOptions("miles", "Miles", "km", "Kilometers", "meters", "Meters")},
		columnsField("left_columns", "Left columns", false, "Optional projection from the left input."),
		columnsField("right_columns", "Right columns", false, "Optional projection from the right input."),
		{Name: "right_prefix", Label: "Right column prefix", FieldType: "text", Required: false, Default: "right_"},
		{Name: "max_left_rows", Label: "Max left rows", FieldType: "number", Required: true, Default: defaultGeoJoinMaxLeftRows},
		{Name: "max_right_rows", Label: "Max right rows", FieldType: "number", Required: true, Default: defaultGeoJoinMaxRightRows},
		{Name: "max_candidate_pairs", Label: "Max candidate pairs", FieldType: "number", Required: true, Default: defaultGeoJoinMaxCandidatePairs},
	}
	return entry
}

func transformCatalogAggregate(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("aggregate", "Aggregate", "Group rows and calculate aggregate measures.", "relational", "stack_block", "transform_stack", "planned", "catalog_only", "aggregate", docs)
	entry.Tags = []string{"group_by", "metrics"}
	entry.DefaultConfig = map[string]any{"kind": "aggregate", "applied": false, "group_by": []string{}, "aggregations": []map[string]string{{"function": "count", "source_column": "", "target_column": "row_count"}}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnsField("group_by", "Group by", false, "Columns used as grouping keys."),
		{Name: "aggregations", Label: "Aggregations", FieldType: "aggregations", Required: true, Repeated: true},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_aggregate", Description: "Output schema contains group keys and aggregate result columns."}
	return entry
}

func transformCatalogExplode(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("explode", "Explode", "Turn array-like values into multiple output rows.", "relational", "stack_block", "transform_stack", "planned", "catalog_only", "explode", docs)
	entry.Tags = []string{"arrays", "rows"}
	entry.DefaultConfig = map[string]any{"kind": "explode", "applied": false, "source_column": "", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("source_column", "Array column", true, "Column containing array-like values."),
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: false, Placeholder: "Defaults to source column"},
	}
	return entry
}

func transformCatalogJSONExtract(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("json_extract", "JSON extract", "Extract a value from a JSON object column using a path.", "parsing", "stack_block", "transform_stack", "planned", "catalog_only", "json", docs)
	entry.Tags = []string{"json", "parse"}
	entry.DefaultConfig = map[string]any{"kind": "json_extract", "applied": false, "source_column": "", "json_path": "$.", "target_column": ""}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("source_column", "JSON column", true, "Column containing JSON objects or strings."),
		{Name: "json_path", Label: "JSON path", FieldType: "text", Required: true, Default: "$.", Placeholder: "$.trail.name"},
		{Name: "target_column", Label: "Target column", FieldType: "text", Required: true, Placeholder: "trail_name"},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_column", Description: "Output schema adds the extracted value column."}
	return entry
}

func transformCatalogCSVParse(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("csv_parse", "CSV parse", "Parse CSV text into tabular columns.", "parsing", "stack_block", "transform_stack", "planned", "catalog_only", "csv", docs)
	entry.Tags = []string{"csv", "parse"}
	entry.DefaultConfig = map[string]any{"kind": "csv_parse", "applied": false, "source_column": "", "delimiter": ",", "has_header": true}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("source_column", "CSV column", true, "Column containing CSV text."),
		{Name: "delimiter", Label: "Delimiter", FieldType: "text", Required: true, Default: ","},
		{Name: "has_header", Label: "Has header", FieldType: "boolean", Required: false, Default: true},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_expand", Description: "Output schema expands parsed CSV fields into columns."}
	return entry
}

func transformCatalogGPXParse(docs []string) pipelineTransformCatalogEntry {
	gpxDocs := append([]string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/transforms-geospatial",
		"https://www.palantir.com/docs/foundry/code-examples/geospatial-computation-transforms/",
	}, docs...)
	entry := baseTransformCatalogEntry("gpx_parse", "GPX parse", "Parse GPX track or route XML into trail metrics, trailhead point, route bbox, and GeoJSON LineString columns.", "parsing", "gpx_parse", "gpx_parser", "available", "lightweight_table", "route", gpxDocs)
	entry.TransformType = "gpx_parse"
	entry.Tags = []string{"gpx", "geospatial", "geojson", "trail", "parse"}
	entry.DefaultConfig = map[string]any{
		"gpx_column":         "",
		"trail_name_column":  "",
		"trail_id_column":    "",
		"file_name_column":   "",
		"source_name_column": "",
	}
	entry.Form.Fields = []pipelineTransformCatalogField{
		columnField("gpx_column", "GPX XML column", true, "Column containing GPX XML text."),
		columnField("trail_name_column", "Trail name column", false, "Optional column overriding the GPX metadata/track name."),
		columnField("trail_id_column", "Trail ID column", false, "Optional stable ID column. Defaults to a slug of the trail name."),
		columnField("file_name_column", "File name column", false, "Optional upload filename used as a name fallback."),
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_replace_with_trail", Description: "Output schema is the Trail row contract with metrics, elevation, point coordinates, Ontology GeoPoint, bbox, and GeoJSON route."}
	return entry
}

func transformCatalogPythonTransform(docs []string) pipelineTransformCatalogEntry {
	pythonDocs := append([]string{
		"https://www.palantir.com/docs/foundry/transforms-python/lightweight-overview",
		"https://www.palantir.com/docs/foundry/transforms-python/pipelines",
	}, docs...)
	entry := baseTransformCatalogEntry("python_transform", "Python transform", "Run a Python sidecar transform with typed input/output contracts, package constraints, captured logs, and safe timeout defaults.", "parsing", "python_node", "code_node", "requires_python_sidecar", "python_sidecar", "python", pythonDocs)
	entry.TransformType = "python"
	entry.Tags = []string{"python", "sidecar", "code", "gpx", "json"}
	entry.DefaultConfig = map[string]any{
		"source":           "result_rows = input_rows\nrows_affected = len(result_rows)",
		"timeout_seconds":  defaultPythonNodeTimeoutSeconds,
		"packages":         []string{},
		"allowed_packages": []string{},
		"input_schema":     map[string]any{"fields": []map[string]any{}},
		"output_schema":    map[string]any{"fields": []map[string]any{}},
	}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "source", Label: "Python source", FieldType: "code_python", Required: true, HelpText: "The sidecar receives config, prepared_inputs, input_rows, input_dataset_ids, and output_dataset_id."},
		{Name: "timeout_seconds", Label: "Timeout seconds", FieldType: "number", Required: false, Default: defaultPythonNodeTimeoutSeconds, HelpText: "Clamped to the service maximum before sidecar execution."},
		{Name: "packages", Label: "Declared packages", FieldType: "string_list", Required: false, Repeated: true, HelpText: "Optional package names or constraints documented by this node."},
		{Name: "allowed_packages", Label: "Allowed packages", FieldType: "string_list", Required: false, Repeated: true, HelpText: "When set, imported and declared packages must be listed here."},
		{Name: "input_schema", Label: "Input contract", FieldType: "schema", Required: false, HelpText: "Optional typed input schema contract."},
		{Name: "output_schema", Label: "Output contract", FieldType: "schema", Required: false, HelpText: "Typed result_rows schema used by validation and downstream output nodes."},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_from_python_output_contract", Description: "Output schema comes from the configured output_schema or sidecar result_rows."}
	return entry
}

func transformCatalogLLMNode(docs []string) pipelineTransformCatalogEntry {
	llmDocs := append([]string{
		"https://www.palantir.com/docs/foundry/pipeline-builder/pipeline-builder-llm",
		"https://www.palantir.com/docs/foundry/pipeline-builder/pipeline-builder-aip/index.html",
	}, docs...)
	entry := baseTransformCatalogEntry("llm_node", "Use LLM", "Apply an LLM prompt to upstream rows and write the response into a configured output column.", "ai_assisted", "llm_prompt", "llm_node", "requires_ai_service", "ai_service", "sparkles", llmDocs)
	entry.TransformType = "llm"
	entry.Tags = []string{"llm", "aip", "prompt", "ai_service"}
	entry.DefaultConfig = map[string]any{
		"prompt":        "",
		"system_prompt": "You are transforming tabular pipeline data. Return concise values suitable for a table cell.",
		"input_column":  "",
		"output_column": "llm_output",
		"output_type":   "string",
		"max_rows":      5,
		"max_tokens":    256,
	}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "prompt", Label: "Prompt", FieldType: "prompt", Required: true, Placeholder: "Summarize the trail conditions for {{trail_name}}."},
		{Name: "system_prompt", Label: "System prompt", FieldType: "textarea", Required: false},
		columnField("input_column", "Input column", false, "Optional column appended to the prompt for each row."),
		{Name: "output_column", Label: "Output column", FieldType: "text", Required: true, Default: "llm_output"},
		{Name: "output_type", Label: "Output type", FieldType: "select", Required: true, Default: "string", Options: valueLabelOptions("string", "String", "json", "JSON", "boolean", "Boolean", "double", "Double")},
		{Name: "max_rows", Label: "Preview row limit", FieldType: "number", Required: false, Default: 5},
		{Name: "max_tokens", Label: "Max tokens", FieldType: "number", Required: false, Default: 256},
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "schema_add_column", Description: "Output schema adds or replaces the configured LLM output column."}
	return entry
}

func transformCatalogOutputMapping(docs []string) pipelineTransformCatalogEntry {
	entry := baseTransformCatalogEntry("output_mapping", "Output mapping", "Map table columns to dataset or ontology output fields.", "outputs", "output_mapping", "output_drawer", "available", "lightweight_table", "output", docs)
	entry.TransformType = "output_dataset"
	entry.Tags = []string{"output", "deploy"}
	entry.DefaultConfig = map[string]any{"_output": map[string]any{"kind": "dataset", "columns_total": 0, "columns_mapped": 0, "primary_keys": []string{}}}
	entry.Form.Fields = []pipelineTransformCatalogField{
		{Name: "kind", Label: "Output kind", FieldType: "select", Required: true, Default: "dataset", Options: valueLabelOptions("dataset", "Dataset", "object_type", "Object type", "link_type", "Link type", "time_series", "Time series", "virtual_table", "Virtual table")},
		columnsField("columns", "Mapped columns", false, "Columns written to the output."),
		columnsField("primary_keys", "Primary keys", false, "Required for object type outputs."),
	}
	entry.OutputContract = pipelineTransformOutputContract{Mode: "materialized_output", Description: "Produces a dataset or ontology-backed output resource."}
	return entry
}

func columnField(name, label string, required bool, help string) pipelineTransformCatalogField {
	return pipelineTransformCatalogField{Name: name, Label: label, FieldType: "column", Required: required, HelpText: help}
}

func columnsField(name, label string, required bool, help string) pipelineTransformCatalogField {
	return pipelineTransformCatalogField{Name: name, Label: label, FieldType: "columns", Required: required, Repeated: true, HelpText: help}
}

func scalarTypeOptions() []pipelineTransformCatalogOption {
	return []pipelineTransformCatalogOption{
		{Value: "Timestamp", Label: "Timestamp"},
		{Value: "Date", Label: "Date"},
		{Value: "String", Label: "String"},
		{Value: "Integer", Label: "Integer"},
		{Value: "Long", Label: "Long"},
		{Value: "Double", Label: "Double"},
		{Value: "Boolean", Label: "Boolean"},
	}
}

func valueLabelOptions(valuesAndLabels ...string) []pipelineTransformCatalogOption {
	out := make([]pipelineTransformCatalogOption, 0, len(valuesAndLabels)/2)
	for i := 0; i+1 < len(valuesAndLabels); i += 2 {
		out = append(out, pipelineTransformCatalogOption{Value: valuesAndLabels[i], Label: valuesAndLabels[i+1]})
	}
	return out
}
