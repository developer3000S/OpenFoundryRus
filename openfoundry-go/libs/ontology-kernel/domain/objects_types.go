// Shared per-handler view types used across multiple kernel domains.
//
// `ObjectInstance` and `LinkInstance` originate in the Rust source's
// `handlers::{objects, links}` modules. They live in `domain` here
// because several domain helpers (indexer, read_models, access,
// link instance collection) need them and a Go cycle through the
// handlers package would otherwise force a split that the Rust
// crate sidesteps with module visibility.
//
// JSON tag set is byte-identical to the Rust derive — same field
// names, same `Option<T>` → `*T` mapping with `omitempty`, same
// `chrono::DateTime<Utc>` → `time.Time` mapping.
package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ObjectInstance mirrors `pub struct ObjectInstance` in
// `libs/ontology-kernel/src/handlers/objects.rs`. OrganizationID drops
// `omitempty` because Rust's `Option<Uuid>` carries no
// `skip_serializing_if` — None serialises to `"organization_id":null`
// and the Go port must emit the same key on the wire.
type ObjectInstance struct {
	ID             uuid.UUID       `json:"id"`
	ObjectTypeID   uuid.UUID       `json:"object_type_id"`
	Properties     json.RawMessage `json:"properties"`
	CreatedBy      uuid.UUID       `json:"created_by"`
	OrganizationID *uuid.UUID      `json:"organization_id"`
	Marking        string          `json:"marking"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// LinkInstance mirrors `pub struct LinkInstance` in
// `libs/ontology-kernel/src/handlers/links.rs`. Properties drops
// `omitempty` because Rust's `Option<serde_json::Value>` carries no
// `skip_serializing_if`; None serialises to `"properties":null`.
type LinkInstance struct {
	ID             uuid.UUID       `json:"id"`
	LinkTypeID     uuid.UUID       `json:"link_type_id"`
	SourceObjectID uuid.UUID       `json:"source_object_id"`
	TargetObjectID uuid.UUID       `json:"target_object_id"`
	Properties     json.RawMessage `json:"properties"`
	CreatedBy      uuid.UUID       `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
}
