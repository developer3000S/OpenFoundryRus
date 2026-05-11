package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

type validatePipelineGraphRequest struct {
	Status         string                         `json:"status,omitempty"`
	ScheduleConfig *models.PipelineScheduleConfig `json:"schedule_config,omitempty"`
	DAG            json.RawMessage                `json:"dag,omitempty"`
	IR             *models.PipelineIR             `json:"ir,omitempty"`
	Nodes          *[]models.PipelineNode         `json:"nodes,omitempty"`
}

type pipelineStrictValidationReport struct {
	PipelineID string                          `json:"pipeline_id"`
	AllValid   bool                            `json:"all_valid"`
	Nodes      []pipelineStrictNodeReport      `json:"nodes"`
	Errors     []pipelineStrictValidationError `json:"errors,omitempty"`
}

type pipelineStrictNodeReport struct {
	NodeID       string                          `json:"node_id"`
	Status       string                          `json:"status"`
	Errors       []pipelineStrictValidationError `json:"errors"`
	OutputSchema []pipelineStrictValidationField `json:"output_schema,omitempty"`
}

type pipelineStrictValidationError struct {
	NodeID  string  `json:"node_id"`
	Column  *string `json:"column,omitempty"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
}

type pipelineValidationSummary struct {
	NodeCount   int      `json:"node_count"`
	EdgeCount   int      `json:"edge_count"`
	RootNodeIDs []string `json:"root_node_ids"`
	LeafNodeIDs []string `json:"leaf_node_ids"`
}

type pipelineGraphValidationResponse struct {
	Valid     bool                       `json:"valid"`
	Errors    []string                   `json:"errors"`
	Warnings  []string                   `json:"warnings"`
	NextRunAt any                        `json:"next_run_at"`
	Summary   pipelineValidationSummary  `json:"summary"`
	Nodes     []pipelineStrictNodeReport `json:"nodes,omitempty"`
}

type pipelineStrictValidationField struct {
	Name      string                     `json:"name"`
	FieldType string                     `json:"field_type"`
	Nullable  bool                       `json:"nullable"`
	Metadata  map[string]json.RawMessage `json:"metadata,omitempty"`
}

type pipelineStrictSchema struct {
	Known  bool
	Fields []pipelineStrictValidationField
}

type pipelineStrictValidationFailure struct {
	Report pipelineStrictValidationReport
}

func (e pipelineStrictValidationFailure) Error() string {
	messages := e.Report.errorMessages()
	if len(messages) == 0 {
		return "pipeline schema validation failed"
	}
	return strings.Join(messages, "; ")
}

// ValidatePipelineGraph validates an in-flight graph from the Pipeline Builder
// canvas. Validation failures are returned as a normal 200 payload so the UI
// can render squiggles and per-node status without treating diagnostics as a
// transport failure.
func ValidatePipelineGraph(w http.ResponseWriter, r *http.Request) {
	req, _, err := decodeValidatePipelineGraphRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	ir, err := pipelineIRFromValidationRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
		return
	}
	report := validatePipelineIRStrict("draft", ir)
	writeJSON(w, http.StatusOK, validationResponseFromStrictReport(report, ir))
}

// ValidatePipelineByID validates either the request graph or, when no graph is
// supplied, the persisted pipeline row. This is the id-scoped endpoint used by
// the editor's explicit Validate button.
func ValidatePipelineByID(w http.ResponseWriter, r *http.Request) {
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	req, hasBody, err := decodeValidatePipelineGraphRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	if hasBody && validationRequestHasGraph(req) {
		ir, err := pipelineIRFromValidationRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, validatePipelineIRStrict(pipelineID.String(), ir))
		return
	}
	repo, ok := requirePipelineAuthoringRepository(w, "ValidatePipelineByID requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	pipeline, err := repo.GetPipeline(r.Context(), pipelineID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_pipeline_failed", "detail": err.Error()})
		return
	}
	if pipeline == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	report, err := validatePipelineStrictFromPipeline(pipeline)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func validatePipelineStrictFromPipeline(pipeline *models.Pipeline) (pipelineStrictValidationReport, error) {
	ir, err := pipeline.ParsedIR()
	if err != nil {
		return pipelineStrictValidationReport{}, err
	}
	return validatePipelineIRStrict(pipeline.ID.String(), ir), nil
}

func pipelineIRFromValidationRequest(req validatePipelineGraphRequest) (models.PipelineIR, error) {
	if req.IR != nil {
		return req.IR.Normalize(), nil
	}
	if len(bytes.TrimSpace(req.DAG)) > 0 {
		return models.ParsePipelineIR(req.DAG)
	}
	if req.Nodes != nil {
		return models.NewPipelineIRFromNodes(*req.Nodes), nil
	}
	return models.PipelineIR{}, models.ErrNoPipelineGraph
}

func decodeValidatePipelineGraphRequest(r *http.Request) (validatePipelineGraphRequest, bool, error) {
	var req validatePipelineGraphRequest
	if r.Body == nil {
		return req, false, nil
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return req, false, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return req, false, nil
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, true, err
	}
	return req, true, nil
}

func validationRequestHasGraph(req validatePipelineGraphRequest) bool {
	return req.IR != nil || len(bytes.TrimSpace(req.DAG)) > 0 || req.Nodes != nil
}

func validatePipelineIRStrict(pipelineID string, ir models.PipelineIR) pipelineStrictValidationReport {
	report, _ := validatePipelineIRStrictWithSchemas(pipelineID, ir)
	return report
}

func validatePipelineIRStrictWithSchemas(pipelineID string, ir models.PipelineIR) (pipelineStrictValidationReport, map[string]pipelineStrictSchema) {
	report := pipelineStrictValidationReport{PipelineID: pipelineID, AllValid: true}
	structural := ir.Validate()
	for _, err := range structural.Errors {
		report.addError(err.NodeID, nil, err.Code, err.Message)
	}

	byID := map[string]models.PipelineIRNode{}
	for _, node := range ir.Nodes {
		byID[node.ID] = node
	}
	state := map[string]int{}
	schemas := map[string]pipelineStrictSchema{}
	var visit func(string) pipelineStrictSchema
	visit = func(id string) pipelineStrictSchema {
		if state[id] == 2 {
			return schemas[id]
		}
		if state[id] == 1 {
			report.addError(id, nil, "cycle_detected", fmt.Sprintf("cycle detected while validating node %q", id))
			return pipelineStrictSchema{}
		}
		node, ok := byID[id]
		if !ok {
			report.addError(id, nil, "missing_dependency", fmt.Sprintf("pipeline node %q is missing", id))
			return pipelineStrictSchema{}
		}
		state[id] = 1
		deps := make([]pipelineStrictSchema, 0, len(node.DependsOn))
		for _, dep := range node.DependsOn {
			if _, ok := byID[dep]; !ok {
				report.addError(node.ID, strPtr(dep), "missing_dependency", fmt.Sprintf("pipeline node %q depends on missing node %q", node.ID, dep))
				deps = append(deps, pipelineStrictSchema{})
				continue
			}
			deps = append(deps, visit(dep))
		}
		schema := validateStrictNode(node, deps, &report)
		if node.OutputSchema != nil && normaliseTableTransform(node.TransformType) != "input" {
			declared := schemaFromIRSchema(*node.OutputSchema)
			validateDeclaredOutputSchema(node.ID, schema, declared, &report)
			if declared.Known {
				schema = declared
			}
		}
		report.addNode(node.ID, schema)
		schemas[id] = schema
		state[id] = 2
		return schema
	}

	for _, node := range ir.Nodes {
		visit(node.ID)
	}
	validateLinkOutputReferences(ir, schemas, &report)
	report.AllValid = len(report.Errors) == 0
	if report.Nodes == nil {
		report.Nodes = []pipelineStrictNodeReport{}
	}
	return report, schemas
}

func validateStrictNode(node models.PipelineIRNode, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	cfg, err := parseTableRuntimeConfig(node.Config)
	if err != nil {
		report.addError(node.ID, nil, "invalid_config_json", err.Error())
		return firstSchema(deps)
	}
	kind := normaliseTableTransform(node.TransformType)
	switch kind {
	case "input":
		return sourceSchemaForNode(node, cfg, report)
	case "filter":
		base := requireOneInput(node.ID, kind, deps, report)
		validateFilterNode(node.ID, cfg, base, report)
		return base
	case "select":
		base := requireOneInput(node.ID, kind, deps, report)
		return validateSelectNode(node.ID, cfg, base, report)
	case "drop":
		base := requireOneInput(node.ID, kind, deps, report)
		return validateDropNode(node.ID, cfg, base, report)
	case "rename":
		base := requireOneInput(node.ID, kind, deps, report)
		return validateRenameNode(node.ID, cfg, base, report)
	case "output":
		base := requireOneInput(node.ID, kind, deps, report)
		validateOutputNode(node.ID, node.TransformType, node.Config, base, report)
		return base
	case "sql":
		return validateStructuredSQLNode(node, cfg, deps, report)
	case "function":
		base := requireOneInput(node.ID, kind, deps, report)
		return validateFunctionNode(node.ID, runtimeFunctionConfig{
			FunctionID:          cfg.FunctionID,
			FunctionName:        cfg.FunctionName,
			FunctionVersion:     cfg.FunctionVersion,
			FunctionAutoUpgrade: cfg.FunctionAutoUpgrade,
			TargetColumn:        cfg.TargetColumn,
			ResultType:          cfg.ResultType,
			Arguments:           cfg.Arguments,
			Args:                cfg.Args,
		}, base, report)
	case "gpx_parse":
		return validateGPXParseNode(node.ID, cfg, deps, report)
	case "python":
		base := firstSchema(deps)
		return validatePythonNode(node, cfg, base, report)
	case "passthrough":
		if len(deps) == 0 {
			return sourceSchemaForNode(node, cfg, report)
		}
		return deps[0]
	default:
		if len(deps) > 0 {
			return deps[0]
		}
		return sourceSchemaForNode(node, cfg, report)
	}
}

func validateStructuredSQLNode(node models.PipelineIRNode, cfg tableRuntimeConfig, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if cfg.Stack != nil {
		base := requireOneInput(node.ID, "transform_stack", deps, report)
		return validateTransformStack(node.ID, *cfg.Stack, base, report)
	}
	if cfg.Join != nil {
		return validateJoinNode(node.ID, *cfg.Join, deps, report)
	}
	if cfg.GeoJoin != nil {
		return validateGeoJoinNode(node.ID, *cfg.GeoJoin, deps, report)
	}
	if cfg.Union != nil {
		return validateUnionNode(node.ID, *cfg.Union, deps, report)
	}
	if cfg.FunctionID != "" || cfg.FunctionName != "" {
		base := requireOneInput(node.ID, "function", deps, report)
		return validateFunctionNode(node.ID, runtimeFunctionConfig{
			FunctionID:          cfg.FunctionID,
			FunctionName:        cfg.FunctionName,
			FunctionVersion:     cfg.FunctionVersion,
			FunctionAutoUpgrade: cfg.FunctionAutoUpgrade,
			TargetColumn:        cfg.TargetColumn,
			ResultType:          cfg.ResultType,
			Arguments:           cfg.Arguments,
			Args:                cfg.Args,
		}, base, report)
	}
	if cfg.Predicate != "" || cfg.Expression != "" {
		base := requireOneInput(node.ID, "sql", deps, report)
		validateFilterNode(node.ID, cfg, base, report)
		if len(firstStrings(cfg.Columns, cfg.Select)) > 0 {
			return validateSelectNode(node.ID, cfg, base, report)
		}
		return base
	}
	if len(firstStrings(cfg.Columns, cfg.Select)) > 0 {
		base := requireOneInput(node.ID, "sql", deps, report)
		return validateSelectNode(node.ID, cfg, base, report)
	}
	if rows := cfg.inlineRows(); rows != nil {
		return schemaFromRows(rowsToMaps(rows))
	}
	return firstSchema(deps)
}

func sourceSchemaForNode(node models.PipelineIRNode, cfg tableRuntimeConfig, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if node.OutputSchema != nil {
		schema := schemaFromIRSchema(*node.OutputSchema)
		validateSchemaInternals(node.ID, schema, report)
		return schema
	}
	if node.PreviewSchema != nil {
		schema := schemaFromIRSchema(*node.PreviewSchema)
		validateSchemaInternals(node.ID, schema, report)
		return schema
	}
	if rows := cfg.inlineRows(); rows != nil {
		return schemaFromRows(rowsToMaps(rows))
	}
	columns := firstStrings(cfg.Columns, cfg.Select)
	if len(columns) > 0 {
		return schemaFromColumnNames(columns)
	}
	return pipelineStrictSchema{}
}

func validateFilterNode(nodeID string, cfg tableRuntimeConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) {
	predicate := strings.TrimSpace(firstNonEmpty(cfg.Predicate, cfg.Expression))
	if predicate == "" {
		report.addError(nodeID, nil, "filter_missing_predicate", "filter requires a non-empty predicate")
		return
	}
	if !schema.Known {
		return
	}
	parsed, err := pipelineexpression.ParseExpr(predicate)
	if err != nil {
		report.addError(nodeID, nil, "predicate_parse_error", err.Error())
		return
	}
	inferred, typeErrors := pipelineexpression.InferExpr(parsed, schemaToColumnEnv(schema))
	for _, typeErr := range typeErrors {
		report.addError(nodeID, strictColumnFromTypeError(typeErr), validationCodeFromTypeError(typeErr), typeErr.Error())
	}
	if len(typeErrors) == 0 && inferred.Kind != pipelineexpression.KindBoolean {
		report.addError(nodeID, nil, "predicate_must_return_boolean", fmt.Sprintf("predicate must return Boolean, got %s", strictTypeName(inferred)))
	}
}

func validateSelectNode(nodeID string, cfg tableRuntimeConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	columns := firstStrings(cfg.Columns, cfg.Select)
	if len(columns) == 0 {
		return schema
	}
	checkDuplicateNames(nodeID, columns, "duplicate_column_name", report)
	if !schema.Known {
		return schemaFromColumnNames(columns)
	}
	fields := make([]pipelineStrictValidationField, 0, len(columns))
	for _, column := range columns {
		field, ok := schema.field(column)
		if !ok {
			report.addError(nodeID, strPtr(column), "missing_column", fmt.Sprintf("column %q does not exist in upstream schema", column))
			continue
		}
		fields = append(fields, field)
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func validateDropNode(nodeID string, cfg tableRuntimeConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	columns := firstStrings(cfg.DropColumns, cfg.Columns)
	checkDuplicateNames(nodeID, columns, "duplicate_column_name", report)
	if !schema.Known {
		return schema
	}
	drop := map[string]struct{}{}
	for _, column := range columns {
		if _, ok := schema.field(column); !ok {
			report.addError(nodeID, strPtr(column), "missing_column", fmt.Sprintf("column %q does not exist in upstream schema", column))
			continue
		}
		drop[column] = struct{}{}
	}
	fields := make([]pipelineStrictValidationField, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		if _, ok := drop[field.Name]; !ok {
			fields = append(fields, field)
		}
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func validateRenameNode(nodeID string, cfg tableRuntimeConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	renames := renameMappingsFromConfig(cfg)
	return applyRenameMappings(nodeID, schema, renames, report)
}

func validateTransformStack(nodeID string, stack runtimeTransformStack, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	out := schema
	for _, block := range stack.Blocks {
		if !block.Applied {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(block.Kind)) {
		case "filter":
			validateFilterBlock(nodeID, block, out, report)
		case "select":
			out = validateSelectNode(nodeID, tableRuntimeConfig{Columns: block.Columns}, out, report)
		case "drop":
			out = validateDropNode(nodeID, tableRuntimeConfig{Columns: block.Columns}, out, report)
		case "rename":
			out = applyRenameMappings(nodeID, out, block.Renames, report)
		case "normalize":
			out = applyNormalizeSchema(nodeID, out, block.RemoveSpecialCharacters, report)
		case "cast":
			out = applyCastBlock(nodeID, out, block, report)
		case "haversine", "haversine_distance", "geo_distance":
			out = applyHaversineSchema(nodeID, out, block, report)
		case "function", "udf", "reusable_function":
			out = validateFunctionNode(nodeID, runtimeFunctionConfig{
				FunctionID:          block.FunctionID,
				FunctionName:        block.FunctionName,
				FunctionVersion:     block.FunctionVersion,
				FunctionAutoUpgrade: block.FunctionAutoUpgrade,
				TargetColumn:        block.TargetColumn,
				ResultType:          block.ResultType,
				Arguments:           block.Arguments,
				Args:                block.Args,
			}, out, report)
		default:
			report.addError(nodeID, nil, "unsupported_transform_block", fmt.Sprintf("unsupported transform stack block %q", block.Kind))
		}
	}
	return out
}

func validateFunctionNode(nodeID string, cfg runtimeFunctionConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	target := strings.TrimSpace(cfg.TargetColumn)
	if target == "" {
		target = strings.TrimSpace(cfg.FunctionName)
	}
	if target == "" {
		target = strings.TrimSpace(cfg.FunctionID)
	}
	if target == "" {
		report.addError(nodeID, nil, "function_missing_target_column", "function node requires a target column or function name")
		return schema
	}
	if !schema.Known {
		return schema
	}
	resultType := strictTypeName(pipelineFunctionType(firstNonEmpty(cfg.ResultType, "String")))
	fields := make([]pipelineStrictValidationField, 0, len(schema.Fields)+1)
	replaced := false
	for _, field := range schema.Fields {
		if field.Name == target {
			field.FieldType = resultType
			field.Nullable = true
			fields = append(fields, field)
			replaced = true
			continue
		}
		fields = append(fields, field)
	}
	if !replaced {
		fields = append(fields, pipelineStrictValidationField{Name: target, FieldType: resultType, Nullable: true})
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func validatePythonNode(node models.PipelineIRNode, cfg tableRuntimeConfig, schema pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	var raw map[string]json.RawMessage
	if len(node.Config) > 0 {
		_ = json.Unmarshal(node.Config, &raw)
	}
	source := pythonSourceFromConfig(raw)
	if strings.TrimSpace(source) == "" {
		report.addError(node.ID, nil, "python_missing_source", "python transform requires source, code, or python_source")
	}
	if err := validatePythonPackageConstraints(source, raw); err != nil {
		report.addError(node.ID, nil, "python_package_constraint_failed", err.Error())
	}
	if schemaCfg, ok := pythonIRSchemaFromConfig(raw, "output_schema", "result_schema"); ok {
		out := schemaFromIRSchema(schemaCfg)
		validateSchemaInternals(node.ID, out, report)
		return out
	}
	if node.OutputSchema != nil {
		out := schemaFromIRSchema(*node.OutputSchema)
		validateSchemaInternals(node.ID, out, report)
		return out
	}
	if node.PreviewSchema != nil {
		out := schemaFromIRSchema(*node.PreviewSchema)
		validateSchemaInternals(node.ID, out, report)
		return out
	}
	if rows := cfg.inlineRows(); rows != nil {
		return schemaFromRows(rowsToMaps(rows))
	}
	return schema
}

func validateFilterBlock(nodeID string, block runtimeTransformBlock, schema pipelineStrictSchema, report *pipelineStrictValidationReport) {
	if len(block.Conditions) == 0 {
		report.addError(nodeID, nil, "filter_missing_conditions", "filter block requires at least one condition")
		return
	}
	if !schema.Known {
		return
	}
	for _, condition := range block.Conditions {
		column := strings.TrimSpace(condition.Column)
		if column == "" {
			report.addError(nodeID, nil, "filter_missing_column", "filter condition requires a column")
			continue
		}
		field, ok := schema.field(column)
		if !ok {
			report.addError(nodeID, strPtr(column), "missing_column", fmt.Sprintf("column %q does not exist in upstream schema", column))
			continue
		}
		switch condition.Operator {
		case "greater_than", "less_than":
			if !fieldType(field).IsNumeric() && !fieldType(field).IsTemporal() {
				report.addError(nodeID, strPtr(column), "incompatible_filter_condition", fmt.Sprintf("operator %q requires a numeric or temporal column, got %s", condition.Operator, field.FieldType))
			}
		}
	}
}

func applyCastBlock(nodeID string, schema pipelineStrictSchema, block runtimeTransformBlock, report *pipelineStrictValidationReport) pipelineStrictSchema {
	source := strings.TrimSpace(block.SourceColumn)
	if source == "" {
		report.addError(nodeID, nil, "cast_missing_source_column", "cast block requires source_column")
		return schema
	}
	targetType, ok := parseStrictType(block.TargetType)
	if !ok {
		report.addError(nodeID, strPtr(source), "invalid_cast_target", fmt.Sprintf("cast target %q is not a supported type", block.TargetType))
		return schema
	}
	if !schema.Known {
		return schema
	}
	fields := cloneStrictFields(schema.Fields)
	for i, field := range fields {
		if field.Name != source {
			continue
		}
		if !canExplicitCast(fieldType(field), targetType) {
			report.addError(nodeID, strPtr(source), "invalid_cast", fmt.Sprintf("cannot cast column %q from %s to %s", source, field.FieldType, strictTypeName(targetType)))
			return schema
		}
		targetName := strings.TrimSpace(block.TargetColumn)
		if targetName == "" {
			targetName = source
		}
		fields[i] = pipelineStrictValidationField{Name: targetName, FieldType: strictTypeName(targetType), Nullable: field.Nullable, Metadata: cloneRawMap(field.Metadata)}
		checkSchemaDuplicateFields(nodeID, pipelineStrictSchema{Known: true, Fields: fields}, report)
		return pipelineStrictSchema{Known: true, Fields: fields}
	}
	report.addError(nodeID, strPtr(source), "missing_column", fmt.Sprintf("column %q does not exist in upstream schema", source))
	return schema
}

func validateJoinNode(nodeID string, draft runtimeJoinDraft, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if len(deps) < 2 {
		report.addError(nodeID, nil, "join_requires_two_inputs", "join requires two upstream inputs")
		return firstSchema(deps)
	}
	left, right := deps[0], deps[1]
	if len(validJoinMatches(draft.Matches)) == 0 && strings.ToLower(strings.TrimSpace(draft.JoinType)) != "cross" {
		report.addError(nodeID, nil, "join_missing_match_conditions", "join requires at least one match condition")
	}
	for _, match := range validJoinMatches(draft.Matches) {
		leftField, leftOK := left.field(match.LeftColumn)
		rightField, rightOK := right.field(match.RightColumn)
		if left.Known && !leftOK {
			report.addError(nodeID, strPtr(match.LeftColumn), "missing_join_column", fmt.Sprintf("left join column %q does not exist", match.LeftColumn))
		}
		if right.Known && !rightOK {
			report.addError(nodeID, strPtr(match.RightColumn), "missing_join_column", fmt.Sprintf("right join column %q does not exist", match.RightColumn))
		}
		if leftOK && rightOK && !compatibleJoinTypes(fieldType(leftField), fieldType(rightField)) {
			report.addError(nodeID, strPtr(match.LeftColumn), "incompatible_join_key_types", fmt.Sprintf("join key %q (%s) is not compatible with %q (%s)", match.LeftColumn, leftField.FieldType, match.RightColumn, rightField.FieldType))
		}
	}
	if !left.Known || !right.Known {
		return pipelineStrictSchema{}
	}
	leftFields := selectFieldsForJoin(left, draft.LeftColumns, draft.AutoSelectLeft || len(draft.LeftColumns) == 0, "", nodeID, report)
	rightFields := selectFieldsForJoin(right, draft.RightColumns, draft.AutoSelectRight, strings.TrimSpace(draft.RightPrefix), nodeID, report)
	out := pipelineStrictSchema{Known: true, Fields: append(leftFields, rightFields...)}
	checkSchemaDuplicateFields(nodeID, out, report)
	return out
}

func validateGeoJoinNode(nodeID string, draft runtimeGeoJoinDraft, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if len(deps) < 2 {
		report.addError(nodeID, nil, "geo_join_requires_two_inputs", "geospatial join requires two upstream inputs")
		return firstSchema(deps)
	}
	mode := normalizedGeoJoinMode(draft.Mode)
	switch mode {
	case "intersection", "distance", "nearest":
	default:
		report.addError(nodeID, nil, "geo_join_unsupported_mode", fmt.Sprintf("unsupported geospatial join mode %q", draft.Mode))
	}
	if mode == "distance" && draft.MaxDistance <= 0 {
		report.addError(nodeID, nil, "geo_join_missing_max_distance", "distance geospatial join requires max_distance")
	}
	if draft.K > defaultGeoJoinMaxK {
		report.addError(nodeID, nil, "geo_join_k_exceeds_limit", fmt.Sprintf("nearest-neighbor geospatial join k cannot exceed %d", defaultGeoJoinMaxK))
	}
	left, right := deps[0], deps[1]
	validateGeoJoinInputColumns(nodeID, "left", left, draft.LeftGeometryColumn, draft.LeftLatColumn, draft.LeftLonColumn, report)
	validateGeoJoinInputColumns(nodeID, "right", right, draft.RightGeometryColumn, draft.RightLatColumn, draft.RightLonColumn, report)
	if !left.Known || !right.Known {
		return pipelineStrictSchema{}
	}
	leftFields := selectFieldsForJoin(left, draft.LeftColumns, draft.AutoSelectLeft || len(draft.LeftColumns) == 0, "", nodeID, report)
	rightFields := selectFieldsForJoin(right, draft.RightColumns, draft.AutoSelectRight || len(draft.RightColumns) == 0, strings.TrimSpace(firstNonEmpty(draft.RightPrefix, "right_")), nodeID, report)
	fields := append(leftFields, rightFields...)
	if mode != "intersection" || strings.TrimSpace(draft.DistanceColumn) != "" {
		fields = append(fields, pipelineStrictValidationField{Name: firstNonEmpty(draft.DistanceColumn, "geo_distance_"+geoJoinUnitSuffix(firstNonEmpty(draft.Unit, "miles"))), FieldType: "DOUBLE", Nullable: true})
	}
	if mode == "nearest" {
		fields = append(fields, pipelineStrictValidationField{Name: firstNonEmpty(draft.RankColumn, "geo_rank"), FieldType: "LONG", Nullable: false})
	}
	out := pipelineStrictSchema{Known: true, Fields: fields}
	checkSchemaDuplicateFields(nodeID, out, report)
	return out
}

func validateGeoJoinInputColumns(nodeID, side string, schema pipelineStrictSchema, geometryColumn, latColumn, lonColumn string, report *pipelineStrictValidationReport) {
	if !schema.Known {
		return
	}
	geometryColumn = strings.TrimSpace(geometryColumn)
	if geometryColumn != "" {
		field, ok := schema.field(geometryColumn)
		if !ok {
			report.addError(nodeID, strPtr(geometryColumn), "missing_geo_join_geometry_column", fmt.Sprintf("%s geometry column %q does not exist", side, geometryColumn))
			return
		}
		ty := fieldType(field)
		if ty.Kind != pipelineexpression.KindString && ty.Kind != pipelineexpression.KindGeometry {
			report.addError(nodeID, strPtr(geometryColumn), "incompatible_geo_join_geometry_column", fmt.Sprintf("%s geometry column %q must be STRING or GEOMETRY, got %s", side, geometryColumn, field.FieldType))
		}
		return
	}
	for _, column := range []struct {
		name string
		role string
	}{
		{strings.TrimSpace(latColumn), "latitude"},
		{strings.TrimSpace(lonColumn), "longitude"},
	} {
		if column.name == "" {
			report.addError(nodeID, nil, "geo_join_missing_coordinate_column", fmt.Sprintf("%s geospatial join requires geometry_column or %s column", side, column.role))
			continue
		}
		field, ok := schema.field(column.name)
		if !ok {
			report.addError(nodeID, strPtr(column.name), "missing_geo_join_coordinate_column", fmt.Sprintf("%s %s column %q does not exist", side, column.role, column.name))
			continue
		}
		if !fieldType(field).IsNumeric() {
			report.addError(nodeID, strPtr(column.name), "incompatible_geo_join_coordinate_column", fmt.Sprintf("%s %s column %q must be numeric, got %s", side, column.role, column.name, field.FieldType))
		}
	}
}

func validateUnionNode(nodeID string, draft runtimeUnionDraft, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if len(deps) < 2 {
		report.addError(nodeID, nil, "union_requires_two_inputs", "union requires at least two upstream inputs")
		return firstSchema(deps)
	}
	for _, dep := range deps {
		if !dep.Known {
			return pipelineStrictSchema{}
		}
	}
	if strings.EqualFold(strings.TrimSpace(draft.UnionType), "by_position") {
		base := deps[0]
		for idx, dep := range deps[1:] {
			if len(dep.Fields) != len(base.Fields) {
				report.addError(nodeID, nil, "union_position_arity_mismatch", fmt.Sprintf("union input %d has %d columns, expected %d", idx+2, len(dep.Fields), len(base.Fields)))
				continue
			}
			for i, field := range dep.Fields {
				if !compatibleUnionTypes(fieldType(base.Fields[i]), fieldType(field)) {
					report.addError(nodeID, strPtr(field.Name), "incompatible_union_column_types", fmt.Sprintf("union column position %d has incompatible types %s and %s", i+1, base.Fields[i].FieldType, field.FieldType))
				}
			}
		}
		return base
	}
	base := deps[0]
	for idx, dep := range deps[1:] {
		for _, field := range base.Fields {
			other, ok := dep.field(field.Name)
			if !ok {
				report.addError(nodeID, strPtr(field.Name), "missing_union_column", fmt.Sprintf("union input %d is missing column %q", idx+2, field.Name))
				continue
			}
			if !compatibleUnionTypes(fieldType(field), fieldType(other)) {
				report.addError(nodeID, strPtr(field.Name), "incompatible_union_column_types", fmt.Sprintf("union column %q has incompatible types %s and %s", field.Name, field.FieldType, other.FieldType))
			}
		}
		for _, field := range dep.Fields {
			if _, ok := base.field(field.Name); !ok {
				report.addError(nodeID, strPtr(field.Name), "extra_union_column", fmt.Sprintf("union input %d has extra column %q", idx+2, field.Name))
			}
		}
	}
	return base
}

func validateOutputNode(nodeID, transformType string, rawConfig json.RawMessage, schema pipelineStrictSchema, report *pipelineStrictValidationReport) {
	keys := outputPrimaryKeys(rawConfig)
	requiresKey := outputRequiresPrimaryKey(transformType, rawConfig)
	if requiresKey && len(keys) == 0 {
		report.addError(nodeID, nil, "output_primary_key_required", "Ontology object outputs require at least one primary key column")
		return
	}
	checkDuplicateNames(nodeID, keys, "duplicate_primary_key", report)
	if !schema.Known {
		return
	}
	for _, key := range keys {
		field, ok := schema.field(key)
		if !ok {
			report.addError(nodeID, strPtr(key), "missing_primary_key_column", fmt.Sprintf("primary key column %q does not exist in output schema", key))
			continue
		}
		if field.Nullable {
			report.addError(nodeID, strPtr(key), "nullable_primary_key", fmt.Sprintf("primary key column %q must be non-nullable", key))
		}
	}
}

func validateLinkOutputReferences(ir models.PipelineIR, schemas map[string]pipelineStrictSchema, report *pipelineStrictValidationReport) {
	byID := map[string]models.PipelineIRNode{}
	for _, node := range ir.Nodes {
		byID[node.ID] = node
	}
	for _, node := range ir.Nodes {
		cfg := parseOutputDatasetConfig(node.Config)
		if outputKindForIRNode(node, cfg) != "link_type" {
			continue
		}
		deps := map[string]struct{}{}
		for _, dep := range node.DependsOn {
			deps[dep] = struct{}{}
		}
		cardinality := normaliseLinkCardinality(cfg.Cardinality)
		switch cardinality {
		case "one_to_many", "many_to_many", "many_to_one":
		case "one_to_one":
			report.addError(node.ID, nil, "unsupported_link_cardinality", "Pipeline Builder link outputs do not support one-to-one cardinality")
		default:
			report.addError(node.ID, nil, "unsupported_link_cardinality", fmt.Sprintf("unsupported link cardinality %q", cfg.Cardinality))
		}
		if strings.TrimSpace(cfg.SourceObjectNodeID) == "" {
			report.addError(node.ID, nil, "link_source_object_required", "link outputs require a source object output node")
		} else if ref, ok := byID[cfg.SourceObjectNodeID]; !ok {
			report.addError(node.ID, strPtr(cfg.SourceObjectNodeID), "missing_link_source_object", fmt.Sprintf("source object output node %q does not exist", cfg.SourceObjectNodeID))
		} else if !isObjectOutputIRNode(ref) {
			report.addError(node.ID, strPtr(cfg.SourceObjectNodeID), "invalid_link_source_object", fmt.Sprintf("source node %q is not an object output", cfg.SourceObjectNodeID))
		} else if _, ok := deps[cfg.SourceObjectNodeID]; !ok {
			report.addError(node.ID, strPtr(cfg.SourceObjectNodeID), "link_object_dependency_required", fmt.Sprintf("link output must depend on source object output node %q so it deploys first", cfg.SourceObjectNodeID))
		}
		if strings.TrimSpace(cfg.TargetObjectNodeID) == "" {
			report.addError(node.ID, nil, "link_target_object_required", "link outputs require a target object output node")
		} else if ref, ok := byID[cfg.TargetObjectNodeID]; !ok {
			report.addError(node.ID, strPtr(cfg.TargetObjectNodeID), "missing_link_target_object", fmt.Sprintf("target object output node %q does not exist", cfg.TargetObjectNodeID))
		} else if !isObjectOutputIRNode(ref) {
			report.addError(node.ID, strPtr(cfg.TargetObjectNodeID), "invalid_link_target_object", fmt.Sprintf("target node %q is not an object output", cfg.TargetObjectNodeID))
		} else if _, ok := deps[cfg.TargetObjectNodeID]; !ok {
			report.addError(node.ID, strPtr(cfg.TargetObjectNodeID), "link_object_dependency_required", fmt.Sprintf("link output must depend on target object output node %q so it deploys first", cfg.TargetObjectNodeID))
		}
		schema := schemas[node.ID]
		if !schema.Known {
			continue
		}
		for _, column := range []struct {
			name string
			code string
			side string
		}{
			{outputLinkSourceKeyColumn(cfg), "missing_link_source_key_column", "source"},
			{outputLinkTargetKeyColumn(cfg), "missing_link_target_key_column", "target"},
		} {
			if strings.TrimSpace(column.name) == "" {
				report.addError(node.ID, nil, column.code, fmt.Sprintf("link outputs require a %s key column", column.side))
				continue
			}
			field, ok := schema.field(column.name)
			if !ok {
				report.addError(node.ID, strPtr(column.name), column.code, fmt.Sprintf("%s link key column %q does not exist in output schema", column.side, column.name))
				continue
			}
			if field.Nullable {
				report.addError(node.ID, strPtr(column.name), "nullable_link_key_column", fmt.Sprintf("%s link key column %q must be non-nullable", column.side, column.name))
			}
		}
	}
}

func outputKindForIRNode(node models.PipelineIRNode, cfg outputDatasetConfig) string {
	if strings.TrimSpace(cfg.Kind) != "" {
		return strings.ToLower(strings.TrimSpace(cfg.Kind))
	}
	lower := strings.ToLower(node.TransformType)
	if strings.Contains(lower, "link") {
		return "link_type"
	}
	if strings.Contains(lower, "object") || strings.Contains(lower, "ontology") {
		return "object_type"
	}
	return "dataset"
}

func isObjectOutputIRNode(node models.PipelineIRNode) bool {
	return outputKindForIRNode(node, parseOutputDatasetConfig(node.Config)) == "object_type"
}

func validateDeclaredOutputSchema(nodeID string, actual, declared pipelineStrictSchema, report *pipelineStrictValidationReport) {
	validateSchemaInternals(nodeID, declared, report)
	if !actual.Known || !declared.Known {
		return
	}
	for _, expected := range declared.Fields {
		actualField, ok := actual.field(expected.Name)
		if !ok {
			report.addError(nodeID, strPtr(expected.Name), "declared_output_column_missing", fmt.Sprintf("declared output column %q is not produced by the node", expected.Name))
			continue
		}
		if !compatibleAssignment(fieldType(actualField), fieldType(expected)) {
			report.addError(nodeID, strPtr(expected.Name), "declared_output_type_mismatch", fmt.Sprintf("declared output column %q expects %s, got %s", expected.Name, expected.FieldType, actualField.FieldType))
		}
		if !expected.Nullable && actualField.Nullable {
			report.addError(nodeID, strPtr(expected.Name), "declared_output_nullability_mismatch", fmt.Sprintf("declared output column %q is non-nullable but upstream data may contain nulls", expected.Name))
		}
	}
}

func validateSchemaInternals(nodeID string, schema pipelineStrictSchema, report *pipelineStrictValidationReport) {
	checkSchemaDuplicateFields(nodeID, schema, report)
	for _, field := range schema.Fields {
		if _, ok := parseStrictType(field.FieldType); !ok {
			report.addError(nodeID, strPtr(field.Name), "unsupported_field_type", fmt.Sprintf("field %q has unsupported type %q", field.Name, field.FieldType))
		}
		validateGeospatialLogicalType(nodeID, field, report)
	}
}

func validateGeospatialLogicalType(nodeID string, field pipelineStrictValidationField, report *pipelineStrictValidationReport) {
	logical := strings.ToLower(strings.TrimSpace(fieldLogicalType(field)))
	if logical == "" && strings.EqualFold(field.FieldType, "GEOMETRY") {
		return
	}
	if logical == "" {
		return
	}
	ty := fieldType(field)
	logicalType, err := geospatialcore.ParseGeospatialLogicalType(logical)
	if err != nil {
		return
	}
	metadata, err := geospatialMetadataFromField(field, logicalType)
	if err != nil {
		report.addError(nodeID, strPtr(field.Name), "invalid_geospatial_crs_policy", fmt.Sprintf("geospatial metadata on %q is invalid: %s", field.Name, err.Error()))
		return
	}
	if err := metadata.Validate(); err != nil {
		report.addError(nodeID, strPtr(field.Name), "invalid_geospatial_crs_policy", fmt.Sprintf("geospatial metadata on %q is invalid: %s", field.Name, err.Error()))
		return
	}
	switch logicalType {
	case geospatialcore.LogicalTypeH3Index, geospatialcore.LogicalTypeCRSMetadata:
		if ty.Kind != pipelineexpression.KindString {
			report.addError(nodeID, strPtr(field.Name), "invalid_geospatial_logical_type", fmt.Sprintf("geospatial logical type %q on %q requires STRING backing, got %s", logical, field.Name, field.FieldType))
		}
	case geospatialcore.LogicalTypeGeoPoint, geospatialcore.LogicalTypeGeometry:
		if ty.Kind != pipelineexpression.KindString && ty.Kind != pipelineexpression.KindGeometry {
			report.addError(nodeID, strPtr(field.Name), "invalid_geospatial_logical_type", fmt.Sprintf("geospatial logical type %q on %q requires STRING or GEOMETRY backing, got %s", logical, field.Name, field.FieldType))
		}
	case geospatialcore.LogicalTypeGeoJSON, geospatialcore.LogicalTypeBoundingBox:
		if ty.Kind != pipelineexpression.KindString {
			report.addError(nodeID, strPtr(field.Name), "invalid_geospatial_logical_type", fmt.Sprintf("geospatial logical type %q on %q requires serialized STRING backing, got %s", logical, field.Name, field.FieldType))
		}
	}
}

func geospatialMetadataFromField(field pipelineStrictValidationField, logicalType geospatialcore.GeospatialLogicalType) (geospatialcore.FieldMetadata, error) {
	metadata := geospatialcore.FieldMetadata{LogicalType: logicalType}
	if raw, ok := firstFieldMetadataRaw(field.Metadata, "crs", "coordinate_reference_system", "coordinateReferenceSystem"); ok {
		crs, err := parseOptionalCRSMetadata(raw)
		if err != nil {
			return geospatialcore.FieldMetadata{}, err
		}
		metadata.CRS = crs
	}
	if raw, ok := firstFieldMetadataRaw(field.Metadata, "coordinate_order", "coordinateOrder"); ok {
		order, err := parseOptionalCoordinateOrder(raw)
		if err != nil {
			return geospatialcore.FieldMetadata{}, err
		}
		metadata.CoordinateOrder = order
	}
	return metadata, nil
}

func parseOptionalCRSMetadata(raw json.RawMessage) (*geospatialcore.CRSMetadata, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var crsString string
	if err := json.Unmarshal(trimmed, &crsString); err == nil {
		crs, err := geospatialcore.ParseCRSMetadata(crsString)
		if err != nil {
			return nil, err
		}
		return &crs, nil
	}
	var crs geospatialcore.CRSMetadata
	if err := json.Unmarshal(trimmed, &crs); err != nil {
		return nil, err
	}
	normalized, err := geospatialcore.NormalizeCRS(&crs)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func parseOptionalCoordinateOrder(raw json.RawMessage) (geospatialcore.CoordinateOrder, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", err
	}
	return geospatialcore.ParseCoordinateOrder(value)
}

func schemaFromIRSchema(schema models.PipelineIRSchema) pipelineStrictSchema {
	fields := make([]pipelineStrictValidationField, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		fields = append(fields, pipelineStrictValidationField{Name: field.Name, FieldType: field.FieldType, Nullable: field.Nullable, Metadata: cloneRawMap(field.Metadata)})
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func schemaFromRows(rows []map[string]json.RawMessage) pipelineStrictSchema {
	if len(rows) == 0 {
		return pipelineStrictSchema{}
	}
	namesSet := map[string]struct{}{}
	for _, row := range rows {
		for name := range row {
			namesSet[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(namesSet))
	for name := range namesSet {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]pipelineStrictValidationField, 0, len(names))
	for _, name := range names {
		nullable := false
		var inferred pipelineexpression.PipelineType
		inferredSet := false
		for _, row := range rows {
			raw, ok := row[name]
			if !ok || len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
				nullable = true
				continue
			}
			next := pipelineexpression.EvalValueFromJSON(raw).TypeHint()
			if !inferredSet {
				inferred = next
				inferredSet = true
				continue
			}
			if promoted, ok := pipelineexpression.Promote(inferred, next); ok {
				inferred = promoted
			} else {
				inferred = pipelineexpression.StringType()
			}
		}
		if !inferredSet {
			inferred = pipelineexpression.StringType()
		}
		fields = append(fields, pipelineStrictValidationField{Name: name, FieldType: strictTypeName(inferred), Nullable: nullable})
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func schemaFromColumnNames(columns []string) pipelineStrictSchema {
	fields := make([]pipelineStrictValidationField, 0, len(columns))
	seen := map[string]struct{}{}
	for _, column := range columns {
		if strings.TrimSpace(column) == "" {
			continue
		}
		if _, ok := seen[column]; ok {
			continue
		}
		seen[column] = struct{}{}
		fields = append(fields, pipelineStrictValidationField{Name: column, FieldType: "STRING", Nullable: true})
	}
	return pipelineStrictSchema{Known: true, Fields: fields}
}

func (s pipelineStrictSchema) field(name string) (pipelineStrictValidationField, bool) {
	if !s.Known {
		return pipelineStrictValidationField{}, false
	}
	for _, field := range s.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return pipelineStrictValidationField{}, false
}

func schemaToColumnEnv(schema pipelineStrictSchema) pipelineexpression.ColumnEnv {
	env := pipelineexpression.NewColumnEnv()
	for _, field := range schema.Fields {
		env.Insert(field.Name, fieldType(field))
	}
	return env
}

func fieldType(field pipelineStrictValidationField) pipelineexpression.PipelineType {
	ty, ok := parseStrictType(field.FieldType)
	if !ok {
		return pipelineexpression.StringType()
	}
	return ty
}

func parseStrictType(raw string) (pipelineexpression.PipelineType, bool) {
	if strings.TrimSpace(raw) == "" {
		return pipelineexpression.StringType(), false
	}
	return pipelineexpression.ParseTypeLiteral(raw)
}

func strictTypeName(ty pipelineexpression.PipelineType) string {
	return string(ty.Kind)
}

func firstSchema(schemas []pipelineStrictSchema) pipelineStrictSchema {
	if len(schemas) == 0 {
		return pipelineStrictSchema{}
	}
	return schemas[0]
}

func requireOneInput(nodeID, transform string, deps []pipelineStrictSchema, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if len(deps) == 0 {
		report.addError(nodeID, nil, "missing_input", fmt.Sprintf("%s requires one upstream input", transform))
		return pipelineStrictSchema{}
	}
	return deps[0]
}

func firstStrings(first, second []string) []string {
	if len(first) > 0 {
		return append([]string(nil), first...)
	}
	return append([]string(nil), second...)
}

func renameMappingsFromConfig(cfg tableRuntimeConfig) []runtimeRenameMapping {
	out := []runtimeRenameMapping{}
	keys := make([]string, 0, len(cfg.Renames))
	for source := range cfg.Renames {
		keys = append(keys, source)
	}
	sort.Strings(keys)
	for _, source := range keys {
		out = append(out, runtimeRenameMapping{From: source, To: cfg.Renames[source]})
	}
	for _, mapping := range cfg.ColumnMappings {
		out = append(out, runtimeRenameMapping{From: mapping.SourceColumn, To: mapping.TargetColumn})
	}
	return out
}

func applyRenameMappings(nodeID string, schema pipelineStrictSchema, renames []runtimeRenameMapping, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if len(renames) == 0 {
		return schema
	}
	if !schema.Known {
		return schema
	}
	fields := cloneStrictFields(schema.Fields)
	for _, mapping := range renames {
		source := strings.TrimSpace(mapping.From)
		target := strings.TrimSpace(mapping.To)
		if source == "" || target == "" {
			report.addError(nodeID, nil, "rename_mapping_incomplete", "rename mappings require both source and target columns")
			continue
		}
		found := false
		for i, field := range fields {
			if field.Name == source {
				fields[i].Name = target
				found = true
				break
			}
		}
		if !found {
			report.addError(nodeID, strPtr(source), "missing_column", fmt.Sprintf("column %q does not exist in upstream schema", source))
		}
	}
	out := pipelineStrictSchema{Known: true, Fields: fields}
	checkSchemaDuplicateFields(nodeID, out, report)
	return out
}

func applyNormalizeSchema(nodeID string, schema pipelineStrictSchema, removeSpecial bool, report *pipelineStrictValidationReport) pipelineStrictSchema {
	if !schema.Known {
		return schema
	}
	fields := cloneStrictFields(schema.Fields)
	for i := range fields {
		fields[i].Name = normalizeRuntimeColumnName(fields[i].Name, removeSpecial)
	}
	out := pipelineStrictSchema{Known: true, Fields: fields}
	checkSchemaDuplicateFields(nodeID, out, report)
	return out
}

func selectFieldsForJoin(schema pipelineStrictSchema, explicit []string, auto bool, prefix, nodeID string, report *pipelineStrictValidationReport) []pipelineStrictValidationField {
	columns := explicit
	if auto {
		columns = make([]string, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			columns = append(columns, field.Name)
		}
	}
	out := make([]pipelineStrictValidationField, 0, len(columns))
	for _, column := range columns {
		field, ok := schema.field(column)
		if !ok {
			report.addError(nodeID, strPtr(column), "missing_column", fmt.Sprintf("join projection column %q does not exist", column))
			continue
		}
		field.Name = prefix + field.Name
		out = append(out, field)
	}
	return out
}

func outputPrimaryKeys(rawConfig json.RawMessage) []string {
	obj := map[string]json.RawMessage{}
	_ = json.Unmarshal(rawConfig, &obj)
	out := []string{}
	appendKey := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
			out = append(out, s)
			return
		}
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			out = append(out, arr...)
		}
	}
	for _, key := range []string{"primary_key", "primary_keys", "object_primary_key", "object_primary_keys"} {
		appendKey(obj[key])
	}
	if nested := obj["_output"]; len(nested) > 0 {
		nestedObj := map[string]json.RawMessage{}
		_ = json.Unmarshal(nested, &nestedObj)
		for _, key := range []string{"primary_key", "primary_keys", "object_primary_key", "object_primary_keys"} {
			appendKey(nestedObj[key])
		}
	}
	return compactStrings(out)
}

func outputRequiresPrimaryKey(transformType string, rawConfig json.RawMessage) bool {
	lower := strings.ToLower(transformType)
	if strings.Contains(lower, "object") || strings.Contains(lower, "ontology") {
		return true
	}
	obj := map[string]json.RawMessage{}
	_ = json.Unmarshal(rawConfig, &obj)
	nested := map[string]json.RawMessage{}
	if len(obj["_output"]) > 0 {
		_ = json.Unmarshal(obj["_output"], &nested)
	}
	for _, raw := range []json.RawMessage{obj["kind"], obj["output_type"], nested["kind"], nested["output_type"]} {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			v := strings.ToLower(strings.TrimSpace(value))
			if strings.Contains(v, "object") || strings.Contains(v, "ontology") {
				return true
			}
		}
	}
	return false
}

func checkDuplicateNames(nodeID string, names []string, code string, report *pipelineStrictValidationReport) {
	seen := map[string]struct{}{}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			report.addError(nodeID, strPtr(name), code, fmt.Sprintf("duplicate column name %q", name))
		}
		seen[name] = struct{}{}
	}
}

func checkSchemaDuplicateFields(nodeID string, schema pipelineStrictSchema, report *pipelineStrictValidationReport) {
	names := make([]string, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		names = append(names, field.Name)
	}
	checkDuplicateNames(nodeID, names, "duplicate_column_name", report)
}

func compatibleJoinTypes(left, right pipelineexpression.PipelineType) bool {
	return left.Equal(right) || (left.IsNumeric() && right.IsNumeric()) || (left.IsTextual() && right.IsTextual()) || (left.IsTemporal() && right.IsTemporal())
}

func compatibleUnionTypes(left, right pipelineexpression.PipelineType) bool {
	return compatibleAssignment(left, right) || compatibleAssignment(right, left)
}

func compatibleAssignment(actual, expected pipelineexpression.PipelineType) bool {
	return pipelineexpression.CanPromote(actual, expected)
}

func canExplicitCast(from, to pipelineexpression.PipelineType) bool {
	if from.Equal(to) || to.Kind == pipelineexpression.KindString || from.Kind == pipelineexpression.KindString {
		return true
	}
	if from.IsNumeric() && to.IsNumeric() {
		return true
	}
	if from.IsTemporal() && to.IsTemporal() {
		return true
	}
	if from.Kind == pipelineexpression.KindBoolean && to.Kind == pipelineexpression.KindBoolean {
		return true
	}
	if from.Kind == pipelineexpression.KindGeometry && to.Kind == pipelineexpression.KindGeometry {
		return true
	}
	return false
}

func validationCodeFromTypeError(err pipelineexpression.TypeError) string {
	switch err.Kind {
	case pipelineexpression.TypeErrUnknownColumn:
		return "missing_column"
	case pipelineexpression.TypeErrInvalidCastTarget:
		return "invalid_cast_target"
	default:
		return "expression_type_error"
	}
}

func strictColumnFromTypeError(err pipelineexpression.TypeError) *string {
	if err.Kind == pipelineexpression.TypeErrUnknownColumn {
		return strPtr(err.Detail)
	}
	return nil
}

func fieldLogicalType(field pipelineStrictValidationField) string {
	if raw, ok := firstFieldMetadataRaw(field.Metadata, "logical_type", "logicalType", "semantic_type", "semanticType"); ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return ""
}

func firstFieldMetadataRaw(metadata map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		if raw, ok := metadata[key]; ok {
			return raw, true
		}
	}
	return nil, false
}

func cloneStrictFields(fields []pipelineStrictValidationField) []pipelineStrictValidationField {
	out := make([]pipelineStrictValidationField, len(fields))
	for i, field := range fields {
		out[i] = field
		out[i].Metadata = cloneRawMap(field.Metadata)
	}
	return out
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func (r *pipelineStrictValidationReport) addError(nodeID string, column *string, code, message string) {
	err := pipelineStrictValidationError{NodeID: nodeID, Column: column, Code: code, Message: message}
	r.Errors = append(r.Errors, err)
	for i := range r.Nodes {
		if r.Nodes[i].NodeID == nodeID {
			r.Nodes[i].Status = "INVALID"
			r.Nodes[i].Errors = append(r.Nodes[i].Errors, err)
		}
	}
}

func (r *pipelineStrictValidationReport) addNode(nodeID string, schema pipelineStrictSchema) {
	errors := []pipelineStrictValidationError{}
	for _, err := range r.Errors {
		if err.NodeID == nodeID {
			errors = append(errors, err)
		}
	}
	status := "VALID"
	if len(errors) > 0 {
		status = "INVALID"
	}
	if errors == nil {
		errors = []pipelineStrictValidationError{}
	}
	nodeReport := pipelineStrictNodeReport{NodeID: nodeID, Status: status, Errors: errors}
	if schema.Known {
		nodeReport.OutputSchema = cloneStrictFields(schema.Fields)
	}
	r.Nodes = append(r.Nodes, nodeReport)
}

func (r pipelineStrictValidationReport) errorMessages() []string {
	out := make([]string, 0, len(r.Errors))
	for _, err := range r.Errors {
		if err.NodeID != "" {
			out = append(out, fmt.Sprintf("%s: %s", err.NodeID, err.Message))
		} else {
			out = append(out, err.Message)
		}
	}
	return out
}

func validationResponseFromStrictReport(report pipelineStrictValidationReport, ir models.PipelineIR) pipelineGraphValidationResponse {
	return pipelineGraphValidationResponse{
		Valid:     report.AllValid,
		Errors:    report.errorMessages(),
		Warnings:  []string{},
		NextRunAt: nil,
		Summary:   graphSummaryFromIR(ir),
		Nodes:     report.Nodes,
	}
}

func graphSummaryFromIR(ir models.PipelineIR) pipelineValidationSummary {
	dependents := map[string]int{}
	roots := []string{}
	for _, node := range ir.Nodes {
		if len(node.DependsOn) == 0 {
			roots = append(roots, node.ID)
		}
		for _, dep := range node.DependsOn {
			dependents[dep]++
		}
	}
	leaves := []string{}
	for _, node := range ir.Nodes {
		if dependents[node.ID] == 0 {
			leaves = append(leaves, node.ID)
		}
	}
	sort.Strings(roots)
	sort.Strings(leaves)
	return pipelineValidationSummary{NodeCount: len(ir.Nodes), EdgeCount: len(ir.Edges), RootNodeIDs: roots, LeafNodeIDs: leaves}
}

func writePipelineSchemaValidationFailure(w http.ResponseWriter, report pipelineStrictValidationReport) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pipeline_schema_validation_failed", "report": report})
}

func validationFailureForPipeline(pipeline *models.Pipeline) (*pipelineStrictValidationFailure, error) {
	report, err := validatePipelineStrictFromPipeline(pipeline)
	if err != nil {
		return nil, err
	}
	if report.AllValid {
		return nil, nil
	}
	return &pipelineStrictValidationFailure{Report: report}, nil
}

func validationFailureForRuntimePipeline(pipeline *models.Pipeline) (*pipelineStrictValidationFailure, error) {
	ir, err := pipeline.RuntimeIR()
	if err != nil {
		return nil, err
	}
	report := validatePipelineIRStrict(pipeline.ID.String(), ir)
	if report.AllValid {
		return nil, nil
	}
	return &pipelineStrictValidationFailure{Report: report}, nil
}
