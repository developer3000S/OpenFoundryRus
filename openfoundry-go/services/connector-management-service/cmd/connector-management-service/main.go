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

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/server"
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
	h := &handlers.Handlers{
		Repo:            &repo.Repo{Pool: pool},
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
