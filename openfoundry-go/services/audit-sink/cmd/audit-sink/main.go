// Command audit-sink is the audit.events.v1 → Iceberg consumer.
//
// 1:1 functional parity with services/audit-sink/ in the Rust workspace.
// See services/audit-sink/README.md for cutover protocol.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/runtime"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/server"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/writer"
)

// version is injected at build time via -ldflags "-X main.version=...".
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

	w, err := buildWriter(cfg)
	if err != nil {
		log.Error("writer build failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = w.Close() }()

	sub, err := databus.NewKafkaSubscriber(cfg.DataBus, config.ConsumerGroup, []string{config.SourceTopic})
	if err != nil {
		log.Error("kafka subscriber init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = sub.Close() }()

	metrics := runtime.NewMetrics()
	httpSrv := server.New(cfg.MetricsAddr, cfg.Service.Name, cfg.Service.Version, metrics)

	log.Info("audit-sink starting Kafka -> Writer runtime",
		slog.String("topic", config.SourceTopic),
		slog.String("consumer_group", config.ConsumerGroup),
		slog.String("target", config.IcebergCatalog+"."+config.IcebergNamespace+"."+config.IcebergTable),
		slog.String("metrics_addr", cfg.MetricsAddr))

	var wg sync.WaitGroup
	wg.Add(2)

	httpErr := make(chan error, 1)
	runErr := make(chan error, 1)

	go func() {
		defer wg.Done()
		if err := httpSrv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			httpErr <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := runtime.Run(ctx, cfg, sub, w, metrics, log); err != nil && !errors.Is(err, context.Canceled) {
			runErr <- err
		}
	}()

	// Whichever finishes first cancels the other.
	select {
	case err := <-httpErr:
		log.Error("metrics http server failed", slog.String("error", err.Error()))
		cancel()
	case err := <-runErr:
		log.Error("kafka runtime failed", slog.String("error", err.Error()))
		cancel()
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}
	wg.Wait()
}

// buildWriter picks the Writer implementation per config.
//
// Default: when AUDIT_SINK_JSONL_PATH is set, the JSONL writer is used.
// Otherwise the Iceberg writer (currently a stub) is selected — the
// runtime fails loudly on the first batch via writer.ErrNotImplemented
// so an operator who forgets to set the JSONL path notices immediately.
func buildWriter(cfg *config.Config) (writer.Writer, error) {
	if cfg.JSONLWriterPath != "" {
		return writer.NewJSONLWriter(cfg.JSONLWriterPath)
	}
	return writer.NewIcebergWriter(
		cfg.CatalogURL, cfg.Warehouse,
		config.IcebergNamespace, config.IcebergTable,
	), nil
}
