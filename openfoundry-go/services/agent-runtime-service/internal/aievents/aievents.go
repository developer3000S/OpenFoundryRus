// Package aievents pins the wire format of the AI-event publisher
// substrate (S5.3.a). The matching ai-sink consumer subscribes to
// the same Topic and dispatches by AiEventKind into the four
// of_ai.* Iceberg tables; the constants must stay in lockstep.
package aievents

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Topic is the Kafka topic name. Also pinned in the matching ACL CR
// at infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml
// (Write for this service).
const Topic = "ai.events.v1"

// TxnIDPrefix is the producer transactional-id prefix declared in
// the ACL.
const TxnIDPrefix = "agent-runtime-"

// Producer is the canonical producer name set on every envelope.
// Post-ADR-0030 only this service produces; legacy
// "prompt-workflow-service" retired.
const Producer = "agent-runtime-service"

// AiEventKind discriminates the four kinds the ai-sink dispatches on.
// JSON form is lowercase to match Rust serde rename_all="lowercase".
type AiEventKind string

const (
	KindPrompt     AiEventKind = "prompt"
	KindResponse   AiEventKind = "response"
	KindEvaluation AiEventKind = "evaluation"
	KindTrace      AiEventKind = "trace"
)

// TargetTable returns the Iceberg target table for this kind. The
// sink uses this directly so the routing logic does not drift
// between producer and consumer.
func (k AiEventKind) TargetTable() string {
	switch k {
	case KindPrompt:
		return "prompts"
	case KindResponse:
		return "responses"
	case KindEvaluation:
		return "evaluations"
	case KindTrace:
		return "traces"
	}
	return ""
}

// AiEventEnvelope is the wire envelope every record on Topic
// deserialises into. Payload is opaque JSON sized to the table
// dictated by Kind. Schema evolution lives in the payload, not
// the envelope.
type AiEventEnvelope struct {
	// EventID is a deterministic v5 UUID — the dedup key for the sink.
	EventID uuid.UUID `json:"event_id"`
	// At is microseconds since unix epoch (partition source: day(at)).
	At int64 `json:"at"`
	// Kind routes to one of the four target tables.
	Kind AiEventKind `json:"kind"`
	// RunID is the agent runtime's run id (or workflow id from the
	// retired prompt-workflow service). Lets the sink JOIN events
	// back to a run without reading payload.
	RunID *uuid.UUID `json:"run_id"`
	// TraceID is the OpenTelemetry trace id (hex 32) when the event
	// sits inside a trace context.
	TraceID *string `json:"trace_id"`
	// Producer is the producer name — "agent-runtime-service".
	Producer string `json:"producer"`
	// SchemaVersion of payload for the (Kind, version) tuple.
	SchemaVersion uint32 `json:"schema_version"`
	// Payload is opaque JSON; Iceberg writer stores as `string`.
	Payload json.RawMessage `json:"payload"`
}
