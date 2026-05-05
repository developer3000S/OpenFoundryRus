// Command edge-gateway-service is the OpenFoundry HTTP edge gateway.
//
// What it does
//
// Reverse-proxies every /api/v1, /api/v2/admin, /iceberg/v1 and
// /v1/iceberg-clients route to the right bounded-context service via
// a static path-prefix table (see internal/proxy/router_table.go).
// Validates JWTs, enforces zero-trust scope, derives a TenantContext
// from claims, attaches `x-openfoundry-*` headers downstream, and
// rate-limits with a Redis or in-memory token bucket.
//
// 1:1 functional parity with services/edge-gateway-service in Rust —
// see docs/architecture/migration-rust-to-go.md.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/server"
)

// version is injected at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cfgPath := flag.String("config", "services/edge-gateway-service/config.yaml", "path to config file")
	flag.Parse()

	envOverride := os.Getenv("CONFIG_FILE")
	cfg, err := config.Load(*cfgPath, envOverride)
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.Service.Version == "" || cfg.Service.Version == "0.1.0" {
		cfg.Service.Version = version
	}

	log := observability.InitLogging(cfg.Service.Name, cfg.Service.Version)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdownTracing, err := observability.InitTracing(ctx, cfg.Service.Name, cfg.Service.Version)
	if err != nil {
		log.Error("tracing init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	metrics := observability.NewMetrics()

	srv, err := server.New(ctx, cfg, metrics, log)
	if err != nil {
		log.Error("server build failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := srv.Run(ctx); err != nil {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
