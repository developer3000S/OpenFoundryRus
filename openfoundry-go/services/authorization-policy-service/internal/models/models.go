// Package models holds wire types for authorization-policy-service.
//
// The Go service now carries the consolidated HTTP/API surface: Cedar
// policy CRUD, ABAC policy evaluation, RBAC roles/groups/permissions,
// governance/project constraints, checkpoints/purpose records, cipher
// catalogs, and network-boundary policy resources.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CedarPolicy mirrors `cedar_policies` rows. The `Source` field is the
// raw Cedar policy text — every write must round-trip through
// libs/authz-cedar-go (parse + strict schema validation) before being
// persisted, so corrupt rows can't leak into the active PolicySet.
type CedarPolicy struct {
	ID          string    `json:"id"`
	Version     int32     `json:"version"`
	Source      string    `json:"source"`
	Description *string   `json:"description"`
	Active      bool      `json:"active"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateCedarPolicyRequest is the body of POST /api/v1/cedar-policies.
//
// Wire-format invariants:
//   - `id` must be non-empty (it's the Cedar PolicyID; carried byte-for-byte
//     into the in-memory PolicySet).
//   - `source` must parse + validate against the bundled Cedar schema in
//     strict mode. The handler delegates this check to libs/authz-cedar-go.
//   - `version` defaults to 1 when omitted; subsequent writes via the
//     PATCH endpoint bump it.
type CreateCedarPolicyRequest struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
	Version     *int32  `json:"version,omitempty"`
}

// UpdateCedarPolicyRequest mirrors the PATCH semantics — every field
// optional; nil preserves the current value. Bumping `source` always
// re-validates against the schema before swap.
type UpdateCedarPolicyRequest struct {
	Source      *string `json:"source,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

// ListResponse is the canonical envelope for list endpoints. Matches
// the foundation-slice convention (organizations + enrollments).
type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// ABACPolicy mirrors `abac_policies` rows — pre-Cedar legacy rules
// kept around for backwards-compat with policies authored before
// ADR-0027. The ABAC evaluator (follow-up slice) walks `Conditions`
// against the request context.
type ABACPolicy struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Effect      string          `json:"effect"`
	Resource    string          `json:"resource"`
	Action      string          `json:"action"`
	Conditions  json.RawMessage `json:"conditions"`
	RowFilter   *string         `json:"row_filter"`
	Enabled     bool            `json:"enabled"`
	CreatedBy   *uuid.UUID      `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateABACPolicyRequest is the body of POST /api/v1/abac-policies.
//
// `Effect` must be "allow" or "deny" (DB CHECK enforces it; we
// validate up front so the client gets a 400 instead of a 500).
type CreateABACPolicyRequest struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Effect      string          `json:"effect"`
	Resource    string          `json:"resource"`
	Action      string          `json:"action"`
	Conditions  json.RawMessage `json:"conditions,omitempty"`
	RowFilter   *string         `json:"row_filter,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

// UpdateABACPolicyRequest mirrors PATCH semantics — every field
// optional; nil preserves the current value.
type UpdateABACPolicyRequest struct {
	Description *string         `json:"description,omitempty"`
	Effect      *string         `json:"effect,omitempty"`
	Resource    *string         `json:"resource,omitempty"`
	Action      *string         `json:"action,omitempty"`
	Conditions  json.RawMessage `json:"conditions,omitempty"`
	RowFilter   *string         `json:"row_filter,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}
