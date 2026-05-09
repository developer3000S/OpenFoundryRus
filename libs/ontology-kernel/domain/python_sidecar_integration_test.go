//go:build integration

package domain

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	kernelstores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

type ontologySidecarRuntime struct{ mgr *pythonsidecar.Manager }

func (r ontologySidecarRuntime) ExecuteInline(ctx context.Context, source string, inputJSON []byte, timeoutSeconds uint32) (*ontologykernel.InlineRuntimeResult, error) {
	out, err := r.mgr.ExecuteInline(ctx, source, inputJSON, timeoutSeconds)
	if err != nil {
		return nil, err
	}
	return &ontologykernel.InlineRuntimeResult{ResultJSON: out.ResultJSON, Stdout: out.Stdout, Stderr: out.Stderr}, nil
}

func startOntologySidecar(t *testing.T) *pythonsidecar.Manager {
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

func TestExecuteInlinePythonFunctionWithRealSidecar(t *testing.T) {
	mgr := startOntologySidecar(t)
	state := &ontologykernel.AppState{
		Stores:        kernelstores.NewInMemory(),
		JWTConfig:     authmw.NewJWTConfig("test-secret"),
		PythonRuntime: ontologySidecarRuntime{mgr: mgr},
	}
	claims := &authmw.Claims{Sub: uuid.New(), Roles: []string{"admin"}}
	action := &models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "py_fn"}
	resolved := &ResolvedInlineFunction{
		Config: InlineFunctionConfig{Kind: InlineFunctionPython, Python: &InlinePythonFunctionConfig{
			Runtime: "python",
			Source:  `import sys` + "\n" + `print("ontology stdout")` + "\n" + `print("ontology stderr", file=sys.stderr)` + "\n" + `result = {"status": "ok", "answer": 42}`,
		}},
		Capabilities: models.FunctionCapabilities{AllowOntologyRead: true, TimeoutSeconds: 10, MaxSourceBytes: 4096},
	}

	result, err := ExecuteInlineFunction(context.Background(), state, claims, action, nil, map[string]json.RawMessage{"input": json.RawMessage(`"value"`)}, resolved, nil)
	if err != nil {
		t.Fatalf("ExecuteInlineFunction: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("result JSON: %v body=%s", err, result)
	}
	if !strings.Contains(string(result), "ok") || !strings.Contains(string(result), "ontology stdout") || !strings.Contains(string(result), "ontology stderr") {
		t.Fatalf("result/stdout/stderr drift: %s", result)
	}

	resolved.Config.Python.Source = `raise RuntimeError("ontology function boom")`
	_, err = ExecuteInlineFunction(context.Background(), state, claims, action, nil, nil, resolved, nil)
	if err == nil || !strings.Contains(err.Error(), "ontology function boom") {
		t.Fatalf("expected ontology function error, got %v", err)
	}
}
