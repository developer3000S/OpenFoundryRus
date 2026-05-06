// Package server wires the chi router for audit-compliance-service.
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
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/handlers"
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

		api.Get("/audit-events", h.ListAuditEvents)
		api.Get("/audit-policies", h.ListAuditPolicies)
		api.Get("/compliance-reports", h.ListComplianceReports)

		api.Get("/retention-policies", h.ListRetentionPolicies)
		api.Post("/retention-policies", h.CreateRetentionPolicy)
		api.Get("/retention-policies/{id}", h.GetRetentionPolicy)
		api.Patch("/retention-policies/{id}", h.UpdateRetentionPolicy)
		api.Get("/retention-jobs", h.ListRetentionJobs)

		api.Get("/sds-scan-jobs", h.ListSDSScanJobs)
		api.Get("/sds-scan-jobs/{job_id}/issues", h.ListSDSIssues)
		api.Get("/sds-remediation-rules", h.ListSDSRemediationRules)

		api.Get("/lineage-deletion-requests", h.ListLineageDeletionRequests)
		api.Post("/lineage-deletion-requests", h.CreateLineageDeletionRequest)

		api.Get("/saga-audit-events", h.ListSagaAuditEvents)
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
