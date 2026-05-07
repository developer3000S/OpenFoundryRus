package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConnectorBinding mirrors event_streaming::models::stream::ConnectorBinding.
type ConnectorBinding struct {
	ConnectorType string          `json:"connector_type"`
	Endpoint      string          `json:"endpoint"`
	Format        string          `json:"format"`
	Config        json.RawMessage `json:"config"`
}

// StreamField mirrors event_streaming::models::stream::StreamField.
type StreamField struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	Nullable     bool   `json:"nullable"`
	SemanticRole string `json:"semantic_role"`
}

// StreamSchema mirrors event_streaming::models::stream::StreamSchema.
type StreamSchema struct {
	Fields         []StreamField `json:"fields"`
	PrimaryKey     *string       `json:"primary_key,omitempty"`
	WatermarkField *string       `json:"watermark_field,omitempty"`
}

// DomainStreamDefinition is the typed-domain projection of a streaming
// stream row. Distinct from the wire-shape models.StreamDefinition (which
// uses json.RawMessage for schema/source_binding) — the domain layer needs
// the parsed types to drive the engine.
type DomainStreamDefinition struct {
	ID                   uuid.UUID        `json:"id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Status               string           `json:"status"`
	Schema               StreamSchema     `json:"schema"`
	SourceBinding        ConnectorBinding `json:"source_binding"`
	RetentionHours       int32            `json:"retention_hours"`
	Partitions           int32            `json:"partitions"`
	ConsistencyGuarantee string           `json:"consistency_guarantee"`
	StreamType           string           `json:"stream_type"`
	Compression          bool             `json:"compression"`
	IngestConsistency    string           `json:"ingest_consistency"`
	PipelineConsistency  string           `json:"pipeline_consistency"`
	CheckpointIntervalMS int32            `json:"checkpoint_interval_ms"`
	Kind                 string           `json:"kind"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}
