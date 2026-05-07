// Command workflow-automation-service hosts the consolidated
// workflow / automation / approval plane (S8 / ADR-0030):
//
//   - Workflow definitions/runs + Foundry-pattern condition
//     consumer (legacy workflow-automation-service).
//   - Saga substrate + saga.step.requested.v1 consumer (legacy
//     automation-operations-service).
//   - Human-in-the-loop approvals state machine + approval.*.v1
//     outbox (legacy approvals-service).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/libs/idempotency"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/approvals"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/automationoperations"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/automationoperations/steps"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/domain/automationrun"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/domain/conditionconsumer"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/domain/effectdispatcher"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/domain/workflowrunrequested"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/state"

	"github.com/google/uuid"
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

	// HTTP-health-only mode when DATABASE_URL or JWT_SECRET is unset
	// — same Rust-side fallback as lineage-service.
	if cfg.DatabaseURL == "" || cfg.JWTSecret == "" {
		log.Warn("DATABASE_URL or JWT_SECRET unset — booting HTTP-health-only (workflow / saga / approvals surface disabled)")
		srv := server.New(cfg, metrics, nil)
		if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("server exited with error", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

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

	httpClient := &http.Client{Timeout: 30 * time.Second}
	retentionSweepClient := steps.NewHTTPRetentionSweepClient(
		httpClient,
		cfg.AuditComplianceServiceURL,
		cfg.AuditComplianceBearerToken,
	)
	jwtConfig := authmw.NewJWTConfig(cfg.JWTSecret)
	appState := &state.AppState{
		DB:                         pool,
		HTTPClient:                 httpClient,
		JWTConfig:                  jwtConfig,
		NATSURL:                    cfg.NATSURL,
		PipelineServiceURL:         cfg.PipelineServiceURL,
		OntologyServiceURL:         cfg.OntologyServiceURL,
		AuditComplianceServiceURL:  cfg.AuditComplianceServiceURL,
		AuditComplianceBearerToken: cfg.AuditComplianceBearerToken,
		ApprovalTTLHours:           cfg.ApprovalTTLHours,
	}

	srv := server.New(cfg, metrics, &server.Options{
		JWT:         jwtConfig,
		Workflows:   handlers.NewCrudHandlers(appState),
		Approvals:   approvals.NewHandlers(appState),
		Automations: automationoperations.NewHandlers(appState),
	})

	// ── Background consumer best-effort boots ─────────────────────────
	var wg sync.WaitGroup

	// Legacy NATS consumer for `of.workflows.run.requested`.
	if cfg.NATSURL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := workflowrunrequested.Consume(ctx, cfg.NATSURL, func(ctx context.Context, workflowID uuid.UUID, request models.InternalTriggeredRunRequest) (*models.WorkflowRun, error) {
				return handlers.ExecuteInternalTriggeredRun(ctx, appState, workflowID, request)
			}); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("workflow run requested consumer stopped", slog.String("error", err.Error()))
			}
		}()
	}

	// Foundry-pattern Kafka condition consumer.
	if cfg.KafkaBootstrap != "" {
		if consumer, err := buildConditionConsumer(ctx, cfg, pool, httpClient); err != nil {
			log.Warn("skipping automate.condition.v1 consumer; HTTP API still online",
				slog.String("error", err.Error()))
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sub, err := databus.NewKafkaSubscriber(databus.Config{BootstrapServers: []string{cfg.KafkaBootstrap}}, conditionconsumer.ConsumerGroup, conditionconsumer.SubscribeTopics)
				if err != nil {
					log.Error("automate.condition.v1 subscriber init failed", slog.String("error", err.Error()))
					return
				}
				defer sub.Close()
				if err := conditionconsumer.Run(ctx, consumer, sub); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("automate.condition.v1 consumer stopped", slog.String("error", err.Error()))
				}
			}()
		}

		// Foundry-pattern Kafka saga consumer.
		idem := idempotency.NewPgStore(pool, automationoperations.ProcessedEventsTable)
		publisher, err := databus.NewKafkaPublisher(databus.Config{BootstrapServers: []string{cfg.KafkaBootstrap}})
		if err != nil {
			log.Warn("skipping saga.step.requested.v1 consumer (publisher init failed)",
				slog.String("error", err.Error()))
		} else {
			sagaConsumer := automationoperations.NewSagaConsumer(pool, idem, publisher, retentionSweepClient)
			wg.Add(1)
			go func() {
				defer wg.Done()
				sub, err := databus.NewKafkaSubscriber(databus.Config{BootstrapServers: []string{cfg.KafkaBootstrap}}, automationoperations.SagaConsumerGroup, []string{automationoperations.SagaStepRequestedV1})
				if err != nil {
					log.Error("saga.step.requested.v1 subscriber init failed", slog.String("error", err.Error()))
					return
				}
				defer sub.Close()
				if err := automationoperations.Run(ctx, sagaConsumer, sub); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("saga.step.requested.v1 consumer stopped", slog.String("error", err.Error()))
				}
			}()
		}
	} else {
		log.Warn("KAFKA_BOOTSTRAP_SERVERS unset — automate.condition.v1 and saga.step.requested.v1 consumers skipped")
	}

	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	wg.Wait()
}

// buildConditionConsumer mirrors `main::build_condition_consumer`.
//
// Returns an error when the ontology-actions-service URL or bearer
// token are unset — the consumer cannot dispatch effects without them.
func buildConditionConsumer(_ context.Context, cfg *config.Config, pool *pgxpool.Pool, httpClient *http.Client) (*conditionconsumer.Consumer, error) {
	ontologyURL := firstNonEmptyEnv(
		"OF_ONTOLOGY_ACTIONS_URL",
		"ONTOLOGY_ACTIONS_SERVICE_URL",
		"ONTOLOGY_SERVICE_URL",
		"OF_ONTOLOGY_ACTIONS_GRPC_ADDR",
	)
	if ontologyURL == "" {
		ontologyURL = cfg.OntologyServiceURL
	}
	if ontologyURL == "" {
		return nil, errors.New("OF_ONTOLOGY_ACTIONS_URL is not set")
	}
	ontologyToken := firstNonEmptyEnv(
		"OF_ONTOLOGY_ACTIONS_BEARER_TOKEN",
		"ONTOLOGY_ACTIONS_BEARER_TOKEN",
	)
	if ontologyToken == "" {
		return nil, errors.New("OF_ONTOLOGY_ACTIONS_BEARER_TOKEN is not set")
	}

	dispatcher := effectdispatcher.New(httpClient, ontologyURL, ontologyToken)
	publisher, err := databus.NewKafkaPublisher(databus.Config{BootstrapServers: []string{cfg.KafkaBootstrap}})
	if err != nil {
		return nil, err
	}
	idem := idempotency.NewPgStore(pool, automationrun.ProcessedEventsTable)
	return conditionconsumer.NewConsumer(pool, idem, dispatcher, publisher), nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
