from pathlib import Path
import tempfile
import unittest

import http_route_audit as audit


class HttpRouteAuditTests(unittest.TestCase):
    def test_extract_rust_routes_with_nest_and_chained_methods(self):
        with tempfile.TemporaryDirectory() as td:
            repo = Path(td)
            src = repo / "services" / "svc" / "src"
            src.mkdir(parents=True)
            (src / "main.rs").write_text('''
use axum::{Router, routing::{get, post}};
fn build() {
    let api = Router::new()
        .route("/items", get(handlers::list).post(handlers::create))
        .route("/items/{id}", axum::routing::patch(handlers::update));
    let app = Router::new().nest("/api/v1", api).route("/healthz", get(health));
}
''')
            got = {(r.method, r.path, r.handler) for r in audit.extract_rust_routes(repo, "svc")}
            self.assertIn(("GET", "/api/v1/items", "handlers::list"), got)
            self.assertIn(("POST", "/api/v1/items", "handlers::create"), got)
            self.assertIn(("PATCH", "/api/v1/items/{id}", "handlers::update"), got)
            self.assertIn(("GET", "/healthz", "health"), got)

    def test_extract_go_routes_with_nested_chi_route_and_status(self):
        with tempfile.TemporaryDirectory() as td:
            repo = Path(td)
            root = repo / "openfoundry-go" / "services" / "svc" / "internal" / "server"
            root.mkdir(parents=True)
            (root / "server.go").write_text('''
package server
import "net/http"
func New() {
    r.Route("/api/v1", func(api chi.Router) {
        api.Get("/items", h.ListItems)
        api.Post("/items", h.CreateItem)
    })
}
func (h *Handlers) ListItems(w http.ResponseWriter, r *http.Request) { writeEmptyList(w) }
func (h *Handlers) CreateItem(w http.ResponseWriter, r *http.Request) { notImplemented(w, r) }
''')
            got = {(r.method, r.path, r.handler, r.status) for r in audit.extract_go_routes(repo, "svc")}
            self.assertIn(("GET", "/api/v1/items", "h.ListItems", "empty envelope"), got)
            self.assertIn(("POST", "/api/v1/items", "h.CreateItem", "501"), got)


if __name__ == "__main__":
    unittest.main()
