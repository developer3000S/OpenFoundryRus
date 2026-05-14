package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
	isLogical := createViewRequestIsLogical(body)
	if strings.TrimSpace(body.Name) == "" {
		writeJSONErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if !isLogical && strings.TrimSpace(body.SQL) == "" {
		writeJSONErr(w, http.StatusBadRequest, "sql is required for materialized views")
		return
	}
	if isLogical && len(body.BackingDatasets) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "backing_datasets is required for logical views")
		return
	}
	if body.Schema != nil {
		if errs := models.ValidateDatasetSchema(*body.Schema); len(errs) > 0 {
			writeSchemaParseError(w, strings.Join(errs, "; "))
			return
		}
		normalized := models.NormalizeDatasetSchema(*body.Schema)
		body.Schema = &normalized
	}
	if pk := primaryKeyFromCreateView(body); len(pk) > 0 {
		body.PrimaryKey = pk
		body.PrimaryKeys = nil
	}
	if isLogical && strings.TrimSpace(body.Kind) == "" {
		body.Kind = models.DatasetViewKindLogical
	}
	if isLogical && body.Materialized == nil {
		materialized := false
		body.Materialized = &materialized
	}
	if isLogical && body.AutoRebuild == nil {
		autoRebuild := true
		body.AutoRebuild = &autoRebuild
	}
	if isLogical && body.RefreshOnSourceUpdate == nil {
		refresh := true
		body.RefreshOnSourceUpdate = &refresh
	}
	view, err := h.Repo.CreateView(r.Context(), datasetID, &body)
	if err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "view already exists")
			return
		}
		if errors.Is(err, repo.ErrValidation) {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to create view")
		return
	}
	if len(body.BackingDatasets) > 0 {
		backing, err := h.Repo.ReplaceViewBackingDatasets(r.Context(), datasetID, view.ID, body.BackingDatasets)
		if err != nil {
			writeViewError(w, err)
			return
		}
		view.BackingDatasets = backing
		view.Kind = models.DatasetViewKindLogical
		view.Materialized = false
		view.TransformInputOnly = true
	}
	if len(body.PrimaryKey) > 0 {
		primaryKey, err := h.Repo.PutViewPrimaryKey(r.Context(), datasetID, view.ID, body.PrimaryKey)
		if err != nil {
			writeViewError(w, err)
			return
		}
		view.PrimaryKey = primaryKey
	}
	if body.Schema != nil {
		raw, _ := models.MarshalJSONValue(*body.Schema)
		sum := sha256.Sum256(raw)
		schema, err := h.Repo.PutViewSchema(r.Context(), view.ID, datasetID, body.SourceBranch, *body.Schema, hex.EncodeToString(sum[:]))
		if err != nil {
			writeViewError(w, err)
			return
		}
		view.SchemaFields, _ = models.MarshalJSONValue(schema.Schema.Fields)
	}
	writeJSON(w, http.StatusCreated, view)
}

func createViewRequestIsLogical(body models.CreateDatasetViewRequest) bool {
	raw := strings.ToLower(strings.TrimSpace(body.Kind))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(body.ViewType))
	}
	switch raw {
	case "logical", "logical_view", "union", "union_view":
		return true
	}
	return len(body.BackingDatasets) > 0
}

func primaryKeyFromCreateView(body models.CreateDatasetViewRequest) []string {
	if len(body.PrimaryKey) > 0 {
		return append([]string(nil), body.PrimaryKey...)
	}
	return append([]string(nil), body.PrimaryKeys...)
}

func parseViewIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	viewID, err := uuid.Parse(viewIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid view_id")
		return uuid.Nil, false
	}
	return viewID, true
}

func decodeBackingDatasetsRequest(r *http.Request) ([]models.ViewBackingDatasetInput, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, errString("empty body")
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []models.ViewBackingDatasetInput
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	var body models.ViewBackingDatasetsRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	if len(body.BackingDatasets) > 0 {
		return body.BackingDatasets, nil
	}
	return body.Data, nil
}

func decodeRemoveBackingDatasetsRequest(r *http.Request) ([]models.ViewBackingDatasetInput, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, errString("empty body")
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []models.ViewBackingDatasetInput
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	var body models.RemoveViewBackingDatasetsRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	items := append([]models.ViewBackingDatasetInput{}, body.BackingDatasets...)
	items = append(items, body.Data...)
	for _, id := range body.DatasetIDs {
		idCopy := id
		items = append(items, models.ViewBackingDatasetInput{DatasetID: &idCopy})
	}
	for _, rid := range body.DatasetRIDs {
		items = append(items, models.ViewBackingDatasetInput{DatasetRID: rid})
	}
	return items, nil
}

func primaryKeyFromBody(body models.ViewPrimaryKeyRequest) []string {
	if len(body.PrimaryKey) > 0 {
		return append([]string(nil), body.PrimaryKey...)
	}
	if len(body.PrimaryKeys) > 0 {
		return append([]string(nil), body.PrimaryKeys...)
	}
	return append([]string(nil), body.Columns...)
}

func (h *Handlers) viewBackingResponse(ctx context.Context, datasetID uuid.UUID, viewID uuid.UUID, backing []models.ViewBackingDataset) models.ViewBackingDatasetsResponse {
	primaryKey, _ := h.Repo.GetViewPrimaryKey(ctx, datasetID, viewID)
	return models.ViewBackingDatasetsResponse{Data: backing, PrimaryKey: primaryKey}
}

func (h *Handlers) ListViewBackingDatasets(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	backing, err := h.Repo.ListViewBackingDatasets(r.Context(), datasetID, viewID)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.viewBackingResponse(r.Context(), datasetID, viewID, backing))
}

func (h *Handlers) ReplaceViewBackingDatasets(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	backing, err := decodeBackingDatasetsRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.ReplaceViewBackingDatasets(r.Context(), datasetID, viewID, backing)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.viewBackingResponse(r.Context(), datasetID, viewID, out))
}

func (h *Handlers) AddViewBackingDatasets(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	backing, err := decodeBackingDatasetsRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.AddViewBackingDatasets(r.Context(), datasetID, viewID, backing)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.viewBackingResponse(r.Context(), datasetID, viewID, out))
}

func (h *Handlers) RemoveViewBackingDatasets(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	backing, err := decodeRemoveBackingDatasetsRequest(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.RemoveViewBackingDatasets(r.Context(), datasetID, viewID, backing)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.viewBackingResponse(r.Context(), datasetID, viewID, out))
}

func (h *Handlers) PutViewPrimaryKey(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	var body models.ViewPrimaryKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	primaryKey, err := h.Repo.PutViewPrimaryKey(r.Context(), datasetID, viewID, primaryKeyFromBody(body))
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"primary_key": primaryKey})
}

func (h *Handlers) DeleteViewPrimaryKey(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	viewID, ok := parseViewIDParam(w, r)
	if !ok {
		return
	}
	primaryKey, err := h.Repo.PutViewPrimaryKey(r.Context(), datasetID, viewID, nil)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"primary_key": primaryKey})
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
	var version *int32
	if raw := r.URL.Query().Get("version"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeJSONErr(w, http.StatusBadRequest, "invalid version")
			return
		}
		v := int32(parsed)
		version = &v
	}
	if txn != nil && version != nil {
		writeJSONErr(w, http.StatusBadRequest, "transaction_id and version are mutually exclusive")
		return
	}
	if at != nil && (txn != nil || version != nil) {
		writeJSONErr(w, http.StatusBadRequest, "ts cannot be combined with transaction_id or version")
		return
	}
	view, err := h.Repo.GetViewAt(r.Context(), datasetID, branch, at, txn, version)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) CompareViews(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	baseBranch := q.Get("base_branch")
	if baseBranch == "" {
		baseBranch = "master"
	}
	targetBranch := q.Get("target_branch")
	if targetBranch == "" {
		targetBranch = baseBranch
	}
	var baseTxn *uuid.UUID
	if raw := q.Get("base_transaction"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid base_transaction")
			return
		}
		baseTxn = &id
	}
	var targetTxn *uuid.UUID
	if raw := q.Get("target_transaction"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid target_transaction")
			return
		}
		targetTxn = &id
	}
	out, err := h.Repo.CompareViews(r.Context(), datasetID, baseBranch, targetBranch, baseTxn, targetTxn)
	if err != nil {
		writeViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
	if errs := models.ValidateDatasetSchema(body.Schema); len(errs) > 0 {
		writeSchemaParseError(w, strings.Join(errs, "; "))
		return
	}
	body.Schema = models.NormalizeDatasetSchema(body.Schema)
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
	if out, ok, err := h.previewRowsFromTableFiles(r.Context(), datasetID, viewID, q, false); err != nil {
		if previewRequiresTableRead(q) {
			writeViewError(w, err)
			return
		}
	} else if ok {
		writeJSON(w, http.StatusOK, out)
		return
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
	if raw := strings.TrimSpace(r.URL.Query().Get("branch")); raw != "" {
		q.Branch = &raw
	}
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
	if raw := strings.TrimSpace(r.URL.Query().Get("columns")); raw != "" {
		q.Columns = splitCSVQuery(raw)
	}
	if values, ok := r.URL.Query()["column"]; ok {
		q.Columns = append(q.Columns, values...)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("filter")); raw != "" {
		q.Filter = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("sort")); raw != "" {
		q.Sort = splitCSVQuery(raw)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("sample")); raw != "" {
		q.Sample = raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("sample_size")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.SampleSize = &n
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("sample_seed")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			q.SampleSeed = &n
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("transaction_id")); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			q.TransactionID = &id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("transactionRid")); raw != "" {
		if id, err := parseFoundryTransactionRID(raw); err == nil {
			q.TransactionID = &id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("endTransactionRid")); raw != "" {
		if id, err := parseFoundryTransactionRID(raw); err == nil {
			q.TransactionID = &id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("version")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 32); err == nil && n > 0 {
			v := int32(n)
			q.Version = &v
		}
	}
	return q
}

func splitCSVQuery(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func previewRequiresTableRead(q models.PreviewQuery) bool {
	return len(q.Columns) > 0 || q.Filter != nil || len(q.Sort) > 0 || q.Sample || q.TransactionID != nil || q.Version != nil
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
	if errors.Is(err, repo.ErrValidation) {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONErr(w, http.StatusInternalServerError, err.Error())
}
