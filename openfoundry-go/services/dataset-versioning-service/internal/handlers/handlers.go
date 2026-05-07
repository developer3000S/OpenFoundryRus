// Package handlers wires the HTTP endpoints for dataset-versioning-service.
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

type Store interface {
	ListDatasets(ctx context.Context, ownerID *uuid.UUID, limit int) ([]models.Dataset, error)
	GetDataset(ctx context.Context, id uuid.UUID) (*models.Dataset, error)
	GetDatasetForOwner(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Dataset, error)
	CreateDataset(ctx context.Context, body *models.CreateDatasetRequest, ownerID uuid.UUID) (*models.Dataset, error)
	UpdateDataset(ctx context.Context, id uuid.UUID, body *models.UpdateDatasetRequest) (*models.Dataset, error)
	DeleteDataset(ctx context.Context, id uuid.UUID) (bool, error)
	ListVersions(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetVersion, error)
	GetVersion(ctx context.Context, datasetID uuid.UUID, version int32) (*models.DatasetVersion, error)
	CreateVersion(ctx context.Context, datasetID uuid.UUID, body *models.CreateDatasetVersionRequest) (*models.DatasetVersion, error)
	EnsureDefaultBranch(ctx context.Context, dataset *models.Dataset) error
	ListBranches(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetBranch, error)
	GetBranch(ctx context.Context, datasetID uuid.UUID, name string) (*models.DatasetBranch, error)
	CreateBranch(ctx context.Context, dataset *models.Dataset, body *models.CreateDatasetBranchRequest) (*models.DatasetBranch, error)
	ListFiles(ctx context.Context, datasetID uuid.UUID, branch string, prefix string) ([]models.DatasetFile, error)
	GetFile(ctx context.Context, datasetID uuid.UUID, fileID uuid.UUID) (*models.DatasetFile, error)
	GetTransactionStatus(ctx context.Context, datasetID uuid.UUID, transactionID uuid.UUID) (string, bool, error)
}

type Handlers struct {
	Repo       Store
	BackingFS  storageabstraction.BackingFS
	PresignTTL time.Duration
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handlers) ListDatasets(w http.ResponseWriter, r *http.Request) {
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
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.Repo.ListDatasets(r.Context(), ownerID, limit)
	if err != nil {
		slog.Error("list datasets", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list datasets")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Dataset]{Items: items})
}

func (h *Handlers) GetDataset(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetDataset(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateDataset(w http.ResponseWriter, r *http.Request) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.StoragePath == "" {
		writeJSONErr(w, http.StatusBadRequest, "name and storage_path required")
		return
	}
	v, err := h.Repo.CreateDataset(r.Context(), &body, caller.Sub)
	if err != nil {
		slog.Error("create dataset", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateDataset(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateDataset(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) DeleteDataset(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.Repo.DeleteDataset(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parsePage(r *http.Request) (offset int, limit int) {
	limit = 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		if decoded, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
			if text := string(decoded); strings.HasPrefix(text, "of:") {
				if n, err := strconv.Atoi(strings.TrimPrefix(text, "of:")); err == nil && n > 0 {
					offset = n
				}
			}
		}
	}
	return offset, limit
}

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("of:" + strconv.Itoa(offset)))
}

func (h *Handlers) ownedDataset(w http.ResponseWriter, r *http.Request) (*authmw.Claims, *models.Dataset, bool) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return nil, nil, false
	}
	dataset, err := h.Repo.GetDatasetForOwner(r.Context(), id, caller.Sub)
	if err != nil {
		slog.Error("load dataset", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset")
		return nil, nil, false
	}
	if dataset == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return nil, nil, false
	}
	return caller, dataset, true
}

func (h *Handlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	versions, err := h.Repo.ListVersions(r.Context(), dataset.ID)
	if err != nil {
		slog.Error("list versions", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	offset, limit := parsePage(r)
	if offset > len(versions) {
		offset = len(versions)
	}
	end := offset + limit
	if end > len(versions) {
		end = len(versions)
	}
	hasMore := end < len(versions)
	var next *string
	if hasMore {
		v := encodeCursor(offset + limit)
		next = &v
	}
	writeJSON(w, http.StatusOK, models.Page[models.DatasetVersion]{Data: versions[offset:end], NextCursor: next, HasMore: hasMore})
}

func (h *Handlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n < 1 {
		writeJSONErr(w, http.StatusBadRequest, "invalid version")
		return
	}
	v, err := h.Repo.GetVersion(r.Context(), dataset.ID, int32(n))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "version not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateVersion(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	var body models.CreateDatasetVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.StoragePath) == "" {
		writeJSONErr(w, http.StatusBadRequest, "storage_path required")
		return
	}
	v, err := h.Repo.CreateVersion(r.Context(), dataset.ID, &body)
	if repo.IsConflict(err) {
		writeJSONErr(w, http.StatusConflict, "version already exists")
		return
	}
	if err != nil {
		slog.Error("create version", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create version")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) ListBranches(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	if err := h.Repo.EnsureDefaultBranch(r.Context(), dataset); err != nil {
		slog.Error("ensure default branch", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to ensure default branch")
		return
	}
	branches, err := h.Repo.ListBranches(r.Context(), dataset.ID)
	if err != nil {
		slog.Error("list branches", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list branches")
		return
	}
	writeJSON(w, http.StatusOK, branches)
}

func (h *Handlers) GetBranch(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	name := chi.URLParam(r, "branch")
	v, err := h.Repo.GetBranch(r.Context(), dataset.ID, name)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "branch not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateBranch(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	var body models.CreateDatasetBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeJSONErr(w, http.StatusBadRequest, "branch name is required")
		return
	}
	if err := h.Repo.EnsureDefaultBranch(r.Context(), dataset); err != nil {
		slog.Error("ensure default branch", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to ensure default branch")
		return
	}
	v, err := h.Repo.CreateBranch(r.Context(), dataset, &body)
	if repo.IsConflict(err) {
		writeJSONErr(w, http.StatusConflict, "branch already exists")
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "source version does not exist") {
			status = http.StatusBadRequest
		}
		writeJSONErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) ListFiles(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = "main"
	}
	prefix := strings.TrimLeft(strings.TrimSpace(r.URL.Query().Get("prefix")), "/")
	files, err := h.Repo.ListFiles(r.Context(), dataset.ID, branch, prefix)
	if err != nil {
		slog.Error("list files", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list files")
		return
	}
	writeJSON(w, http.StatusOK, models.ListDatasetFilesResponse{Branch: branch, Total: len(files), Files: files})
}

func (h *Handlers) DownloadFile(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	fileID, err := uuid.Parse(chi.URLParam(r, "file_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid file_id")
		return
	}
	file, err := h.Repo.GetFile(r.Context(), dataset.ID, fileID)
	if err != nil {
		slog.Error("get file", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to load file")
		return
	}
	if file == nil {
		writeJSONErr(w, http.StatusNotFound, "file not found")
		return
	}
	if file.DeletedAt != nil {
		writeJSONErr(w, http.StatusGone, "file is soft-deleted")
		return
	}
	if h.BackingFS == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "backing filesystem not configured")
		return
	}
	ttl := h.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	signed, err := h.BackingFS.PresignedURL(storageabstraction.ParsePhysicalURI(file.PhysicalURI), ttl)
	if err != nil {
		slog.Error("presign file", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to presign file")
		return
	}
	writeJSON(w, http.StatusOK, models.DownloadDatasetFileResponse{URL: signed.URL, ExpiresAt: signed.ExpiresAt, Method: signed.Method})
}

func (h *Handlers) CreateFileUploadURL(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	txnID, err := uuid.Parse(chi.URLParam(r, "txn"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid transaction id")
		return
	}
	var body models.CreateDatasetFileUploadURLRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	logical := strings.TrimLeft(strings.TrimSpace(body.LogicalPath), "/")
	if logical == "" {
		writeJSONErr(w, http.StatusBadRequest, "logical_path required")
		return
	}
	status, found, err := h.Repo.GetTransactionStatus(r.Context(), dataset.ID, txnID)
	if err != nil {
		slog.Error("get transaction status", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to load transaction")
		return
	}
	if !found {
		writeJSONErr(w, http.StatusNotFound, "transaction not found")
		return
	}
	if !strings.EqualFold(status, "OPEN") {
		writeJSONErr(w, http.StatusConflict, "transaction is not OPEN")
		return
	}
	if h.BackingFS == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "backing filesystem not configured")
		return
	}
	ttl := h.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	physical := storageabstraction.PhysicalLocation{
		FSID:          h.BackingFS.FSID(),
		BaseDirectory: h.BackingFS.BaseDirectory(),
		RelativePath:  "transactions/" + txnID.String() + "/" + logical,
	}
	signed, err := h.BackingFS.PresignedURL(physical, ttl)
	if err != nil {
		slog.Error("presign upload", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to presign upload")
		return
	}
	method := signed.Method
	if method == "" || method == "GET" {
		method = "PUT"
	}
	writeJSON(w, http.StatusOK, models.CreateDatasetFileUploadURLResponse{URL: signed.URL, PhysicalURI: physical.URI(), ExpiresAt: signed.ExpiresAt, Method: method})
}
