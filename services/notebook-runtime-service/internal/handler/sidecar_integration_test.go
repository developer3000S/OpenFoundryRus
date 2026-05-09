//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/kernel"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

func startNotebookSidecar(t *testing.T) *pythonsidecar.Manager {
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

func TestExecuteCellWithRealPythonSidecar(t *testing.T) {
	mgr := startNotebookSidecar(t)
	nb := uuid.New()
	state := &State{
		Cfg:          &config.Config{DataDir: t.TempDir()},
		MemoryRepo:   NewMemoryNotebookRepo(),
		PythonKernel: kernel.SidecarKernel{Mgr: mgr},
	}
	r := chi.NewRouter()
	r.Post("/api/v1/notebooks/{notebook_id}/cells/{cell_id}/execute", state.ExecuteCell)

	cell := models.Cell{
		ID:         uuid.New(),
		NotebookID: nb,
		CellType:   "code",
		Kernel:     "python",
		Source:     "value = 6 * 7\nprint(value)\nvalue",
		Position:   1,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	state.MemoryRepo.putCell(cell)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var got models.CellOutput
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if got.OutputType == "error" {
		t.Fatalf("unexpected error output: %s", got.Content)
	}
	if got.ExecutionCount != 1 {
		t.Fatalf("execution_count = %d, want 1", got.ExecutionCount)
	}
	if !json.Valid(got.Content) || !strings.Contains(string(got.Content), "42") {
		t.Fatalf("content should be valid JSON containing notebook result/stdout 42, got %s", got.Content)
	}
	persisted, ok := state.MemoryRepo.loadCell(cell.ID)
	if !ok || persisted.ExecutionCount == nil || *persisted.ExecutionCount != 1 || !bytes.Contains(persisted.LastOutput, []byte("42")) {
		t.Fatalf("persisted output drift: %+v", persisted)
	}

	errorCell := models.Cell{
		ID:         uuid.New(),
		NotebookID: nb,
		CellType:   "code",
		Kernel:     "python",
		Source:     `raise RuntimeError("notebook integration boom")`,
		Position:   2,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	state.MemoryRepo.putCell(errorCell)
	w = httptest.NewRecorder()
	req = withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+errorCell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("error status: %d body=%s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("error response JSON: %v", err)
	}
	if got.OutputType != "error" || !bytes.Contains(got.Content, []byte("notebook integration boom")) {
		t.Fatalf("error output drift: %+v content=%s", got, got.Content)
	}
}
