// Package envelope owns the wire format of an `ai.events.v1` Kafka record.
//
// JSON shape mirrors Rust `ai_sink::AiEventEnvelope` byte-for-byte;
// `kind` is the lowercase token (prompt|response|evaluation|trace).
package envelope

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// AiEventKind is the wire token in `kind`.
type AiEventKind string

const (
	KindPrompt     AiEventKind = "prompt"
	KindResponse   AiEventKind = "response"
	KindEvaluation AiEventKind = "evaluation"
	KindTrace      AiEventKind = "trace"
)

// Iceberg table names for each kind. Pinned constants — renaming a
// table here without an Iceberg migration silently mis-routes events.
const (
	TablePrompts     = "prompts"
	TableResponses   = "responses"
	TableEvaluations = "evaluations"
	TableTraces      = "traces"
)

// TargetTable returns the Iceberg table name for `kind`.
func TargetTable(k AiEventKind) (string, bool) {
	switch k {
	case KindPrompt:
		return TablePrompts, true
	case KindResponse:
		return TableResponses, true
	case KindEvaluation:
		return TableEvaluations, true
	case KindTrace:
		return TableTraces, true
	}
	return "", false
}

// AiEventEnvelope is the JSON record landing on `ai.events.v1`.
type AiEventEnvelope struct {
	EventID       uuid.UUID       `json:"event_id"`
	At            int64           `json:"at"`
	Kind          AiEventKind     `json:"kind"`
	RunID         *uuid.UUID      `json:"run_id,omitempty"`
	TraceID       *string         `json:"trace_id,omitempty"`
	Producer      string          `json:"producer"`
	SchemaVersion uint32          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

// Decode is the canonical JSON decoder.
func Decode(b []byte) (AiEventEnvelope, error) {
	var env AiEventEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return AiEventEnvelope{}, &DecodeError{Cause: err}
	}
	return env, nil
}

// Route picks the target Iceberg table for a decoded envelope. Returns
// ErrUnknownKind when the kind is missing or invalid.
func Route(env *AiEventEnvelope) (string, error) {
	if t, ok := TargetTable(env.Kind); ok {
		return t, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownKind, env.Kind)
}

// ErrUnknownKind is returned when an envelope carries a kind outside
// the four known values. Producers send legitimate AI events; a value
// here usually means schema drift on the producing service.
var ErrUnknownKind = errors.New("unknown ai event kind")

// DecodeError wraps a JSON decode failure.
type DecodeError struct{ Cause error }

func (e *DecodeError) Error() string { return fmt.Sprintf("invalid ai event JSON: %s", e.Cause) }
func (e *DecodeError) Unwrap() error { return e.Cause }
