// Command pipeline-build-service serves the build / execution side of
// Pipeline Builder. The Go HTTP surface is intentionally audited
// route-by-route rather than described as 1:1; see
// docs/migration/route-parity-audit.md for generated route state and
// remaining config-gated production adapters.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/handler"
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/postgres"
	runtimepkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/runtime"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
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

	if cfg.FoundryIcebergCatalogURL == "" {
		log.Warn("FoundryIcebergTxn disabled, multi-table atomicity not enforced " +
			"(set FOUNDRY_ICEBERG_CATALOG_URL to enable; ADR-0041)")
	}

	var pythonRuntime runtimepkg.TransformExecutor
	var pyMgr *pythonsidecar.Manager
	if cfg.PythonSidecarBinary != "" {
		pyMgr, err = pythonsidecar.New(pythonsidecar.Config{
			BinaryPath:      cfg.PythonSidecarBinary,
			HardCallTimeout: time.Duration(cfg.PythonSidecarTimeoutSeconds+5) * time.Second,
		}, log)
		if err != nil {
			log.Error("python sidecar config failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		startCtx, cancelStart := context.WithTimeout(ctx, 15*time.Second)
		if err := pyMgr.Start(startCtx); err != nil {
			cancelStart()
			log.Error("python sidecar start failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		cancelStart()
		defer func() { _ = pyMgr.Stop(context.Background()) }()
		pythonRuntime = runtimepkg.NewSidecarTransformExecutor(runtimepkg.SidecarTransform{Mgr: pyMgr})
		log.Info("python sidecar runtime wired")
	} else {
		log.Warn("PYTHON_SIDECAR_BINARY unset; Python pipeline transforms require an injected test fake or will fail explicitly")
	}

	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pool, err = pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Error("postgres pool init failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer pool.Close()
		if err := postgres.RunMigrations(ctx, pool); err != nil {
			log.Error("postgres migrations failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		repo := postgres.NewRepositoryFromPool(pool)
		handler.SetBuildLifecyclePorts(handler.BuildLifecyclePorts{JobSpecs: repo, Versioning: repo, Locks: repo, Builds: repo})
		handler.SetExecutionPorts(handler.ExecutionPorts{Plans: repo, Runs: repo, Python: pythonRuntime, Transactions: handler.ConfigGatedTransactionManager{Metadata: repo, CatalogConfigured: cfg.FoundryIcebergCatalogURL != ""}, Committer: handler.ConfigGatedOutputCommitter{Metadata: repo, CatalogConfigured: cfg.FoundryIcebergCatalogURL != ""}, Audit: repo, Parallelism: cfg.DistributedPipelineWorkers})
		handler.SetBuildQueryRepository(repo)
		handler.SetPipelineAuthoringRepository(repo)
		handler.SetSparkSubmissionRepository(repo)
		handler.SetJobLogService(&livellogs.Service{Store: repo, Subscriber: livellogs.NewMemoryService()})
		log.Info("postgres repositories wired", slog.String("database_url", "set"))
	} else {
		log.Warn("DATABASE_URL unset — production repositories disabled; supported handlers return explicit 503 instead of fake success")
	}

	if kubeClient, err := spark.NewKubernetesClientFromEnv(); err == nil {
		handler.SetSparkClient(kubeClient)
		log.Info("kubernetes SparkApplication client wired")
	} else {
		log.Warn("kubernetes client unavailable — SparkApplication endpoints return explicit 503", slog.String("error", err.Error()))
	}

	metrics := observability.NewMetrics()
	srv := server.New(cfg, metrics)
	if err := run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		log.Info("shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
