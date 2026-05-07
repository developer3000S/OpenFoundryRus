// Package models holds wire types for iceberg-catalog-service.
//
// Foundation slice scope: iceberg_namespaces only. iceberg_tables,
// iceberg_snapshots, iceberg_branches, metadata files, REST Catalog
// OpenAPI surface (10k LOC of Rust handlers) all land in follow-up
// slices.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// IcebergNamespace mirrors `iceberg_namespaces` rows.
type IcebergNamespace struct {
	ID                uuid.UUID       `json:"id"`
	ProjectRID        string          `json:"project_rid"`
	Name              string          `json:"name"`
	ParentNamespaceID *uuid.UUID      `json:"parent_namespace_id"`
	Properties        json.RawMessage `json:"properties"`
	CreatedAt         time.Time       `json:"created_at"`
	CreatedBy         uuid.UUID       `json:"created_by"`
}

// CreateNamespaceRequest is the body of POST /api/v1/namespaces.
type CreateNamespaceRequest struct {
	ProjectRID        string          `json:"project_rid"`
	Name              string          `json:"name"`
	ParentNamespaceID *uuid.UUID      `json:"parent_namespace_id,omitempty"`
	Properties        json.RawMessage `json:"properties,omitempty"`
}

// UpdateNamespaceRequest mirrors PATCH semantics — `properties` is the
// only mutable field (name/parent_namespace_id are immutable per the
// Iceberg REST spec).
type UpdateNamespaceRequest struct {
	Properties json.RawMessage `json:"properties,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

type TableIdentifier struct {
	Namespace []string `json:"namespace"`
	Name      string   `json:"name"`
}

type ListTablesResponse struct {
	Identifiers []TableIdentifier `json:"identifiers"`
}

type IcebergTable struct {
	ID                      uuid.UUID       `json:"id"`
	RID                     string          `json:"rid"`
	NamespaceID             uuid.UUID       `json:"namespace_id"`
	Namespace               []string        `json:"namespace"`
	Name                    string          `json:"name"`
	TableUUID               string          `json:"table_uuid"`
	FormatVersion           int32           `json:"format_version"`
	Location                string          `json:"location"`
	CurrentSnapshotID       *int64          `json:"current_snapshot_id"`
	CurrentMetadataLocation *string         `json:"current_metadata_location"`
	LastSequenceNumber      int64           `json:"last_sequence_number"`
	PartitionSpec           json.RawMessage `json:"partition_spec"`
	SchemaJSON              json.RawMessage `json:"schema_json"`
	SortOrder               json.RawMessage `json:"sort_order"`
	Properties              json.RawMessage `json:"properties"`
	Markings                []string        `json:"markings"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type CreateTableRequest struct {
	Name          string            `json:"name"`
	Schema        json.RawMessage   `json:"schema"`
	PartitionSpec json.RawMessage   `json:"partition-spec,omitempty"`
	SortOrder     json.RawMessage   `json:"sort-order,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
	Location      *string           `json:"location,omitempty"`
	FormatVersion *int32            `json:"format-version,omitempty"`
	StageCreate   *bool             `json:"stage-create,omitempty"`
	Markings      []string          `json:"markings,omitempty"`
}

type LoadTableResponse struct {
	Metadata         json.RawMessage   `json:"metadata"`
	MetadataLocation string            `json:"metadata-location"`
	Config           map[string]string `json:"config"`
}

type Snapshot struct {
	ID                   int64           `json:"id"`
	TableID              uuid.UUID       `json:"table_id"`
	SnapshotID           int64           `json:"snapshot_id"`
	ParentSnapshotID     *int64          `json:"parent_snapshot_id"`
	SequenceNumber       int64           `json:"sequence_number"`
	Operation            string          `json:"operation"`
	ManifestListLocation string          `json:"manifest_list_location"`
	Summary              json.RawMessage `json:"summary"`
	SchemaID             int32           `json:"schema_id"`
	TimestampMS          int64           `json:"timestamp_ms"`
}

type CommitTableRequest struct {
	Identifier   *TableIdentifier  `json:"identifier,omitempty"`
	Requirements []json.RawMessage `json:"requirements,omitempty"`
	Updates      []json.RawMessage `json:"updates,omitempty"`
}

type CommitTableResponse struct {
	Metadata         json.RawMessage `json:"metadata"`
	MetadataLocation string          `json:"metadata-location"`
}

type TableRef struct {
	ID                 uuid.UUID `json:"id,omitempty"`
	TableID            uuid.UUID `json:"table_id,omitempty"`
	Name               string    `json:"name"`
	Kind               string    `json:"type"`
	SnapshotID         int64     `json:"snapshot-id"`
	MaxRefAgeMS        *int64    `json:"max-ref-age-ms,omitempty"`
	MaxSnapshotAgeMS   *int64    `json:"max-snapshot-age-ms,omitempty"`
	MinSnapshotsToKeep *int32    `json:"min-snapshots-to-keep,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
}

type ListRefsResponse struct {
	Refs map[string]TableRef `json:"refs"`
}

type UpdateRefRequest struct {
	Type               string `json:"type"`
	SnapshotID         int64  `json:"snapshot-id"`
	MaxRefAgeMS        *int64 `json:"max-ref-age-ms,omitempty"`
	MaxSnapshotAgeMS   *int64 `json:"max-snapshot-age-ms,omitempty"`
	MinSnapshotsToKeep *int32 `json:"min-snapshots-to-keep,omitempty"`
}

type MetadataFile struct {
	ID        uuid.UUID `json:"id,omitempty"`
	TableID   uuid.UUID `json:"table_id,omitempty"`
	Version   int32     `json:"version"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type ListMetadataFilesResponse struct {
	MetadataFiles []MetadataFile `json:"metadata-files"`
}

type MetadataFileResponse struct {
	Metadata         json.RawMessage `json:"metadata"`
	MetadataLocation string          `json:"metadata-location"`
	Version          int32           `json:"version"`
}

type ListSnapshotsResponse struct {
	Snapshots []Snapshot `json:"snapshots"`
}

type RenameTableRequest struct {
	Source      TableIdentifier `json:"source"`
	Destination TableIdentifier `json:"destination"`
}
