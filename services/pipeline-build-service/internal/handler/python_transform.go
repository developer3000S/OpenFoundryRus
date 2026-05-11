package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	runtimepkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/runtime"
)

const (
	defaultPythonNodeTimeoutSeconds uint32 = 60
	maxPythonNodeTimeoutSeconds     uint32 = 300
)

type pythonPreparedInput struct {
	DatasetID string                       `json:"dataset_id"`
	NodeID    string                       `json:"node_id,omitempty"`
	Rows      []map[string]json.RawMessage `json:"rows"`
	Schema    []pythonPreparedInputField   `json:"schema,omitempty"`
	RowCount  int                          `json:"row_count"`
}

type pythonPreparedInputField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

func pythonSourceFromConfig(cfg map[string]json.RawMessage) string {
	return firstString(cfg, "source", "code", "python_source")
}

func pythonTransformTimeoutSeconds(cfg map[string]json.RawMessage) uint32 {
	timeout := firstUint32(cfg, "timeout_seconds", "timeout_secs")
	if timeout == 0 {
		return defaultPythonNodeTimeoutSeconds
	}
	if timeout > maxPythonNodeTimeoutSeconds {
		return maxPythonNodeTimeoutSeconds
	}
	return timeout
}

func pythonPreparedInputsJSON(table *lightweightTableRuntime, node executor.NodeContext, cfg map[string]json.RawMessage) ([]byte, error) {
	if raw := pythonConfigRaw(cfg, "prepared_inputs"); len(raw) > 0 && strings.TrimSpace(string(raw)) != "null" {
		return append([]byte(nil), raw...), nil
	}
	if table == nil || len(node.Node.DependsOn) == 0 {
		return []byte("[]"), nil
	}
	inputs := make([]pythonPreparedInput, 0, len(node.Node.DependsOn))
	for _, dep := range node.Node.DependsOn {
		rows, err := table.dependencyRows(dep)
		if err != nil {
			continue
		}
		rawRows := rowsToMaps(rows)
		inputs = append(inputs, pythonPreparedInput{
			DatasetID: dep,
			NodeID:    dep,
			Rows:      rawRows,
			Schema:    pythonPreparedInputSchema(rawRows),
			RowCount:  len(rawRows),
		})
	}
	if len(inputs) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(inputs)
}

func pythonPreparedInputSchema(rows []map[string]json.RawMessage) []pythonPreparedInputField {
	schema := schemaFromRows(rows)
	fields := make([]pythonPreparedInputField, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		fields = append(fields, pythonPreparedInputField{Name: field.Name, Type: field.FieldType, Nullable: field.Nullable})
	}
	return fields
}

func pythonRowsFromResult(result *runtimepkg.TransformResult) ([]map[string]json.RawMessage, error) {
	if result == nil {
		return nil, nil
	}
	rows := make([]map[string]json.RawMessage, 0, len(result.ResultRows))
	for _, raw := range result.ResultRows {
		var row map[string]json.RawMessage
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, fmt.Errorf("python_result_row_invalid: %w", err)
		}
		rows = append(rows, cloneRawRow(row))
	}
	if len(rows) > 0 || len(result.ResultRowsJSON) == 0 {
		return rows, nil
	}
	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(result.ResultRowsJSON, &decoded); err != nil {
		return nil, fmt.Errorf("python_result_rows_json_invalid: %w", err)
	}
	for _, row := range decoded {
		rows = append(rows, cloneRawRow(row))
	}
	return rows, nil
}

func pythonRuntimeRows(rows []map[string]json.RawMessage) []pipelineexpression.Row {
	out := make([]pipelineexpression.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, pipelineexpression.Row(cloneRawRow(row)))
	}
	return out
}

func validatePythonPackageConstraints(source string, cfg map[string]json.RawMessage) error {
	allowed := stringSet(pythonConfigStringSlice(cfg, "allowed_packages", "package_allowlist"))
	if len(allowed) == 0 {
		return nil
	}
	candidates := append(pythonConfigStringSlice(cfg, "packages", "requirements"), pythonImportedPackages(source)...)
	for _, candidate := range candidates {
		pkg := pythonPackageName(candidate)
		if pkg == "" {
			continue
		}
		if _, ok := allowed[pkg]; !ok {
			return fmt.Errorf("python_package_not_allowed:%s", pkg)
		}
	}
	return nil
}

func pythonImportedPackages(source string) []string {
	out := []string{}
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		switch {
		case strings.HasPrefix(line, "import "):
			for _, part := range strings.Split(strings.TrimPrefix(line, "import "), ",") {
				fields := strings.Fields(strings.TrimSpace(part))
				if len(fields) > 0 {
					out = append(out, fields[0])
				}
			}
		case strings.HasPrefix(line, "from "):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				out = append(out, fields[1])
			}
		}
	}
	return out
}

func pythonPackageName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "["} {
		if before, _, ok := strings.Cut(raw, sep); ok {
			raw = before
			break
		}
	}
	raw = strings.TrimSpace(raw)
	if before, _, ok := strings.Cut(raw, "."); ok {
		raw = before
	}
	return strings.ReplaceAll(raw, "-", "_")
}

func pythonConfigStringSlice(cfg map[string]json.RawMessage, keys ...string) []string {
	for _, key := range keys {
		raw := pythonConfigRaw(cfg, key)
		if len(raw) == 0 {
			continue
		}
		var list []string
		if json.Unmarshal(raw, &list) == nil {
			return list
		}
		var one string
		if json.Unmarshal(raw, &one) == nil && strings.TrimSpace(one) != "" {
			parts := strings.FieldsFunc(one, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
	}
	return nil
}

func pythonConfigRaw(cfg map[string]json.RawMessage, key string) json.RawMessage {
	if len(cfg[key]) > 0 {
		return cfg[key]
	}
	var nested map[string]json.RawMessage
	if len(cfg["config"]) > 0 && json.Unmarshal(cfg["config"], &nested) == nil {
		return nested[key]
	}
	return nil
}

func pythonIRSchemaFromConfig(cfg map[string]json.RawMessage, keys ...string) (models.PipelineIRSchema, bool) {
	for _, key := range keys {
		raw := pythonConfigRaw(cfg, key)
		if len(raw) == 0 {
			continue
		}
		var schema models.PipelineIRSchema
		if json.Unmarshal(raw, &schema) == nil && len(schema.Fields) > 0 {
			return schema, true
		}
		var fields []models.PipelineIRField
		if json.Unmarshal(raw, &fields) == nil && len(fields) > 0 {
			return models.PipelineIRSchema{Fields: fields}, true
		}
	}
	return models.PipelineIRSchema{}, false
}
