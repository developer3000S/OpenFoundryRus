// Package server wires the chi router for code-repository-review-service.
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

	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/handlers"
)

func New(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics) *http.Server {
	r := buildRouter(cfg, h, m)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// BuildRouter is exposed for tests so httptest can mount it.
func BuildRouter(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics) http.Handler {
	return buildRouter(cfg, h, m)
}

func buildRouter(cfg *config.Config, h *handlers.Handlers, m *observability.Metrics) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	// Plain text /healthz to match Rust ("ok"). The JSON shape is
	// served at /healthz/json so the openfoundry-go convention probe
	// also has a target — both are wire-locked by tests.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/healthz/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	r.Post("/v1/code-security/scans", h.CreateCodeSecurityScan)

	r.Route("/v1/global-branches", func(api chi.Router) {
		api.Get("/", h.ListGlobalBranches)
		api.Post("/", h.CreateGlobalBranch)
		api.Get("/{id}", h.GetGlobalBranch)
		api.Post("/{id}/links", h.AddLink)
		api.Get("/{id}/resources", h.ListResources)
		api.Post("/{id}/promote", h.Promote)
	})

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
