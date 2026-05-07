// Package handlers wires the HTTP endpoints for connector-management-service.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/repo"
)

type Store interface {
	ListConnections(ctx context.Context, ownerID *uuid.UUID) ([]models.Connection, error)
	GetConnection(ctx context.Context, id uuid.UUID) (*models.Connection, error)
	GetConnectionForOwner(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Connection, error)
	CreateConnection(ctx context.Context, body *models.CreateConnectionRequest, ownerID uuid.UUID) (*models.Connection, error)
	UpdateConnection(ctx context.Context, id uuid.UUID, body *models.UpdateConnectionRequest) (*models.Connection, error)
	DeleteConnection(ctx context.Context, id uuid.UUID) (bool, error)
	ListSyncJobs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SyncJob, error)
	GetSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncJob, error)
	CreateSyncJob(ctx context.Context, body *models.CreateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error)
	UpdateSyncJob(ctx context.Context, id uuid.UUID, body *models.UpdateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error)
	RunSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncRun, error)
	ListMediaSetSyncs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.MediaSetSync, error)
	GetMediaSetSync(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.MediaSetSync, error)
	CreateMediaSetSync(ctx context.Context, sourceID uuid.UUID, body *models.CreateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error)
	UpdateMediaSetSync(ctx context.Context, id uuid.UUID, body *models.UpdateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error)
	EnableVirtualTableSource(ctx context.Context, sourceRID string, body *models.EnableVirtualTableSourceRequest) (*models.VirtualTableSourceLink, error)
	CreateVirtualTable(ctx context.Context, sourceRID string, actorID string, body *models.CreateVirtualTableRequest) (*models.VirtualTable, error)
	ListVirtualTables(ctx context.Context, ownerID string, project, source string, limit int) ([]models.VirtualTable, error)
	GetVirtualTable(ctx context.Context, rid string, ownerID string) (*models.VirtualTable, error)
}

type Handlers struct {
	Repo            Store
	MediaSetRuntime MediaSetRuntime
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var ownerID *uuid.UUID
	if raw := r.URL.Query().Get("owner_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid owner_id")
			return
		}
		ownerID = &id
	}
	items, err := h.Repo.ListConnections(r.Context(), ownerID)
	if err != nil {
		slog.Error("list connections", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list connections")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Connection]{Items: items})
}

func (h *Handlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.ConnectorType == "" {
		writeJSONErr(w, http.StatusBadRequest, "name and connector_type required")
		return
	}
	if len(body.Config) > 0 && !json.Valid(body.Config) {
		writeJSONErr(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}
	v, err := h.Repo.CreateConnection(r.Context(), &body, caller.Sub)
	if err != nil {
		slog.Error("create connection", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Config) > 0 && !json.Valid(body.Config) {
		writeJSONErr(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}
	v, err := h.Repo.UpdateConnection(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.Repo.DeleteConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
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

func (h *Handlers) ListSyncJobs(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.Repo.ListSyncJobs(r.Context(), sourceID, claims.Sub)
	if err != nil {
		slog.Error("list sync jobs", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list sync jobs")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) GetSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	v, err := h.Repo.GetSyncJob(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	var body models.CreateSyncJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.SourceID == uuid.Nil || body.OutputDatasetID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "source_id and output_dataset_id required")
		return
	}
	v, err := h.Repo.CreateSyncJob(r.Context(), &body, claims.Sub)
	if err != nil {
		slog.Error("create sync job", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create sync job")
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	var body models.UpdateSyncJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateSyncJob(r.Context(), id, &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) RunSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	v, err := h.Repo.RunSyncJob(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	writeJSON(w, http.StatusAccepted, v)
}

func (h *Handlers) EnableVirtualTableSource(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	sourceRID := strings.TrimSpace(chi.URLParam(r, "source_rid"))
	if sourceRID == "" {
		writeJSONErr(w, http.StatusBadRequest, "source_rid required")
		return
	}
	var body models.EnableVirtualTableSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.EnableVirtualTableSource(r.Context(), sourceRID, &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateVirtualTable(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceRID := strings.TrimSpace(chi.URLParam(r, "source_rid"))
	if sourceRID == "" {
		writeJSONErr(w, http.StatusBadRequest, "source_rid required")
		return
	}
	var body models.CreateVirtualTableRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.ProjectRID) == "" || strings.TrimSpace(body.TableType) == "" {
		writeJSONErr(w, http.StatusBadRequest, "project_rid and table_type required")
		return
	}
	v, err := h.Repo.CreateVirtualTable(r.Context(), sourceRID, claims.Sub.String(), &body)
	if errors.Is(err, repo.ErrConflict) {
		writeJSONErr(w, http.StatusConflict, "virtual table already registered")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not enabled")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) ListVirtualTables(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.Repo.ListVirtualTables(r.Context(), claims.Sub.String(), r.URL.Query().Get("project"), r.URL.Query().Get("source"), limit)
	if err != nil {
		slog.Error("list virtual tables", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list virtual tables")
		return
	}
	writeJSON(w, http.StatusOK, models.ListVirtualTablesResponse{Items: items})
}

func (h *Handlers) GetVirtualTable(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	rid := strings.TrimSpace(chi.URLParam(r, "rid"))
	if rid == "" {
		writeJSONErr(w, http.StatusBadRequest, "rid required")
		return
	}
	v, err := h.Repo.GetVirtualTable(r.Context(), rid, claims.Sub.String())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "virtual table not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) ListMediaSetSyncs(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.Repo.ListMediaSetSyncs(r.Context(), sourceID, claims.Sub)
	if err != nil {
		slog.Error("list media set syncs", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list media set syncs")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) GetMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	v, err := h.Repo.GetMediaSetSync(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.CreateMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if errs := body.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": errs})
		return
	}
	v, err := h.Repo.CreateMediaSetSync(r.Context(), sourceID, &body, claims.Sub)
	if err != nil {
		slog.Error("create media set sync", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create media set sync")
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	var body models.UpdateMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Kind != nil && !body.Kind.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": {"kind must be MEDIA_SET_SYNC or VIRTUAL_MEDIA_SET_SYNC"}})
		return
	}
	if body.TargetMediaSetRID != nil && !strings.HasPrefix(strings.TrimSpace(*body.TargetMediaSetRID), "ri.foundry.main.media_set.") {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": {"target_media_set_rid must start with ri.foundry.main.media_set."}})
		return
	}
	v, err := h.Repo.UpdateMediaSetSync(r.Context(), id, &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) RunMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "sync_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync_id")
		return
	}
	var body models.RunMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	sync, err := h.Repo.GetMediaSetSync(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sync == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	if h.MediaSetRuntime == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "media set runtime not configured")
		return
	}
	report, err := h.MediaSetRuntime.ExecuteMediaSetSync(r.Context(), sync, &body, r.Header.Get("Authorization"))
	if err != nil {
		writeJSONErr(w, runtimeHTTPStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, report)
}
