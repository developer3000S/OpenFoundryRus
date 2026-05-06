// Package models holds wire types for connector-management-service.
//
// Foundation slice scope: connections only. sync_jobs, virtual_tables,
// data_connection_mvp, enterprise_connectivity, media_set_syncs all
// land in follow-up slices (30k LOC of Rust source).
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// Connection mirrors the `connections` row.
type Connection struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	ConnectorType string          `json:"connector_type"`
	Config        json.RawMessage `json:"config"`
	Status        string          `json:"status"`
	OwnerID       uuid.UUID       `json:"owner_id"`
	LastSyncAt    *time.Time      `json:"last_sync_at"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CreateConnectionRequest is POST /api/v1/connections.
type CreateConnectionRequest struct {
	Name          string          `json:"name"`
	ConnectorType string          `json:"connector_type"`
	Config        json.RawMessage `json:"config,omitempty"`
}

// UpdateConnectionRequest mirrors PATCH semantics.
type UpdateConnectionRequest struct {
	Name   *string         `json:"name,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
	Status *string         `json:"status,omitempty"`
}
