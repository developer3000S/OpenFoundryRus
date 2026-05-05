// Package server wires the chi router for identity-federation-service slice 1.
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
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/handlers"
)

// New builds the http.Server with slice-1 routes.
//
// Slice 1 mounts:
//   GET    /healthz
//   GET    /metrics
//   GET    /api/v1/auth/bootstrap-status
//   POST   /api/v1/auth/register
//   POST   /api/v1/auth/login
//   POST   /api/v1/auth/token/refresh
//
// Subsequent slices add: /auth/sessions/*, /auth/sso/*, /users/*,
// /roles/*, /groups/*, /permissions/*, /policies/*, /control-panel/*,
// /scim/v2/*, /jwks/rotate, /audit/metrics.
func New(cfg *config.Config, jwt *authmw.JWTConfig, auth *handlers.Auth, mfa *handlers.MFA, wa *handlers.WebAuthn, sso *handlers.SSO, rbac *handlers.RBAC, m *observability.Metrics) *http.Server {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Method(http.MethodGet, "/metrics", m.Handler())

	// /api/v1/auth/* — public (no bearer required, the endpoints
	// handle credentials themselves).
	r.Route("/api/v1/auth", func(api chi.Router) {
		api.Get("/bootstrap-status", auth.BootstrapStatus)
		api.Post("/register", auth.Register)
		api.Post("/login", auth.Login)
		api.Post("/token/refresh", auth.Refresh)
		api.Post("/mfa/totp/complete-login", mfa.CompleteLogin)
		api.Post("/mfa/webauthn/login/challenge", wa.LoginChallenge)
		api.Post("/mfa/webauthn/login/finish", wa.LoginFinish)
		api.Get("/sso/providers", sso.ListProviders)
		api.Get("/sso/{provider}/start", sso.Start)
		api.Get("/sso/{provider}/callback", sso.Callback)
	})

	// /api/v1/auth/mfa/* — bearer-protected MFA management.
	r.Route("/api/v1/auth/mfa", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		api.Get("/status", mfa.Status)
		api.Post("/totp/enroll", mfa.Enroll)
		api.Post("/totp/verify", mfa.Verify)
		api.Post("/totp/disable", mfa.Disable)
		api.Post("/webauthn/register/challenge", wa.RegisterChallenge)
		api.Post("/webauthn/register/finish", wa.RegisterFinish)
	})

	// /api/v1/{users,roles,groups,permissions,api-keys} — bearer
	// protected admin surface (slice 6 RBAC CRUD).
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/users", rbac.ListUsers)
		api.Get("/users/{id}", rbac.GetUser)
		api.Patch("/users/{id}", rbac.UpdateUser)
		api.Delete("/users/{id}", rbac.DeleteUser)
		api.Get("/users/{id}/roles", rbac.ListUserRoles)
		api.Put("/users/{id}/roles/{role_id}", rbac.AssignUserRole)
		api.Delete("/users/{id}/roles/{role_id}", rbac.RevokeUserRole)

		api.Get("/roles", rbac.ListRoles)
		api.Post("/roles", rbac.CreateRole)
		api.Get("/roles/{id}", rbac.GetRole)
		api.Patch("/roles/{id}", rbac.UpdateRole)
		api.Delete("/roles/{id}", rbac.DeleteRole)
		api.Put("/roles/{id}/permissions/{permission_id}", rbac.AssignRolePermission)
		api.Delete("/roles/{id}/permissions/{permission_id}", rbac.RevokeRolePermission)

		api.Get("/permissions", rbac.ListPermissions)
		api.Post("/permissions", rbac.CreatePermission)
		api.Delete("/permissions/{id}", rbac.DeletePermission)

		api.Get("/groups", rbac.ListGroups)
		api.Post("/groups", rbac.CreateGroup)
		api.Get("/groups/{id}", rbac.GetGroup)
		api.Patch("/groups/{id}", rbac.UpdateGroup)
		api.Delete("/groups/{id}", rbac.DeleteGroup)
		api.Put("/groups/{id}/members/{user_id}", rbac.AddGroupMember)
		api.Delete("/groups/{id}/members/{user_id}", rbac.RemoveGroupMember)

		api.Get("/api-keys", rbac.ListAPIKeys)
		api.Post("/api-keys", rbac.CreateAPIKey)
		api.Delete("/api-keys/{id}", rbac.RevokeAPIKey)
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
