// Package models holds the wire-format types for solution-design-service.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PrimaryItem mirrors the `solution_diagrams` row.
type PrimaryItem struct {
	ID        uuid.UUID       `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreatePrimaryRequest is the POST /api/v1/solution-design body.
type CreatePrimaryRequest struct {
	Payload json.RawMessage `json:"payload"`
}

// SecondaryItem mirrors the `solution_references` row.
type SecondaryItem struct {
	ID        uuid.UUID       `json:"id"`
	ParentID  uuid.UUID       `json:"parent_id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreateSecondaryRequest is the POST .../{parent_id}/references body.
type CreateSecondaryRequest struct {
	Payload json.RawMessage `json:"payload"`
}
