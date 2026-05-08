// Package server wires the chi router for ontology-definition-service.
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
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ontology-definition-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-definition-service/internal/handlers"
)

func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics) *http.Server {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Method(http.MethodGet, "/metrics", m.Handler())

	mountObjectTypes := func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		// Canonical resource = `object-types`. The edge-gateway router
		// (and the apps/web SPA) reach this service at `/types` —
		// register the same handlers under both names so neither
		// surface sees a 404.
		for _, base := range []string{"/object-types", "/types"} {
			api.Get(base, h.ListObjectTypes)
			api.Post(base, h.CreateObjectType)
			api.Get(base+"/{id}", h.GetObjectType)
			api.Patch(base+"/{id}", h.UpdateObjectType)
			api.Delete(base+"/{id}", h.DeleteObjectType)
		}
	}

	// Mount on both the legacy `/api/v1/ontology-definition` prefix
	// (kept for backwards compatibility) and the gateway-canonical
	// `/api/v1/ontology` prefix.
	r.Route("/api/v1/ontology-definition", mountObjectTypes)
	r.Route("/api/v1/ontology", mountObjectTypes)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
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
