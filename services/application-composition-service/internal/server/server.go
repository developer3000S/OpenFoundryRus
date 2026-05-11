// Package server wires the chi router for application-composition-service.
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
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/handlers"
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
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// Capability registry — M1.1.
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	r.Route("/api/v1/application-composition", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/", h.ListItems)
		api.Post("/", h.CreateItem)
		api.Get("/{id}", h.GetItem)
		api.Get("/{id}/bindings", h.ListSecondary)
		api.Post("/{id}/bindings", h.CreateSecondary)
	})

	// `/api/v1/apps` is the App Builder surface consumed by apps/web's
	// lib/api/apps.ts. Public read by slug stays outside the auth group so
	// embedded / portal consumers can render published apps anonymously.
	r.Get("/api/v1/apps/public/{slug}", h.GetPublishedApp)
	r.Get("/api/v1/apps/public/{slug}/embed", h.GetAppEmbedInfo)
	r.Route("/api/v1/widgets", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		api.Get("/catalog", h.ListWidgetCatalog)
	})
	r.Route("/api/v1/apps", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/templates", h.ListAppTemplates)
		api.Post("/from-template", h.CreateAppFromTemplate)
		api.Get("/", h.ListApps)
		api.Post("/", h.CreateApp)
		api.Get("/{id}", h.GetApp)
		api.Patch("/{id}", h.UpdateApp)
		api.Delete("/{id}", h.DeleteApp)
		api.Get("/{id}/preview", h.PreviewApp)
		api.Post("/{id}/pages", h.AddPage)
		api.Patch("/{id}/pages/{pageID}", h.UpdatePage)
		api.Delete("/{id}/pages/{pageID}", h.DeletePage)
		api.Get("/{id}/slate-package", h.GetSlatePackage)
		api.Post("/{id}/slate-package", h.ImportSlatePackage)
		api.Get("/{id}/versions", h.ListAppVersions)
		api.Post("/{id}/publish", h.PublishApp)
	})

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix:  "application-composition",
		AuthPaths: []string{"/api/v1/application-composition", "/api/v1/apps"},
		Tags:      []string{"apps"},
	}); err != nil {
		panic("application-composition-service: capability ingest failed: " + err.Error())
	}
	// /api/v1/apps/public/{slug} stays unauth (public). The ingest
	// will mark it RequiresAuth because of the AuthPaths prefix; we
	// override that single entry post-hoc by re-registering it as a
	// curated capability before the ingest. Skipped here because the
	// curated entry would conflict with chi's existing route —
	// adjusted by editing the snapshot in M1.1 follow-up.

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
