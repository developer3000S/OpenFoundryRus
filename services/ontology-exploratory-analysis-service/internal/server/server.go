// Package server wires the HTTP surface for
// ontology-exploratory-analysis-service. The normal binary mounts substrate
// probes plus the geospatial API absorbed from geospatial-intelligence-service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/openfoundry/openfoundry-go/libs/capabilities"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers/geospatial"
)

func New(cfg *config.Config, m *observability.Metrics, geo *geospatial.AppState, probes ...capabilities.DependencyProbe) *http.Server {
	r := buildRouter(cfg, m, nil, geo, probes...)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// BuildRouter returns the substrate-only router (only /health,
// /readiness, /healthz, /metrics). Mirrors the Rust binary's
// substrate-only surface.
func BuildRouter(cfg *config.Config, m *observability.Metrics, probes ...capabilities.DependencyProbe) http.Handler {
	return buildRouter(cfg, m, nil, nil, probes...)
}

// BuildRouterWithHandlers returns a router with the saved-view /
// saved-map / writeback domain routes mounted alongside the substrate
// probes. Callers thread an OEA Handlers value when they want the
// Rust-equivalent CRUD surface live; the binary main passes nil to
// preserve the substrate-only contract documented in src/main.rs.
func BuildRouterWithHandlers(cfg *config.Config, m *observability.Metrics, h *handlers.Handlers, probes ...capabilities.DependencyProbe) http.Handler {
	return buildRouter(cfg, m, h, nil, probes...)
}

// BuildRouterWithGeospatial returns a router with the productive geospatial
// surface mounted alongside substrate probes. Tests use this constructor to
// smoke the normal binary route set without opening a real listener.
func BuildRouterWithGeospatial(cfg *config.Config, m *observability.Metrics, geo *geospatial.AppState, probes ...capabilities.DependencyProbe) http.Handler {
	return buildRouter(cfg, m, nil, geo, probes...)
}

func buildRouter(cfg *config.Config, m *observability.Metrics, h *handlers.Handlers, geo *geospatial.AppState, probes ...capabilities.DependencyProbe) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(15 * time.Second))

	healthHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	}
	// /health + /readiness preserve the Rust route names verbatim;
	// /healthz is the openfoundry-go convention probe.
	r.Get("/health", healthHandler)
	r.Get("/readiness", healthHandler)
	r.Get("/healthz", healthHandler)
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// Capability registry — see docs/agent-automation/AGENT-CAPABILITIES-ROADMAP.md (M1.1).
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	if h != nil {
		h.MountViews(r)
		h.MountMaps(r)
		if h.Actions != nil {
			h.MountWriteback(r)
		}
	}
	if geo != nil {
		r.Mount("/api/v1/geospatial", geo.Routes())
	}

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix:  "ontology-eda",
		AuthPaths: []string{"/api/v1"},
		Tags:      []string{"ontology"},
	}); err != nil {
		panic("ontology-exploratory-analysis-service: capability ingest failed: " + err.Error())
	}

	return r
}

func Run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		log.Info("shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
