// Command federation-product-exchange-service hosts the consolidated
// federation / product-exchange plane (S8 / ADR-0030 / B21):
// marketplace + marketplace-catalog + product-distribution legacy
// services merged into a single binary.
//
// Architecture + 1 vertical slice scope (this iteration):
//   - Architecture: cmd/main, config, observability, server (substrate
//     /healthz + /metrics on port 50126), pgxpool + 8 SQL migrations
//     copied verbatim spanning marketplace foundation, nexus,
//     marketplace activation P4, devops fleets + branches, spaces +
//     admin lifecycle, phase-3 promotion gates + cells, dataset
//     products, marketplace schedule manifests.
//   - Vertical slice — typed scaffolding: ListResponse[T] envelope +
//     SyncStatus + NexusOverview shared models (the most-referenced
//     types across all three sub-domains).
//
// Follow-up slices (queued):
//   - marketplace sub-domain: listings, installs, dependency planning,
//     activation, fleets, maintenance windows.
//   - marketplace_catalog sub-domain: catalog browsing, slug routing,
//     listing search.
//   - product_distribution sub-domain: import, peer registry, share
//     manifest, sync status projection, replication.
//   - Per-area handlers + Postgres repos for: peers, contracts, spaces,
//     shares, access grants, exchanges, sync_status updates.
//   - Domain pieces: federation registry, schema_compat, encryption,
//     replication, governance, access_proxy.
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
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/marketplace"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/server"
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

	var pool *pgxpool.Pool
	databaseURL := cfg.DatabaseURL
	if databaseURL == "" {
		databaseURL = cfg.MarketplaceDatabaseURL
	}
	if databaseURL != "" {
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			log.Error("pgx pool failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer pool.Close()
		if err := repo.Migrate(ctx, pool); err != nil {
			log.Error("migrations failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	} else {
		log.Warn("DATABASE_URL and MARKETPLACE_DATABASE_URL unset — migrations skipped and marketplace handlers disabled")
	}

	metrics := observability.NewMetrics()
	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	var marketplaceHandlers *marketplace.Handlers
	if pool != nil {
		marketplaceHandlers = marketplace.NewHandlers(marketplace.NewPGXRepository(pool))
	}
	srv := server.New(cfg, jwt, marketplaceHandlers, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
