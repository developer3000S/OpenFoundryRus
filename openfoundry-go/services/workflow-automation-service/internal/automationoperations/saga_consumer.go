// Kafka consumer for `saga.step.requested.v1` — entry point of the
// saga runtime.

package automationoperations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/libs/idempotency"
	saga "github.com/openfoundry/openfoundry-go/libs/saga"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/automationoperations/steps"
)

// SagaConsumer holds the long-lived state shared across the loop.
type SagaConsumer struct {
	Pool        *pgxpool.Pool
	Idempotency idempotency.Store
	// Publisher kept around for the future control-plane publish path
	// (e.g. emitting a synthetic `saga.aborted.v1` to Kafka without
	// going through outbox+Debezium when the service shuts down
	// mid-saga).
	Publisher            databus.Publisher
	RetentionSweepClient steps.RetentionSweepClient
}

// NewSagaConsumer mirrors `SagaConsumer::new`.
func NewSagaConsumer(pool *pgxpool.Pool, idem idempotency.Store, publisher databus.Publisher, retentionClient ...steps.RetentionSweepClient) *SagaConsumer {
	var client steps.RetentionSweepClient
	if len(retentionClient) > 0 {
		client = retentionClient[0]
	}
	return &SagaConsumer{Pool: pool, Idempotency: idem, Publisher: publisher, RetentionSweepClient: client}
}

// Process drives one inbound saga request. Returns a metric label.
func (c *SagaConsumer) Process(ctx context.Context, request saga.SagaStepRequestedV1Payload) (string, error) {
	eventID := DeriveRequestEventID(request.Saga, request.CorrelationID)
	outcome, err := c.Idempotency.CheckAndRecord(ctx, eventID)
	if err != nil {
		return "", err
	}
	if outcome.IsAlreadyProcessed() {
		slog.Info("saga.step.requested already processed; skipping",
			slog.String("event_id", eventID.String()),
			slog.String("saga", request.Saga),
			slog.String("saga_id", request.SagaID.String()),
			slog.String("correlation_id", request.CorrelationID.String()))
		return "deduped", nil
	}

	expected := DeriveSagaID(request.Saga, request.CorrelationID)
	if expected != request.SagaID {
		slog.Warn("saga_id derivation mismatch between producer and consumer; honoring producer",
			slog.String("producer_saga_id", request.SagaID.String()),
			slog.String("consumer_derived", expected.String()),
			slog.String("saga", request.Saga))
	}
	return c.runSaga(ctx, request)
}

func (c *SagaConsumer) runSaga(ctx context.Context, request saga.SagaStepRequestedV1Payload) (string, error) {
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	runner, err := saga.Start(ctx, tx, request.SagaID, request.Saga)
	if err != nil {
		// Commit the failure metadata where possible — Start returns
		// ErrTerminal when the row is already terminal; that's a
		// no-op success label.
		return "", err
	}
	dispatchErr := DispatchSaga(ctx, runner, request.Saga, request.Input, DispatchOptions{RetentionSweepClient: c.RetentionSweepClient})
	// Commit regardless of the dispatch outcome — the runner has
	// already updated saga.state to its terminal value (`completed`,
	// `failed`, or `compensated`) and emitted the matching outbox
	// events. Aborting the TX here would lose the audit trail.
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	if dispatchErr != nil {
		slog.Warn("saga ended in non-completed terminal state",
			slog.String("saga", request.Saga),
			slog.String("saga_id", request.SagaID.String()),
			slog.String("error", dispatchErr.Error()))
		return "failed_or_compensated", nil
	}
	return "completed", nil
}

// DecodeRequest mirrors `decode_request`.
func DecodeRequest(payload []byte) (saga.SagaStepRequestedV1Payload, error) {
	var req saga.SagaStepRequestedV1Payload
	err := json.Unmarshal(payload, &req)
	return req, err
}

// Run is the at-least-once consumer loop.
func Run(ctx context.Context, c *SagaConsumer, sub databus.Subscriber) error {
	for {
		msg, err := sub.Poll(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			return err
		}
		label, err := processMessage(ctx, c, msg)
		if err != nil {
			slog.Error("saga request processing failed; offset uncommitted",
				slog.String("error", err.Error()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		slog.Debug("saga request processed",
			slog.String("label", label), slog.String("topic", msg.Topic))
		if err := sub.CommitMessages(ctx, []*databus.DataMessage{msg}); err != nil {
			return err
		}
	}
}

func processMessage(ctx context.Context, c *SagaConsumer, msg *databus.DataMessage) (string, error) {
	if len(msg.Value) == 0 {
		slog.Warn("skipping saga.step.requested without payload",
			slog.String("topic", msg.Topic),
			slog.Int("partition", msg.Partition),
			slog.Int64("offset", msg.Offset))
		return "empty_payload", nil
	}
	req, err := DecodeRequest(msg.Value)
	if err != nil {
		slog.Warn("skipping malformed saga.step.requested",
			slog.String("error", err.Error()))
		return "decode_error", nil
	}
	return c.Process(ctx, req)
}
