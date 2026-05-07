// Command dataset-versioning-service hosts the foundation slice of
// the Rust crate `dataset-versioning-service` (25k LOC; this slice
// covers datasets, versions, branches, and the Files API backing-fs vertical).
//
// Follow-up slices: dataset_quality, lint, views, health, retention_worker,
// foundry-model surface, and Iceberg writer integration.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/server"
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
	baseURL := os.Getenv("DATASET_FILES_BASE_URL")
	if baseURL == "" {
		baseURL = "http://" + srvAddr(cfg.Server.Host, cfg.Server.Port)
	}
	backingFS := storageabstraction.NewLocalBackingFS(baseURL, os.Getenv("DATASET_FILES_BASE_DIR"), []byte(cfg.JWTSecret))
	h := &handlers.Handlers{Repo: &repo.Repo{Pool: pool}, BackingFS: backingFS, PresignTTL: 15 * time.Minute}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, h, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func srvAddr(host string, port uint16) string {
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return host + ":" + strconv.FormatUint(uint64(port), 10)
}
