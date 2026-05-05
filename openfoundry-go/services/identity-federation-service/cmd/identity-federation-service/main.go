// Command identity-federation-service slice 1 — register / login / token.
//
// 1:1 with services/identity-federation-service/ (Rust) per the slice
// plan in openfoundry-go/INVENTORY-identity-federation-service.md.
// Subsequent slices layer in sessions, MFA, WebAuthn, SSO, RBAC,
// control panel, hardening + SCIM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/service"
	oidcpkg "github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/oidc"
	webauthnpkg "github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/webauthn"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.Service.Version == "dev" {
		cfg.Service.Version = version
	}

	log := observability.InitLogging(cfg.Service.Name, cfg.Service.Version)
	shutdownTracing, err := observability.InitTracing(ctx, cfg.Service.Name, cfg.Service.Version)
	if err != nil {
		log.Error("tracing init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("pgx pool failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()
	if err := repo.Migrate(ctx, pool); err != nil {
		log.Error("migrations failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jwt := authmw.NewJWTConfig(cfg.JWTSecret).
		WithAccessTTL(cfg.AccessTTL).
		WithRefreshTTL(cfg.RefreshTTL)

	r := &repo.Repo{Pool: pool}
	issuer := &service.Issuer{
		JWT:        jwt,
		Repo:       r,
		AccessTTL:  cfg.AccessTTL,
		RefreshTTL: cfg.RefreshTTL,
	}
	waStore := webauthnpkg.NewPostgresStore(pool)
	waService, err := webauthnpkg.NewService(webauthnpkg.FromEnv(), waStore)
	if err != nil {
		log.Error("webauthn service init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// OIDC providers (slice 5a). BASE_URL is the externally-visible
	// base URL — IdPs MUST register `<BASE_URL>/api/v1/auth/sso/<name>/callback`.
	oidcConfigs, err := oidcpkg.LoadProvidersFromEnv(os.Getenv("BASE_URL"))
	if err != nil {
		log.Error("oidc config failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	oidcSvc, err := oidcpkg.NewService(ctx, oidcConfigs)
	if err != nil {
		log.Warn("oidc init failed — SSO endpoints will return 'unknown provider'",
			slog.String("error", err.Error()))
		oidcSvc, _ = oidcpkg.NewService(ctx, nil)
	}

	auth := &handlers.Auth{Repo: r, Issuer: issuer, WebAuthn: waService}
	mfa := &handlers.MFA{JWT: jwt, Repo: r, Issuer: issuer}
	wa := &handlers.WebAuthn{JWT: jwt, Repo: r, Service: waService, Issuer: issuer}
	sso := &handlers.SSO{Repo: r, OIDC: oidcSvc, Issuer: issuer}
	rbac := &handlers.RBAC{Repo: r}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, auth, mfa, wa, sso, rbac, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
