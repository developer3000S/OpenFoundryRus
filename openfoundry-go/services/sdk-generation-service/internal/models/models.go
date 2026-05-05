// Package models holds wire types for sdk-generation-service.
//
// JSON shape preserves the Rust crate's `PrimaryItem` / `SecondaryItem`
// (id / payload / created_at / parent_id) verbatim.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Job is a row in `sdk_generation_jobs` (Rust `PrimaryItem`).
type Job struct {
	ID        uuid.UUID       `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreateJobRequest is the body of POST /sdk-generation-jobs.
type CreateJobRequest struct {
	Payload json.RawMessage `json:"payload"`
}

// Publication is a row in `sdk_generation_publications` (Rust `SecondaryItem`).
type Publication struct {
	ID        uuid.UUID       `json:"id"`
	ParentID  uuid.UUID       `json:"parent_id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreatePublicationRequest is the body of POST /sdk-generation-jobs/{id}/publications.
type CreatePublicationRequest struct {
	Payload json.RawMessage `json:"payload"`
}
