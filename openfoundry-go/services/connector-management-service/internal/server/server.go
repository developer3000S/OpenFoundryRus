// Package server wires the chi router for connector-management-service.
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
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
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

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/connections", h.ListConnections)
		api.Post("/connections", h.CreateConnection)
		api.Get("/connections/{id}", h.GetConnection)
		api.Patch("/connections/{id}", h.UpdateConnection)
		api.Delete("/connections/{id}", h.DeleteConnection)

		api.Get("/data-connection/sources/{id}/syncs", h.ListSyncJobs)
		api.Post("/data-connection/syncs", h.CreateSyncJob)
		api.Get("/data-connection/syncs/{sync_id}", h.GetSyncJob)
		api.Patch("/data-connection/syncs/{sync_id}", h.UpdateSyncJob)
		api.Post("/data-connection/syncs/{sync_id}/run", h.RunSyncJob)

		api.Get("/data-connection/sources/{id}/media-set-syncs", h.ListMediaSetSyncs)
		api.Post("/data-connection/sources/{id}/media-set-syncs", h.CreateMediaSetSync)
		api.Get("/data-connection/media-set-syncs/{sync_id}", h.GetMediaSetSync)
		api.Patch("/data-connection/media-set-syncs/{sync_id}", h.UpdateMediaSetSync)
		api.Post("/data-connection/media-set-syncs/{sync_id}/run", h.RunMediaSetSync)

		api.Post("/virtual-table/sources/{source_rid}/enable", h.EnableVirtualTableSource)
		api.Post("/virtual-table/sources/{source_rid}/virtual-tables", h.CreateVirtualTable)
		api.Get("/virtual-tables", h.ListVirtualTables)
		api.Get("/virtual-tables/{rid}", h.GetVirtualTable)
	})

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
