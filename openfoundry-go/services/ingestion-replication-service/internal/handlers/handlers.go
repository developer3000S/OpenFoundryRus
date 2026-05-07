// Package handlers wires the HTTP endpoints for ingestion-replication-service.
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

type Store interface {
	ListIngestJobs(ctx context.Context, namespace, status string) ([]models.IngestJob, error)
	GetIngestJob(ctx context.Context, id uuid.UUID) (*models.IngestJob, error)
	CreateIngestJob(ctx context.Context, body *models.CreateIngestJobRequest) (*models.IngestJob, error)
	UpdateIngestJob(ctx context.Context, id uuid.UUID, body *models.UpdateIngestJobRequest) (*models.IngestJob, error)
	DeleteIngestJob(ctx context.Context, id uuid.UUID) (bool, error)
	ListStreams(ctx context.Context, ownerID uuid.UUID, status string) ([]models.StreamDefinition, error)
	GetStream(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.StreamDefinition, error)
	CreateStream(ctx context.Context, body *models.CreateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error)
	UpdateStream(ctx context.Context, id uuid.UUID, body *models.UpdateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error)
	ListCdcStreams(ctx context.Context, ownerID uuid.UUID) ([]models.CdcStream, error)
	RegisterCdcStream(ctx context.Context, body *models.RegisterCdcStreamRequest, ownerID uuid.UUID) (*models.CdcStream, *models.IncrementalCheckpoint, *models.ResolutionState, error)
	GetCdcStream(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.CdcStream, error)
	GetCheckpoint(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.IncrementalCheckpoint, error)
	GetResolution(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.ResolutionState, error)
	ApplyCheckpoint(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.CheckpointUpdate) (*models.IncrementalCheckpoint, error)
	ApplyResolution(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.ResolutionUpdate) (*models.ResolutionState, error)
}

// StreamingRuntime hides Kafka/Flink provisioning and CDC registration.
type StreamingRuntime interface {
	ProvisionStream(ctx context.Context, stream *models.StreamDefinition) error
	UpdateStream(ctx context.Context, stream *models.StreamDefinition) error
	RegisterCDC(ctx context.Context, stream *models.CdcStream) (*CdcRegistrationResult, error)
}

type Handlers struct {
	Repo    Store
	Runtime StreamingRuntime
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handlers) ListIngestJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListIngestJobs(r.Context(),
		r.URL.Query().Get("namespace"), r.URL.Query().Get("status"))
	if err != nil {
		slog.Error("list ingest jobs", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list ingest jobs")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.IngestJob]{Items: items})
}

func (h *Handlers) GetIngestJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetIngestJob(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "ingest job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateIngestJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateIngestJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.Namespace == "" {
		writeJSONErr(w, http.StatusBadRequest, "name and namespace required")
		return
	}
	if len(body.Spec) == 0 || !json.Valid(body.Spec) {
		writeJSONErr(w, http.StatusBadRequest, "spec must be valid JSON")
		return
	}
	v, err := h.Repo.CreateIngestJob(r.Context(), &body)
	if err != nil {
		slog.Error("create ingest job", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateIngestJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateIngestJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateIngestJob(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "ingest job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) DeleteIngestJob(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.Repo.DeleteIngestJob(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "ingest job not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireClaims(w http.ResponseWriter, r *http.Request) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return claims, true
}

func (h *Handlers) ListStreams(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListStreams(r.Context(), claims.Sub, r.URL.Query().Get("status"))
	if err != nil {
		slog.Error("list streams", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list streams")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.StreamDefinition]{Items: items})
}

func (h *Handlers) GetStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetStream(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "stream not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	var body models.CreateStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "stream name is required")
		return
	}
	v, err := h.Repo.CreateStream(r.Context(), &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Runtime == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "streaming runtime not configured")
		return
	}
	if err := h.Runtime.ProvisionStream(r.Context(), v); err != nil {
		writeJSONErr(w, runtimeHTTPStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateStream(r.Context(), id, &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "stream not found")
		return
	}
	if h.Runtime == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "streaming runtime not configured")
		return
	}
	if err := h.Runtime.UpdateStream(r.Context(), v); err != nil {
		writeJSONErr(w, runtimeHTTPStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) ListCdcStreams(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListCdcStreams(r.Context(), claims.Sub)
	if err != nil {
		slog.Error("list cdc streams", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list cdc streams")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handlers) RegisterCdcStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	var body models.RegisterCdcStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	stream, checkpoint, resolution, err := h.Repo.RegisterCdcStream(r.Context(), &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Runtime == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "streaming runtime not configured")
		return
	}
	result, err := h.Runtime.RegisterCDC(r.Context(), stream)
	if err != nil {
		writeJSONErr(w, runtimeHTTPStatus(err), err.Error())
		return
	}
	if result != nil && result.Checkpoint != nil {
		checkpoint, err = h.Repo.ApplyCheckpoint(r.Context(), stream.ID, claims.Sub, result.Checkpoint)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if result != nil && result.Resolution != nil {
		resolution, err = h.Repo.ApplyResolution(r.Context(), stream.ID, claims.Sub, result.Resolution)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"stream": stream, "checkpoint": checkpoint, "resolution": resolution})
}

func (h *Handlers) GetCdcStream(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetCdcStream(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "cdc stream not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) GetCdcCheckpoint(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetCheckpoint(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "checkpoint not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) GetCdcResolution(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetResolution(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "resolution not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}
