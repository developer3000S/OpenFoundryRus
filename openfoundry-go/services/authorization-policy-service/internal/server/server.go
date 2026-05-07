// Package server wires the chi router for authorization-policy-service.
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
	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/handlers"
)

// New builds the http.Server for the consolidated authorization policy surface.
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

		api.Get("/cedar-policies", h.ListCedarPolicies)
		api.Post("/cedar-policies", h.CreateCedarPolicy)
		api.Get("/cedar-policies/{id}", h.GetCedarPolicy)
		api.Patch("/cedar-policies/{id}", h.UpdateCedarPolicy)
		api.Delete("/cedar-policies/{id}", h.DeleteCedarPolicy)

		api.Get("/abac-policies", h.ListABACPolicies)
		api.Post("/abac-policies", h.CreateABACPolicy)
		api.Get("/abac-policies/{id}", h.GetABACPolicy)
		api.Patch("/abac-policies/{id}", h.UpdateABACPolicy)
		api.Delete("/abac-policies/{id}", h.DeleteABACPolicy)

		api.Post("/policy-evaluations", h.EvaluatePolicy)
		api.Get("/permissions", h.ListPermissions)
		api.Post("/permissions", h.CreatePermission)

		api.Get("/roles", h.ListRoles)
		api.Post("/roles", h.CreateRole)
		api.Get("/roles/{id}", h.GetRole)
		api.Put("/roles/{id}", h.UpdateRole)
		api.Patch("/roles/{id}", h.UpdateRole)
		api.Delete("/roles/{id}", h.DeleteRole)

		api.Get("/groups", h.ListGroups)
		api.Post("/groups", h.CreateGroup)
		api.Get("/groups/{id}", h.GetGroup)
		api.Put("/groups/{id}", h.UpdateGroup)
		api.Patch("/groups/{id}", h.UpdateGroup)
		api.Delete("/groups/{id}", h.DeleteGroup)

		api.Post("/users/{id}/roles", h.AssignRole)
		api.Delete("/users/{id}/roles/{role_id}", h.RemoveRole)
		api.Post("/users/{id}/groups", h.AddUserToGroup)
		api.Delete("/users/{id}/groups/{group_id}", h.RemoveUserFromGroup)

		api.Get("/governance-template-applications", h.ListGovernanceTemplateApplications)
		api.Post("/governance-template-applications", h.ApplyGovernanceTemplate)
		api.Delete("/governance-template-applications/{id}", h.DeleteGovernanceTemplateApplication)

		api.Get("/project-constraints", h.ListProjectConstraints)
		api.Post("/project-constraints", h.CreateProjectConstraint)
		api.Patch("/project-constraints/{id}", h.UpdateProjectConstraint)
		api.Delete("/project-constraints/{id}", h.DeleteProjectConstraint)

		api.Get("/structural-security-rules", h.ListStructuralSecurityRules)
		api.Post("/structural-security-rules", h.CreateStructuralSecurityRule)
		api.Patch("/structural-security-rules/{id}", h.UpdateStructuralSecurityRule)
		api.Delete("/structural-security-rules/{id}", h.DeleteStructuralSecurityRule)

		api.Get("/checkpoint-policies", h.ListCheckpointPolicies)
		api.Get("/sensitive-interaction-configs", h.ListSensitiveInteractionConfigs)
		api.Get("/purpose-templates", h.ListPurposeTemplates)
		api.Get("/purpose-records", h.ListPurposeRecordsByInteraction)
		api.Post("/purpose-records", h.CreatePurposeRecord)

		api.Get("/cipher-permissions", h.ListCipherPermissions)
		api.Get("/cipher-channels", h.ListCipherChannels)
		api.Post("/cipher-channels", h.CreateCipherChannel)
		api.Patch("/cipher-channels/{id}", h.UpdateCipherChannel)
		api.Delete("/cipher-channels/{id}", h.DeleteCipherChannel)
		api.Get("/cipher-licenses", h.ListCipherLicenses)
		api.Post("/cipher-licenses", h.CreateCipherLicense)
		api.Patch("/cipher-licenses/{id}", h.UpdateCipherLicense)
		api.Delete("/cipher-licenses/{id}", h.DeleteCipherLicense)

		api.Get("/network-boundary-policies", h.ListNetworkBoundaryPolicies)
		api.Post("/network-boundary-policies", h.CreateNetworkBoundaryPolicy)
		api.Patch("/network-boundary-policies/{id}", h.UpdateNetworkBoundaryPolicy)
		api.Delete("/network-boundary-policies/{id}", h.DeleteNetworkBoundaryPolicy)

		api.Get("/network-private-links", h.ListNetworkPrivateLinks)
		api.Post("/network-private-links", h.CreateNetworkPrivateLink)
		api.Patch("/network-private-links/{id}", h.UpdateNetworkPrivateLink)
		api.Delete("/network-private-links/{id}", h.DeleteNetworkPrivateLink)

		api.Get("/network-proxy-definitions", h.ListNetworkProxyDefinitions)
		api.Post("/network-proxy-definitions", h.CreateNetworkProxyDefinition)
		api.Patch("/network-proxy-definitions/{id}", h.UpdateNetworkProxyDefinition)
		api.Delete("/network-proxy-definitions/{id}", h.DeleteNetworkProxyDefinition)
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
