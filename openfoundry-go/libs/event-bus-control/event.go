package controlbus

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
)

// Event is the envelope for every message published through the bus.
//
// JSON shape mirrors the Rust `event_bus_control::schemas::Event` exactly:
//
//	{
//	  "id":         "<uuid v7>",
//	  "timestamp":  "<RFC3339 UTC>",
//	  "event_type": "auth.user.created",
//	  "source":     "identity-federation-service",
//	  "payload":    { ... }
//	}
type Event struct {
	ID        uuid.UUID       `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	EventType string          `json:"event_type"`
	Source    string          `json:"source"`
	Payload   json.RawMessage `json:"payload"`
}

// NewEvent builds an Event with a fresh v7 ID and current UTC timestamp.
//
// `payload` may be any JSON-marshallable value; this helper marshals it
// once so callers don't pay the cost twice.
func NewEvent(eventType, source string, payload any) (*Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:        ids.New(),
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		Source:    source,
		Payload:   raw,
	}, nil
}
