// Command connector-management-service hosts the foundation slice of
// the Foundry Data Connection app (connections CRUD plus sync/media-set verticals).
//
// Foundation slice scope: connections CRUD on Postgres. sync_jobs,
// enterprise_connectivity and remaining connector runtime adapters land in follow-up
// slices.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/kafka"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/workers"
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

	store := &repo.Repo{Pool: pool}
	if cfg.AutoRegistrationIntervalSecs > 0 {
		worker := &workers.AutoRegistrationWorker{Store: store, Clock: workers.RealClock{}, Recorder: workers.DefaultAutoRegistrationRecorder}
		interval := time.Duration(cfg.AutoRegistrationIntervalSecs) * time.Second
		go func() {
			if err := worker.RunLoop(ctx, interval); err != nil && !errors.Is(err, context.Canceled) {
				log.Warn("auto-registration scheduler exited", slog.String("error", err.Error()))
			}
		}()
		log.Info("auto-registration scheduler enabled", slog.Duration("interval", interval))
	}
	if cfg.SyncSchedulerIntervalSecs > 0 {
		worker := &workers.SyncSchedulerWorker{Store: store, Clock: workers.RealClock{}}
		interval := time.Duration(cfg.SyncSchedulerIntervalSecs) * time.Second
		go func() {
			if err := worker.RunLoop(ctx, interval); err != nil && !errors.Is(err, context.Canceled) {
				log.Warn("sync scheduler exited", slog.String("error", err.Error()))
			}
		}()
		log.Info("sync scheduler enabled", slog.Duration("interval", interval))
	}

	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	adapterRegistry := adapters.NewRegistry()
	adapterRegistry.MustRegister("kafka", kafka.Factory())

	h := &handlers.Handlers{
		Repo:            store,
		AdapterRegistry: adapterRegistry,
		MediaSetRuntime: &handlers.HTTPMediaSetRuntime{MediaSetsBaseURL: cfg.MediaSetsServiceURL},
		Config: handlers.RuntimeConfig{
			DatasetServiceURL:            cfg.DatasetServiceURL,
			PipelineServiceURL:           cfg.PipelineServiceURL,
			OntologyServiceURL:           cfg.OntologyServiceURL,
			IngestionReplicationGRPCURL:  cfg.IngestionReplicationGRPCURL,
			NetworkBoundaryServiceURL:    cfg.NetworkBoundaryServiceURL,
			SyncPollIntervalSecs:         cfg.SyncPollIntervalSecs,
			AllowPrivateNetworkEgress:    cfg.AllowPrivateNetworkEgress,
			AllowedEgressHosts:           cfg.AllowedEgressHosts,
			AgentStaleAfterSecs:          cfg.AgentStaleAfterSecs,
			CredentialEncryptionKey:      cfg.CredentialEncryptionKey,
			CredentialKey:                cfg.CredentialKey,
			SecretManagerURL:             cfg.SecretManagerURL,
			OutboxEnabled:                cfg.OutboxEnabled,
			AutoRegistrationIntervalSecs: cfg.AutoRegistrationIntervalSecs,
			VendedCredentialsTTLSeconds:  cfg.VendedCredentialsTTLSeconds,
		},
	}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, h, metrics, pool.Ping)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
