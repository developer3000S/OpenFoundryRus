// Command notebook-runtime-service serves the notebook + notepad
// HTTP surface (notebooks, cells, sessions, kernel execute, workspace
// files, notepad documents + presence + export).
//
// Current port: notebook + cell + session CRUD use pgx when
// DATABASE_URL is configured and fall back to empty envelopes for
// smoke/test mode. Notepad document/presence CRUD remains the
// productive stub surface; Python execution is gated on
// PYTHON_SIDECAR_BINARY. The two domain pieces ported 1:1 with full
// coverage:
//
//   - Workspace file CRUD (filesystem-backed, [`internal/domain/environment`]).
//   - Notepad export HTML rendering ([`internal/domain/notepad`]).
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
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/kernel"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/server"
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

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Warn("could not create DATA_DIR; workspace endpoints may fail",
			slog.String("data_dir", cfg.DataDir),
			slog.String("error", err.Error()))
	}

	// Best-effort DB pool: when DATABASE_URL is unset or unreachable,
	// CRUD endpoints fall back to the empty-envelope shape so smoke
	// clusters / unit tests keep working. Matches the sql-bi-gateway
	// pattern.
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			log.Warn("DATABASE_URL parse failed; CRUD endpoints will return empty envelopes",
				slog.String("error", err.Error()))
		} else {
			poolCtx, cancelPool := context.WithTimeout(ctx, 10*time.Second)
			pool, err = pgxpool.NewWithConfig(poolCtx, pcfg)
			cancelPool()
			if err != nil {
				log.Warn("DB pool init failed; CRUD endpoints will return empty envelopes",
					slog.String("error", err.Error()))
				pool = nil
			} else {
				defer pool.Close()
				log.Info("DB pool ready")
			}
		}
	} else {
		log.Warn("DATABASE_URL unset; CRUD endpoints will return empty envelopes")
	}

	var pyKernel *kernel.SidecarKernel
	var pyMgr *pythonsidecar.Manager
	if cfg.PythonSidecarBinary != "" {
		pyMgr, err = pythonsidecar.New(pythonsidecar.Config{
			BinaryPath: cfg.PythonSidecarBinary,
			Env: []string{
				"OPENFOUNDRY_NOTEBOOK_DATA_DIR=" + cfg.DataDir,
			},
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
		pyKernel = &kernel.SidecarKernel{Mgr: pyMgr}
	} else {
		log.Warn("PYTHON_SIDECAR_BINARY unset; Python ExecuteCell will return an explicit sidecar-not-configured error")
	}

	metrics := observability.NewMetrics()
	srv := server.NewWithKernel(cfg, pool, metrics, pyKernel)
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
