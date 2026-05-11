package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

type lightweightTableRuntime struct {
	mu        sync.Mutex
	tables    map[string][]pipelineexpression.Row
	functions PipelineFunctionRegistry
}

type tableRuntimeConfig struct {
	Rows           []map[string]json.RawMessage `json:"rows,omitempty"`
	SeedRows       []map[string]json.RawMessage `json:"seed_rows,omitempty"`
	Records        []map[string]json.RawMessage `json:"records,omitempty"`
	Data           []map[string]json.RawMessage `json:"data,omitempty"`
	Predicate      string                       `json:"predicate,omitempty"`
	Expression     string                       `json:"expression,omitempty"`
	Columns        []string                     `json:"columns,omitempty"`
	Select         []string                     `json:"select,omitempty"`
	DropColumns    []string                     `json:"drop_columns,omitempty"`
	Renames        map[string]string            `json:"renames,omitempty"`
	ColumnMappings []struct {
		SourceColumn string `json:"source_column"`
		TargetColumn string `json:"target_column"`
	} `json:"column_mappings,omitempty"`
	SQL                 string                    `json:"sql,omitempty"`
	SampleSize          int                       `json:"sample_size,omitempty"`
	FunctionID          string                    `json:"function_id,omitempty"`
	FunctionName        string                    `json:"function_name,omitempty"`
	FunctionVersion     string                    `json:"function_version,omitempty"`
	FunctionAutoUpgrade bool                      `json:"function_auto_upgrade,omitempty"`
	TargetColumn        string                    `json:"target_column,omitempty"`
	ResultType          string                    `json:"result_type,omitempty"`
	Arguments           []runtimeFunctionArgument `json:"arguments,omitempty"`
	Args                map[string]string         `json:"args,omitempty"`
	Stack               *runtimeTransformStack    `json:"_stack,omitempty"`
	Join                *runtimeJoinDraft         `json:"_join,omitempty"`
	GeoJoin             *runtimeGeoJoinDraft      `json:"_geo_join,omitempty"`
	Union               *runtimeUnionDraft        `json:"_union,omitempty"`
	GPX                 string                    `json:"gpx,omitempty"`
	GPXXML              string                    `json:"gpx_xml,omitempty"`
	GPXContent          string                    `json:"gpx_content,omitempty"`
	SourceColumn        string                    `json:"source_column,omitempty"`
	GPXColumn           string                    `json:"gpx_column,omitempty"`
	ContentColumn       string                    `json:"content_column,omitempty"`
	TrailID             string                    `json:"trail_id,omitempty"`
	TrailIDColumn       string                    `json:"trail_id_column,omitempty"`
	TrailName           string                    `json:"trail_name,omitempty"`
	TrailNameColumn     string                    `json:"trail_name_column,omitempty"`
	SourceName          string                    `json:"source_name,omitempty"`
	SourceNameColumn    string                    `json:"source_name_column,omitempty"`
	FileNameColumn      string                    `json:"file_name_column,omitempty"`
}

type runtimeTransformStack struct {
	Blocks []runtimeTransformBlock `json:"blocks,omitempty"`
}

type runtimeTransformBlock struct {
	Kind                    string                    `json:"kind,omitempty"`
	Applied                 bool                      `json:"applied,omitempty"`
	Columns                 []string                  `json:"columns,omitempty"`
	Renames                 []runtimeRenameMapping    `json:"renames,omitempty"`
	SourceColumn            string                    `json:"source_column,omitempty"`
	TargetType              string                    `json:"target_type,omitempty"`
	TargetColumn            string                    `json:"target_column,omitempty"`
	StartLatColumn          string                    `json:"start_lat_column,omitempty"`
	StartLonColumn          string                    `json:"start_lon_column,omitempty"`
	EndLatColumn            string                    `json:"end_lat_column,omitempty"`
	EndLonColumn            string                    `json:"end_lon_column,omitempty"`
	Lat1Column              string                    `json:"lat1_column,omitempty"`
	Lon1Column              string                    `json:"lon1_column,omitempty"`
	Lat2Column              string                    `json:"lat2_column,omitempty"`
	Lon2Column              string                    `json:"lon2_column,omitempty"`
	Unit                    string                    `json:"unit,omitempty"`
	Mode                    string                    `json:"mode,omitempty"`
	Match                   string                    `json:"match,omitempty"`
	Conditions              []runtimeFilterCondition  `json:"conditions,omitempty"`
	RemoveSpecialCharacters bool                      `json:"remove_special_characters,omitempty"`
	FunctionID              string                    `json:"function_id,omitempty"`
	FunctionName            string                    `json:"function_name,omitempty"`
	FunctionVersion         string                    `json:"function_version,omitempty"`
	FunctionAutoUpgrade     bool                      `json:"function_auto_upgrade,omitempty"`
	Arguments               []runtimeFunctionArgument `json:"arguments,omitempty"`
	Args                    map[string]string         `json:"args,omitempty"`
	ResultType              string                    `json:"result_type,omitempty"`
}

type runtimeFunctionArgument struct {
	Name       string          `json:"name,omitempty"`
	Expression string          `json:"expression,omitempty"`
	Column     string          `json:"column,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}

type runtimeRenameMapping struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type runtimeFilterCondition struct {
	Column           string `json:"column,omitempty"`
	Operator         string `json:"operator,omitempty"`
	Value            string `json:"value,omitempty"`
	TreatEmptyAsNull bool   `json:"treat_empty_as_null,omitempty"`
}

type runtimeJoinDraft struct {
	JoinType        string             `json:"join_type,omitempty"`
	Matches         []runtimeJoinMatch `json:"matches,omitempty"`
	LeftColumns     []string           `json:"left_columns,omitempty"`
	RightColumns    []string           `json:"right_columns,omitempty"`
	RightPrefix     string             `json:"right_prefix,omitempty"`
	AutoSelectLeft  bool               `json:"auto_select_left,omitempty"`
	AutoSelectRight bool               `json:"auto_select_right,omitempty"`
}

type runtimeJoinMatch struct {
	LeftColumn  string `json:"left_column,omitempty"`
	RightColumn string `json:"right_column,omitempty"`
}

type runtimeGeoJoinDraft struct {
	Mode                string   `json:"mode,omitempty"`
	JoinType            string   `json:"join_type,omitempty"`
	LeftGeometryColumn  string   `json:"left_geometry_column,omitempty"`
	RightGeometryColumn string   `json:"right_geometry_column,omitempty"`
	LeftLatColumn       string   `json:"left_lat_column,omitempty"`
	LeftLonColumn       string   `json:"left_lon_column,omitempty"`
	RightLatColumn      string   `json:"right_lat_column,omitempty"`
	RightLonColumn      string   `json:"right_lon_column,omitempty"`
	Unit                string   `json:"unit,omitempty"`
	MaxDistance         float64  `json:"max_distance,omitempty"`
	K                   int      `json:"k,omitempty"`
	DistanceColumn      string   `json:"distance_column,omitempty"`
	RankColumn          string   `json:"rank_column,omitempty"`
	LeftColumns         []string `json:"left_columns,omitempty"`
	RightColumns        []string `json:"right_columns,omitempty"`
	RightPrefix         string   `json:"right_prefix,omitempty"`
	AutoSelectLeft      bool     `json:"auto_select_left,omitempty"`
	AutoSelectRight     bool     `json:"auto_select_right,omitempty"`
	MaxLeftRows         int      `json:"max_left_rows,omitempty"`
	MaxRightRows        int      `json:"max_right_rows,omitempty"`
	MaxCandidatePairs   int      `json:"max_candidate_pairs,omitempty"`
}

type runtimeUnionDraft struct {
	UnionType string `json:"union_type,omitempty"`
}

func newRuntimeNodeRunner(ports ExecutionPorts) runtimeNodeRunner {
	distributed := ports.Distributed
	if distributed == nil {
		distributed = NewSparkFlinkDistributedRunner(DistributedRuntimeConfig{})
	}
	return runtimeNodeRunner{
		JobRunner: ports.JobRunner,
		Python:    ports.Python,
		LLM:       ports.LLM,
		Table:     newLightweightTableRuntime(ports.Functions),
		Dist:      distributed,
	}
}

func newLightweightTableRuntime(functions ...PipelineFunctionRegistry) *lightweightTableRuntime {
	var registry PipelineFunctionRegistry
	if len(functions) > 0 {
		registry = functions[0]
	}
	return &lightweightTableRuntime{tables: map[string][]pipelineexpression.Row{}, functions: registry}
}

func (rt *lightweightTableRuntime) Supports(transformType string) bool {
	switch normaliseTableTransform(transformType) {
	case "input", "filter", "select", "drop", "rename", "passthrough", "output", "sql", "function", "gpx_parse":
		return true
	default:
		return false
	}
}

func (rt *lightweightTableRuntime) Run(ctx context.Context, node executor.NodeContext, payload json.RawMessage, transformType string) (executor.NodeResult, error) {
	if err := ctx.Err(); err != nil {
		return executor.NodeResult{}, err
	}
	cfg, err := parseTableRuntimeConfig(payload)
	if err != nil {
		return executor.NodeResult{}, err
	}

	kind := normaliseTableTransform(transformType)
	var rows []pipelineexpression.Row
	switch kind {
	case "input":
		rows = cfg.inlineRows()
		if rows == nil {
			rows = syntheticRows(node.Node.ID, cfg.SampleSize)
		}
	case "filter":
		rows, err = rt.runFilter(node, cfg)
	case "select":
		rows, err = rt.runSelect(node, cfg)
	case "drop":
		rows, err = rt.runDrop(node, cfg)
	case "rename":
		rows, err = rt.runRename(node, cfg)
	case "passthrough":
		if len(node.Node.DependsOn) == 0 {
			rows = cfg.inlineRows()
			if rows == nil {
				rows = syntheticRows(node.Node.ID, cfg.SampleSize)
			}
			break
		}
		rows, err = rt.firstDependencyRows(node)
	case "output":
		rows, err = rt.firstDependencyRows(node)
	case "sql":
		rows, err = rt.runStructuredSQL(ctx, node, cfg)
	case "function":
		rows, err = rt.runFunction(ctx, node, cfg)
	case "gpx_parse":
		rows, err = rt.runGPXParse(node, cfg)
	default:
		err = fmt.Errorf("unsupported lightweight transform type: %s", transformType)
	}
	if err != nil {
		return executor.NodeResult{}, err
	}
	rt.storeRows(node.Node.ID, rows)

	metaRows := cloneRows(rows)
	if len(metaRows) > 5 {
		metaRows = metaRows[:5]
	}
	meta := map[string]any{
		"runtime":        "lightweight_table",
		"engine":         "pipeline-expression",
		"transform_type": transformType,
		"rows_affected":  len(rows),
		"columns":        deriveRuntimeColumns(rows),
		"sample_rows":    rowsToMaps(metaRows),
		"data_rows":      rowsToMaps(rows),
	}
	if kind == "function" {
		meta["function_id"] = cfg.FunctionID
		meta["function_name"] = cfg.FunctionName
		meta["function_version"] = cfg.FunctionVersion
		meta["target_column"] = cfg.TargetColumn
	}
	return executor.NodeResult{
		OutputContentHash: hashRuntimeRows(node.Node.ID, transformType, rows),
		Metadata:          meta,
	}, nil
}

func parseTableRuntimeConfig(payload json.RawMessage) (tableRuntimeConfig, error) {
	var cfg tableRuntimeConfig
	if len(payload) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return cfg, fmt.Errorf("parse lightweight table config: %w", err)
	}
	if !cfg.empty() {
		return cfg, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return cfg, nil
	}
	if nested := envelope["config"]; len(nested) > 0 {
		return parseTableRuntimeConfig(nested)
	}
	return cfg, nil
}

func (cfg tableRuntimeConfig) empty() bool {
	return len(cfg.Rows) == 0 &&
		len(cfg.SeedRows) == 0 &&
		len(cfg.Records) == 0 &&
		len(cfg.Data) == 0 &&
		cfg.Predicate == "" &&
		cfg.Expression == "" &&
		len(cfg.Columns) == 0 &&
		len(cfg.Select) == 0 &&
		len(cfg.DropColumns) == 0 &&
		len(cfg.Renames) == 0 &&
		len(cfg.ColumnMappings) == 0 &&
		cfg.SQL == "" &&
		cfg.FunctionID == "" &&
		cfg.FunctionName == "" &&
		cfg.FunctionVersion == "" &&
		cfg.TargetColumn == "" &&
		cfg.ResultType == "" &&
		len(cfg.Arguments) == 0 &&
		len(cfg.Args) == 0 &&
		cfg.Stack == nil &&
		cfg.Join == nil &&
		cfg.GeoJoin == nil &&
		cfg.Union == nil &&
		cfg.GPX == "" &&
		cfg.GPXXML == "" &&
		cfg.GPXContent == "" &&
		cfg.SourceColumn == "" &&
		cfg.GPXColumn == "" &&
		cfg.ContentColumn == "" &&
		cfg.TrailID == "" &&
		cfg.TrailIDColumn == "" &&
		cfg.TrailName == "" &&
		cfg.TrailNameColumn == "" &&
		cfg.SourceName == "" &&
		cfg.SourceNameColumn == "" &&
		cfg.FileNameColumn == ""
}

func (cfg tableRuntimeConfig) inlineRows() []pipelineexpression.Row {
	for _, raw := range [][]map[string]json.RawMessage{cfg.Rows, cfg.SeedRows, cfg.Records, cfg.Data} {
		if len(raw) > 0 {
			out := make([]pipelineexpression.Row, 0, len(raw))
			for _, row := range raw {
				out = append(out, cloneRow(pipelineexpression.Row(row)))
			}
			return out
		}
	}
	return nil
}

func (rt *lightweightTableRuntime) runFilter(node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	predicate := strings.TrimSpace(firstNonEmpty(cfg.Predicate, cfg.Expression))
	if predicate == "" {
		return nil, errors.New("lightweight_filter_missing_predicate")
	}
	parsed, err := pipelineexpression.ParseExpr(predicate)
	if err != nil {
		return nil, fmt.Errorf("lightweight_filter_parse: %w", err)
	}
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		result, err := pipelineexpression.Eval(parsed, row)
		if err != nil {
			return nil, fmt.Errorf("lightweight_filter_eval: %w", err)
		}
		if keep, ok := result.AsBool(); ok && keep {
			out = append(out, cloneRow(row))
		}
	}
	return out, nil
}

func (rt *lightweightTableRuntime) runSelect(node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	columns := cfg.Columns
	if len(columns) == 0 {
		columns = cfg.Select
	}
	if len(columns) == 0 {
		return cloneRows(rows), nil
	}
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		next := pipelineexpression.Row{}
		for _, col := range columns {
			if value, ok := row[col]; ok {
				next[col] = append(json.RawMessage(nil), value...)
			}
		}
		out = append(out, next)
	}
	return out, nil
}

func (rt *lightweightTableRuntime) runDrop(node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	columns := cfg.DropColumns
	if len(columns) == 0 {
		columns = cfg.Columns
	}
	drop := map[string]struct{}{}
	for _, col := range columns {
		drop[col] = struct{}{}
	}
	out := cloneRows(rows)
	for _, row := range out {
		for col := range drop {
			delete(row, col)
		}
	}
	return out, nil
}

func (rt *lightweightTableRuntime) runRename(node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	renames := map[string]string{}
	for source, target := range cfg.Renames {
		renames[source] = target
	}
	for _, mapping := range cfg.ColumnMappings {
		if mapping.SourceColumn != "" && mapping.TargetColumn != "" {
			renames[mapping.SourceColumn] = mapping.TargetColumn
		}
	}
	out := cloneRows(rows)
	for _, row := range out {
		for source, target := range renames {
			if value, ok := row[source]; ok {
				row[target] = append(json.RawMessage(nil), value...)
				delete(row, source)
			}
		}
	}
	return out, nil
}

func (rt *lightweightTableRuntime) runStructuredSQL(ctx context.Context, node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	if cfg.Stack != nil {
		return rt.runTransformStack(ctx, node, *cfg.Stack)
	}
	if cfg.Join != nil {
		return rt.runJoin(node, *cfg.Join)
	}
	if cfg.GeoJoin != nil {
		return rt.runGeoJoin(node, *cfg.GeoJoin)
	}
	if cfg.Union != nil {
		return rt.runUnion(node, *cfg.Union)
	}
	if cfg.FunctionID != "" || cfg.FunctionName != "" {
		return rt.runFunction(ctx, node, cfg)
	}
	if cfg.Predicate != "" || cfg.Expression != "" {
		filtered, err := rt.runFilter(node, cfg)
		if err != nil {
			return nil, err
		}
		if len(cfg.Columns) > 0 || len(cfg.Select) > 0 {
			rt.storeRows(node.Node.ID+":sql-filter", filtered)
			filterNode := node
			filterNode.Node.DependsOn = []string{node.Node.ID + ":sql-filter"}
			return rt.runSelect(filterNode, cfg)
		}
		return filtered, nil
	}
	if len(cfg.Columns) > 0 || len(cfg.Select) > 0 {
		return rt.runSelect(node, cfg)
	}
	if rows := cfg.inlineRows(); rows != nil {
		return rows, nil
	}
	if len(node.Node.DependsOn) > 0 {
		return rt.firstDependencyRows(node)
	}
	return nil, errors.New("lightweight_sql_requires_rows_or_structured_config")
}

func (rt *lightweightTableRuntime) runTransformStack(ctx context.Context, node executor.NodeContext, stack runtimeTransformStack) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	out := cloneRows(rows)
	for _, block := range stack.Blocks {
		if !block.Applied {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(block.Kind)) {
		case "filter":
			out, err = applyRuntimeFilterBlock(out, block)
		case "select":
			out = applyRuntimeSelectBlock(out, block.Columns)
		case "drop":
			out = applyRuntimeDropBlock(out, block.Columns)
		case "rename":
			out = applyRuntimeRenameBlock(out, block.Renames)
		case "normalize":
			out = applyRuntimeNormalizeBlock(out, block.RemoveSpecialCharacters)
		case "cast":
			out = applyRuntimeCastBlock(out, block)
		case "haversine", "haversine_distance", "geo_distance":
			out, err = applyRuntimeHaversineBlock(out, block)
		case "function", "udf", "reusable_function":
			out, err = rt.applyFunction(ctx, out, runtimeFunctionConfig{
				FunctionID:          block.FunctionID,
				FunctionName:        block.FunctionName,
				FunctionVersion:     block.FunctionVersion,
				FunctionAutoUpgrade: block.FunctionAutoUpgrade,
				TargetColumn:        block.TargetColumn,
				ResultType:          block.ResultType,
				Arguments:           block.Arguments,
				Args:                block.Args,
			})
		default:
			err = fmt.Errorf("lightweight_stack_unsupported_block:%s", block.Kind)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type runtimeFunctionConfig struct {
	FunctionID          string
	FunctionName        string
	FunctionVersion     string
	FunctionAutoUpgrade bool
	TargetColumn        string
	ResultType          string
	Arguments           []runtimeFunctionArgument
	Args                map[string]string
}

func (rt *lightweightTableRuntime) runFunction(ctx context.Context, node executor.NodeContext, cfg tableRuntimeConfig) ([]pipelineexpression.Row, error) {
	rows, err := rt.firstDependencyRows(node)
	if err != nil {
		return nil, err
	}
	return rt.applyFunction(ctx, rows, runtimeFunctionConfig{
		FunctionID:          cfg.FunctionID,
		FunctionName:        cfg.FunctionName,
		FunctionVersion:     cfg.FunctionVersion,
		FunctionAutoUpgrade: cfg.FunctionAutoUpgrade,
		TargetColumn:        cfg.TargetColumn,
		ResultType:          cfg.ResultType,
		Arguments:           cfg.Arguments,
		Args:                cfg.Args,
	})
}

func (rt *lightweightTableRuntime) applyFunction(ctx context.Context, rows []pipelineexpression.Row, cfg runtimeFunctionConfig) ([]pipelineexpression.Row, error) {
	if rt.functions == nil {
		return nil, errors.New("pipeline_function_registry_not_configured")
	}
	def, err := rt.functions.ResolvePipelineFunction(ctx, PipelineFunctionRef{
		ID:          cfg.FunctionID,
		Name:        cfg.FunctionName,
		Version:     cfg.FunctionVersion,
		AutoUpgrade: cfg.FunctionAutoUpgrade,
	})
	if err != nil {
		return nil, fmt.Errorf("pipeline_function_resolve_failed: %w", err)
	}
	def = normalisePipelineFunction(def)
	switch def.Runtime {
	case PipelineFunctionRuntimeExpression:
		return applyExpressionFunction(rows, def, cfg)
	case PipelineFunctionRuntimePython:
		return nil, errors.New("python_reusable_function_runtime_not_configured")
	default:
		return nil, fmt.Errorf("unsupported_pipeline_function_runtime:%s", def.Runtime)
	}
}

func applyExpressionFunction(rows []pipelineexpression.Row, def PipelineFunctionDefinition, cfg runtimeFunctionConfig) ([]pipelineexpression.Row, error) {
	if strings.TrimSpace(def.Expression) == "" {
		return nil, fmt.Errorf("pipeline_function_missing_expression:%s", def.Name)
	}
	expr, err := pipelineexpression.ParseExpr(def.Expression)
	if err != nil {
		return nil, fmt.Errorf("pipeline_function_expression_parse:%w", err)
	}
	target := strings.TrimSpace(cfg.TargetColumn)
	if target == "" {
		target = def.Name
	}
	if target == "" {
		return nil, errors.New("pipeline_function_missing_target_column")
	}
	argExprs, err := compileFunctionArguments(def, cfg)
	if err != nil {
		return nil, err
	}
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		scope := cloneRow(row)
		for name, argExpr := range argExprs {
			value, err := pipelineexpression.Eval(argExpr, row)
			if err != nil {
				return nil, fmt.Errorf("pipeline_function_argument_eval:%s:%w", name, err)
			}
			scope[name] = value.ToJSON()
		}
		value, err := pipelineexpression.Eval(expr, scope)
		if err != nil {
			return nil, fmt.Errorf("pipeline_function_eval:%s:%w", def.Name, err)
		}
		next := cloneRow(row)
		next[target] = value.ToJSON()
		out = append(out, next)
	}
	return out, nil
}

func compileFunctionArguments(def PipelineFunctionDefinition, cfg runtimeFunctionConfig) (map[string]pipelineexpression.Expr, error) {
	byName := map[string]string{}
	for key, value := range cfg.Args {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			byName[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	for _, arg := range cfg.Arguments {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		switch {
		case strings.TrimSpace(arg.Expression) != "":
			byName[name] = strings.TrimSpace(arg.Expression)
		case strings.TrimSpace(arg.Column) != "":
			byName[name] = strings.TrimSpace(arg.Column)
		case len(arg.Value) > 0:
			byName[name] = strings.TrimSpace(string(arg.Value))
		}
	}

	compiled := map[string]pipelineexpression.Expr{}
	for _, param := range def.Parameters {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		exprText := strings.TrimSpace(byName[name])
		if exprText == "" {
			exprText = name
		}
		expr, err := pipelineexpression.ParseExpr(exprText)
		if err != nil {
			return nil, fmt.Errorf("pipeline_function_argument_parse:%s:%w", name, err)
		}
		compiled[name] = expr
	}
	return compiled, nil
}

func (rt *lightweightTableRuntime) runJoin(node executor.NodeContext, draft runtimeJoinDraft) ([]pipelineexpression.Row, error) {
	if len(node.Node.DependsOn) < 2 {
		return nil, errors.New("lightweight_join_requires_two_inputs")
	}
	left, err := rt.dependencyRows(node.Node.DependsOn[0])
	if err != nil {
		return nil, err
	}
	right, err := rt.dependencyRows(node.Node.DependsOn[1])
	if err != nil {
		return nil, err
	}
	joinType := strings.ToLower(strings.TrimSpace(draft.JoinType))
	if joinType == "" {
		joinType = "left"
	}
	if joinType != "cross" && len(validJoinMatches(draft.Matches)) == 0 {
		return nil, errors.New("lightweight_join_missing_match_conditions")
	}

	leftColumns := selectRuntimeColumns(left, draft.LeftColumns, draft.AutoSelectLeft || len(draft.LeftColumns) == 0)
	rightColumns := selectRuntimeColumns(right, draft.RightColumns, draft.AutoSelectRight)
	out := make([]pipelineexpression.Row, 0)
	rightMatched := make([]bool, len(right))
	for _, lrow := range left {
		matched := false
		for idx, rrow := range right {
			if rowsJoinMatch(lrow, rrow, draft.Matches, joinType == "cross") {
				matched = true
				rightMatched[idx] = true
				out = append(out, composeRuntimeJoinRow(lrow, rrow, leftColumns, rightColumns, draft.RightPrefix))
			}
		}
		if !matched && (joinType == "left" || joinType == "outer") {
			out = append(out, composeRuntimeJoinRow(lrow, nil, leftColumns, rightColumns, draft.RightPrefix))
		}
	}
	if joinType == "right" || joinType == "outer" {
		for idx, rrow := range right {
			if rightMatched[idx] {
				continue
			}
			out = append(out, composeRuntimeJoinRow(nil, rrow, leftColumns, rightColumns, draft.RightPrefix))
		}
	}
	return out, nil
}

func (rt *lightweightTableRuntime) runUnion(node executor.NodeContext, draft runtimeUnionDraft) ([]pipelineexpression.Row, error) {
	if len(node.Node.DependsOn) < 2 {
		return nil, errors.New("lightweight_union_requires_two_inputs")
	}
	batches := make([][]pipelineexpression.Row, 0, len(node.Node.DependsOn))
	for _, dep := range node.Node.DependsOn {
		rows, err := rt.dependencyRows(dep)
		if err != nil {
			return nil, err
		}
		batches = append(batches, rows)
	}
	if strings.EqualFold(strings.TrimSpace(draft.UnionType), "by_position") {
		out := []pipelineexpression.Row{}
		for _, batch := range batches {
			out = append(out, cloneRows(batch)...)
		}
		return out, nil
	}
	columns := []string{}
	seen := map[string]struct{}{}
	for _, batch := range batches {
		for _, col := range deriveRuntimeColumns(batch) {
			if _, ok := seen[col]; ok {
				continue
			}
			seen[col] = struct{}{}
			columns = append(columns, col)
		}
	}
	out := []pipelineexpression.Row{}
	for _, batch := range batches {
		for _, row := range batch {
			next := pipelineexpression.Row{}
			for _, col := range columns {
				if value, ok := row[col]; ok {
					next[col] = append(json.RawMessage(nil), value...)
				} else {
					next[col] = json.RawMessage("null")
				}
			}
			out = append(out, next)
		}
	}
	return out, nil
}

func (rt *lightweightTableRuntime) firstDependencyRows(node executor.NodeContext) ([]pipelineexpression.Row, error) {
	if len(node.Node.DependsOn) == 0 {
		return nil, fmt.Errorf("lightweight_table_missing_input:%s", node.Node.ID)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, dep := range node.Node.DependsOn {
		if rows, ok := rt.tables[dep]; ok {
			return cloneRows(rows), nil
		}
	}
	return nil, fmt.Errorf("lightweight_table_input_not_ready:%s", strings.Join(node.Node.DependsOn, ","))
}

func (rt *lightweightTableRuntime) dependencyRows(nodeID string) ([]pipelineexpression.Row, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rows, ok := rt.tables[nodeID]
	if !ok {
		return nil, fmt.Errorf("lightweight_table_input_not_ready:%s", nodeID)
	}
	return cloneRows(rows), nil
}

func (rt *lightweightTableRuntime) storeRows(nodeID string, rows []pipelineexpression.Row) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.tables[nodeID] = cloneRows(rows)
}

func (rt *lightweightTableRuntime) snapshotRows(nodeID string) []pipelineexpression.Row {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return cloneRows(rt.tables[nodeID])
}

func (rt *lightweightTableRuntime) trimRows(nodeID string, limit int) {
	if limit <= 0 {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rows, ok := rt.tables[nodeID]; ok && len(rows) > limit {
		rt.tables[nodeID] = cloneRows(rows[:limit])
	}
}

func applyRuntimeFilterBlock(rows []pipelineexpression.Row, block runtimeTransformBlock) ([]pipelineexpression.Row, error) {
	if len(block.Conditions) == 0 {
		return nil, errors.New("lightweight_stack_filter_missing_conditions")
	}
	mode := strings.ToLower(strings.TrimSpace(block.Mode))
	if mode == "" {
		mode = "keep"
	}
	match := strings.ToLower(strings.TrimSpace(block.Match))
	if match == "" {
		match = "all"
	}
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		conditionResults := make([]bool, 0, len(block.Conditions))
		for _, condition := range block.Conditions {
			if strings.TrimSpace(condition.Column) == "" {
				return nil, errors.New("lightweight_stack_filter_missing_column")
			}
			conditionResults = append(conditionResults, runtimeConditionMatches(row, condition))
		}
		ok := match == "any" && anyBool(conditionResults)
		if match != "any" {
			ok = allBool(conditionResults)
		}
		if mode == "drop" {
			ok = !ok
		}
		if ok {
			out = append(out, cloneRow(row))
		}
	}
	return out, nil
}

func applyRuntimeDropBlock(rows []pipelineexpression.Row, columns []string) []pipelineexpression.Row {
	drop := map[string]struct{}{}
	for _, col := range columns {
		if strings.TrimSpace(col) != "" {
			drop[col] = struct{}{}
		}
	}
	out := cloneRows(rows)
	for _, row := range out {
		for col := range drop {
			delete(row, col)
		}
	}
	return out
}

func applyRuntimeSelectBlock(rows []pipelineexpression.Row, columns []string) []pipelineexpression.Row {
	if len(columns) == 0 {
		return cloneRows(rows)
	}
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		next := pipelineexpression.Row{}
		for _, col := range columns {
			if strings.TrimSpace(col) == "" {
				continue
			}
			if value, ok := row[col]; ok {
				next[col] = append(json.RawMessage(nil), value...)
			}
		}
		out = append(out, next)
	}
	return out
}

func applyRuntimeRenameBlock(rows []pipelineexpression.Row, renames []runtimeRenameMapping) []pipelineexpression.Row {
	out := cloneRows(rows)
	for _, row := range out {
		for _, mapping := range renames {
			if strings.TrimSpace(mapping.From) == "" || strings.TrimSpace(mapping.To) == "" {
				continue
			}
			if value, ok := row[mapping.From]; ok {
				row[mapping.To] = append(json.RawMessage(nil), value...)
				delete(row, mapping.From)
			}
		}
	}
	return out
}

func applyRuntimeNormalizeBlock(rows []pipelineexpression.Row, removeSpecial bool) []pipelineexpression.Row {
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		next := pipelineexpression.Row{}
		for col, value := range row {
			target := normalizeRuntimeColumnName(col, removeSpecial)
			if target == "" {
				target = col
			}
			next[target] = append(json.RawMessage(nil), value...)
		}
		out = append(out, next)
	}
	return out
}

func applyRuntimeCastBlock(rows []pipelineexpression.Row, block runtimeTransformBlock) []pipelineexpression.Row {
	source := strings.TrimSpace(block.SourceColumn)
	if source == "" {
		return cloneRows(rows)
	}
	target := strings.TrimSpace(block.TargetColumn)
	if target == "" {
		target = source
	}
	out := cloneRows(rows)
	for _, row := range out {
		raw, ok := row[source]
		if !ok {
			continue
		}
		row[target] = castRuntimeJSON(raw, block.TargetType)
		if target != source {
			delete(row, source)
		}
	}
	return out
}

func runtimeConditionMatches(row pipelineexpression.Row, condition runtimeFilterCondition) bool {
	raw, ok := row[condition.Column]
	nullish := !ok || isRuntimeNullish(raw, condition.TreatEmptyAsNull)
	switch strings.ToLower(strings.TrimSpace(condition.Operator)) {
	case "is_null":
		return nullish
	case "is_not_null", "":
		return !nullish
	case "equals":
		return runtimeScalarString(raw) == condition.Value
	case "not_equals":
		return runtimeScalarString(raw) != condition.Value
	case "greater_than":
		left, lok := runtimeScalarFloat(raw)
		right, rok := strconv.ParseFloat(condition.Value, 64)
		return lok && rok == nil && left > right
	case "less_than":
		left, lok := runtimeScalarFloat(raw)
		right, rok := strconv.ParseFloat(condition.Value, 64)
		return lok && rok == nil && left < right
	default:
		return false
	}
}

func isRuntimeNullish(raw json.RawMessage, treatEmptyAsNull bool) bool {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return true
	}
	if !treatEmptyAsNull {
		return false
	}
	return runtimeScalarString(raw) == ""
}

func runtimeScalarString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true"
		}
		return "false"
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.TrimSpace(string(raw))
}

func runtimeScalarFloat(raw json.RawMessage) (float64, bool) {
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return parsed, err == nil
	}
	return 0, false
}

func castRuntimeJSON(raw json.RawMessage, targetType string) json.RawMessage {
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case "integer", "long", "int", "bigint":
		if f, ok := runtimeScalarFloat(raw); ok {
			return mustRuntimeJSON(int64(f))
		}
	case "double", "float":
		if f, ok := runtimeScalarFloat(raw); ok {
			return mustRuntimeJSON(f)
		}
	case "boolean", "bool":
		if s := strings.ToLower(runtimeScalarString(raw)); s == "true" || s == "1" {
			return json.RawMessage("true")
		} else if s == "false" || s == "0" {
			return json.RawMessage("false")
		}
	case "string", "varchar", "date", "timestamp":
		return mustRuntimeJSON(runtimeScalarString(raw))
	}
	return append(json.RawMessage(nil), raw...)
}

func normalizeRuntimeColumnName(name string, removeSpecial bool) string {
	var b strings.Builder
	var prevUnderscore bool
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 && !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		case r == '_' || unicode.IsSpace(r) || r == '-':
			if !prevUnderscore {
				b.WriteRune('_')
				prevUnderscore = true
			}
		case !removeSpecial:
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		}
	}
	return strings.Trim(b.String(), "_")
}

func validJoinMatches(matches []runtimeJoinMatch) []runtimeJoinMatch {
	out := make([]runtimeJoinMatch, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.LeftColumn) != "" && strings.TrimSpace(match.RightColumn) != "" {
			out = append(out, match)
		}
	}
	return out
}

func rowsJoinMatch(left, right pipelineexpression.Row, matches []runtimeJoinMatch, cross bool) bool {
	if cross {
		return true
	}
	for _, match := range validJoinMatches(matches) {
		if runtimeScalarString(left[match.LeftColumn]) != runtimeScalarString(right[match.RightColumn]) {
			return false
		}
	}
	return true
}

func selectRuntimeColumns(rows []pipelineexpression.Row, explicit []string, auto bool) []string {
	if !auto {
		return append([]string(nil), explicit...)
	}
	return deriveRuntimeColumns(rows)
}

func composeRuntimeJoinRow(left, right pipelineexpression.Row, leftColumns, rightColumns []string, rightPrefix string) pipelineexpression.Row {
	out := pipelineexpression.Row{}
	for _, col := range leftColumns {
		if left != nil {
			if value, ok := left[col]; ok {
				out[col] = append(json.RawMessage(nil), value...)
				continue
			}
		}
		out[col] = json.RawMessage("null")
	}
	for _, col := range rightColumns {
		target := col
		if strings.TrimSpace(rightPrefix) != "" {
			target = strings.TrimSpace(rightPrefix) + col
		}
		if right != nil {
			if value, ok := right[col]; ok {
				out[target] = append(json.RawMessage(nil), value...)
				continue
			}
		}
		out[target] = json.RawMessage("null")
	}
	return out
}

func anyBool(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func allBool(values []bool) bool {
	for _, value := range values {
		if !value {
			return false
		}
	}
	return true
}

func normaliseTableTransform(transformType string) string {
	switch strings.ToLower(strings.TrimSpace(transformType)) {
	case "dataset_input", "input", "source", "source_dataset", "table_input", "external":
		return "input"
	case "filter", "row_filter":
		return "filter"
	case "select", "project", "projection":
		return "select"
	case "drop", "drop_columns":
		return "drop"
	case "rename", "rename_columns":
		return "rename"
	case "passthrough", "noop":
		return "passthrough"
	case "output", "dataset_output", "output_dataset", "table_output", "output_object_type", "object_type_output", "output_link_type", "link_type_output":
		return "output"
	case "sql", "structured_sql", "join", "table_join", "union", "union_all", "transform_stack",
		"geo_join", "geospatial_join", "geometry_join",
		"geo_distance_join", "geometry_distance_join",
		"geo_intersection_join", "geometry_intersection_join",
		"geo_nearest_join", "geo_nearest_neighbor_join", "geometry_nearest_neighbor_join", "knn_geo_join":
		return "sql"
	case "function", "function_call", "udf", "reusable_function":
		return "function"
	case "gpx_parse", "parse_gpx", "gpx", "gpx_parser":
		return "gpx_parse"
	default:
		return strings.ToLower(strings.TrimSpace(transformType))
	}
}

func syntheticRows(nodeID string, sampleSize int) []pipelineexpression.Row {
	if sampleSize <= 0 || sampleSize > 100 {
		sampleSize = 10
	}
	out := make([]pipelineexpression.Row, 0, sampleSize)
	for i := 0; i < sampleSize; i++ {
		out = append(out, pipelineexpression.Row{
			"id":          mustRuntimeJSON(i),
			"source_node": mustRuntimeJSON(nodeID),
			"synthetic":   json.RawMessage("true"),
			"value":       mustRuntimeJSON(i * 10),
		})
	}
	return out
}

func cloneRows(rows []pipelineexpression.Row) []pipelineexpression.Row {
	out := make([]pipelineexpression.Row, len(rows))
	for i, row := range rows {
		out[i] = cloneRow(row)
	}
	return out
}

func cloneRow(row pipelineexpression.Row) pipelineexpression.Row {
	out := make(pipelineexpression.Row, len(row))
	for k, v := range row {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func rowsToMaps(rows []pipelineexpression.Row) []map[string]json.RawMessage {
	out := make([]map[string]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		next := map[string]json.RawMessage{}
		for k, v := range row {
			next[k] = append(json.RawMessage(nil), v...)
		}
		out = append(out, next)
	}
	return out
}

func deriveRuntimeColumns(rows []pipelineexpression.Row) []string {
	if len(rows) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	for _, row := range rows {
		for col := range row {
			seen[col] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for col := range seen {
		out = append(out, col)
	}
	sort.Strings(out)
	return out
}

func hashRuntimeRows(nodeID, transformType string, rows []pipelineexpression.Row) string {
	payload, _ := json.Marshal(rowsToMaps(rows))
	sum := sha256.Sum256(append([]byte(nodeID+"|"+transformType+"|"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mustRuntimeJSON(v any) json.RawMessage {
	out, _ := json.Marshal(v)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
