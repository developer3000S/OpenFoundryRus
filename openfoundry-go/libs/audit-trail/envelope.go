package audittrail

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TopicAuditEvents is the Kafka topic audit-sink subscribes to.
// Pinned here so a typo at wiring time is a compile error rather than
// silent log loss.
const TopicAuditEvents = "audit.events.v1"

// auditEventNamespace is the v5 UUID namespace used to derive
// deterministic event ids. Same bytes as the Rust constant — DO NOT
// CHANGE without coordinating an audit-sink + outbox migration.
var auditEventNamespace = uuid.UUID{
	0x4b, 0x32, 0x9e, 0x71, 0x6d, 0xa4, 0x4f, 0x8e,
	0x8c, 0x40, 0xc1, 0xa3, 0xee, 0x12, 0x55, 0xaa,
}

// AuditContext is request-side metadata captured by middleware (or
// supplied directly by background workers). All fields are optional.
type AuditContext struct {
	ActorID       string
	IP            string
	UserAgent     string
	RequestID     string
	CorrelationID string
	LatencyMS     *uint64
	SourceService string
}

// ForActor builds the minimal context background workers need.
func ForActor(actorID string) AuditContext { return AuditContext{ActorID: actorID} }

// AuditEnvelope is the wire shape published on TopicAuditEvents.
//
// Fields mirror the Rust AuditEnvelope verbatim — JSON tags + omitempty
// behaviour ensure cross-language round-trip is byte-identical.
type AuditEnvelope struct {
	EventID         uuid.UUID         `json:"event_id"`
	At              int64             `json:"at"`
	Kind            EventKind         `json:"kind"`
	Categories      []string          `json:"categories"`
	ResourceRID     string            `json:"resource_rid"`
	ProjectRID      string            `json:"project_rid"`
	MarkingsAtEvent []string          `json:"markings_at_event"`
	ActorID         string            `json:"actor_id,omitempty"`
	IP              string            `json:"ip,omitempty"`
	UserAgent       string            `json:"user_agent,omitempty"`
	RequestID       string            `json:"request_id,omitempty"`
	CorrelationID   string            `json:"correlation_id,omitempty"`
	LatencyMS       *uint64           `json:"latency_ms,omitempty"`
	SourceService   string            `json:"source_service,omitempty"`
	OccurredAt      time.Time         `json:"occurred_at"`
	Payload         json.RawMessage   `json:"payload"`
}

// Build constructs an AuditEnvelope with a deterministic v5 event_id.
//
// Identity seed precedence: request_id → correlation_id → resource_rid
// (matches the Rust impl). A retried handler converges to the same
// outbox row under the table's primary key.
func Build(event AuditEvent, ctx AuditContext, occurredAt time.Time) (AuditEnvelope, error) {
	categories := CategoriesFor(event.Kind)
	catStrings := make([]string, len(categories))
	for i, c := range categories {
		catStrings[i] = string(c)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return AuditEnvelope{}, err
	}

	identitySeed := ctx.RequestID
	if identitySeed == "" {
		identitySeed = ctx.CorrelationID
	}
	if identitySeed == "" {
		identitySeed = event.ResourceRID
	}
	eventID := DeriveEventID(string(event.Kind), event.ResourceRID, identitySeed)

	return AuditEnvelope{
		EventID:         eventID,
		At:              occurredAt.UnixMicro(),
		Kind:            event.Kind,
		Categories:      catStrings,
		ResourceRID:     event.ResourceRID,
		ProjectRID:      event.ProjectRID,
		MarkingsAtEvent: event.MarkingsAtEvent,
		ActorID:         ctx.ActorID,
		IP:              ctx.IP,
		UserAgent:       ctx.UserAgent,
		RequestID:       ctx.RequestID,
		CorrelationID:   ctx.CorrelationID,
		LatencyMS:       ctx.LatencyMS,
		SourceService:   ctx.SourceService,
		OccurredAt:      occurredAt,
		Payload:         payload,
	}, nil
}

// DeriveEventID is the deterministic v5 UUID derivation.
// Same inputs → same UUID, identical to the Rust `derive_event_id`.
func DeriveEventID(kind, resourceRID, identitySeed string) uuid.UUID {
	name := "audit/" + kind + "/" + resourceRID + "/" + identitySeed
	return uuid.NewSHA1(auditEventNamespace, []byte(name))
}
