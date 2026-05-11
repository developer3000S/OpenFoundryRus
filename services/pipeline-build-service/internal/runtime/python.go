// Package runtime wires pipeline-build Python transforms to the
// openfoundry-pyruntime sidecar via libs/python-sidecar.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

const defaultTransformTimeoutSeconds uint32 = 60

// TransformRequest is the sidecar-ready envelope for one Python
// transform. It is the Go replacement for the locals Rust injected
// into PyO3 and maps 1:1 to runtime.ExecutePipelineTransformRequest:
// source, config_json, prepared_inputs_json, input_dataset_ids,
// output_dataset_id and timeout_seconds.
type TransformRequest struct {
	Source             string
	ConfigJSON         []byte
	PreparedInputsJSON []byte
	InputDatasetIDs    []string
	OutputDatasetID    string
	TimeoutSeconds     uint32
}

// TransformResult is the Go runtime model equivalent of Rust's
// PythonExecutionResult plus the normalized result_rows needed by the
// DAG executor's future dataset upload step.
type TransformResult struct {
	RowsAffected   *uint64
	Output         json.RawMessage
	ResultRows     []json.RawMessage
	ResultRowsJSON json.RawMessage
	Stdout         string
	Stderr         string
}

// TransformExecutor is the injectable Python transform executor. Tests
// can use fakes; production uses SidecarTransformExecutor.
type TransformExecutor interface {
	ExecutePythonTransform(ctx context.Context, req TransformRequest) (*TransformResult, error)
}

// PythonTransformClient is the narrow python-sidecar client shape used
// by the executor. *pythonsidecar.Manager is adapted by SidecarTransform.
type PythonTransformClient interface {
	Execute(ctx context.Context, source string, configJSON, preparedInputsJSON []byte, inputDatasetIDs []string, outputDatasetID string, timeoutSeconds uint32) (*pythonsidecar.PipelineTransformResult, error)
}

// SidecarTransform adapts *pythonsidecar.Manager to PythonTransformClient.
type SidecarTransform struct{ Mgr *pythonsidecar.Manager }

func (s SidecarTransform) Execute(ctx context.Context, source string, configJSON, preparedInputsJSON []byte, inputDatasetIDs []string, outputDatasetID string, timeoutSeconds uint32) (*pythonsidecar.PipelineTransformResult, error) {
	if s.Mgr == nil {
		return nil, errors.New("python-sidecar manager is nil")
	}
	return s.Mgr.ExecutePipeline(ctx, source, configJSON, preparedInputsJSON, inputDatasetIDs, outputDatasetID, timeoutSeconds)
}

// SidecarTransformExecutor validates the Rust-compatible request and
// maps the sidecar response into TransformResult.
type SidecarTransformExecutor struct {
	Client PythonTransformClient
}

func NewSidecarTransformExecutor(client PythonTransformClient) *SidecarTransformExecutor {
	return &SidecarTransformExecutor{Client: client}
}

func (e *SidecarTransformExecutor) ExecutePythonTransform(ctx context.Context, req TransformRequest) (*TransformResult, error) {
	if e == nil || e.Client == nil {
		return nil, errors.New("python transform executor is not configured")
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		return nil, errors.New("Python transform has no 'source' or 'code' config")
	}
	configJSON := defaultJSON(req.ConfigJSON, []byte("{}"))
	preparedInputsJSON := defaultJSON(req.PreparedInputsJSON, []byte("[]"))
	if !json.Valid(configJSON) {
		return nil, errors.New("Python transform config_json is not valid JSON")
	}
	if !json.Valid(preparedInputsJSON) {
		return nil, errors.New("Python transform prepared_inputs_json is not valid JSON")
	}

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultTransformTimeoutSeconds
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	out, err := e.Client.Execute(callCtx, source, configJSON, preparedInputsJSON, req.InputDatasetIDs, req.OutputDatasetID, timeoutSeconds)
	if err != nil {
		return nil, err
	}
	return mapPipelineSidecarResult(out)
}

func mapPipelineSidecarResult(out *pythonsidecar.PipelineTransformResult) (*TransformResult, error) {
	if out == nil {
		return nil, errors.New("python transform returned nil result")
	}
	output := json.RawMessage(out.OutputJSON)
	if len(output) == 0 {
		return nil, errors.New("python transform returned empty output_json")
	}
	if !json.Valid(output) {
		return nil, errors.New("python transform returned malformed output_json")
	}
	stderr := out.Stderr
	if stderr == "" {
		stderr = stderrFromPipelineOutput(output)
	}

	var rowsAffected *uint64
	if out.RowsAffectedSet {
		if out.RowsAffected < 0 {
			return nil, fmt.Errorf("python transform returned negative rows_affected: %d", out.RowsAffected)
		}
		v := uint64(out.RowsAffected)
		rowsAffected = &v
	}

	resultRows, resultRowsJSON, err := normalizeResultRows(out.ResultRowsJSON)
	if err != nil {
		return nil, err
	}
	if rowsAffected == nil && len(resultRows) > 0 {
		v := uint64(len(resultRows))
		rowsAffected = &v
	}

	return &TransformResult{
		RowsAffected:   rowsAffected,
		Output:         output,
		ResultRows:     resultRows,
		ResultRowsJSON: resultRowsJSON,
		Stdout:         out.Stdout,
		Stderr:         stderr,
	}, nil
}

func stderrFromPipelineOutput(output json.RawMessage) string {
	var payload struct {
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return ""
	}
	return payload.Stderr
}

func normalizeResultRows(raw []byte) ([]json.RawMessage, json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	if !json.Valid(raw) {
		return nil, nil, errors.New("python transform returned malformed result_rows_json")
	}
	var one map[string]json.RawMessage
	if err := json.Unmarshal(raw, &one); err == nil {
		row, _ := json.Marshal(one)
		canonical, _ := json.Marshal([]json.RawMessage{row})
		return []json.RawMessage{row}, canonical, nil
	}
	var many []json.RawMessage
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, nil, errors.New("result_rows must serialize to an object or array of objects")
	}
	for _, row := range many {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(row, &obj); err != nil {
			return nil, nil, errors.New("result_rows must serialize to an object or array of objects")
		}
	}
	canonical, _ := json.Marshal(many)
	return many, canonical, nil
}

func defaultJSON(value []byte, fallback []byte) []byte {
	if len(value) == 0 {
		return fallback
	}
	return value
}
