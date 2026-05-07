// Package server wires the chi router for tenancy-organizations-service.
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
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/workspace"
)

// New builds the http.Server for the foundation slice + workspace surface.
func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, ph *handlers.ProjectsHandlers, ws *workspace.Handlers, m *observability.Metrics) *http.Server {
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

		api.Get("/organizations", h.ListOrganizations)
		api.Post("/organizations", h.CreateOrganization)
		api.Get("/organizations/{id}", h.GetOrganization)
		api.Patch("/organizations/{id}", h.UpdateOrganization)
		api.Delete("/organizations/{id}", h.DeleteOrganization)

		api.Get("/organizations/{id}/enrollments", h.ListEnrollments)
		api.Post("/enrollments", h.CreateEnrollment)
		api.Delete("/enrollments/{id}", h.DeleteEnrollment)

		api.Get("/projects", ph.ListProjects)
		api.Post("/projects", ph.CreateProject)
		api.Get("/projects/{id}", ph.GetProject)
		api.Patch("/projects/{id}", ph.UpdateProject)
		api.Delete("/projects/{id}", ph.DeleteProject)
		api.Get("/projects/{id}/memberships", ph.ListProjectMemberships)
		api.Put("/projects/{id}/memberships", ph.UpsertProjectMembership)
		api.Delete("/projects/{id}/memberships/{user_id}", ph.DeleteProjectMembership)
		api.Get("/projects/{id}/folders", ph.ListProjectFolders)
		api.Post("/projects/{id}/folders", ph.CreateProjectFolder)
		api.Get("/projects/{id}/resources", ph.ListProjectResources)
		api.Post("/projects/{id}/resources", ph.BindProjectResource)
		api.Delete("/projects/{id}/resources/{kind}/{resource_id}", ph.UnbindProjectResource)

		api.Route("/workspace", func(wr chi.Router) {
			wr.Get("/favorites", ws.ListFavorites)
			wr.Post("/favorites", ws.CreateFavorite)
			wr.Delete("/favorites/{kind}/{id}", ws.DeleteFavorite)
			wr.Get("/recents", ws.ListRecents)
			wr.Post("/recents", ws.RecordAccess)
			wr.Post("/resources/{kind}/{id}/share", ws.CreateShare)
			wr.Get("/resources/{kind}/{id}/shares", ws.ListResourceShares)
			wr.Delete("/shares/{id}", ws.RevokeShare)
			wr.Get("/shared-with-me", ws.ListSharedWithMe)
			wr.Get("/shared-by-me", ws.ListSharedByMe)
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// Run blocks until ctx is done or the listener returns.
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
