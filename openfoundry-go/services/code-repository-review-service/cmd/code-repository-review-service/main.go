// Command code-repository-review-service hosts the consolidated
// code-repository / review / global-branching plane (S8 / ADR-0030).
//
// Foundation slice scope: global-branching HTTP CRUD + outbox-emit
// on `foundry.global.branch.promote.requested.v1` + Postgres-backed
// SubscriberPort for `foundry.branch.events.v1`. The kafka-go consumer
// loop wraps the SubscriberPort in a follow-up slice; tests drive
// the port directly. The code-security scan/finding tables migrate
// in but a scanner integration is not yet wired (see codesecurity.FindingsTopic).
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
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/server"
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

	r := &repo.GlobalBranchRepo{Pool: pool}
	h := &handlers.Handlers{Repo: r, Pool: pool, Actor: cfg.Actor}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, h, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
