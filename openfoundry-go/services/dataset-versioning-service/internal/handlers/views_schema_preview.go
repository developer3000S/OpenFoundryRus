package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func viewIDParam(r *http.Request) string       { return chi.URLParam(r, "view_id") }
func viewOrActionParam(r *http.Request) string { return chi.URLParam(r, "view_or_action") }

func (h *Handlers) ListViews(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	views, err := h.Repo.ListViews(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list views")
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handlers) CreateView(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.CreateDatasetViewRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.SQL) == "" {
		writeJSONErr(w, http.StatusBadRequest, "name and sql are required")
		return
	}
	view, err := h.Repo.CreateView(r.Context(), datasetID, &body)
	if err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "view already exists")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to create view")
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (h *Handlers) GetView(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	view, err := h.Repo.GetDatasetView(r.Context(), datasetID, viewOrActionParam(r))
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) ViewAction(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewAction := viewOrActionParam(r)
	viewName, action, ok := strings.Cut(viewAction, ":")
	if !ok || action != "refresh" {
		writeJSONErr(w, http.StatusBadRequest, "unsupported view action; only ':refresh' is supported")
		return
	}
	view, err := h.Repo.RefreshDatasetView(r.Context(), datasetID, viewName)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) GetCurrentView(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "master"
	}
	view, err := h.Repo.GetCurrentView(r.Context(), datasetID, branch)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) GetViewAt(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "master"
	}
	var at *time.Time
	if raw := r.URL.Query().Get("ts"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid ts")
			return
		}
		at = &parsed
	}
	var txn *uuid.UUID
	if raw := r.URL.Query().Get("transaction_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid transaction_id")
			return
		}
		txn = &id
	}
	view, err := h.Repo.GetViewAt(r.Context(), datasetID, branch, at, txn)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) ListViewFiles(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	viewID, err := uuid.Parse(viewIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid view_id")
		return
	}
	files, err := h.Repo.ListViewFiles(r.Context(), datasetID, viewID)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *Handlers) GetViewSchema(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.resolveDatasetForCatalog(w, r); !ok {
		return
	}
	viewID, err := uuid.Parse(viewIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid view_id")
		return
	}
	schema, err := h.Repo.GetViewSchema(r.Context(), viewID)
	if err != nil {
		writeViewError(w, err)
		return
	}
	if schema == nil {
		writeJSONErr(w, http.StatusNotFound, "schema not found")
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handlers) PutViewSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, err := uuid.Parse(viewIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid view_id")
		return
	}
	var body models.PutSchemaBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := validateDatasetSchema(body.Schema); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	branch := r.URL.Query().Get("branch")
	var branchPtr *string
	if branch != "" {
		branchPtr = &branch
	}
	raw, _ := models.MarshalJSONValue(body.Schema)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	out, err := h.Repo.PutViewSchema(r.Context(), viewID, datasetID, branchPtr, body.Schema, hash)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) PreviewViewData(w http.ResponseWriter, r *http.Request) { h.previewData(w, r, true) }
func (h *Handlers) PreviewMaterializedView(w http.ResponseWriter, r *http.Request) {
	h.previewData(w, r, true)
}
func (h *Handlers) PreviewDataset(w http.ResponseWriter, r *http.Request) { h.previewData(w, r, false) }

func (h *Handlers) previewData(w http.ResponseWriter, r *http.Request, scopedView bool) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	q := previewQuery(r)
	var viewID *uuid.UUID
	if scopedView {
		id, err := uuid.Parse(viewIDParam(r))
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid view_id")
			return
		}
		viewID = &id
	}
	out, err := h.Repo.PreviewData(r.Context(), datasetID, viewID, q)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) GetCurrentSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "master"
	}
	schema, err := h.Repo.GetCurrentSchema(r.Context(), datasetID, branch)
	if err != nil {
		writeViewError(w, err)
		return
	}
	if schema == nil {
		writeJSONErr(w, http.StatusNotFound, "schema not found")
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

func (h *Handlers) ValidateSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	var body models.ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.ValidateSchema(r.Context(), datasetID, body.Schema)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func previewQuery(r *http.Request) models.PreviewQuery {
	q := models.PreviewQuery{}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			q.Limit = &n
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			q.Offset = &n
		}
	}
	if raw := r.URL.Query().Get("format"); raw != "" {
		q.Format = &raw
	}
	return q
}

func validateDatasetSchema(schema models.DatasetSchema) error {
	if strings.TrimSpace(schema.FileFormat) == "" {
		return errString("file_format is required")
	}
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			return errString("field name is required")
		}
		if seen[name] {
			return errString("duplicate field: " + name)
		}
		seen[name] = true
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func writeViewError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if err == repo.ErrNotFound {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	if repo.IsConflict(err) {
		writeJSONErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONErr(w, http.StatusInternalServerError, err.Error())
}
