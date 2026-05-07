// Package models holds wire types for ingestion-replication-service.
//
// Foundation slice scope: ingest_jobs only. Streaming + cdc_metadata
// sub-modules (~30k LOC across 20+ submodule migrations) land in
// follow-up slices.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// IngestJob mirrors `ingest_jobs` rows. Each job materializes a Kafka
// connector + Flink deployment pair tracked by status.
type IngestJob struct {
	ID                  uuid.UUID       `json:"id"`
	Name                string          `json:"name"`
	Namespace           string          `json:"namespace"`
	Spec                json.RawMessage `json:"spec"`
	Status              string          `json:"status"`
	KafkaConnectorName  *string         `json:"kafka_connector_name"`
	FlinkDeploymentName *string         `json:"flink_deployment_name"`
	Error               *string         `json:"error"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// CreateIngestJobRequest is POST /api/v1/ingest-jobs.
type CreateIngestJobRequest struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Spec      json.RawMessage `json:"spec"`
}

// UpdateIngestJobRequest mirrors PATCH semantics — used by the runtime
// to advance status and stamp connector/deployment names.
type UpdateIngestJobRequest struct {
	Status              *string `json:"status,omitempty"`
	KafkaConnectorName  *string `json:"kafka_connector_name,omitempty"`
	FlinkDeploymentName *string `json:"flink_deployment_name,omitempty"`
	Error               *string `json:"error,omitempty"`
}

// StreamDefinition is the minimal event-streaming CRUD/config projection.
type StreamDefinition struct {
	ID                   uuid.UUID       `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	Status               string          `json:"status"`
	Schema               json.RawMessage `json:"schema"`
	SourceBinding        json.RawMessage `json:"source_binding"`
	RetentionHours       int32           `json:"retention_hours"`
	Partitions           int32           `json:"partitions"`
	ConsistencyGuarantee string          `json:"consistency_guarantee"`
	StreamType           string          `json:"stream_type"`
	Compression          bool            `json:"compression"`
	IngestConsistency    string          `json:"ingest_consistency"`
	PipelineConsistency  string          `json:"pipeline_consistency"`
	CheckpointIntervalMS int32           `json:"checkpoint_interval_ms"`
	Kind                 string          `json:"kind"`
	OwnerID              uuid.UUID       `json:"owner_id"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type CreateStreamRequest struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	Status               string          `json:"status,omitempty"`
	Schema               json.RawMessage `json:"schema,omitempty"`
	SourceBinding        json.RawMessage `json:"source_binding,omitempty"`
	RetentionHours       *int32          `json:"retention_hours,omitempty"`
	Partitions           *int32          `json:"partitions,omitempty"`
	ConsistencyGuarantee string          `json:"consistency_guarantee,omitempty"`
	StreamType           string          `json:"stream_type,omitempty"`
	Compression          *bool           `json:"compression,omitempty"`
	IngestConsistency    string          `json:"ingest_consistency,omitempty"`
	PipelineConsistency  string          `json:"pipeline_consistency,omitempty"`
	CheckpointIntervalMS *int32          `json:"checkpoint_interval_ms,omitempty"`
	Kind                 string          `json:"kind,omitempty"`
}

type UpdateStreamRequest struct {
	Name                 *string         `json:"name,omitempty"`
	Description          *string         `json:"description,omitempty"`
	Status               *string         `json:"status,omitempty"`
	Schema               json.RawMessage `json:"schema,omitempty"`
	SourceBinding        json.RawMessage `json:"source_binding,omitempty"`
	RetentionHours       *int32          `json:"retention_hours,omitempty"`
	Partitions           *int32          `json:"partitions,omitempty"`
	ConsistencyGuarantee *string         `json:"consistency_guarantee,omitempty"`
	StreamType           *string         `json:"stream_type,omitempty"`
	Compression          *bool           `json:"compression,omitempty"`
	IngestConsistency    *string         `json:"ingest_consistency,omitempty"`
	PipelineConsistency  *string         `json:"pipeline_consistency,omitempty"`
	CheckpointIntervalMS *int32          `json:"checkpoint_interval_ms,omitempty"`
	Kind                 *string         `json:"kind,omitempty"`
}

type CdcStream struct {
	ID              uuid.UUID       `json:"id"`
	Slug            string          `json:"slug"`
	SourceKind      string          `json:"source_kind"`
	SourceRef       string          `json:"source_ref"`
	UpstreamTopic   *string         `json:"upstream_topic"`
	PrimaryKeys     json.RawMessage `json:"primary_keys"`
	WatermarkColumn *string         `json:"watermark_column"`
	IncrementalMode string          `json:"incremental_mode"`
	Status          string          `json:"status"`
	OwnerID         uuid.UUID       `json:"owner_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type RegisterCdcStreamRequest struct {
	Slug            string   `json:"slug"`
	SourceKind      string   `json:"source_kind"`
	SourceRef       string   `json:"source_ref"`
	UpstreamTopic   *string  `json:"upstream_topic,omitempty"`
	PrimaryKeys     []string `json:"primary_keys,omitempty"`
	WatermarkColumn *string  `json:"watermark_column,omitempty"`
	IncrementalMode string   `json:"incremental_mode,omitempty"`
}

type IncrementalCheckpoint struct {
	StreamID        uuid.UUID  `json:"stream_id"`
	LastOffset      *string    `json:"last_offset"`
	LastLSN         *string    `json:"last_lsn"`
	LastEventAt     *time.Time `json:"last_event_at"`
	RecordsObserved int64      `json:"records_observed"`
	RecordsApplied  int64      `json:"records_applied"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ResolutionState struct {
	StreamID           uuid.UUID  `json:"stream_id"`
	Status             string     `json:"status"`
	Watermark          *time.Time `json:"watermark"`
	ConflictCount      int64      `json:"conflict_count"`
	PendingResolutions int64      `json:"pending_resolutions"`
	Notes              *string    `json:"notes"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CheckpointUpdate is emitted by the streaming runtime when Kafka/Flink CDC
// registration or callbacks advance the durable source offset.
type CheckpointUpdate struct {
	LastOffset      *string    `json:"last_offset,omitempty"`
	LastLSN         *string    `json:"last_lsn,omitempty"`
	LastEventAt     *time.Time `json:"last_event_at,omitempty"`
	RecordsObserved *int64     `json:"records_observed,omitempty"`
	RecordsApplied  *int64     `json:"records_applied,omitempty"`
}

// ResolutionUpdate is emitted by the runtime when CDC conflict-resolution state
// changes after registration or checkpoint callbacks.
type ResolutionUpdate struct {
	Status             *string    `json:"status,omitempty"`
	Watermark          *time.Time `json:"watermark,omitempty"`
	ConflictCount      *int64     `json:"conflict_count,omitempty"`
	PendingResolutions *int64     `json:"pending_resolutions,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
}
