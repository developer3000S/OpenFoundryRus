// Command media-transform-runtime-service hosts the worker that runs
// Foundry-style media access patterns.
//
// Current slice scope:
//   - Catalog endpoint (`GET /catalog`, `GET /catalog/{kind}`) ports
//     the full Rust catalog shape and marks implemented Go-native image
//     transforms as Native.
//   - REST runtime (`POST /transform`) routes by catalog status, executes
//     Native image handlers, and preserves the Rust 501 envelope for
//     NotImplemented + External entries.
//   - `/healthz` returns plain text "ok" matching Rust.
//
// Follow-up slices wire external binaries (ffmpeg / pdfium / tesseract /
// qpdf / dcmtk), AI-backed media transforms, geospatial tiles, and cost
// metering.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/media-transform-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/media-transform-runtime-service/internal/server"
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

	metrics := observability.NewMetrics()
	srv := server.New(cfg, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
