// Command ontology-exploratory-analysis-service is the substrate-only
// shell binary for the consolidated exploratory-analysis plane.
//
// Per docs/architecture/service-consolidation-map.md this crate is the
// *target* of four pending merges:
//   - ontology-timeseries-analytics-service
//   - time-series-data-service
//   - geospatial-intelligence-service (S8 / ADR-0030 — absorbed)
//   - scenario-simulation-service
//
// Until those merges land and a kernel `handlers::exploratory` ships,
// this binary mounts only `/health` + `/readiness` (matching Rust).
// The migrations for the consolidated tables (`scenario_simulations`,
// `scenario_runs`, `time_series`, `time_series_points`,
// `time_series_storage_partitions`) DO ship — when the merges land,
// the schema is already in place.
//
// Models for ExploratoryView / ExploratoryMap / WritebackProposal
// land as typed scaffolding (matching the Rust `#[allow(dead_code)]`
// modules).
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/server"
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

	metrics := observability.NewMetrics()
	srv := server.New(cfg, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
