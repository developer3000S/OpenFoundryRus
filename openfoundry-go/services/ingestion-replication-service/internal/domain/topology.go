package domain

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

// BackpressurePolicy mirrors event_streaming::models::topology::BackpressurePolicy.
type BackpressurePolicy struct {
	MaxInFlight      int32  `json:"max_in_flight"`
	QueueCapacity    int32  `json:"queue_capacity"`
	ThrottleStrategy string `json:"throttle_strategy"`
}

// DefaultBackpressurePolicy mirrors the Default impl on the Rust struct.
func DefaultBackpressurePolicy() BackpressurePolicy {
	return BackpressurePolicy{
		MaxInFlight:      512,
		QueueCapacity:    2048,
		ThrottleStrategy: "credit-based",
	}
}

// TopologyDefinition mirrors event_streaming::models::topology::TopologyDefinition.
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

// TopologyRunMetrics mirrors event_streaming::models::topology::TopologyRunMetrics.
type TopologyRunMetrics struct {
	InputEvents         int32   `json:"input_events"`
	OutputEvents        int32   `json:"output_events"`
	AvgLatencyMS        int32   `json:"avg_latency_ms"`
	P95LatencyMS        int32   `json:"p95_latency_ms"`
	ThroughputPerSecond float32 `json:"throughput_per_second"`
	DroppedEvents       int32   `json:"dropped_events"`
	BackpressureRatio   float32 `json:"backpressure_ratio"`
	JoinOutputRows      int32   `json:"join_output_rows"`
	CepMatchCount       int32   `json:"cep_match_count"`
	StateEntries        int32   `json:"state_entries"`
}

// TopologyRun mirrors event_streaming::models::topology::TopologyRun.
type TopologyRun struct {
	ID                   uuid.UUID            `json:"id"`
	TopologyID           uuid.UUID            `json:"topology_id"`
	Status               string               `json:"status"`
	Metrics              TopologyRunMetrics   `json:"metrics"`
	AggregateWindows     []WindowAggregate    `json:"aggregate_windows"`
	LiveTail             []LiveTailEvent      `json:"live_tail"`
	CepMatches           []CepMatch           `json:"cep_matches"`
	StateSnapshot        StateStoreSnapshot   `json:"state_snapshot"`
	BackpressureSnapshot BackpressureSnapshot `json:"backpressure_snapshot"`
	StartedAt            time.Time            `json:"started_at"`
	CompletedAt          *time.Time           `json:"completed_at,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
}

// TopologyRuntimePreview mirrors event_streaming::models::topology::TopologyRuntimePreview.
type TopologyRuntimePreview struct {
	Metrics              TopologyRunMetrics   `json:"metrics"`
	AggregateWindows     []WindowAggregate    `json:"aggregate_windows"`
	BackpressureSnapshot BackpressureSnapshot `json:"backpressure_snapshot"`
	StateSnapshot        StateStoreSnapshot   `json:"state_snapshot"`
	BacklogEvents        int32                `json:"backlog_events"`
	GeneratedAt          time.Time            `json:"generated_at"`
}

// TopologyRuntimeSnapshot mirrors event_streaming::models::topology::TopologyRuntimeSnapshot.
type TopologyRuntimeSnapshot struct {
	Topology          TopologyDefinition      `json:"topology"`
	LatestRun         *TopologyRun            `json:"latest_run,omitempty"`
	Preview           *TopologyRuntimePreview `json:"preview,omitempty"`
	ConnectorStatuses []ConnectorCatalogEntry `json:"connector_statuses"`
	LatestEvents      []LiveTailEvent         `json:"latest_events"`
	LatestMatches     []CepMatch              `json:"latest_matches"`
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
