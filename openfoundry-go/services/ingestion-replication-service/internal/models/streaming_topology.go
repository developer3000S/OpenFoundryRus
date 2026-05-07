package models

// Wire-shape DTOs for the streaming topology surface (DAG runtime). The
// engine in internal/engine consumes the typed projections in
// internal/domain; this package mirrors the byte-exact JSON shapes that
// cross the HTTP boundary. Mirrors event_streaming::models::topology in
// the Rust source.

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TopologyNode mirrors event_streaming::models::topology::TopologyNode.
type TopologyNode struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	NodeType string          `json:"node_type"`
	StreamID *uuid.UUID      `json:"stream_id,omitempty"`
	WindowID *uuid.UUID      `json:"window_id,omitempty"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// TopologyEdge mirrors event_streaming::models::topology::TopologyEdge.
type TopologyEdge struct {
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	Label        string `json:"label"`
}

// JoinDefinition mirrors event_streaming::models::topology::JoinDefinition.
type JoinDefinition struct {
	JoinType      string    `json:"join_type"`
	LeftStreamID  uuid.UUID `json:"left_stream_id"`
	RightStreamID uuid.UUID `json:"right_stream_id"`
	TableName     string    `json:"table_name"`
	KeyFields     []string  `json:"key_fields"`
	WindowSeconds int32     `json:"window_seconds"`
}

// CepDefinition mirrors event_streaming::models::topology::CepDefinition.
type CepDefinition struct {
	PatternName   string   `json:"pattern_name"`
	Sequence      []string `json:"sequence"`
	WithinSeconds int32    `json:"within_seconds"`
	OutputStream  string   `json:"output_stream"`
}

// BackpressurePolicy mirrors the Rust default 512/2048/credit-based.
type BackpressurePolicy struct {
	MaxInFlight      int32  `json:"max_in_flight"`
	QueueCapacity    int32  `json:"queue_capacity"`
	ThrottleStrategy string `json:"throttle_strategy"`
}

// DefaultBackpressurePolicy mirrors the Rust Default impl.
func DefaultBackpressurePolicy() BackpressurePolicy {
	return BackpressurePolicy{
		MaxInFlight:      512,
		QueueCapacity:    2048,
		ThrottleStrategy: "credit-based",
	}
}

// ConnectorBinding mirrors the Rust ConnectorBinding (sink/source binding).
type ConnectorBinding struct {
	ConnectorType string          `json:"connector_type"`
	Endpoint      string          `json:"endpoint"`
	Format        string          `json:"format"`
	Config        json.RawMessage `json:"config"`
}

// TopologyDefinition is the Foundry-parity persisted topology.
type TopologyDefinition struct {
	ID                   uuid.UUID          `json:"id"`
	Name                 string             `json:"name"`
	Description          string             `json:"description"`
	Status               string             `json:"status"`
	Nodes                []TopologyNode     `json:"nodes"`
	Edges                []TopologyEdge     `json:"edges"`
	JoinDefinition       *JoinDefinition    `json:"join_definition,omitempty"`
	CepDefinition        *CepDefinition     `json:"cep_definition,omitempty"`
	BackpressurePolicy   BackpressurePolicy `json:"backpressure_policy"`
	SourceStreamIDs      []uuid.UUID        `json:"source_stream_ids"`
	SinkBindings         []ConnectorBinding `json:"sink_bindings"`
	StateBackend         string             `json:"state_backend"`
	CheckpointIntervalMS int32              `json:"checkpoint_interval_ms"`
	RuntimeKind          string             `json:"runtime_kind"`
	FlinkJobName         *string            `json:"flink_job_name,omitempty"`
	FlinkDeploymentName  *string            `json:"flink_deployment_name,omitempty"`
	FlinkJobID           *string            `json:"flink_job_id,omitempty"`
	FlinkNamespace       *string            `json:"flink_namespace,omitempty"`
	ConsistencyGuarantee string             `json:"consistency_guarantee"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// WindowDefinition is the lightweight projection of streaming_windows that
// the engine adapter consumes. The engine itself works on the rich
// internal/domain.WindowDefinition which adds aggregation/measure metadata.
type WindowDefinition struct {
	ID                     uuid.UUID `json:"id"`
	Name                   string    `json:"name"`
	WindowType             string    `json:"window_type"`
	DurationSeconds        int32     `json:"duration_seconds"`
	SlideSeconds           int32     `json:"slide_seconds"`
	SessionGapSeconds      int32     `json:"session_gap_seconds"`
	AllowedLatenessSeconds int32     `json:"allowed_lateness_seconds"`
	AggregationKeys        []string  `json:"aggregation_keys,omitempty"`
	MeasureFields          []string  `json:"measure_fields,omitempty"`
}

// TopologyRun is the persisted run record. Metric/snapshot blobs are
// stored as JSON RawMessage so we can survive future engine fields
// without changing the column shape.
type TopologyRun struct {
	ID                   uuid.UUID       `json:"id"`
	TopologyID           uuid.UUID       `json:"topology_id"`
	Status               string          `json:"status"`
	Metrics              json.RawMessage `json:"metrics"`
	AggregateWindows     json.RawMessage `json:"aggregate_windows"`
	LiveTail             json.RawMessage `json:"live_tail"`
	CepMatches           json.RawMessage `json:"cep_matches"`
	StateSnapshot        json.RawMessage `json:"state_snapshot"`
	BackpressureSnapshot json.RawMessage `json:"backpressure_snapshot"`
	StartedAt            time.Time       `json:"started_at"`
	CompletedAt          *time.Time      `json:"completed_at"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// ReplayTopologyRequest mirrors event_streaming::models::topology::ReplayTopologyRequest.
type ReplayTopologyRequest struct {
	StreamIDs        []uuid.UUID `json:"stream_ids,omitempty"`
	FromSequenceNo   *int64      `json:"from_sequence_no,omitempty"`
}

// ReplayTopologyResponse mirrors event_streaming::models::topology::ReplayTopologyResponse.
type ReplayTopologyResponse struct {
	TopologyID           uuid.UUID   `json:"topology_id"`
	StreamIDs            []uuid.UUID `json:"stream_ids"`
	ReplayFromSequenceNo *int64      `json:"replay_from_sequence_no,omitempty"`
	RestoredEventCount   int64       `json:"restored_event_count"`
}
