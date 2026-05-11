package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

type fakeTransformClient struct {
	result *pythonsidecar.PipelineTransformResult
	err    error
	block  bool
	seen   TransformRequest
}

func (f *fakeTransformClient) Execute(ctx context.Context, source string, configJSON, preparedInputsJSON []byte, inputDatasetIDs []string, outputDatasetID string, timeoutSeconds uint32) (*pythonsidecar.PipelineTransformResult, error) {
	f.seen = TransformRequest{
		Source:             source,
		ConfigJSON:         append([]byte(nil), configJSON...),
		PreparedInputsJSON: append([]byte(nil), preparedInputsJSON...),
		InputDatasetIDs:    append([]string(nil), inputDatasetIDs...),
		OutputDatasetID:    outputDatasetID,
		TimeoutSeconds:     timeoutSeconds,
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestExecutePythonTransformOK(t *testing.T) {
	client := &fakeTransformClient{result: &pythonsidecar.PipelineTransformResult{
		OutputJSON:     []byte(`{"stdout":"hello\n","stderr":"warn\n","result":"done","sample_rows":[{"a":1}]}`),
		ResultRowsJSON: []byte(`[{"a":1}]`),
		Stdout:         "hello\n",
	}}
	exec := NewSidecarTransformExecutor(client)

	got, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{
		Source:             "print('hello')",
		ConfigJSON:         []byte(`{"source":"print('hello')"}`),
		PreparedInputsJSON: []byte(`[]`),
		InputDatasetIDs:    []string{"dataset-a"},
		OutputDatasetID:    "dataset-out",
		TimeoutSeconds:     7,
	})
	if err != nil {
		t.Fatalf("ExecutePythonTransform: %v", err)
	}
	if got.Stdout != "hello\n" || got.Stderr != "warn\n" {
		t.Fatalf("log mapping drift: stdout=%q stderr=%q", got.Stdout, got.Stderr)
	}
	if got.RowsAffected == nil || *got.RowsAffected != 1 {
		t.Fatalf("rowsAffected fallback = %v, want 1", got.RowsAffected)
	}
	if len(got.ResultRows) != 1 || string(got.ResultRowsJSON) != `[{"a":1}]` {
		t.Fatalf("result rows drift: rows=%s json=%s", got.ResultRows, got.ResultRowsJSON)
	}
	if client.seen.TimeoutSeconds != 7 || client.seen.OutputDatasetID != "dataset-out" || len(client.seen.InputDatasetIDs) != 1 {
		t.Fatalf("request drift: %+v", client.seen)
	}
	var output map[string]any
	if err := json.Unmarshal(got.Output, &output); err != nil || output["result"] != "done" {
		t.Fatalf("output drift: output=%s err=%v", got.Output, err)
	}
}

func TestExecutePythonTransformPythonError(t *testing.T) {
	exec := NewSidecarTransformExecutor(&fakeTransformClient{err: errors.New("pipeline transform error: Traceback boom")})
	_, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{Source: "raise Exception('boom')"})
	if err == nil || !strings.Contains(err.Error(), "Traceback boom") {
		t.Fatalf("expected Python error, got %v", err)
	}
}

func TestExecutePythonTransformTimeout(t *testing.T) {
	exec := NewSidecarTransformExecutor(&fakeTransformClient{block: true})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := exec.ExecutePythonTransform(ctx, TransformRequest{Source: "while True: pass"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestExecutePythonTransformMalformedResult(t *testing.T) {
	exec := NewSidecarTransformExecutor(&fakeTransformClient{result: &pythonsidecar.PipelineTransformResult{
		OutputJSON: []byte(`not-json`),
	}})
	_, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{Source: "result = 'ok'"})
	if err == nil || !strings.Contains(err.Error(), "malformed output_json") {
		t.Fatalf("expected malformed output_json error, got %v", err)
	}
}

func TestExecutePythonTransformRowsAffected(t *testing.T) {
	exec := NewSidecarTransformExecutor(&fakeTransformClient{result: &pythonsidecar.PipelineTransformResult{
		RowsAffected:    42,
		RowsAffectedSet: true,
		OutputJSON:      []byte(`{"stdout":"","result":null,"sample_rows":null}`),
	}})
	got, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{Source: "rows_affected = 42"})
	if err != nil {
		t.Fatalf("ExecutePythonTransform: %v", err)
	}
	if got.RowsAffected == nil || *got.RowsAffected != 42 {
		t.Fatalf("rowsAffected = %v, want 42", got.RowsAffected)
	}
}
