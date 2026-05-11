// Package server wires the chi router for object-database-service.
//
// Note: this service intentionally does NOT mount JWT auth on its
// /api/v1 routes — the upstream Rust binary is the same way; the
// HTTP surface is internal-only and gated by edge-gateway. The
// Cassandra-backed implementation lands in a follow-up slice along
// with libs/cassandra-kernel-go ports; foundation runs the same
// InMemory fallback Rust used when CASSANDRA_CONTACT_POINTS is unset.
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
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/handlers"
)

func New(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) *http.Server {
	r := buildRouter(cfg, h, m, probes...)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// BuildRouter is exposed for tests via Server() so httptest can mount it.
func BuildRouter(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) http.Handler {
	return buildRouter(cfg, h, m, probes...)
}

func buildRouter(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	// Top-level probes mirror Rust: /health is plain text "ok",
	// /ready + /readiness emit the structured StatusResponse.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/ready", h.Readiness)
	r.Get("/readiness", h.Readiness)
	r.Get("/status", h.Status)

	// /healthz reuses the openfoundry-go convention so probes don't
	// have to special-case this service.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// Capability registry — M1.1. Surface internal-only routes too
	// (this service has no per-route auth; the gateway gates access).
	// M1.2: caller-supplied probes (typically Cassandra) feed `/_meta/health`.
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	r.Route("/api/v1/object-database", func(api chi.Router) {
		api.Get("/status", h.Status)
		api.Get("/objects/{tenant}/{object_id}", h.GetObject)
		api.Put("/objects/{tenant}/{object_id}", h.PutObject)
		api.Delete("/objects/{tenant}/{object_id}", h.DeleteObject)
		api.Get("/objects/{tenant}/by-type/{type_id}", h.ListByType)
		api.Get("/objects/{tenant}/by-owner/{owner_id}", h.ListByOwner)
		api.Get("/objects/{tenant}/by-marking/{marking_id}", h.ListByMarking)
		api.Post("/links/{tenant}/{link_type}", h.PutLink)
		api.Get("/links/{tenant}/{link_type}/outgoing/{from}", h.ListOutgoingLinks)
		api.Get("/links/{tenant}/{link_type}/incoming/{to}", h.ListIncomingLinks)
	})

	// edge-gateway forwards `/api/v1/ontology/types/{id}/objects[/...]` to
	// this service unchanged. Mount adapter handlers that translate to/from
	// the SPA's ObjectInstance wire shape.
	r.Route("/api/v1/ontology/types/{type_id}/objects", func(api chi.Router) {
		api.Get("/", h.ListObjectsByOntologyType)
		api.Post("/", h.CreateObjectByOntologyType)
		api.Post("/query", h.QueryObjectsByOntologyType)
		api.Get("/{object_id}", h.GetObjectByOntologyType)
		api.Patch("/{object_id}", h.UpdateObjectByOntologyType)
		api.Delete("/{object_id}", h.DeleteObjectByOntologyType)
	})

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix: "object-database",
		// No per-route auth on this service — leave RequiresAuth=false.
		AuthPaths: nil,
		Tags:      []string{"objects"},
	}); err != nil {
		panic("object-database-service: capability ingest failed: " + err.Error())
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
