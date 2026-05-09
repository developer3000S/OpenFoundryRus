//go:build integration

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

func startPipelineSidecar(t *testing.T) *pythonsidecar.Manager {
	t.Helper()
	bin := os.Getenv("PYTHON_SIDECAR_BINARY")
	if bin == "" {
		t.Skip("PYTHON_SIDECAR_BINARY not set — install openfoundry-pyruntime in a venv and re-run")
	}
	mgr, err := pythonsidecar.New(pythonsidecar.Config{
		BinaryPath:      bin,
		StartupTimeout:  5 * time.Second,
		HardCallTimeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("New sidecar manager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start sidecar: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })
	return mgr
}

func TestExecutePythonTransformWithRealSidecar(t *testing.T) {
	mgr := startPipelineSidecar(t)
	exec := NewSidecarTransformExecutor(SidecarTransform{Mgr: mgr})

	got, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{
		Source:             `print("pipeline stdout")` + "\n" + `rows_affected = 2` + "\n" + `result_rows = [{"id": "a", "score": 1}, {"id": "b", "score": 2}]` + "\n" + `result = {"status": "ok"}`,
		ConfigJSON:         []byte(`{"kind":"integration"}`),
		PreparedInputsJSON: []byte(`[{"dataset_id":"input-a","rows":[{"score":1}]}]`),
		InputDatasetIDs:    []string{"input-a"},
		OutputDatasetID:    "output-a",
		TimeoutSeconds:     10,
	})
	if err != nil {
		t.Fatalf("ExecutePythonTransform: %v", err)
	}
	if got.Stdout != "pipeline stdout\n" || got.Stderr != "" {
		t.Fatalf("stdout/stderr drift: stdout=%q stderr=%q", got.Stdout, got.Stderr)
	}
	if got.RowsAffected == nil || *got.RowsAffected != 2 {
		t.Fatalf("rows_affected = %v, want 2", got.RowsAffected)
	}
	if len(got.ResultRows) != 2 || !json.Valid(got.ResultRowsJSON) || !strings.Contains(string(got.ResultRowsJSON), `"score":2`) {
		t.Fatalf("result_rows drift: rows=%s json=%s", got.ResultRows, got.ResultRowsJSON)
	}
	var output map[string]any
	if err := json.Unmarshal(got.Output, &output); err != nil {
		t.Fatalf("output JSON: %v body=%s", err, got.Output)
	}
	if !strings.Contains(string(got.Output), "ok") {
		t.Fatalf("output result drift: %s", got.Output)
	}

	_, err = exec.ExecutePythonTransform(context.Background(), TransformRequest{Source: `raise RuntimeError("pipeline integration boom")`, TimeoutSeconds: 10})
	if err == nil || !strings.Contains(err.Error(), "pipeline integration boom") {
		t.Fatalf("expected pipeline integration error, got %v", err)
	}
}
