// Package envelope owns the wire format of an `audit.events.v1` Kafka record.
//
// The shape MUST match Rust `audit_sink::AuditEnvelope` and the
// upstream emitter `libs/audit-trail` byte-for-byte: every field name +
// JSON tag is locked. A change here requires the audit-sink rollout
// playbook (drain → upgrade producers → upgrade sink → unpause).
package envelope

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// AuditEnvelope is the JSON record landing on `audit.events.v1`.
//
// `at` is Unix epoch microseconds — the partition key for the
// downstream `day(at)` Iceberg transform.
type AuditEnvelope struct {
	EventID       uuid.UUID       `json:"event_id"`
	At            int64           `json:"at"`
	CorrelationID *string         `json:"correlation_id,omitempty"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
}

// Decode is the canonical JSON decoder. Returns DecodeError so callers
// can branch on `errors.Is(err, ErrInvalidJSON)` without inspecting strings.
func Decode(b []byte) (AuditEnvelope, error) {
	var env AuditEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return AuditEnvelope{}, &DecodeError{Cause: err}
	}
	return env, nil
}

// DecodeError wraps a JSON decode failure.
type DecodeError struct{ Cause error }

func (e *DecodeError) Error() string { return fmt.Sprintf("invalid audit JSON: %s", e.Cause) }
func (e *DecodeError) Unwrap() error { return e.Cause }
