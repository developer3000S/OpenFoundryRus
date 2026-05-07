package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) resolveDatasetForCatalog(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	id, err := h.Repo.ResolveDatasetID(r.Context(), datasetIDParam(r))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
		} else {
			writeJSONErr(w, http.StatusInternalServerError, "failed to resolve dataset")
		}
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handlers) requireDatasetWrite(w http.ResponseWriter, r *http.Request, datasetID uuid.UUID) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if claims.HasRole("admin") || claims.HasPermissionKey("dataset.write") || claims.HasPermission("dataset", "write") {
		return claims, true
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden", "required_scope": "dataset.write", "dataset_rid": datasetID.String()})
	return nil, false
}

func (h *Handlers) requireDatasetAdmin(w http.ResponseWriter, r *http.Request, datasetID uuid.UUID) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if claims.HasRole("admin") || claims.HasPermissionKey("dataset.admin") || claims.HasPermission("dataset", "admin") {
		return claims, true
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden", "required_scope": "dataset.admin", "dataset_rid": datasetID.String()})
	return nil, false
}

func (h *Handlers) GetDatasetModel(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	model, err := h.Repo.GetDatasetRichModel(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset model")
		return
	}
	if model == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	writeJSON(w, http.StatusOK, model)
}

func (h *Handlers) PatchDatasetMetadata(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.DatasetMetadataPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil {
		if err := validateDatasetName(*body.Name); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.Format != nil {
		normalized := strings.ToLower(*body.Format)
		if err := validateDatasetFormat(normalized); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		body.Format = &normalized
	}
	if body.HealthStatus != nil {
		if err := validateHealthStatus(*body.HealthStatus); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.CurrentViewID != nil {
		belongs, err := h.Repo.DatasetViewBelongsToDataset(r.Context(), datasetID, *body.CurrentViewID)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, "failed to validate current_view_id")
			return
		}
		if !belongs {
			writeJSONErr(w, http.StatusBadRequest, "current_view_id must belong to the dataset")
			return
		}
	}
	updated, err := h.Repo.PatchDatasetMetadata(r.Context(), datasetID, &body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to update dataset metadata")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handlers) ListDatasetMarkings(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetMarkings(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset markings")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetMarkings(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetAdmin(w, r, datasetID)
	if !ok {
		return
	}
	var body models.PutDatasetMarkingsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Repo.ReplaceDatasetMarkings(r.Context(), datasetID, body.Markings, claims.Sub); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset markings")
		return
	}
	h.ListDatasetMarkings(w, r)
}

func (h *Handlers) ListDatasetPermissions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetPermissions(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset permissions")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetPermissions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetAdmin(w, r, datasetID); !ok {
		return
	}
	var body models.PutDatasetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, edge := range body.Permissions {
		source := "direct"
		if edge.Source != nil {
			source = *edge.Source
		}
		if err := validatePermissionEdge(edge.PrincipalKind, edge.PrincipalID, edge.Role, edge.Actions, source, edge.InheritedFrom); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetPermissions(r.Context(), datasetID, body.Permissions); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset permission conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset permissions")
		return
	}
	h.ListDatasetPermissions(w, r)
}

func (h *Handlers) ListDatasetLineageLinks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetLineageLinks(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset lineage links")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetLineageLinks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.PutDatasetLineageLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, link := range body.Links {
		if err := validateLineageLink(link.Direction, link.TargetRID); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetLineageLinks(r.Context(), datasetID, body.Links); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset lineage conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset lineage links")
		return
	}
	h.ListDatasetLineageLinks(w, r)
}

func (h *Handlers) ListDatasetFileIndex(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetFileIndex(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset file index")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetFileIndex(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.PutDatasetFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, file := range body.Files {
		entryType := "file"
		if file.EntryType != nil {
			entryType = *file.EntryType
		}
		size := int64(0)
		if file.SizeBytes != nil {
			size = *file.SizeBytes
		}
		if err := validateFileIndexEntry(file.Path, file.StoragePath, entryType, size); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetFileIndex(r.Context(), datasetID, body.Files); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset file index conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset file index")
		return
	}
	h.ListDatasetFileIndex(w, r)
}

func validateDatasetName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("dataset name is required")
	}
	if len(trimmed) > 255 {
		return errors.New("dataset name must be 255 characters or fewer")
	}
	return nil
}

func validateDatasetFormat(format string) error {
	switch strings.ToLower(format) {
	case "parquet", "avro", "csv", "json", "text", "unknown":
		return nil
	default:
		return errors.New("unsupported dataset format: " + format)
	}
}

func validateHealthStatus(status string) error {
	if containsString([]string{"unknown", "healthy", "warning", "degraded", "critical"}, status) {
		return nil
	}
	return errors.New("health_status must be one of: unknown, healthy, warning, degraded, critical")
}

func validatePermissionEdge(principalKind, principalID, role string, actions []string, source string, inheritedFrom *string) error {
	if !containsString([]string{"user", "group", "role", "organization", "project", "service"}, principalKind) {
		return errors.New("principal_kind must be one of: user, group, role, organization, project, service")
	}
	if !containsString([]string{"direct", "inherited_from_project", "inherited_from_folder", "inherited_from_parent"}, source) {
		return errors.New("source must be one of: direct, inherited_from_project, inherited_from_folder, inherited_from_parent")
	}
	if strings.TrimSpace(principalID) == "" {
		return errors.New("principal_id is required")
	}
	if strings.TrimSpace(role) == "" {
		return errors.New("role is required")
	}
	for _, action := range actions {
		if strings.TrimSpace(action) == "" {
			return errors.New("permission actions cannot be empty")
		}
	}
	if source == "direct" && inheritedFrom != nil {
		return errors.New("direct permissions cannot set inherited_from")
	}
	if source != "direct" && (inheritedFrom == nil || strings.TrimSpace(*inheritedFrom) == "") {
		return errors.New("inherited permissions require inherited_from")
	}
	return nil
}

func validateLineageLink(direction, targetRID string) error {
	if !containsString([]string{"upstream", "downstream"}, direction) {
		return errors.New("direction must be one of: upstream, downstream")
	}
	if strings.TrimSpace(targetRID) == "" {
		return errors.New("target_rid is required")
	}
	return nil
}

func validateFileIndexEntry(path, storagePath, entryType string, sizeBytes int64) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file path is required")
	}
	if strings.TrimSpace(storagePath) == "" {
		return errors.New("storage_path is required")
	}
	if !containsString([]string{"file", "directory"}, entryType) {
		return errors.New("entry_type must be one of: file, directory")
	}
	if sizeBytes < 0 {
		return errors.New("size_bytes must be non-negative")
	}
	return nil
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
