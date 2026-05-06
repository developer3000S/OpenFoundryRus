// Command agent-runtime-service hosts the agent runtime + tool
// registry plane (S8.1.b ADR-0030 absorbed tool-registry-service).
//
// Foundation port: full agents CRUD + runs + steps + human-approval
// + OpenAI-style chat-completion stub + copilot stub. The
// tool-registry HTTP routes (`/api/v1/agent-runtime/tools`) port
// alongside libs/ai-kernel-go/handlers/tools in a follow-up slice.
//
// AI-event Kafka producer wires alongside libs/event-bus-data-go in a
// follow-up slice; the Topic, TxnIDPrefix, AiEventKind enum and
// envelope shape are pinned now in internal/aievents.
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
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/server"
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

	if cfg.KafkaBootstrap == "" {
		log.Warn("KAFKA_BOOTSTRAP_SERVERS unset — ai.events.v1 producer wires in follow-up slice")
	}

	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	h := &handlers.Handlers{Repo: &repo.Repo{Pool: pool}}
	metrics := observability.NewMetrics()

	srv := server.New(cfg, jwt, h, metrics)
	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
