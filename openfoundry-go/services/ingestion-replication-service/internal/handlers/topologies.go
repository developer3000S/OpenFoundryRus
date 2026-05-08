package handlers

// IRF-1 — DAG topology runtime endpoints. Mirrors the Rust handler at
// services/ingestion-replication-service/src/event_streaming/handlers/topologies.rs
// for the run/replay paths that drive the in-process engine.
//
//   POST   /api/v1/streaming/topologies/{id}:run
//   POST   /api/v1/streaming/topologies/{id}:replay
//
// run/replay route into the engine in internal/engine. Production callers
// can inject Engine directly, or provide a Store that implements the
// engine.RuntimeStore plus optional sink/lineage interfaces so the handler
// can construct the in-process engine on demand. When neither path is
// available, handlers return a stable configuration error instead of
// panicking.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/domain"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/engine"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

// Stable error codes mirrored from the Rust contract.
const (
	ErrTopologyNotFound        = "STREAMING_TOPOLOGY_NOT_FOUND"
	ErrTopologyRuntimeNotWired = "STREAMING_TOPOLOGY_RUNTIME_NOT_WIRED"
)

// ErrTopologyMissing is returned by stores when the requested topology id
// has no row. Handlers map it to 404.
var ErrTopologyMissing = errors.New("topology not found")

// TopologyStore is the slice of repo methods the IRF-1 handler needs.
// Kept narrow so test fakes don't have to implement the full streaming
// CRUD surface that lands in sibling slices.
type TopologyStore interface {
	GetTopology(ctx context.Context, id uuid.UUID) (*models.TopologyDefinition, error)
	AllStreams(ctx context.Context) ([]models.StreamDefinition, error)
	AllWindows(ctx context.Context) ([]models.WindowDefinition, error)
}

// TopologyRunRecorder is the optional persistence sink for engine runs.
// Wired separately from TopologyStore so callers that only want to
// invoke the engine in-process (CLI, tests) don't have to implement it.
type TopologyRunRecorder interface {
	InsertTopologyRun(ctx context.Context, run models.TopologyRun) error
}

// TopologyEngine is the runtime surface the HTTP handlers need. The
// concrete in-process engine.Engine implements it, while tests can inject
// small fakes without constructing a runtime store.
type TopologyEngine interface {
	RunTopology(ctx context.Context, topology *domain.TopologyDefinition, streams []domain.DomainStreamDefinition, windows []domain.WindowDefinition) (engine.TopologyExecution, error)
	ReplayTopology(ctx context.Context, topology *domain.TopologyDefinition, streamIDs []uuid.UUID, fromSequenceNo *int64) (int64, error)
}

// TopologiesHandler bundles the dependencies for the topology runtime
// routes. Engine is optional when Store implements engine.RuntimeStore;
// in that case a productive in-process engine is constructed per request.
// If no engine can be resolved, run/replay return a stable configuration
// error.
type TopologiesHandler struct {
	Store       TopologyStore
	Engine      TopologyEngine
	RunRecorder TopologyRunRecorder
}

// RunTopology mirrors the Rust POST /:id:run.
//
// Loads the topology + window/stream metadata, hands them to the engine,
// records the resulting run via the optional recorder, and returns the
// run record. If the engine cannot be resolved, the route returns a
// stable STREAMING_TOPOLOGY_RUNTIME_NOT_WIRED configuration error.
func (h *TopologiesHandler) RunTopology(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid topology id")
		return
	}
	t, err := h.Store.GetTopology(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if t == nil {
		writeJSONErr(w, http.StatusNotFound, ErrTopologyNotFound)
		return
	}
	runtimeEngine := h.runtimeEngine()
	if runtimeEngine == nil {
		writeTopologyRuntimeNotWired(w)
		return
	}
	streams, err := h.Store.AllStreams(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	windows, err := h.Store.AllWindows(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	domTopo := engine.FromModelsTopology(*t)
	domStreams := engine.FromModelsStreams(streams)
	domWindows := engine.FromModelsWindows(windows)

	exec, err := runtimeEngine.RunTopology(r.Context(), &domTopo, domStreams, domWindows)
	if err != nil {
		slog.Error("run topology", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "topology execution failed: "+err.Error())
		return
	}
	run, err := engine.ToModelsTopologyRun(id, exec, time.Now)
	if err != nil {
		slog.Error("encode topology run", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "encode topology run failed")
		return
	}
	if h.RunRecorder != nil {
		if err := h.RunRecorder.InsertTopologyRun(r.Context(), run); err != nil {
			slog.Error("insert topology run", slog.String("error", err.Error()))
			writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
			return
		}
	}
	writeJSON(w, http.StatusOK, run)
}

// ReplayTopology mirrors the Rust POST /:id:replay. Same gating as
// RunTopology. The body is optional — an empty body replays the
// topology's full source-stream set from the beginning.
func (h *TopologiesHandler) ReplayTopology(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid topology id")
		return
	}
	t, err := h.Store.GetTopology(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if t == nil {
		writeJSONErr(w, http.StatusNotFound, ErrTopologyNotFound)
		return
	}
	runtimeEngine := h.runtimeEngine()
	if runtimeEngine == nil {
		writeTopologyRuntimeNotWired(w)
		return
	}
	var body models.ReplayTopologyRequest
	if r.ContentLength != 0 {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && !errors.Is(err, errEmptyBody) {
			writeJSONErr(w, http.StatusBadRequest, "invalid body")
			return
		}
	}
	domTopo := engine.FromModelsTopology(*t)
	restored, err := runtimeEngine.ReplayTopology(r.Context(), &domTopo, body.StreamIDs, body.FromSequenceNo)
	if err != nil {
		slog.Error("replay topology", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "topology replay failed: "+err.Error())
		return
	}
	targets := body.StreamIDs
	if len(targets) == 0 {
		targets = append([]uuid.UUID(nil), t.SourceStreamIDs...)
	}
	writeJSON(w, http.StatusOK, models.ReplayTopologyResponse{
		TopologyID:           id,
		StreamIDs:            targets,
		ReplayFromSequenceNo: body.FromSequenceNo,
		RestoredEventCount:   restored,
	})
}

// errEmptyBody sentinel matched against decoder.Decode errors to allow
// empty replay bodies. Currently unused (decoder returns io.EOF for
// empty bodies which is masked by ContentLength==0), but kept here so a
// future refactor doesn't have to revisit the matching pattern.
var errEmptyBody = errors.New("empty body")

// runtimeEngine returns the explicitly configured engine, or builds the
// productive in-process engine from Store when Store also exposes the
// engine runtime-store contract. Sink upload and lineage persistence are
// optional; engine.New supplies no-op implementations when they are absent.
func (h *TopologiesHandler) runtimeEngine() TopologyEngine {
	if h.Engine != nil {
		return h.Engine
	}
	rt, ok := h.Store.(engine.RuntimeStore)
	if !ok || rt == nil {
		return nil
	}
	var sink engine.SinkUploader
	if s, ok := h.Store.(engine.SinkUploader); ok {
		sink = s
	}
	var lineage engine.LineageWriter
	if l, ok := h.Store.(engine.LineageWriter); ok {
		lineage = l
	}
	return engine.New(rt, sink, lineage)
}

func writeTopologyRuntimeNotWired(w http.ResponseWriter) {
	writeJSONErr(w, http.StatusInternalServerError,
		ErrTopologyRuntimeNotWired+": topology runtime engine is not configured")
}
