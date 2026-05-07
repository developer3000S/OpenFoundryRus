package server

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
)

func TestRouteAuditNotebookRuntimeSurface(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{JWTSecret: "test"}
	cfg.Service.Name = "notebook-runtime-service"
	cfg.Service.Version = "test"
	router, ok := BuildRouter(cfg, nil, nil).(chi.Routes)
	if !ok {
		t.Fatal("router does not expose chi routes")
	}

	got := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	expected := []string{
		"GET /health", "GET /healthz",
		"GET /api/v1/notebooks", "POST /api/v1/notebooks",
		"GET /api/v1/notebooks/{notebook_id}", "PUT /api/v1/notebooks/{notebook_id}", "PATCH /api/v1/notebooks/{notebook_id}", "DELETE /api/v1/notebooks/{notebook_id}",
		"POST /api/v1/notebooks/{notebook_id}/cells", "PATCH /api/v1/notebooks/{notebook_id}/cells/{cell_id}", "DELETE /api/v1/notebooks/{notebook_id}/cells/{cell_id}",
		"GET /api/v1/notebooks/{notebook_id}/sessions", "POST /api/v1/notebooks/{notebook_id}/sessions", "POST /api/v1/notebooks/{notebook_id}/sessions/{session_id}/stop",
		"POST /api/v1/notebooks/{notebook_id}/cells/{cell_id}/execute", "POST /api/v1/notebooks/{notebook_id}/cells/execute-all",
		"GET /api/v1/notebooks/{notebook_id}/workspace", "PUT /api/v1/notebooks/{notebook_id}/workspace", "DELETE /api/v1/notebooks/{notebook_id}/workspace",
		"GET /api/v1/notepad/documents", "POST /api/v1/notepad/documents", "GET /api/v1/notepad/documents/{document_id}", "PATCH /api/v1/notepad/documents/{document_id}", "DELETE /api/v1/notepad/documents/{document_id}",
		"GET /api/v1/notepad/documents/{document_id}/presence", "POST /api/v1/notepad/documents/{document_id}/presence", "POST /api/v1/notepad/documents/{document_id}/export",
	}
	for _, route := range expected {
		if !got[route] {
			t.Fatalf("missing route: %s", route)
		}
	}
}
