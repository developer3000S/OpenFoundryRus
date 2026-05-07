package handlers

// IRF-5 — Foundry "Reset stream" workflow. Mirrors the Rust handler at
// services/ingestion-replication-service/src/event_streaming/handlers/stream_views.rs.
//
//   POST /api/v1/streaming/streams/{id}:reset
//
// Rotates the stream's view RID so push consumers must re-fetch the
// POST URL, retires the previous active view, mints a fresh one with
// generation+1, and best-effort truncates the underlying Kafka topic
// + resets consumer offsets via the streaming runtime.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/repo"
)

// Stable error codes mirrored from Rust ERR_RESET_REQUIRES_INGEST and
// ERR_RESET_DOWNSTREAM_ACTIVE.
const (
	ErrResetRequiresIngest   = "STREAM_RESET_ONLY_INGEST_KIND"
	ErrResetDownstreamActive = "STREAM_RESET_DOWNSTREAM_PIPELINES_ACTIVE"
)

const permStreamWrite = "streaming:write"

var rolesAllowedToWriteStreams = []string{"admin", "streaming_admin", "data_engineer"}

// PushURLBuilder renders the externally-reachable URL push consumers
// POST against after a reset rotates the view RID. Set on the Handlers
// struct at server-construction time. When nil, the reset response
// carries an empty push_url and clients fall back to their cached value.
type PushURLBuilder struct {
	BaseURL string
}

// Build returns the Foundry-style /streams-push/{view_rid}/records URL.
// Mirrors render_push_url in Rust.
func (b *PushURLBuilder) Build(viewRID string) string {
	if b == nil {
		return ""
	}
	trimmed := strings.TrimRight(b.BaseURL, "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/streams-push/" + viewRID + "/records"
}

// ResetStream is POST /api/v1/streaming/streams/{id}:reset.
func (h *Handlers) ResetStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	if !canWriteStreams(claims) {
		writeJSONErr(w, http.StatusForbidden, "caller lacks 'streaming:write' permission")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid stream id")
		return
	}
	stream, err := h.Repo.GetStream(r.Context(), id, claims.Sub)
	if err != nil {
		slog.Error("reset stream: load", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	if stream == nil {
		writeJSONErr(w, http.StatusNotFound, "stream not found")
		return
	}
	if stream.Kind != models.StreamKindIngest {
		writeJSONErr(w, http.StatusUnprocessableEntity,
			ErrResetRequiresIngest+": resets are only available for ingest streams")
		return
	}

	var body models.ResetStreamRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid body")
			return
		}
	}

	if !body.Force {
		active, err := h.Repo.DownstreamPipelinesActive(r.Context(), id)
		if err != nil {
			slog.Error("reset stream: downstream check", slog.String("error", err.Error()))
			writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		if active {
			writeJSONErr(w, http.StatusConflict,
				ErrResetDownstreamActive+": downstream pipelines are still active; pass force=true after acknowledging the replay requirement")
			return
		}
	}

	result, err := h.Repo.ResetStream(r.Context(), id, claims.Sub, claims.Sub.String(), &body)
	if err != nil {
		if errors.Is(err, repo.ErrStreamNotFound) {
			writeJSONErr(w, http.StatusNotFound, "stream not found")
			return
		}
		slog.Error("reset stream: persist", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "database operation failed")
		return
	}

	// Best-effort hot buffer rotation. Failures are logged but the
	// metadata mutation already committed — operators retry via the
	// runtime's own backoff path. Mirrors the Rust hot_buffer warn-
	// and-continue branch.
	if h.Runtime != nil {
		if err := h.Runtime.ResetStream(r.Context(), &result.Stream); err != nil {
			slog.Warn("reset stream: hot buffer rotation",
				slog.String("stream_id", id.String()),
				slog.String("error", err.Error()))
		}
	}

	previousViewRID := result.NewView.ViewRID
	if result.PreviousView != nil {
		previousViewRID = result.PreviousView.ViewRID
	}
	pushURL := ""
	if h.PushURL != nil {
		pushURL = h.PushURL.Build(result.NewView.ViewRID)
	}

	slog.Info("audit stream.reset",
		slog.String("actor_sub", claims.Sub.String()),
		slog.String("actor_email", claims.Email),
		slog.String("resource_id", id.String()),
		slog.String("stream_rid", result.NewView.StreamRID),
		slog.String("old_view_rid", previousViewRID),
		slog.String("new_view_rid", result.NewView.ViewRID),
		slog.Int("generation", int(result.NewView.Generation)),
		slog.Bool("schema_changed", result.SchemaChanged),
		slog.Bool("config_changed", result.ConfigChanged),
		slog.Bool("forced", body.Force))

	writeJSON(w, http.StatusOK, models.ResetStreamResponse{
		StreamRID:  result.NewView.StreamRID,
		OldViewRID: previousViewRID,
		NewViewRID: result.NewView.ViewRID,
		Generation: result.NewView.Generation,
		View:       result.NewView,
		PushURL:    pushURL,
		Forced:     body.Force,
	})
}

func canWriteStreams(c *authmw.Claims) bool {
	if c.HasAnyRole(rolesAllowedToWriteStreams) {
		return true
	}
	return c.HasPermissionKey(permStreamWrite)
}
