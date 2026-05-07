package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeComparablePath(t *testing.T) {
	if got := normalizePath("/api//v1/items/{id:uuid}/"); got != "/api/v1/items/{id}" {
		t.Fatalf("normalizePath mismatch: %q", got)
	}
	if got := comparablePath("/api/v1/items/{item_id}/runs/{run_id}"); got != "/api/v1/items/{}/runs/{}" {
		t.Fatalf("comparablePath mismatch: %q", got)
	}
}

func TestExtractRustRoutesWithNestAndMergeAlias(t *testing.T) {
	repo := t.TempDir()
	src := filepath.Join(repo, "services", "svc", "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `use axum::{Router, routing::{get, post}};
fn build() -> Router {
    let actions = Router::new()
        .route("/actions", get(list_actions).post(create_action))
        .route("/actions/{id}", axum::routing::patch(update_action));
    let protected = actions;
    Router::new().nest("/api/v1/ontology", protected)
}
`
	if err := os.WriteFile(filepath.Join(src, "lib.rs"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	routes := extractRustRoutes(repo, "svc")
	want := map[string]string{
		"GET /api/v1/ontology/actions":      "list_actions",
		"POST /api/v1/ontology/actions":     "create_action",
		"PATCH /api/v1/ontology/actions/{}": "update_action",
	}
	if len(routes) != len(want) {
		t.Fatalf("expected %d routes, got %d: %#v", len(want), len(routes), routes)
	}
	for _, r := range routes {
		key := r.Method + " " + comparablePath(r.Path)
		if want[key] != r.Handler {
			t.Fatalf("unexpected route %s handler %q", key, r.Handler)
		}
	}
}

func TestExtractGoRoutesAndClassifyPlaceholders(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "openfoundry-go", "services", "svc", "internal", "server")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `package server
import (
  "net/http"
  "github.com/go-chi/chi/v5"
)
func Build() http.Handler {
  r := chi.NewRouter()
  r.Route("/api/v1", func(api chi.Router) {
    api.Get("/things", listThings)
    api.Method(http.MethodPost, "/things", createThing)
  })
  return r
}
func listThings(w http.ResponseWriter, r *http.Request) { writeEmptyList(w) }
func createThing(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotImplemented) }
`
	if err := os.WriteFile(filepath.Join(root, "server.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	routes := extractGoRoutes(repo, "svc")
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d: %#v", len(routes), routes)
	}
	statuses := map[string]string{}
	for _, r := range routes {
		statuses[r.Method+" "+r.Path] = r.Status
	}
	if statuses["GET /api/v1/things"] != "empty-envelope" {
		t.Fatalf("GET status mismatch: %#v", statuses)
	}
	if statuses["POST /api/v1/things"] != "501" {
		t.Fatalf("POST status mismatch: %#v", statuses)
	}
}
