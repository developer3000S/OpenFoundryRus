// Command ontology-indexer hosts the worker that consumes
// `ontology.objects.changed.v1` / `ontology.links.changed.v1` and
// projects ontology objects + links into the configured search
// backend (Vespa / OpenSearch).
//
// Foundation slice scope: skeleton + ops HTTP surface (/healthz,
// /metrics) + stub runtime that boots cleanly without a Kafka
// bootstrap or search endpoint configured. The kafka-go consumer
// and libs/search-abstraction-go SearchBackend port land in
// follow-up slices.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ontology-indexer/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-indexer/internal/runtime"
	"github.com/openfoundry/openfoundry-go/services/ontology-indexer/internal/server"
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

	runtimeErr := make(chan error, 1)
	go func() {
		runtimeErr <- runtime.Run(ctx, cfg, log)
	}()

	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := <-runtimeErr; err != nil && !errors.Is(err, context.Canceled) {
		log.Error("runtime exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
