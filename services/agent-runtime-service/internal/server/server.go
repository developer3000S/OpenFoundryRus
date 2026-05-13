// Package server wires the chi router for agent-runtime-service.
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

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/capabilities"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/handlers"
)

func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) *http.Server {
	r := buildRouter(cfg, jwt, h, m, probes...)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func BuildRouter(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) http.Handler {
	return buildRouter(cfg, jwt, h, m, probes...)
}

func buildRouter(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// Capability registry — see docs/agent-automation/AGENT-CAPABILITIES-ROADMAP.md (M1.1).
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	r.Route("/api/v1/agent-runtime", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/logic/files", h.ListLogicFiles)
		api.Post("/logic/files", h.CreateLogicFile)
		api.Get("/logic/files/{id}", h.GetLogicFile)
		api.Patch("/logic/files/{id}", h.UpdateLogicFileMetadata)
		api.Post("/logic/files/{id}/move", h.MoveLogicFile)
		api.Post("/logic/files/{id}/duplicate", h.DuplicateLogicFile)
		api.Delete("/logic/files/{id}", h.ArchiveLogicFile)
		api.Post("/logic/files/{id}/restore", h.RestoreLogicFile)

		api.Get("/agents", h.ListAgents)
		api.Post("/agents", h.CreateAgent)
		api.Get("/agents/{id}", h.GetAgent)
		api.Patch("/agents/{id}", h.UpdateAgent)

		api.Get("/agents/{id}/runs", h.ListRuns)
		api.Post("/agents/{id}/runs", h.StartRun)
		api.Post("/agents/{id}/runs/{run_id}/steps", h.RecordStep)
		api.Post("/agents/{id}/runs/{run_id}/human-approval", h.SubmitHumanApproval)

		api.Post("/chat/completions", h.CreateChatCompletion)
		api.Post("/copilot/ask", h.AskCopilot)
	})

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix:  "agent-runtime",
		AuthPaths: []string{"/api/v1/agent-runtime"},
		Tags:      []string{"agent"},
	}); err != nil {
		panic("agent-runtime-service: capability ingest failed: " + err.Error())
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
