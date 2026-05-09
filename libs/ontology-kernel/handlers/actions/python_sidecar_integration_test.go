//go:build integration

package actions

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

type actionSidecarRuntime struct{ mgr *pythonsidecar.Manager }

func (r actionSidecarRuntime) ExecuteInline(ctx context.Context, source string, inputJSON []byte, timeoutSeconds uint32) (*ontologykernel.InlineRuntimeResult, error) {
	out, err := r.mgr.ExecuteInline(ctx, source, inputJSON, timeoutSeconds)
	if err != nil {
		return nil, err
	}
	return &ontologykernel.InlineRuntimeResult{ResultJSON: out.ResultJSON, Stdout: out.Stdout, Stderr: out.Stderr}, nil
}

func startActionSidecar(t *testing.T) *pythonsidecar.Manager {
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

func TestExecuteInlinePythonActionWithRealSidecar(t *testing.T) {
	mgr := startActionSidecar(t)
	state := newTestState(t)
	state.PythonRuntime = actionSidecarRuntime{mgr: mgr}
	plan := inlinePythonPlan(`import sys` + "\n" + `print("action stdout")` + "\n" + `print("action stderr", file=sys.stderr)` + "\n" + `result = {"action_status": "ok"}`)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}

	executed, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New(), Roles: []string{"admin"}}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	body := string(executed.result)
	if !strings.Contains(body, "action_status") || !strings.Contains(body, "action stdout") || !strings.Contains(body, "action stderr") {
		t.Fatalf("action result/stdout/stderr drift: %s", body)
	}

	plan = inlinePythonPlan(`raise RuntimeError("ontology action boom")`)
	_, err = executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New(), Roles: []string{"admin"}}, action, plan)
	if err == nil || !strings.Contains(err.Error(), "ontology action boom") {
		t.Fatalf("expected ontology action error, got %v", err)
	}
}
