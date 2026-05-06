// Package models holds the wire-format types for the future
// exploratory-analysis surface (saved views, maps, writeback proposals).
//
// Foundation slice: types only — not mounted on the public router yet,
// matching the Rust binary which keeps the same handlers as
// `#[allow(dead_code)]` until the four consolidation merges land
// (per src/main.rs).
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ExploratoryView is a saved object-set view (filter + layout).
type ExploratoryView struct {
	ID         uuid.UUID       `json:"id"`
	Slug       string          `json:"slug"`
	Name       string          `json:"name"`
	ObjectType string          `json:"object_type"`
	FilterSpec json.RawMessage `json:"filter_spec"`
	Layout     json.RawMessage `json:"layout"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type CreateViewRequest struct {
	Slug       string          `json:"slug"`
	Name       string          `json:"name"`
	ObjectType string          `json:"object_type"`
	FilterSpec json.RawMessage `json:"filter_spec"`
	Layout     json.RawMessage `json:"layout,omitempty"`
}

// ExploratoryMap is a saved map (heatmap / choropleth / geo).
type ExploratoryMap struct {
	ID        uuid.UUID       `json:"id"`
	ViewID    *uuid.UUID      `json:"view_id"`
	Name      string          `json:"name"`
	MapKind   string          `json:"map_kind"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
}

type CreateMapRequest struct {
	ViewID  *uuid.UUID      `json:"view_id,omitempty"`
	Name    string          `json:"name"`
	MapKind string          `json:"map_kind"`
	Config  json.RawMessage `json:"config"`
}

// WritebackProposal is a proposed patch on an ontology object.
// It maps to an `actions_log` Cassandra append in the Rust layout.
type WritebackProposal struct {
	ID         uuid.UUID       `json:"id"`
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	Patch      json.RawMessage `json:"patch"`
	Note       *string         `json:"note"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
}

type WritebackProposalRequest struct {
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	Patch      json.RawMessage `json:"patch"`
	Note       *string         `json:"note,omitempty"`
}
