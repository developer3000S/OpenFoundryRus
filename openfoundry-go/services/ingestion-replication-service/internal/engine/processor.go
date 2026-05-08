// Package engine ports the in-process topology DAG executor that lives at
// services/ingestion-replication-service/src/event_streaming/domain/engine/processor.rs.
//
// The runtime walks a DAG of source/window/join/CEP/sink nodes, emits
// live tail + aggregate windows + CEP matches, persists checkpoints, and
// pushes materialised rows to dataset sinks. Pure helpers (BuildJoinedEvents,
// BuildWindowAggregates, ...) live in internal/domain/engine and are
// reused here so the domain layer stays testable in isolation.
//
// The executor is intentionally batch-oriented like the Rust source: each
// RunTopology call ingests every event past the per-stream checkpoint,
// produces output, and advances the checkpoint atomically. Channels and
// goroutines are not used for the in-process path — Rust drives the same
// surface from a single async task — but the SinkUploader / RuntimeStore
// indirection lets external callers swap in concurrent / streaming
// implementations without touching the engine.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/domain"
	domeng "github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/domain/engine"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

// RuntimeStore is the slice of domain.RuntimeStore the engine actually
// touches. Defining it here lets callers inject lightweight fakes in
// tests without depending on the much wider domain.RuntimeStore surface.
type RuntimeStore interface {
	LoadTopologyOffsets(ctx context.Context, topologyID uuid.UUID) (map[uuid.UUID]domain.StreamCheckpointOffset, error)
	SaveTopologyOffsets(ctx context.Context, topologyID uuid.UUID, offsets map[uuid.UUID]int64) error
	ListEventsSince(ctx context.Context, streamID uuid.UUID, afterSequenceNo int64) ([]domain.RuntimeEvent, error)
	ListRecentEvents(ctx context.Context, streamID uuid.UUID, limit int) ([]domain.RuntimeEvent, error)
	MarkEventsProcessed(ctx context.Context, eventIDs []uuid.UUID) error
	RestoreEvents(ctx context.Context, streamIDs []uuid.UUID, fromSequenceNo *int64) (int64, error)
}

// SinkUploader pushes materialised rows to a dataset sink. Mirrors the
// Rust upload_dataset_rows helper that POSTs to the dataset service.
type SinkUploader interface {
	UploadDatasetRows(ctx context.Context, datasetID uuid.UUID, rows []json.RawMessage) error
}

// LineageWriter persists the source-stream → dataset edge written when a
// sink materialisation succeeds. Mirrors the streaming_lineage_edges
// INSERT in the Rust handler.
type LineageWriter interface {
	RecordLineageEdge(ctx context.Context, topologyID, sourceStreamID, datasetID uuid.UUID) error
}

// NoopSinkUploader skips materialisation. Used by the in-memory tests and
// by callers that want to run the engine in dry-run mode.
type NoopSinkUploader struct{}

// UploadDatasetRows is a no-op.
func (NoopSinkUploader) UploadDatasetRows(context.Context, uuid.UUID, []json.RawMessage) error {
	return nil
}

// NoopLineageWriter skips lineage persistence. Used by tests.
type NoopLineageWriter struct{}

// RecordLineageEdge is a no-op.
func (NoopLineageWriter) RecordLineageEdge(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

// Engine evaluates a topology DAG end-to-end. Mirrors the Rust
// event_streaming::domain::engine::processor module: it owns no state of
// its own, every call reads the runtime store, builds joined/aggregated/
// matched events from the persisted hot-tier rows, materialises sinks,
// and advances checkpoints.
type Engine struct {
	Runtime RuntimeStore
	Sink    SinkUploader
	Lineage LineageWriter
	// Now overrides time.Now for tests. Returned values are coerced to UTC.
	Now func() time.Time
	// LiveTailLimit caps how many trailing events are returned in the live
	// tail of each Run/Preview call. Defaults to 24 (matches the Rust
	// `.rev().take(24)`).
	LiveTailLimit int
	// RecentLimitPerStream caps the per-stream lookback used when computing
	// preview throughput. Defaults to 12 (matches the Rust constant).
	RecentLimitPerStream int
}

// New constructs an Engine. nil sink/lineage values are replaced with
// no-op implementations so callers don't need to wrap them.
func New(rt RuntimeStore, sink SinkUploader, lineage LineageWriter) *Engine {
	if sink == nil {
		sink = NoopSinkUploader{}
	}
	if lineage == nil {
		lineage = NoopLineageWriter{}
	}
	return &Engine{
		Runtime:              rt,
		Sink:                 sink,
		Lineage:              lineage,
		Now:                  func() time.Time { return time.Now().UTC() },
		LiveTailLimit:        24,
		RecentLimitPerStream: 12,
	}
}

// TopologyExecution mirrors event_streaming::domain::engine::processor::TopologyExecution.
type TopologyExecution struct {
	Metrics              domain.TopologyRunMetrics   `json:"metrics"`
	LiveTail             []domain.LiveTailEvent      `json:"live_tail"`
	CepMatches           []domain.CepMatch           `json:"cep_matches"`
	AggregateWindows     []domain.WindowAggregate    `json:"aggregate_windows"`
	StateSnapshot        domain.StateStoreSnapshot   `json:"state_snapshot"`
	BackpressureSnapshot domain.BackpressureSnapshot `json:"backpressure_snapshot"`
	StartedAt            time.Time                   `json:"started_at"`
	CompletedAt          time.Time                   `json:"completed_at"`
}

// TopologyRuntimeAnalysis mirrors event_streaming::domain::engine::processor::TopologyRuntimeAnalysis.
type TopologyRuntimeAnalysis struct {
	Preview                domain.TopologyRuntimePreview `json:"preview"`
	LatestEvents           []domain.LiveTailEvent        `json:"latest_events"`
	LatestMatches          []domain.CepMatch             `json:"latest_matches"`
	SourceBacklog          map[uuid.UUID]int32           `json:"source_backlog"`
	SourceThroughputPerSec map[uuid.UUID]float32         `json:"source_throughput_per_second"`
}

// RunTopology mirrors run_topology.
//
// Pulls events past the persisted checkpoints, joins/aggregates/matches
// them through the configured DAG, materialises into sinks, advances
// checkpoints, and returns the execution snapshot. Idempotent on an empty
// hot-tier (no events → no-op + zero metrics).
func (e *Engine) RunTopology(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	streams []domain.DomainStreamDefinition,
	windows []domain.WindowDefinition,
) (TopologyExecution, error) {
	if err := e.validateRuntime(topology); err != nil {
		return TopologyExecution{}, err
	}
	startedAt := e.now()
	checkpointMap, err := e.Runtime.LoadTopologyOffsets(ctx, topology.ID)
	if err != nil {
		return TopologyExecution{}, fmt.Errorf("load checkpoints: %w", err)
	}
	streamLookup := indexStreams(streams)
	sourceEvents, err := e.loadSourceEventsSinceCheckpoint(ctx, topology, streamLookup, checkpointMap, startedAt)
	if err != nil {
		return TopologyExecution{}, err
	}
	liveTail := buildLiveTail(topology.ID, sourceEvents, e.liveTailLimit())

	joinedEvents := domeng.BuildJoinedEvents(topology, sourceEvents)
	materialisation := sourceEvents
	if len(joinedEvents) > 0 {
		materialisation = joinedEvents
	}
	aggregates := domeng.BuildWindowAggregates(topology, windows, materialisation)
	cepMatches := domeng.DetectCepMatches(topology, materialisation)
	sourceBacklog := domeng.GroupEventCountByStream(sourceEvents)
	backpressure := domain.DeriveBackpressureSnapshot(
		topology.BackpressurePolicy,
		int32(len(sourceEvents)),
		maxStreamBacklog(sourceBacklog),
		len(topology.SourceStreamIDs),
	)

	if err := e.materialiseSinks(ctx, topology, materialisation, aggregates, cepMatches); err != nil {
		return TopologyExecution{}, err
	}
	if err := e.persistCheckpoints(ctx, topology.ID, sourceEvents); err != nil {
		return TopologyExecution{}, err
	}
	if err := e.markEventsProcessed(ctx, sourceEvents); err != nil {
		return TopologyExecution{}, err
	}

	completedAt := e.now()
	metrics := domeng.BuildRunMetrics(sourceEvents, materialisation, aggregates, cepMatches, completedAt)
	state := domeng.BuildStateSnapshot(
		topology,
		int32(len(aggregates))+int32(len(cepMatches))+metrics.InputEvents,
		int32(len(topology.SourceStreamIDs)),
		completedAt,
	)
	metrics.BackpressureRatio = domeng.BackpressureRatio(backpressure)
	metrics.StateEntries = state.KeyCount

	return TopologyExecution{
		Metrics:              metrics,
		LiveTail:             liveTail,
		CepMatches:           cepMatches,
		AggregateWindows:     aggregates,
		StateSnapshot:        state,
		BackpressureSnapshot: backpressure,
		StartedAt:            startedAt,
		CompletedAt:          completedAt,
	}, nil
}

// PreviewTopologyRuntime mirrors preview_topology_runtime.
//
// Like RunTopology but without the materialisation/checkpoint side-effects:
// returns what the next run *would* see, plus the recent-events tail used
// by the runtime page.
func (e *Engine) PreviewTopologyRuntime(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	streams []domain.DomainStreamDefinition,
	windows []domain.WindowDefinition,
) (TopologyRuntimeAnalysis, error) {
	if err := e.validateRuntime(topology); err != nil {
		return TopologyRuntimeAnalysis{}, err
	}
	generatedAt := e.now()
	checkpointMap, err := e.Runtime.LoadTopologyOffsets(ctx, topology.ID)
	if err != nil {
		return TopologyRuntimeAnalysis{}, fmt.Errorf("load checkpoints: %w", err)
	}
	streamLookup := indexStreams(streams)

	pendingEvents, err := e.loadSourceEventsSinceCheckpoint(ctx, topology, streamLookup, checkpointMap, generatedAt)
	if err != nil {
		return TopologyRuntimeAnalysis{}, err
	}
	recentEvents, err := e.loadRecentSourceEvents(ctx, topology, streamLookup, generatedAt, e.recentLimit())
	if err != nil {
		return TopologyRuntimeAnalysis{}, err
	}
	analysisEvents := pendingEvents
	if len(pendingEvents) == 0 {
		analysisEvents = recentEvents
	}
	joinedEvents := domeng.BuildJoinedEvents(topology, analysisEvents)
	materialisation := analysisEvents
	if len(joinedEvents) > 0 {
		materialisation = joinedEvents
	}
	aggregates := domeng.BuildWindowAggregates(topology, windows, materialisation)
	latestMatches := domeng.DetectCepMatches(topology, materialisation)
	latestEvents := buildLiveTail(topology.ID, recentEvents, e.liveTailLimit())
	sourceBacklog := domeng.GroupEventCountByStream(pendingEvents)
	sourceThroughput := domeng.GroupThroughputByStream(recentEvents)
	backpressure := domain.DeriveBackpressureSnapshot(
		topology.BackpressurePolicy,
		int32(len(pendingEvents)),
		maxStreamBacklog(sourceBacklog),
		len(topology.SourceStreamIDs),
	)
	metrics := domeng.BuildPreviewMetrics(pendingEvents, materialisation, aggregates, latestMatches, recentEvents, backpressure)
	lastCheckpointAt := generatedAt
	for _, cp := range checkpointMap {
		if cp.UpdatedAt.After(lastCheckpointAt) {
			lastCheckpointAt = cp.UpdatedAt
		}
	}
	state := domeng.BuildStateSnapshot(
		topology,
		int32(len(aggregates))+int32(len(latestMatches))+int32(len(pendingEvents)),
		int32(len(checkpointMap)),
		lastCheckpointAt,
	)
	metrics.StateEntries = state.KeyCount

	return TopologyRuntimeAnalysis{
		Preview: domain.TopologyRuntimePreview{
			Metrics:              metrics,
			AggregateWindows:     aggregates,
			BackpressureSnapshot: backpressure,
			StateSnapshot:        state,
			BacklogEvents:        int32(len(pendingEvents)),
			GeneratedAt:          generatedAt,
		},
		LatestEvents:           latestEvents,
		LatestMatches:          latestMatches,
		SourceBacklog:          sourceBacklog,
		SourceThroughputPerSec: sourceThroughput,
	}, nil
}

// ReplayTopology resets the persisted checkpoint for the supplied streams
// and asks the runtime store to restore previously processed/archived
// rows so they flow through the engine again. When streamIDs is empty the
// topology's full source-stream set is used.
func (e *Engine) ReplayTopology(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	streamIDs []uuid.UUID,
	fromSequenceNo *int64,
) (int64, error) {
	if err := e.validateRuntime(topology); err != nil {
		return 0, err
	}
	targets := streamIDs
	if len(targets) == 0 {
		targets = append([]uuid.UUID(nil), topology.SourceStreamIDs...)
	}
	if len(targets) == 0 {
		return 0, nil
	}
	restored, err := e.Runtime.RestoreEvents(ctx, targets, fromSequenceNo)
	if err != nil {
		return 0, fmt.Errorf("restore events: %w", err)
	}
	return restored, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (e *Engine) validateRuntime(topology *domain.TopologyDefinition) error {
	if e == nil || e.Runtime == nil {
		return errors.New("topology runtime store is not configured")
	}
	if topology == nil {
		return errors.New("topology definition is required")
	}
	return nil
}

func (e *Engine) now() time.Time {
	if e.Now == nil {
		return time.Now().UTC()
	}
	return e.Now().UTC()
}

func (e *Engine) liveTailLimit() int {
	if e.LiveTailLimit <= 0 {
		return 24
	}
	return e.LiveTailLimit
}

func (e *Engine) recentLimit() int {
	if e.RecentLimitPerStream <= 0 {
		return 12
	}
	return e.RecentLimitPerStream
}

func indexStreams(streams []domain.DomainStreamDefinition) map[uuid.UUID]*domain.DomainStreamDefinition {
	out := make(map[uuid.UUID]*domain.DomainStreamDefinition, len(streams))
	for i := range streams {
		s := &streams[i]
		out[s.ID] = s
	}
	return out
}

func (e *Engine) loadSourceEventsSinceCheckpoint(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	streamLookup map[uuid.UUID]*domain.DomainStreamDefinition,
	checkpointMap map[uuid.UUID]domain.StreamCheckpointOffset,
	processingTime time.Time,
) ([]domeng.ProcessedEvent, error) {
	var out []domeng.ProcessedEvent
	for _, streamID := range topology.SourceStreamIDs {
		stream, ok := streamLookup[streamID]
		if !ok {
			continue
		}
		var last int64
		if cp, ok := checkpointMap[streamID]; ok {
			last = cp.LastSequenceNo
		}
		rows, err := e.Runtime.ListEventsSince(ctx, streamID, last)
		if err != nil {
			return nil, fmt.Errorf("list events since: %w", err)
		}
		for _, row := range rows {
			out = append(out, domeng.ToProcessedEvent(stream, row, processingTime))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SequenceNo < out[j].SequenceNo })
	return out, nil
}

func (e *Engine) loadRecentSourceEvents(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	streamLookup map[uuid.UUID]*domain.DomainStreamDefinition,
	processingTime time.Time,
	limitPerStream int,
) ([]domeng.ProcessedEvent, error) {
	var out []domeng.ProcessedEvent
	for _, streamID := range topology.SourceStreamIDs {
		stream, ok := streamLookup[streamID]
		if !ok {
			continue
		}
		rows, err := e.Runtime.ListRecentEvents(ctx, streamID, limitPerStream)
		if err != nil {
			return nil, fmt.Errorf("list recent events: %w", err)
		}
		for _, row := range rows {
			out = append(out, domeng.ToProcessedEvent(stream, row, processingTime))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EventTime.Equal(out[j].EventTime) {
			return out[i].SequenceNo < out[j].SequenceNo
		}
		return out[i].EventTime.Before(out[j].EventTime)
	})
	return out, nil
}

// buildLiveTail mirrors the Rust `.iter().rev().take(limit).map(...)`
// chain — newest event first, capped at `limit`.
func buildLiveTail(topologyID uuid.UUID, events []domeng.ProcessedEvent, limit int) []domain.LiveTailEvent {
	if len(events) == 0 || limit <= 0 {
		return []domain.LiveTailEvent{}
	}
	if limit > len(events) {
		limit = len(events)
	}
	out := make([]domain.LiveTailEvent, 0, limit)
	for i := 0; i < limit; i++ {
		idx := len(events) - 1 - i
		out = append(out, domeng.ToLiveTailEvent(topologyID, events[idx]))
	}
	return out
}

func maxStreamBacklog(counts map[uuid.UUID]int32) int32 {
	var m int32
	for _, v := range counts {
		if v > m {
			m = v
		}
	}
	return m
}

func (e *Engine) materialiseSinks(
	ctx context.Context,
	topology *domain.TopologyDefinition,
	events []domeng.ProcessedEvent,
	aggregates []domain.WindowAggregate,
	cepMatches []domain.CepMatch,
) error {
	for _, sink := range topology.SinkBindings {
		if sink.ConnectorType != "dataset" {
			continue
		}
		datasetID, ok := DatasetIDFromBinding(sink)
		if !ok {
			continue
		}
		rows := domeng.MaterializationRows(events, aggregates, cepMatches)
		if err := e.Sink.UploadDatasetRows(ctx, datasetID, rows); err != nil {
			return fmt.Errorf("upload dataset rows: %w", err)
		}
		for _, streamID := range topology.SourceStreamIDs {
			if err := e.Lineage.RecordLineageEdge(ctx, topology.ID, streamID, datasetID); err != nil {
				return fmt.Errorf("record lineage edge: %w", err)
			}
		}
	}
	return nil
}

func (e *Engine) persistCheckpoints(ctx context.Context, topologyID uuid.UUID, events []domeng.ProcessedEvent) error {
	if len(events) == 0 {
		return nil
	}
	maxOffsets := map[uuid.UUID]int64{}
	for _, evt := range events {
		if cur, ok := maxOffsets[evt.StreamID]; !ok || evt.SequenceNo > cur {
			maxOffsets[evt.StreamID] = evt.SequenceNo
		}
	}
	if err := e.Runtime.SaveTopologyOffsets(ctx, topologyID, maxOffsets); err != nil {
		return fmt.Errorf("save topology offsets: %w", err)
	}
	return nil
}

func (e *Engine) markEventsProcessed(ctx context.Context, events []domeng.ProcessedEvent) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(events))
	for _, evt := range events {
		ids = append(ids, evt.ID)
	}
	if err := e.Runtime.MarkEventsProcessed(ctx, ids); err != nil {
		return fmt.Errorf("mark events processed: %w", err)
	}
	return nil
}

// DatasetIDFromBinding mirrors the Rust dataset_id_from_binding helper:
// look up `dataset_id` from the connector config map first, then fall
// back to parsing the `dataset://<uuid>` endpoint form.
func DatasetIDFromBinding(binding domain.ConnectorBinding) (uuid.UUID, bool) {
	if obj, ok := decodeJSONObject(binding.Config); ok {
		if raw, ok := obj["dataset_id"]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				if id, err := uuid.Parse(s); err == nil {
					return id, true
				}
			}
		}
	}
	if rest, ok := strings.CutPrefix(binding.Endpoint, "dataset://"); ok {
		if id, err := uuid.Parse(rest); err == nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

func decodeJSONObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	obj := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// ---------------------------------------------------------------------------
// models <-> domain adapters used by the HTTP handler. Kept here so the
// engine package owns the conversion contract — handlers stay thin.
// ---------------------------------------------------------------------------

// FromModelsTopology converts the wire-shape topology to the typed
// domain projection the engine operates on.
func FromModelsTopology(t models.TopologyDefinition) domain.TopologyDefinition {
	nodes := make([]domain.TopologyNode, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = domain.TopologyNode{
			ID:       n.ID,
			Label:    n.Label,
			NodeType: n.NodeType,
			StreamID: n.StreamID,
			WindowID: n.WindowID,
			Config:   n.Config,
		}
	}
	edges := make([]domain.TopologyEdge, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = domain.TopologyEdge{SourceNodeID: e.SourceNodeID, TargetNodeID: e.TargetNodeID, Label: e.Label}
	}
	sinks := make([]domain.ConnectorBinding, len(t.SinkBindings))
	for i, s := range t.SinkBindings {
		sinks[i] = domain.ConnectorBinding{
			ConnectorType: s.ConnectorType,
			Endpoint:      s.Endpoint,
			Format:        s.Format,
			Config:        s.Config,
		}
	}
	var join *domain.JoinDefinition
	if t.JoinDefinition != nil {
		cp := domain.JoinDefinition{
			JoinType:      t.JoinDefinition.JoinType,
			LeftStreamID:  t.JoinDefinition.LeftStreamID,
			RightStreamID: t.JoinDefinition.RightStreamID,
			TableName:     t.JoinDefinition.TableName,
			KeyFields:     append([]string(nil), t.JoinDefinition.KeyFields...),
			WindowSeconds: t.JoinDefinition.WindowSeconds,
		}
		join = &cp
	}
	var cep *domain.CepDefinition
	if t.CepDefinition != nil {
		cp := domain.CepDefinition{
			PatternName:   t.CepDefinition.PatternName,
			Sequence:      append([]string(nil), t.CepDefinition.Sequence...),
			WithinSeconds: t.CepDefinition.WithinSeconds,
			OutputStream:  t.CepDefinition.OutputStream,
		}
		cep = &cp
	}
	return domain.TopologyDefinition{
		ID:             t.ID,
		Name:           t.Name,
		Description:    t.Description,
		Status:         t.Status,
		Nodes:          nodes,
		Edges:          edges,
		JoinDefinition: join,
		CepDefinition:  cep,
		BackpressurePolicy: domain.BackpressurePolicy{
			MaxInFlight:      t.BackpressurePolicy.MaxInFlight,
			QueueCapacity:    t.BackpressurePolicy.QueueCapacity,
			ThrottleStrategy: t.BackpressurePolicy.ThrottleStrategy,
		},
		SourceStreamIDs:      append([]uuid.UUID(nil), t.SourceStreamIDs...),
		SinkBindings:         sinks,
		StateBackend:         t.StateBackend,
		CheckpointIntervalMS: t.CheckpointIntervalMS,
		RuntimeKind:          t.RuntimeKind,
		FlinkJobName:         t.FlinkJobName,
		FlinkDeploymentName:  t.FlinkDeploymentName,
		FlinkJobID:           t.FlinkJobID,
		FlinkNamespace:       t.FlinkNamespace,
		ConsistencyGuarantee: t.ConsistencyGuarantee,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}
}

// FromModelsStreams parses the JSON source_binding/schema blobs into
// typed projections so the engine can read connector_type without doing
// JSON work on every event.
func FromModelsStreams(streams []models.StreamDefinition) []domain.DomainStreamDefinition {
	out := make([]domain.DomainStreamDefinition, len(streams))
	for i, s := range streams {
		d := domain.DomainStreamDefinition{
			ID:                   s.ID,
			Name:                 s.Name,
			Description:          s.Description,
			Status:               s.Status,
			RetentionHours:       s.RetentionHours,
			Partitions:           s.Partitions,
			ConsistencyGuarantee: s.ConsistencyGuarantee,
			StreamType:           s.StreamType,
			Compression:          s.Compression,
			IngestConsistency:    s.IngestConsistency,
			PipelineConsistency:  s.PipelineConsistency,
			CheckpointIntervalMS: s.CheckpointIntervalMS,
			Kind:                 s.Kind,
			CreatedAt:            s.CreatedAt,
			UpdatedAt:            s.UpdatedAt,
		}
		if len(s.SourceBinding) > 0 {
			_ = json.Unmarshal(s.SourceBinding, &d.SourceBinding)
		}
		if len(s.Schema) > 0 {
			_ = json.Unmarshal(s.Schema, &d.Schema)
		}
		out[i] = d
	}
	return out
}

// FromModelsWindows projects the wire-shape window into the engine's
// full WindowDefinition. The lightweight wire form copies aggregation
// keys + measure fields when present so the engine can group properly.
func FromModelsWindows(windows []models.WindowDefinition) []domain.WindowDefinition {
	out := make([]domain.WindowDefinition, len(windows))
	for i, w := range windows {
		out[i] = domain.WindowDefinition{
			ID:                     w.ID,
			Name:                   w.Name,
			WindowType:             w.WindowType,
			DurationSeconds:        w.DurationSeconds,
			SlideSeconds:           w.SlideSeconds,
			SessionGapSeconds:      w.SessionGapSeconds,
			AllowedLatenessSeconds: w.AllowedLatenessSeconds,
			AggregationKeys:        append([]string(nil), w.AggregationKeys...),
			MeasureFields:          append([]string(nil), w.MeasureFields...),
		}
	}
	return out
}

// ToModelsTopologyRun packs an execution snapshot into the wire-shape
// TopologyRun row written to streaming_topology_runs.
func ToModelsTopologyRun(topologyID uuid.UUID, exec TopologyExecution, now func() time.Time) (models.TopologyRun, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	stamp := now().UTC()
	metrics, err := json.Marshal(exec.Metrics)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal metrics: %w", err)
	}
	aggregates, err := json.Marshal(exec.AggregateWindows)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal aggregate_windows: %w", err)
	}
	live, err := json.Marshal(exec.LiveTail)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal live_tail: %w", err)
	}
	cep, err := json.Marshal(exec.CepMatches)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal cep_matches: %w", err)
	}
	state, err := json.Marshal(exec.StateSnapshot)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal state_snapshot: %w", err)
	}
	bp, err := json.Marshal(exec.BackpressureSnapshot)
	if err != nil {
		return models.TopologyRun{}, fmt.Errorf("marshal backpressure_snapshot: %w", err)
	}
	completed := exec.CompletedAt
	return models.TopologyRun{
		ID:                   uuid.New(),
		TopologyID:           topologyID,
		Status:               "completed",
		Metrics:              metrics,
		AggregateWindows:     aggregates,
		LiveTail:             live,
		CepMatches:           cep,
		StateSnapshot:        state,
		BackpressureSnapshot: bp,
		StartedAt:            exec.StartedAt,
		CompletedAt:          &completed,
		CreatedAt:            stamp,
		UpdatedAt:            stamp,
	}, nil
}
