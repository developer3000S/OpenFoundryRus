// Package server wires the HTTP surface for ontology-actions-service.
//
// Mirrors `services/ontology-actions-service/src/lib.rs::build_router`
// 1:1 at the URL level: every action / funnel / function / rule path
// from the Rust router is mounted here, JWT-protected, and delegated to
// the state-backed ontology-kernel handlers.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	kernelactions "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/actions"
	kernelfunctions "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/functions"
	kernelfunnel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/funnel"
	kernelrules "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/rules"
	kernelstorage "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/storage"
	ontologymetrics "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/metrics"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
)

// New builds the HTTP server bound to cfg.Server.{Host,Port}.
func New(cfg *config.Config, state *ontologykernel.AppState, m *observability.Metrics) *http.Server {
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           BuildRouter(cfg, state, m),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// BuildRouter exposes the chi.Router for in-process tests
// (parity with `tower::ServiceExt::oneshot` callers in Rust).
func BuildRouter(cfg *config.Config, state *ontologykernel.AppState, m *observability.Metrics) http.Handler {
	if state == nil {
		panic("ontology-actions-service requires non-nil AppState; set DATABASE_URL or enable OF_DEV_STUB_MODE for explicit local/test in-memory state")
	}
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// Public probes.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		ontologymetrics.RegisterActionMetrics(m.Registry)
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// /api/v1/ontology/* requires a Bearer token. The Rust crate
	// applies `auth_middleware::layer::auth_layer` exactly here.
	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	r.Route("/api/v1/ontology", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		mountActions(api, state)
		mountFunnel(api, state)
		mountFunctions(api, state)
		mountRules(api, state)
	})

	return r
}

func mountActions(r chi.Router, state *ontologykernel.AppState) {
	kernelactions.Mount(r, state)
}

func mountFunnel(r chi.Router, state *ontologykernel.AppState) {
	r.Get("/storage/insights", kernelstorage.GetStorageInsights(state))
	kernelfunnel.Mount(r, state)
}

func mountFunctions(r chi.Router, state *ontologykernel.AppState) {
	kernelfunctions.Mount(r, state)
}

func mountRules(r chi.Router, state *ontologykernel.AppState) {
	kernelrules.Mount(r, state)
}
