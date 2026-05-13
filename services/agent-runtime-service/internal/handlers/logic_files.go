package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/models"
)

func logicClaims(w http.ResponseWriter, r *http.Request) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok || claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return claims, true
}

func validateExecutionMode(mode *string) bool {
	if mode == nil {
		return true
	}
	switch *mode {
	case "user_scoped", "project_scoped":
		return true
	default:
		return false
	}
}

func validateLogicName(name string) bool {
	return strings.TrimSpace(name) != ""
}

func isZeroUUID(id uuid.UUID) bool { return id == uuid.Nil }

func parseOptionalUUIDQuery(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseLogicFileID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handlers) CreateLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	var body models.CreateLogicFileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validateLogicName(body.Name) {
		writeError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	if isZeroUUID(body.ProjectID) || isZeroUUID(body.FolderID) {
		writeError(w, http.StatusBadRequest, "project_id and folder_id are required for Logic files")
		return
	}
	if !validateExecutionMode(body.ExecutionMode) {
		writeError(w, http.StatusBadRequest, "execution_mode must be user_scoped or project_scoped")
		return
	}
	lf, err := h.Repo.CreateLogicFile(r.Context(), claims.Sub, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, lf)
}

func (h *Handlers) GetLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	lf, err := h.Repo.GetLogicFile(r.Context(), id, claims.Sub, includeArchived, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusOK, lf)
}

func (h *Handlers) ListLogicFiles(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	projectID, err := parseOptionalUUIDQuery(r.URL.Query().Get("project_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "project_id must be a uuid")
		return
	}
	folderID, err := parseOptionalUUIDQuery(r.URL.Query().Get("folder_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "folder_id must be a uuid")
		return
	}
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	files, err := h.Repo.ListLogicFiles(r.Context(), projectID, folderID, claims.Sub, includeArchived, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *Handlers) UpdateLogicFileMetadata(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	var body models.UpdateLogicFileMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name != nil && !validateLogicName(*body.Name) {
		writeError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	if !validateExecutionMode(body.ExecutionMode) {
		writeError(w, http.StatusBadRequest, "execution_mode must be user_scoped or project_scoped")
		return
	}
	lf, err := h.Repo.UpdateLogicFileMetadata(r.Context(), id, claims.Sub, body, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusOK, lf)
}

func (h *Handlers) MoveLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	var body models.MoveLogicFileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if isZeroUUID(body.ProjectID) || isZeroUUID(body.FolderID) {
		writeError(w, http.StatusBadRequest, "project_id and folder_id are required for Logic files")
		return
	}
	lf, err := h.Repo.MoveLogicFile(r.Context(), id, claims.Sub, body, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusOK, lf)
}

func (h *Handlers) DuplicateLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	var body models.DuplicateLogicFileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name != nil && !validateLogicName(*body.Name) {
		writeError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	if (body.ProjectID == nil) != (body.FolderID == nil) {
		writeError(w, http.StatusBadRequest, "project_id and folder_id must be provided together")
		return
	}
	if body.ProjectID != nil && (isZeroUUID(*body.ProjectID) || isZeroUUID(*body.FolderID)) {
		writeError(w, http.StatusBadRequest, "project_id and folder_id are required for Logic files")
		return
	}
	lf, err := h.Repo.DuplicateLogicFile(r.Context(), id, claims.Sub, body, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusCreated, lf)
}

func (h *Handlers) ArchiveLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	lf, err := h.Repo.ArchiveLogicFile(r.Context(), id, claims.Sub, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusOK, lf)
}

func (h *Handlers) RestoreLogicFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := logicClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseLogicFileID(w, r)
	if !ok {
		return
	}
	lf, err := h.Repo.RestoreLogicFile(r.Context(), id, claims.Sub, claims.HasRole("admin"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if lf == nil {
		writeError(w, http.StatusNotFound, "logic file not found")
		return
	}
	writeJSON(w, http.StatusOK, lf)
}
