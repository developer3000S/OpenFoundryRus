// Command ingestion-replication-service hosts the foundation slice of
// the Foundry ingestion + replication runtime (Kafka/Flink jobs +
// streaming + cdc_metadata).
//
// Runtime scope: ingest_jobs CRUD plus streaming/CDC control-plane
// provisioning through Kafka and Flink runtime adapters.
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
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/server"
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
	runtime := handlers.NewProductionStreamingRuntime(
		&handlers.HTTPKafkaAdmin{BaseURL: os.Getenv("KAFKA_RUNTIME_URL")},
		&handlers.HTTPFlinkDeployer{BaseURL: os.Getenv("FLINK_RUNTIME_URL")},
	)
	h := &handlers.Handlers{Repo: &repo.Repo{Pool: pool}, Runtime: runtime}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, h, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
