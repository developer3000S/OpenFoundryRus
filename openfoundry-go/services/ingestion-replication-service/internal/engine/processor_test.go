package engine_test

// IRF-1 — runtime engine end-to-end tests.
//
// processor.rs has no #[cfg(test)] block (it's pure composition over
// helpers tested in their own modules). The slice instructions add a
// 3-node DAG correctness test on top of helper-level checks.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/domain"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/engine"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

// recordingSink captures the rows the engine pushes so the test can
// verify the materialisation contract without an HTTP fake.
type recordingSink struct {
	mu       sync.Mutex
	uploads  []sinkUpload
	uploaded int
}

type sinkUpload struct {
	datasetID uuid.UUID
	rows      []json.RawMessage
}

func (s *recordingSink) UploadDatasetRows(_ context.Context, datasetID uuid.UUID, rows []json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads = append(s.uploads, sinkUpload{datasetID: datasetID, rows: append([]json.RawMessage(nil), rows...)})
	s.uploaded += len(rows)
	return nil
}

type recordingLineage struct {
	mu    sync.Mutex
	edges []lineageEdge
}

type lineageEdge struct {
	topologyID, sourceStreamID, datasetID uuid.UUID
}

func (l *recordingLineage) RecordLineageEdge(_ context.Context, t, s, d uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.edges = append(l.edges, lineageEdge{topologyID: t, sourceStreamID: s, datasetID: d})
	return nil
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

// 3-node DAG correctness test.
//
// Topology: source(orders) -> window(per-second count) -> dataset sink.
// Feeds 3 events at the same event-time and asserts:
//
//   - the engine reads them through the runtime store
//   - the window aggregates roll up to one bucket
//   - the dataset sink receives the materialised aggregate row
//   - the topology offset advances to the highest sequence number
//   - the events are stamped processed_at
//   - metrics report 3 inputs / 1 output / cep_match=0
func TestEngineRunsThreeNodeDAG(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := domain.NewMemoryRuntimeStore()
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	store.SetClock(func() time.Time { return now })
	sink := &recordingSink{}
	lineage := &recordingLineage{}

	streamID := uuid.New()
	windowID := uuid.New()
	topologyID := uuid.New()
	datasetID := uuid.New()

	// Append 3 events at the same event-time so they fall in one
	// per-second bucket.
	eventTime := now.Add(-2 * time.Second)
	for i := 0; i < 3; i++ {
		_, err := store.AppendEvent(ctx, streamID, mustJSON(t, map[string]any{"value": float64(i + 1)}), eventTime)
		require.NoError(t, err)
	}

	stream := domain.DomainStreamDefinition{
		ID:   streamID,
		Name: "Orders",
		SourceBinding: domain.ConnectorBinding{
			ConnectorType: "kafka",
			Endpoint:      "kafka://orders",
		},
	}
	window := domain.WindowDefinition{
		ID:              windowID,
		Name:            "per_second",
		WindowType:      "tumbling",
		DurationSeconds: 1,
		// No aggregation_keys/measure_fields → engine emits the
		// synthetic events_per_window measure (1.0 per event).
	}
	topology := domain.TopologyDefinition{
		ID:                 topologyID,
		Name:               "demo",
		Status:             "active",
		BackpressurePolicy: domain.DefaultBackpressurePolicy(),
		StateBackend:       "rocksdb",
		SourceStreamIDs:    []uuid.UUID{streamID},
		Nodes: []domain.TopologyNode{
			{ID: "src", Label: "Source", NodeType: "source", StreamID: &streamID},
			{ID: "win", Label: "Per-second", NodeType: "window", WindowID: &windowID},
			{ID: "snk", Label: "Sink", NodeType: "sink"},
		},
		Edges: []domain.TopologyEdge{
			{SourceNodeID: "src", TargetNodeID: "win", Label: "ingest"},
			{SourceNodeID: "win", TargetNodeID: "snk", Label: "emit"},
		},
		SinkBindings: []domain.ConnectorBinding{
			{
				ConnectorType: "dataset",
				Endpoint:      "dataset://" + datasetID.String(),
				Format:        "json",
			},
		},
	}

	eng := engine.New(store, sink, lineage)
	eng.Now = func() time.Time { return now }

	exec, err := eng.RunTopology(ctx, &topology, []domain.DomainStreamDefinition{stream}, []domain.WindowDefinition{window})
	require.NoError(t, err)

	// 3 inputs → one bucket → one materialised row.
	assert.EqualValues(t, 3, exec.Metrics.InputEvents)
	assert.EqualValues(t, 1, exec.Metrics.OutputEvents)
	assert.EqualValues(t, 0, exec.Metrics.CepMatchCount)
	assert.EqualValues(t, 0, exec.Metrics.JoinOutputRows)
	require.Len(t, exec.AggregateWindows, 1)
	assert.EqualValues(t, 3.0, exec.AggregateWindows[0].Value)
	assert.Equal(t, "events_per_window", exec.AggregateWindows[0].MeasureName)
	assert.Equal(t, "all", exec.AggregateWindows[0].GroupKey)

	// Live tail mirrors the source events newest-first.
	require.Len(t, exec.LiveTail, 3)
	assert.Equal(t, "stream:"+streamID.String(), exec.LiveTail[0].Tags[0])
	assert.Equal(t, "orders-3", exec.LiveTail[0].ID)

	// State snapshot reports keys ≥ inputs and uses the topology backend.
	assert.Equal(t, "rocksdb", exec.StateSnapshot.Backend)
	assert.GreaterOrEqual(t, exec.StateSnapshot.KeyCount, int32(3))
	assert.Equal(t, "demo", exec.StateSnapshot.Namespace)

	// Sink upload happened once with one materialised row.
	require.Len(t, sink.uploads, 1)
	assert.Equal(t, datasetID, sink.uploads[0].datasetID)
	require.Len(t, sink.uploads[0].rows, 1)
	require.Len(t, lineage.edges, 1)
	assert.Equal(t, topologyID, lineage.edges[0].topologyID)
	assert.Equal(t, streamID, lineage.edges[0].sourceStreamID)
	assert.Equal(t, datasetID, lineage.edges[0].datasetID)

	// Offset advanced to the latest sequence number.
	offsets, err := store.LoadTopologyOffsets(ctx, topologyID)
	require.NoError(t, err)
	require.Contains(t, offsets, streamID)
	assert.EqualValues(t, 3, offsets[streamID].LastSequenceNo)

	// All 3 events should be marked processed_at.
	rows, err := store.ListEventsSince(ctx, streamID, 0)
	require.NoError(t, err)
	for _, row := range rows {
		assert.NotNil(t, row.ProcessedAt, "event %d not stamped processed_at", row.SequenceNo)
	}

	// Second run with no new events: 0 inputs, 0 aggregates. The Rust
	// source still calls the dataset upload unconditionally per sink —
	// 1:1 port preserves that — but the upload carries an empty rows
	// slice so the receiving sink isn't tricked into re-materialising.
	exec2, err := eng.RunTopology(ctx, &topology, []domain.DomainStreamDefinition{stream}, []domain.WindowDefinition{window})
	require.NoError(t, err)
	assert.EqualValues(t, 0, exec2.Metrics.InputEvents)
	assert.Empty(t, exec2.AggregateWindows)
	require.Len(t, sink.uploads, 2)
	assert.Empty(t, sink.uploads[1].rows, "second run must not push real rows")
}

func TestEngineRunTopologyEmptyHotTier(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := domain.NewMemoryRuntimeStore()
	streamID := uuid.New()
	stream := domain.DomainStreamDefinition{ID: streamID, Name: "S"}
	topology := domain.TopologyDefinition{
		ID:                 uuid.New(),
		Name:               "noop",
		BackpressurePolicy: domain.DefaultBackpressurePolicy(),
		StateBackend:       "rocksdb",
		SourceStreamIDs:    []uuid.UUID{streamID},
		Nodes: []domain.TopologyNode{
			{ID: "src", NodeType: "source", StreamID: &streamID},
		},
	}
	eng := engine.New(store, nil, nil)
	exec, err := eng.RunTopology(ctx, &topology, []domain.DomainStreamDefinition{stream}, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, exec.Metrics.InputEvents)
	assert.EqualValues(t, 0, exec.Metrics.OutputEvents)
	assert.Empty(t, exec.LiveTail)
}

func TestEngineReplayRestoresProcessedEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := domain.NewMemoryRuntimeStore()
	streamID := uuid.New()
	for i := 0; i < 4; i++ {
		_, err := store.AppendEvent(ctx, streamID, json.RawMessage(`{}`), time.Now().Add(-time.Duration(i)*time.Second))
		require.NoError(t, err)
	}

	// Mark all of them processed so RestoreEvents has work to do.
	rows, err := store.ListEventsSince(ctx, streamID, 0)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	require.NoError(t, store.MarkEventsProcessed(ctx, ids))

	topology := domain.TopologyDefinition{ID: uuid.New(), Name: "x", SourceStreamIDs: []uuid.UUID{streamID}}

	eng := engine.New(store, nil, nil)
	from := int64(2)
	restored, err := eng.ReplayTopology(ctx, &topology, nil, &from)
	require.NoError(t, err)
	assert.EqualValues(t, 3, restored, "expected sequence_no >= 2 (2,3,4)")
}

func TestDatasetIDFromBinding(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	t.Run("from config", func(t *testing.T) {
		t.Parallel()
		binding := domain.ConnectorBinding{
			Config: json.RawMessage(`{"dataset_id":"` + id.String() + `"}`),
		}
		got, ok := engine.DatasetIDFromBinding(binding)
		require.True(t, ok)
		assert.Equal(t, id, got)
	})
	t.Run("from endpoint", func(t *testing.T) {
		t.Parallel()
		binding := domain.ConnectorBinding{
			Endpoint: "dataset://" + id.String(),
		}
		got, ok := engine.DatasetIDFromBinding(binding)
		require.True(t, ok)
		assert.Equal(t, id, got)
	})
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		_, ok := engine.DatasetIDFromBinding(domain.ConnectorBinding{Endpoint: "kafka://x"})
		assert.False(t, ok)
	})
	t.Run("config beats endpoint", func(t *testing.T) {
		t.Parallel()
		other := uuid.New()
		binding := domain.ConnectorBinding{
			Config:   json.RawMessage(`{"dataset_id":"` + id.String() + `"}`),
			Endpoint: "dataset://" + other.String(),
		}
		got, ok := engine.DatasetIDFromBinding(binding)
		require.True(t, ok)
		assert.Equal(t, id, got)
	})
}

// ---------------------------------------------------------------------------
// Handler integration: 501 when engine is nil + 200 when engine is wired.
// ---------------------------------------------------------------------------

type fakeTopologyStore struct {
	topologies map[uuid.UUID]models.TopologyDefinition
	streams    []models.StreamDefinition
	windows    []models.WindowDefinition
}

func (f *fakeTopologyStore) GetTopology(_ context.Context, id uuid.UUID) (*models.TopologyDefinition, error) {
	t, ok := f.topologies[id]
	if !ok {
		return nil, nil
	}
	cp := t
	return &cp, nil
}
func (f *fakeTopologyStore) AllStreams(context.Context) ([]models.StreamDefinition, error) {
	return f.streams, nil
}
func (f *fakeTopologyStore) AllWindows(context.Context) ([]models.WindowDefinition, error) {
	return f.windows, nil
}

type runtimeTopologyStore struct {
	*fakeTopologyStore
	*domain.MemoryRuntimeStore
}

type recordingRecorder struct {
	mu   sync.Mutex
	runs []models.TopologyRun
}

func (r *recordingRecorder) InsertTopologyRun(_ context.Context, run models.TopologyRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, run)
	return nil
}

type fakeHandlerEngine struct {
	runExec        engine.TopologyExecution
	replayRestored int64
	runCalls       int
	replayCalls    int
}

func (f *fakeHandlerEngine) RunTopology(_ context.Context, _ *domain.TopologyDefinition, _ []domain.DomainStreamDefinition, _ []domain.WindowDefinition) (engine.TopologyExecution, error) {
	f.runCalls++
	return f.runExec, nil
}

func (f *fakeHandlerEngine) ReplayTopology(_ context.Context, _ *domain.TopologyDefinition, _ []uuid.UUID, _ *int64) (int64, error) {
	f.replayCalls++
	return f.replayRestored, nil
}

func writerClaims() *authmw.Claims {
	return &authmw.Claims{Sub: uuid.New(), Roles: []string{"streaming_admin"}}
}

func authedRequest(method, target, body string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, http.NoBody)
	}
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(authmw.ContextWithClaims(req.Context(), writerClaims()))
}

func withParams(req *http.Request, kv ...string) *http.Request {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestRunTopologyReturnsStableErrorWhenEngineNil verifies the handler
// returns a stable configuration error when neither an explicit engine
// nor a runtime-store-backed productive engine is available.
func TestRunTopologyReturnsStableErrorWhenEngineNil(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x"},
	}}
	h := &handlers.TopologiesHandler{Store: store}
	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/run", ""), "id", tid.String())
	rec := httptest.NewRecorder()
	h.RunTopology(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), handlers.ErrTopologyRuntimeNotWired)
}

func TestReplayTopologyReturnsStableErrorWhenEngineNil(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	streamID := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x", SourceStreamIDs: []uuid.UUID{streamID}},
	}}
	h := &handlers.TopologiesHandler{Store: store}
	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/replay", `{}`), "id", tid.String())
	rec := httptest.NewRecorder()
	h.ReplayTopology(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), handlers.ErrTopologyRuntimeNotWired)
}

func TestRunTopologySucceedsWithFakeEngine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tid := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x", BackpressurePolicy: models.DefaultBackpressurePolicy()},
	}}
	fake := &fakeHandlerEngine{runExec: engine.TopologyExecution{
		Metrics:              domain.TopologyRunMetrics{InputEvents: 7, OutputEvents: 3},
		StateSnapshot:        domain.StateStoreSnapshot{Backend: "rocksdb", Namespace: "x"},
		BackpressureSnapshot: domain.BackpressureSnapshot{Status: "healthy"},
		StartedAt:            now,
		CompletedAt:          now.Add(time.Second),
	}}
	h := &handlers.TopologiesHandler{Store: store, Engine: fake}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/run", ""), "id", tid.String())
	rec := httptest.NewRecorder()
	h.RunTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, fake.runCalls)

	var run models.TopologyRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &run))
	assert.Equal(t, tid, run.TopologyID)
	assert.Equal(t, "completed", run.Status)
	var metrics domain.TopologyRunMetrics
	require.NoError(t, json.Unmarshal(run.Metrics, &metrics))
	assert.EqualValues(t, 7, metrics.InputEvents)
}

func TestReplayTopologySucceedsWithFakeEngine(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	streamID := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x", SourceStreamIDs: []uuid.UUID{streamID}},
	}}
	fake := &fakeHandlerEngine{replayRestored: 5}
	h := &handlers.TopologiesHandler{Store: store, Engine: fake}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/replay", `{}`), "id", tid.String())
	rec := httptest.NewRecorder()
	h.ReplayTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, fake.replayCalls)

	var resp models.ReplayTopologyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.EqualValues(t, 5, resp.RestoredEventCount)
	assert.Equal(t, []uuid.UUID{streamID}, resp.StreamIDs)
}

func TestRunTopologyPersistsRunWhenRecorderConfigured(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tid := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x", BackpressurePolicy: models.DefaultBackpressurePolicy()},
	}}
	recorder := &recordingRecorder{}
	h := &handlers.TopologiesHandler{
		Store:       store,
		Engine:      &fakeHandlerEngine{runExec: engine.TopologyExecution{StartedAt: now, CompletedAt: now.Add(time.Second)}},
		RunRecorder: recorder,
	}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/run", ""), "id", tid.String())
	rec := httptest.NewRecorder()
	h.RunTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, recorder.runs, 1)
	assert.Equal(t, tid, recorder.runs[0].TopologyID)
	assert.Equal(t, "completed", recorder.runs[0].Status)
	require.NotNil(t, recorder.runs[0].CompletedAt)
	assert.Equal(t, now.Add(time.Second), *recorder.runs[0].CompletedAt)
}

// TestRunTopologyExecutesEngine wires the engine end-to-end through the
// HTTP handler and asserts the response carries the run + metrics that
// the engine produced.
func TestRunTopologyExecutesEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt := domain.NewMemoryRuntimeStore()
	streamID := uuid.New()
	_, err := rt.AppendEvent(ctx, streamID, json.RawMessage(`{"value":1}`), time.Now().Add(-time.Second))
	require.NoError(t, err)
	_, err = rt.AppendEvent(ctx, streamID, json.RawMessage(`{"value":2}`), time.Now().Add(-time.Second))
	require.NoError(t, err)

	tid := uuid.New()
	dataset := uuid.New()
	store := &fakeTopologyStore{
		topologies: map[uuid.UUID]models.TopologyDefinition{
			tid: {
				ID:                 tid,
				Name:               "demo",
				Status:             "active",
				BackpressurePolicy: models.DefaultBackpressurePolicy(),
				StateBackend:       "rocksdb",
				SourceStreamIDs:    []uuid.UUID{streamID},
				Nodes: []models.TopologyNode{
					{ID: "src", NodeType: "source", StreamID: &streamID},
					{ID: "snk", NodeType: "sink"},
				},
				SinkBindings: []models.ConnectorBinding{
					{ConnectorType: "dataset", Endpoint: "dataset://" + dataset.String(), Format: "json"},
				},
			},
		},
		streams: []models.StreamDefinition{{ID: streamID, Name: "orders"}},
	}
	recorder := &recordingRecorder{}
	sink := &recordingSink{}
	h := &handlers.TopologiesHandler{
		Store:       store,
		Engine:      engine.New(rt, sink, engine.NoopLineageWriter{}),
		RunRecorder: recorder,
	}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/run", ""), "id", tid.String())
	rec := httptest.NewRecorder()
	h.RunTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var run models.TopologyRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &run))
	assert.Equal(t, tid, run.TopologyID)
	assert.Equal(t, "completed", run.Status)
	require.NotNil(t, run.CompletedAt)

	// Metrics blob round-trips and reports 2 inputs.
	var metrics domain.TopologyRunMetrics
	require.NoError(t, json.Unmarshal(run.Metrics, &metrics))
	assert.EqualValues(t, 2, metrics.InputEvents)
	assert.EqualValues(t, 2, metrics.OutputEvents)

	require.Len(t, recorder.runs, 1)
	require.Len(t, sink.uploads, 1)
	assert.Equal(t, dataset, sink.uploads[0].datasetID)
}

func TestRunTopologyBuildsProductiveEngineFromRuntimeStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt := domain.NewMemoryRuntimeStore()
	streamID := uuid.New()
	_, err := rt.AppendEvent(ctx, streamID, json.RawMessage(`{"value":1}`), time.Now().Add(-time.Second))
	require.NoError(t, err)

	tid := uuid.New()
	store := &runtimeTopologyStore{
		fakeTopologyStore: &fakeTopologyStore{
			topologies: map[uuid.UUID]models.TopologyDefinition{
				tid: {
					ID:                 tid,
					Name:               "demo",
					Status:             "active",
					BackpressurePolicy: models.DefaultBackpressurePolicy(),
					SourceStreamIDs:    []uuid.UUID{streamID},
				},
			},
			streams: []models.StreamDefinition{{ID: streamID, Name: "orders"}},
		},
		MemoryRuntimeStore: rt,
	}
	h := &handlers.TopologiesHandler{Store: store}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/run", ""), "id", tid.String())
	rec := httptest.NewRecorder()
	h.RunTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var run models.TopologyRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &run))
	var metrics domain.TopologyRunMetrics
	require.NoError(t, json.Unmarshal(run.Metrics, &metrics))
	assert.EqualValues(t, 1, metrics.InputEvents)
}

// TestReplayTopologyExecutesEngine walks the replay path end-to-end:
// pre-populates two events, marks them processed, then issues replay
// to verify both come back as restored rows.
func TestReplayTopologyExecutesEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt := domain.NewMemoryRuntimeStore()
	streamID := uuid.New()
	for i := 0; i < 2; i++ {
		_, err := rt.AppendEvent(ctx, streamID, json.RawMessage(`{}`), time.Now())
		require.NoError(t, err)
	}
	rows, err := rt.ListEventsSince(ctx, streamID, 0)
	require.NoError(t, err)
	ids := []uuid.UUID{rows[0].ID, rows[1].ID}
	require.NoError(t, rt.MarkEventsProcessed(ctx, ids))

	tid := uuid.New()
	store := &fakeTopologyStore{topologies: map[uuid.UUID]models.TopologyDefinition{
		tid: {ID: tid, Name: "x", SourceStreamIDs: []uuid.UUID{streamID}},
	}}
	h := &handlers.TopologiesHandler{Store: store, Engine: engine.New(rt, nil, nil)}

	req := withParams(authedRequest("POST", "/topologies/"+tid.String()+"/replay", `{}`), "id", tid.String())
	rec := httptest.NewRecorder()
	h.ReplayTopology(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp models.ReplayTopologyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.EqualValues(t, 2, resp.RestoredEventCount)
	assert.Equal(t, []uuid.UUID{streamID}, resp.StreamIDs)
}
