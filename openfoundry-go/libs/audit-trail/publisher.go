package audittrail

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/libs/outbox"
)

// ToOutboxEvent renders an AuditEnvelope as an outbox.OutboxEvent
// ready for outbox.Enqueue.
//
// Aggregate is "audit_event"; aggregate_id is the resource_rid so
// Debezium produces a stable Kafka partition key per resource — events
// for the same media set arrive in order on the consumer side.
//
// `ol-run-id` and `x-request-id` headers are populated when present
// so OpenLineage threading is preserved end-to-end.
func ToOutboxEvent(env AuditEnvelope) (*outbox.OutboxEvent, error) {
	payload, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("encode audit envelope: %w", err)
	}
	evt := outbox.New(env.EventID, "audit_event", env.ResourceRID, TopicAuditEvents, payload)
	if env.CorrelationID != "" {
		evt.WithHeader("ol-run-id", env.CorrelationID)
	}
	if env.RequestID != "" {
		evt.WithHeader("x-request-id", env.RequestID)
	}
	return evt, nil
}

// EmitToOutbox builds the audit envelope, appends it to outbox.events
// inside the caller's transaction, and returns. The caller still owns
// the surrounding tx.Commit() — the SQL mutation and the audit
// emission therefore land atomically (ADR-0022).
func EmitToOutbox(ctx context.Context, tx pgx.Tx, event AuditEvent, auditCtx AuditContext) error {
	envelope, err := Build(event, auditCtx, time.Now().UTC())
	if err != nil {
		return err
	}
	outboxEvt, err := ToOutboxEvent(envelope)
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, outboxEvt)
}
