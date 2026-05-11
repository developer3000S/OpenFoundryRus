package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

type pipelineSchemaGuidanceRequest struct {
	Status         string                         `json:"status,omitempty"`
	ScheduleConfig *models.PipelineScheduleConfig `json:"schedule_config,omitempty"`
	DAG            json.RawMessage                `json:"dag,omitempty"`
	IR             *models.PipelineIR             `json:"ir,omitempty"`
	Nodes          *[]models.PipelineNode         `json:"nodes,omitempty"`
	Kind           string                         `json:"kind,omitempty"`
	NodeID         string                         `json:"node_id,omitempty"`
	LeftNodeID     string                         `json:"left_node_id,omitempty"`
	RightNodeID    string                         `json:"right_node_id,omitempty"`
	InputNodeIDs   []string                       `json:"input_node_ids,omitempty"`
	Join           *runtimeJoinDraft              `json:"join,omitempty"`
	Union          *runtimeUnionDraft             `json:"union,omitempty"`
}

type pipelineSchemaGuidanceResponse struct {
	PipelineID  string                             `json:"pipeline_id"`
	Kind        string                             `json:"kind"`
	NodeID      string                             `json:"node_id,omitempty"`
	Valid       bool                               `json:"valid"`
	Errors      []pipelineStrictValidationError    `json:"errors,omitempty"`
	Join        *pipelineJoinSchemaGuidance        `json:"join,omitempty"`
	Union       *pipelineUnionSchemaGuidance       `json:"union,omitempty"`
	NodeSchemas []pipelineSchemaGuidanceNodeSchema `json:"node_schemas,omitempty"`
}

type pipelineSchemaGuidanceNodeSchema struct {
	NodeID string                          `json:"node_id"`
	Fields []pipelineStrictValidationField `json:"fields"`
}

type pipelineJoinSchemaGuidance struct {
	LeftNodeID       string                             `json:"left_node_id"`
	RightNodeID      string                             `json:"right_node_id"`
	LeftSchema       []pipelineStrictValidationField    `json:"left_schema"`
	RightSchema      []pipelineStrictValidationField    `json:"right_schema"`
	CandidateKeys    []pipelineJoinCandidateKey         `json:"candidate_keys"`
	MatchDiagnostics []pipelineSchemaGuidanceDiagnostic `json:"match_diagnostics"`
}

type pipelineJoinCandidateKey struct {
	LeftColumn  string `json:"left_column"`
	RightColumn string `json:"right_column"`
	LeftType    string `json:"left_type"`
	RightType   string `json:"right_type"`
	Compatible  bool   `json:"compatible"`
	Score       int    `json:"score"`
	Reason      string `json:"reason"`
}

type pipelineUnionSchemaGuidance struct {
	InputNodeIDs []string                           `json:"input_node_ids"`
	UnionType    string                             `json:"union_type"`
	InputSchemas []pipelineSchemaGuidanceNodeSchema `json:"input_schemas"`
	Diagnostics  []pipelineSchemaGuidanceDiagnostic `json:"diagnostics"`
	OutputSchema []pipelineStrictValidationField    `json:"output_schema,omitempty"`
}

type pipelineSchemaGuidanceDiagnostic struct {
	Severity    string  `json:"severity"`
	Code        string  `json:"code"`
	Message     string  `json:"message"`
	NodeID      string  `json:"node_id,omitempty"`
	Column      *string `json:"column,omitempty"`
	LeftColumn  string  `json:"left_column,omitempty"`
	RightColumn string  `json:"right_column,omitempty"`
	LeftType    string  `json:"left_type,omitempty"`
	RightType   string  `json:"right_type,omitempty"`
	InputIndex  int     `json:"input_index,omitempty"`
	InputNodeID string  `json:"input_node_id,omitempty"`
	Expected    string  `json:"expected,omitempty"`
	Actual      string  `json:"actual,omitempty"`
}

func PipelineSchemaGuidance(w http.ResponseWriter, r *http.Request) {
	req, _, err := decodePipelineSchemaGuidanceRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	ir, err := pipelineIRFromValidationRequest(req.validationRequest())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
		return
	}
	response, err := schemaGuidanceFromIR("draft", ir, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schema_guidance_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func PipelineSchemaGuidanceByID(w http.ResponseWriter, r *http.Request) {
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	req, hasBody, err := decodePipelineSchemaGuidanceRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	if hasBody && validationRequestHasGraph(req.validationRequest()) {
		ir, err := pipelineIRFromValidationRequest(req.validationRequest())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
			return
		}
		response, err := schemaGuidanceFromIR(pipelineID.String(), ir, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schema_guidance_failed", "detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	repo, ok := requirePipelineAuthoringRepository(w, "PipelineSchemaGuidanceByID requires DATABASE_URL-backed pipeline authoring repository wiring")
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
	ir, err := pipeline.ParsedIR()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
		return
	}
	response, err := schemaGuidanceFromIR(pipeline.ID.String(), ir, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schema_guidance_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func decodePipelineSchemaGuidanceRequest(r *http.Request) (pipelineSchemaGuidanceRequest, bool, error) {
	var req pipelineSchemaGuidanceRequest
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

func (r pipelineSchemaGuidanceRequest) validationRequest() validatePipelineGraphRequest {
	return validatePipelineGraphRequest{
		Status:         r.Status,
		ScheduleConfig: r.ScheduleConfig,
		DAG:            r.DAG,
		IR:             r.IR,
		Nodes:          r.Nodes,
	}
}

func schemaGuidanceFromIR(pipelineID string, ir models.PipelineIR, req pipelineSchemaGuidanceRequest) (pipelineSchemaGuidanceResponse, error) {
	ir = ir.Normalize()
	report, schemas := validatePipelineIRStrictWithSchemas(pipelineID, ir)
	nodes := map[string]models.PipelineIRNode{}
	for _, node := range ir.Nodes {
		nodes[node.ID] = node
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" && req.NodeID != "" {
		if node, ok := nodes[req.NodeID]; ok {
			kind = guidanceKindFromNode(node)
		}
	}
	if kind == "" {
		return pipelineSchemaGuidanceResponse{}, fmt.Errorf("kind is required when node_id cannot infer join or union")
	}
	response := pipelineSchemaGuidanceResponse{
		PipelineID: pipelineID,
		Kind:       kind,
		NodeID:     req.NodeID,
		Valid:      report.AllValid,
		Errors:     report.Errors,
	}
	switch kind {
	case "join":
		join, err := joinSchemaGuidance(req, nodes, schemas)
		if err != nil {
			return pipelineSchemaGuidanceResponse{}, err
		}
		response.Join = &join
		response.Valid = report.AllValid && !hasSchemaGuidanceErrors(join.MatchDiagnostics)
	case "union":
		union, err := unionSchemaGuidance(req, nodes, schemas)
		if err != nil {
			return pipelineSchemaGuidanceResponse{}, err
		}
		response.Union = &union
		response.NodeSchemas = union.InputSchemas
		response.Valid = report.AllValid && !hasSchemaGuidanceErrors(union.Diagnostics)
	default:
		return pipelineSchemaGuidanceResponse{}, fmt.Errorf("unsupported guidance kind %q", kind)
	}
	return response, nil
}

func guidanceKindFromNode(node models.PipelineIRNode) string {
	cfg, err := parseTableRuntimeConfig(node.Config)
	if err == nil {
		if cfg.Join != nil {
			return "join"
		}
		if cfg.Union != nil {
			return "union"
		}
	}
	switch strings.ToLower(strings.TrimSpace(node.TransformType)) {
	case "join", "table_join":
		return "join"
	case "union", "union_all":
		return "union"
	}
	switch normaliseTableTransform(node.TransformType) {
	case "join":
		return "join"
	case "union":
		return "union"
	}
	return ""
}

func hasSchemaGuidanceErrors(diagnostics []pipelineSchemaGuidanceDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		if strings.EqualFold(diagnostic.Severity, "error") {
			return true
		}
	}
	return false
}

func joinSchemaGuidance(req pipelineSchemaGuidanceRequest, nodes map[string]models.PipelineIRNode, schemas map[string]pipelineStrictSchema) (pipelineJoinSchemaGuidance, error) {
	leftID, rightID := strings.TrimSpace(req.LeftNodeID), strings.TrimSpace(req.RightNodeID)
	draft := req.Join
	if req.NodeID != "" {
		node, ok := nodes[req.NodeID]
		if !ok {
			return pipelineJoinSchemaGuidance{}, fmt.Errorf("node %q was not found", req.NodeID)
		}
		if leftID == "" && len(node.DependsOn) > 0 {
			leftID = node.DependsOn[0]
		}
		if rightID == "" && len(node.DependsOn) > 1 {
			rightID = node.DependsOn[1]
		}
		if draft == nil {
			cfg, _ := parseTableRuntimeConfig(node.Config)
			draft = cfg.Join
		}
	}
	if leftID == "" || rightID == "" {
		return pipelineJoinSchemaGuidance{}, fmt.Errorf("join guidance requires left_node_id and right_node_id")
	}
	if draft == nil {
		draft = &runtimeJoinDraft{JoinType: "left"}
	}
	left, right := schemas[leftID], schemas[rightID]
	guidance := pipelineJoinSchemaGuidance{
		LeftNodeID:       leftID,
		RightNodeID:      rightID,
		LeftSchema:       cloneStrictFields(left.Fields),
		RightSchema:      cloneStrictFields(right.Fields),
		CandidateKeys:    candidateJoinKeys(left, right),
		MatchDiagnostics: matchDiagnosticsForJoin(*draft, left, right),
	}
	if guidance.MatchDiagnostics == nil {
		guidance.MatchDiagnostics = []pipelineSchemaGuidanceDiagnostic{}
	}
	return guidance, nil
}

func candidateJoinKeys(left, right pipelineStrictSchema) []pipelineJoinCandidateKey {
	if !left.Known || !right.Known {
		return []pipelineJoinCandidateKey{}
	}
	out := []pipelineJoinCandidateKey{}
	for _, lf := range left.Fields {
		for _, rf := range right.Fields {
			score, reason := joinCandidateScore(lf.Name, rf.Name)
			if score == 0 {
				continue
			}
			compatible := compatibleJoinTypes(fieldType(lf), fieldType(rf))
			if compatible {
				score += 25
			}
			out = append(out, pipelineJoinCandidateKey{
				LeftColumn:  lf.Name,
				RightColumn: rf.Name,
				LeftType:    lf.FieldType,
				RightType:   rf.FieldType,
				Compatible:  compatible,
				Score:       score,
				Reason:      reason,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Compatible != out[j].Compatible {
			return out[i].Compatible
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].LeftColumn != out[j].LeftColumn {
			return out[i].LeftColumn < out[j].LeftColumn
		}
		return out[i].RightColumn < out[j].RightColumn
	})
	if len(out) > 12 {
		return out[:12]
	}
	return out
}

func joinCandidateScore(left, right string) (int, string) {
	lowerLeft, lowerRight := strings.ToLower(strings.TrimSpace(left)), strings.ToLower(strings.TrimSpace(right))
	if lowerLeft == "" || lowerRight == "" {
		return 0, ""
	}
	if lowerLeft == lowerRight {
		return 100, "same column name"
	}
	normLeft, normRight := normalizeGuidanceColumnName(lowerLeft), normalizeGuidanceColumnName(lowerRight)
	if normLeft == normRight {
		return 90, "same normalized column name"
	}
	for _, suffix := range []string{"id", "rid", "key", "uuid"} {
		if strings.HasSuffix(normLeft, suffix) && strings.HasSuffix(normRight, suffix) {
			return 65, "matching key-like suffix"
		}
	}
	if strings.Contains(normLeft, normRight) || strings.Contains(normRight, normLeft) {
		return 45, "similar column name"
	}
	return 0, ""
}

func matchDiagnosticsForJoin(draft runtimeJoinDraft, left, right pipelineStrictSchema) []pipelineSchemaGuidanceDiagnostic {
	out := []pipelineSchemaGuidanceDiagnostic{}
	if strings.ToLower(strings.TrimSpace(draft.JoinType)) != "cross" && len(validJoinMatches(draft.Matches)) == 0 {
		out = append(out, pipelineSchemaGuidanceDiagnostic{Severity: "error", Code: "join_missing_match_conditions", Message: "join requires at least one match condition"})
	}
	for _, match := range validJoinMatches(draft.Matches) {
		leftField, leftOK := left.field(match.LeftColumn)
		rightField, rightOK := right.field(match.RightColumn)
		if left.Known && !leftOK {
			out = append(out, pipelineSchemaGuidanceDiagnostic{Severity: "error", Code: "missing_join_column", Column: strPtr(match.LeftColumn), LeftColumn: match.LeftColumn, RightColumn: match.RightColumn, Message: fmt.Sprintf("left join column %q does not exist", match.LeftColumn)})
		}
		if right.Known && !rightOK {
			out = append(out, pipelineSchemaGuidanceDiagnostic{Severity: "error", Code: "missing_join_column", Column: strPtr(match.RightColumn), LeftColumn: match.LeftColumn, RightColumn: match.RightColumn, Message: fmt.Sprintf("right join column %q does not exist", match.RightColumn)})
		}
		if leftOK && rightOK && !compatibleJoinTypes(fieldType(leftField), fieldType(rightField)) {
			out = append(out, pipelineSchemaGuidanceDiagnostic{
				Severity:    "error",
				Code:        "incompatible_join_key_types",
				Column:      strPtr(match.LeftColumn),
				LeftColumn:  match.LeftColumn,
				RightColumn: match.RightColumn,
				LeftType:    leftField.FieldType,
				RightType:   rightField.FieldType,
				Message:     fmt.Sprintf("join key %q (%s) is not compatible with %q (%s)", match.LeftColumn, leftField.FieldType, match.RightColumn, rightField.FieldType),
			})
		}
	}
	return out
}

func unionSchemaGuidance(req pipelineSchemaGuidanceRequest, nodes map[string]models.PipelineIRNode, schemas map[string]pipelineStrictSchema) (pipelineUnionSchemaGuidance, error) {
	inputIDs := compactStrings(req.InputNodeIDs)
	unionType := "by_name"
	if req.Union != nil && strings.TrimSpace(req.Union.UnionType) != "" {
		unionType = req.Union.UnionType
	}
	if req.NodeID != "" {
		node, ok := nodes[req.NodeID]
		if !ok {
			return pipelineUnionSchemaGuidance{}, fmt.Errorf("node %q was not found", req.NodeID)
		}
		if len(inputIDs) == 0 {
			inputIDs = append([]string(nil), node.DependsOn...)
		}
		if req.Union == nil {
			cfg, _ := parseTableRuntimeConfig(node.Config)
			if cfg.Union != nil && strings.TrimSpace(cfg.Union.UnionType) != "" {
				unionType = cfg.Union.UnionType
			}
		}
	}
	if len(inputIDs) < 2 {
		return pipelineUnionSchemaGuidance{}, fmt.Errorf("union guidance requires at least two input_node_ids")
	}
	inputSchemas := make([]pipelineSchemaGuidanceNodeSchema, 0, len(inputIDs))
	deps := make([]pipelineStrictSchema, 0, len(inputIDs))
	for _, id := range inputIDs {
		schema := schemas[id]
		deps = append(deps, schema)
		inputSchemas = append(inputSchemas, pipelineSchemaGuidanceNodeSchema{NodeID: id, Fields: cloneStrictFields(schema.Fields)})
	}
	diagnostics, output := unionDiagnostics(unionType, inputIDs, deps)
	if diagnostics == nil {
		diagnostics = []pipelineSchemaGuidanceDiagnostic{}
	}
	return pipelineUnionSchemaGuidance{
		InputNodeIDs: inputIDs,
		UnionType:    unionType,
		InputSchemas: inputSchemas,
		Diagnostics:  diagnostics,
		OutputSchema: cloneStrictFields(output.Fields),
	}, nil
}

func unionDiagnostics(unionType string, inputIDs []string, deps []pipelineStrictSchema) ([]pipelineSchemaGuidanceDiagnostic, pipelineStrictSchema) {
	if len(deps) == 0 {
		return []pipelineSchemaGuidanceDiagnostic{}, pipelineStrictSchema{}
	}
	for _, dep := range deps {
		if !dep.Known {
			return []pipelineSchemaGuidanceDiagnostic{{Severity: "warning", Code: "unknown_input_schema", Message: "one or more input schemas are unknown"}}, pipelineStrictSchema{}
		}
	}
	out := []pipelineSchemaGuidanceDiagnostic{}
	base := deps[0]
	if strings.EqualFold(strings.TrimSpace(unionType), "by_position") {
		for idx, dep := range deps[1:] {
			inputIndex := idx + 2
			inputID := inputIDs[idx+1]
			if len(dep.Fields) != len(base.Fields) {
				out = append(out, pipelineSchemaGuidanceDiagnostic{
					Severity:    "error",
					Code:        "union_position_arity_mismatch",
					InputIndex:  inputIndex,
					InputNodeID: inputID,
					Expected:    fmt.Sprintf("%d columns", len(base.Fields)),
					Actual:      fmt.Sprintf("%d columns", len(dep.Fields)),
					Message:     fmt.Sprintf("union input %d has %d columns, expected %d", inputIndex, len(dep.Fields), len(base.Fields)),
				})
				continue
			}
			for i, field := range dep.Fields {
				expected := base.Fields[i]
				if !compatibleUnionTypes(fieldType(expected), fieldType(field)) {
					out = append(out, pipelineSchemaGuidanceDiagnostic{
						Severity:    "error",
						Code:        "incompatible_union_column_types",
						Column:      strPtr(field.Name),
						InputIndex:  inputIndex,
						InputNodeID: inputID,
						Expected:    expected.FieldType,
						Actual:      field.FieldType,
						Message:     fmt.Sprintf("union position %d has incompatible types %s and %s", i+1, expected.FieldType, field.FieldType),
					})
				}
			}
		}
		return out, base
	}
	for idx, dep := range deps[1:] {
		inputIndex := idx + 2
		inputID := inputIDs[idx+1]
		for _, field := range base.Fields {
			other, ok := dep.field(field.Name)
			if !ok {
				out = append(out, pipelineSchemaGuidanceDiagnostic{
					Severity:    "error",
					Code:        "missing_union_column",
					Column:      strPtr(field.Name),
					InputIndex:  inputIndex,
					InputNodeID: inputID,
					Expected:    field.FieldType,
					Message:     fmt.Sprintf("union input %d is missing column %q", inputIndex, field.Name),
				})
				continue
			}
			if !compatibleUnionTypes(fieldType(field), fieldType(other)) {
				out = append(out, pipelineSchemaGuidanceDiagnostic{
					Severity:    "error",
					Code:        "incompatible_union_column_types",
					Column:      strPtr(field.Name),
					InputIndex:  inputIndex,
					InputNodeID: inputID,
					Expected:    field.FieldType,
					Actual:      other.FieldType,
					Message:     fmt.Sprintf("union column %q has incompatible types %s and %s", field.Name, field.FieldType, other.FieldType),
				})
			}
		}
		for _, field := range dep.Fields {
			if _, ok := base.field(field.Name); !ok {
				out = append(out, pipelineSchemaGuidanceDiagnostic{
					Severity:    "warning",
					Code:        "extra_union_column",
					Column:      strPtr(field.Name),
					InputIndex:  inputIndex,
					InputNodeID: inputID,
					Actual:      field.FieldType,
					Message:     fmt.Sprintf("union input %d has extra column %q", inputIndex, field.Name),
				})
			}
		}
	}
	return out, base
}

func normalizeGuidanceColumnName(name string) string {
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(strings.ToLower(strings.TrimSpace(name)))
}
