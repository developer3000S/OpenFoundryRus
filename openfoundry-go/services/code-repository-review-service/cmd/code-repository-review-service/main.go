// Command code-repository-review-service hosts the consolidated
// code-repository / review / global-branching plane (S8 / ADR-0030).
//
// Runtime scope: global-branching HTTP CRUD + outbox-emit on
// `foundry.global.branch.promote.requested.v1`, a kafka-go consumer loop for
// `foundry.branch.events.v1`, and a basic code-security scanner integration
// that persists scan/finding rows.
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
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/codesecurity"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/subscriber"
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
	securityRepo := &repo.CodeSecurityRepo{DB: pool}
	h := &handlers.Handlers{
		Repo:  r,
		Pool:  pool,
		Actor: cfg.Actor,
		CodeSecurity: &codesecurity.Service{
			Scanner: codesecurity.FakeScanner{},
			Repo:    securityRepo,
		},
	}
	metrics := observability.NewMetrics()

	if len(cfg.KafkaBrokers) > 0 {
		branchConsumer := subscriber.NewConsumer(
			cfg.KafkaBrokers,
			cfg.BranchEventsConsumerGroup,
			&subscriber.PostgresSubscriber{Repo: r},
			log,
		)
		defer func() { _ = branchConsumer.Close() }()
		go func() {
			if err := branchConsumer.Run(ctx); err != nil {
				log.Error("branch event consumer exited", slog.String("error", err.Error()))
				cancel()
			}
		}()
	} else {
		log.Warn("KAFKA_BROKERS unset — branch event consumer disabled")
	}

	srv := server.New(cfg, h, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
