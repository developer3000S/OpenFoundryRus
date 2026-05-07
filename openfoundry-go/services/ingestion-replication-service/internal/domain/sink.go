package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConnectorCatalogEntry mirrors event_streaming::models::sink::ConnectorCatalogEntry.
type ConnectorCatalogEntry struct {
	ConnectorType       string          `json:"connector_type"`
	Direction           string          `json:"direction"`
	Endpoint            string          `json:"endpoint"`
	Status              string          `json:"status"`
	Backlog             int32           `json:"backlog"`
	ThroughputPerSecond float32         `json:"throughput_per_second"`
	Details             json.RawMessage `json:"details"`
}

// BackpressureSnapshot mirrors event_streaming::models::sink::BackpressureSnapshot.
type BackpressureSnapshot struct {
	QueueDepth     int32   `json:"queue_depth"`
	QueueCapacity  int32   `json:"queue_capacity"`
	LagMS          int32   `json:"lag_ms"`
	ThrottleFactor float32 `json:"throttle_factor"`
	Status         string  `json:"status"`
}

// StateStoreSnapshot mirrors event_streaming::models::sink::StateStoreSnapshot.
type StateStoreSnapshot struct {
	Backend          string    `json:"backend"`
	Namespace        string    `json:"namespace"`
	KeyCount         int32     `json:"key_count"`
	DiskUsageMB      int32     `json:"disk_usage_mb"`
	CheckpointCount  int32     `json:"checkpoint_count"`
	LastCheckpointAt time.Time `json:"last_checkpoint_at"`
}

// WindowAggregate mirrors event_streaming::models::sink::WindowAggregate.
type WindowAggregate struct {
	WindowName  string    `json:"window_name"`
	WindowType  string    `json:"window_type"`
	BucketStart time.Time `json:"bucket_start"`
	BucketEnd   time.Time `json:"bucket_end"`
	GroupKey    string    `json:"group_key"`
	MeasureName string    `json:"measure_name"`
	Value       float64   `json:"value"`
}

// LiveTailEvent mirrors event_streaming::models::sink::LiveTailEvent.
type LiveTailEvent struct {
	ID             string          `json:"id"`
	TopologyID     uuid.UUID       `json:"topology_id"`
	StreamName     string          `json:"stream_name"`
	ConnectorType  string          `json:"connector_type"`
	Payload        json.RawMessage `json:"payload"`
	EventTime      time.Time       `json:"event_time"`
	ProcessingTime time.Time       `json:"processing_time"`
	Tags           []string        `json:"tags"`
}

// CepMatch mirrors event_streaming::models::sink::CepMatch.
type CepMatch struct {
	PatternName     string    `json:"pattern_name"`
	MatchedSequence []string  `json:"matched_sequence"`
	Confidence      float32   `json:"confidence"`
	DetectedAt      time.Time `json:"detected_at"`
}
