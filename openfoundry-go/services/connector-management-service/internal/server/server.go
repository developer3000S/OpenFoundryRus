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

type ReadyCheck func(context.Context) error

func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, readyChecks ...ReadyCheck) *http.Server {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		for _, check := range readyChecks {
			if check == nil {
				continue
			}
			if err := check(r.Context()); err != nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "not_ready", "error": err.Error()})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
	r.Method(http.MethodGet, "/metrics", m.Handler())

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt, authmw.Options{AllowAnonymous: true}))

		// Data Connection catalog/read surfaces stay open during bring-up, matching Rust's optional auth layer.
		api.Get("/data-connection/catalog", h.GetConnectorCatalog)
		api.Get("/data-connection/catalog/contracts", h.GetConnectorContracts)
		api.Get("/data-connection/streaming-sources", h.ListStreamingSources)

		// Source CRUD uses the same implementation as the legacy /connections aliases.
		api.Get("/data-connection/sources", h.ListConnections)
		api.Post("/data-connection/sources", h.CreateConnection)
		api.Get("/data-connection/sources/{id}", h.GetConnection)
		api.Delete("/data-connection/sources/{id}", h.DeleteConnection)
		api.Post("/data-connection/sources/{id}/test-connection", h.TestConnection)
		api.Get("/data-connection/sources/{id}/capabilities", h.GetConnectionCapabilities)
		api.Get("/data-connection/sources/{id}/credentials", h.ListCredentials)
		api.Post("/data-connection/sources/{id}/credentials", h.SetCredential)
		api.Get("/data-connection/sources/{id}/egress-policies", h.ListSourcePolicies)
		api.Post("/data-connection/sources/{id}/egress-policies", h.AttachPolicy)
		api.Delete("/data-connection/sources/{source_id}/egress-policies/{policy_id}", h.DetachPolicy)

		api.Get("/data-connection/sources/{id}/syncs", h.ListSyncJobs)
		api.Post("/data-connection/syncs", h.CreateSyncJob)
		api.Get("/data-connection/syncs/{id}", h.GetSyncJob)
		api.Patch("/data-connection/syncs/{id}", h.UpdateSyncJob)
		api.Post("/data-connection/syncs/{id}/run", h.RunSyncJob)
		api.Get("/data-connection/syncs/{id}/runs", h.ListRuns)

		api.Get("/data-connection/sources/{id}/media-set-syncs", h.ListMediaSetSyncs)
		api.Post("/data-connection/sources/{id}/media-set-syncs", h.CreateMediaSetSync)
		api.Get("/data-connection/media-set-syncs/{id}", h.GetMediaSetSync)
		api.Patch("/data-connection/media-set-syncs/{id}", h.UpdateMediaSetSync)
		api.Post("/data-connection/media-set-syncs/{id}/run", h.RunMediaSetSync)

		api.Get("/data-connection/sources/{id}/registrations", h.ListRegistrations)
		api.Post("/data-connection/sources/{id}/registrations/discover", h.DiscoverRegistrations)
		api.Post("/data-connection/sources/{id}/registrations/bulk", h.BulkRegister)
		api.Post("/data-connection/sources/{id}/registrations/bulk/preview", h.BulkRegisterPreview)
		api.Post("/data-connection/sources/{id}/registrations/auto", h.AutoRegister)
		api.Put("/data-connection/sources/{id}/registrations/auto", h.UpdateAutoRegistration)
		api.Get("/data-connection/sources/{id}/registrations/auto/status", h.AutoRegisterStatus)
		api.Delete("/data-connection/sources/{source_id}/registrations/{registration_id}", h.DeleteRegistration)
		api.Post("/data-connection/sources/{source_id}/registrations/{registration_id}/query", h.QueryRegistration)
		api.Post("/data-connection/sources/{source_id}/registrations/{registration_id}/query/arrow", h.QueryRegistrationArrow)

		api.Get("/connections", h.ListConnections)
		api.Post("/connections", h.CreateConnection)
		api.Get("/connections/{id}", h.GetConnection)
		api.Patch("/connections/{id}", h.UpdateConnection)
		api.Delete("/connections/{id}", h.DeleteConnection)
		api.Post("/connections/{id}/test", h.TestConnection)

		api.Post("/webhooks/{id}/invoke", h.InvokeWebhook)

		if cfg.OpenFoundryDevAuth {
			api.Post("/auth/login", h.DevAuthLogin)
			api.Post("/auth/refresh", h.DevAuthRefresh)
			api.Get("/auth/bootstrap-status", h.DevAuthBootstrapStatus)
			api.Get("/users/me", h.DevAuthMe)
		}

		api.Post("/virtual-table/sources/{source_rid}/enable", h.EnableVirtualTableSource)
		api.Post("/virtual-table/sources/{source_rid}/virtual-tables", h.CreateVirtualTable)
		api.Get("/virtual-tables", h.ListVirtualTables)
		api.Get("/virtual-tables/{rid}", h.GetVirtualTable)
	})

	r.Route("/iceberg/v1", func(iceberg chi.Router) {
		iceberg.Use(authmw.Middleware(jwt, authmw.Options{AllowAnonymous: true}))
		iceberg.Get("/config", h.IcebergGetConfig)
		iceberg.Get("/namespaces", h.IcebergListNamespaces)
		iceberg.Get("/namespaces/{namespace}", h.IcebergGetNamespace)
		iceberg.Get("/namespaces/{namespace}/tables", h.IcebergListTables)
		iceberg.Get("/namespaces/{namespace}/tables/{table}", h.IcebergLoadTable)
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
