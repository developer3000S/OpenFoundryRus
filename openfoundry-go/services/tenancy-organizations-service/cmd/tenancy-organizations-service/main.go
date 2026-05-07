// Command tenancy-organizations-service hosts the foundation slice of
// the Rust `tenancy-organizations-service` crate: organizations + enrollments
// CRUD over Postgres.
//
// Spaces, projects, sharing, trash, favorites, recents, resource_resolve and
// resource_ops land in follow-up slices — see
// INVENTORY-tenancy-organizations-service.md.
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
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/workspace"
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

	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	h := &handlers.Handlers{Repo: &repo.Repo{Pool: pool}}
	ph := &handlers.ProjectsHandlers{Pool: pool}
	ws := &workspace.Handlers{Repo: &workspace.Repo{Pool: pool}}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, h, ph, ws, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
