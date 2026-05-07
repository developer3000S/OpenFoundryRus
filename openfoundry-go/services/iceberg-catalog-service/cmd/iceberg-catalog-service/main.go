// Command iceberg-catalog-service hosts the foundation slice of the
// Foundry Iceberg REST Catalog (Apache Iceberg REST Catalog OpenAPI
// spec + Foundry-internal admin surface).
//
// Foundation slice scope: namespaces CRUD on Postgres. Tables, snapshots,
// branches, metadata files, OpenAPI REST surface, marking enforcement
// (10k LOC of Rust handlers) all land in follow-up slices.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/authz"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/handlers/auth"
	icmetrics "github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/metrics"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/server"
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
		WithIssuer(cfg.JWTIssuer).
		WithAudience(cfg.JWTAudience)
	repoBackend := &repo.Repo{Pool: pool}
	h := &handlers.Handlers{Repo: repoBackend, WarehouseURI: cfg.WarehouseURI}
	mh := &handlers.MarkingsHandlers{
		Store:         repoBackend,
		Authz:         authz.NewPolicyEngine(cfg.DefaultTenant),
		DefaultTenant: cfg.DefaultTenant,
	}
	bearer := &auth.Config{
		Secret:              auth.LoadSecret(),
		JWTAudience:         cfg.JWTAudience,
		JWTIssuer:           cfg.JWTIssuer,
		DefaultTokenTTLSecs: cfg.DefaultTokenTTLSecs,
		DefaultTenant:       cfg.DefaultTenant,
	}
	oauthValidator := &auth.HTTPClientValidator{BaseURL: cfg.OAuthIntegrationURL, HTTP: http.DefaultClient}
	metrics := observability.NewMetrics()
	icebergMetrics := icmetrics.New(metrics)

	deps := server.Deps{
		Handlers:       h,
		Markings:       mh,
		Bearer:         bearer,
		BearerStore:    repoBackend,
		IssueAPIStore:  repoBackend,
		OAuthValidator: oauthValidator,
		Metrics:        icebergMetrics,
	}
	srv := server.New(cfg, jwt, deps, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
