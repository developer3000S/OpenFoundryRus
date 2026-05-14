package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

func (h *Handlers) ListFoundryBranches(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if dataset, err := h.Repo.GetDataset(r.Context(), datasetID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if dataset != nil {
		if err := h.Repo.EnsureDefaultBranch(r.Context(), dataset); err != nil {
			writeBranchError(w, err)
			return
		}
	}
	rows, err := h.Repo.ListActiveRuntimeBranches(r.Context(), datasetID)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	offset, limit, err := parseFoundryPage(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	data := make([]models.FoundryBranch, 0, end-offset)
	for _, row := range rows[offset:end] {
		data = append(data, models.FoundryBranchFromRuntime(row))
	}
	var next *string
	if end < len(rows) {
		value := encodeCursor(end)
		next = &value
	}
	writeJSON(w, http.StatusOK, models.FoundryListBranchesResponse{Data: data, NextPageToken: next})
}

func (h *Handlers) GetFoundryBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.FoundryBranchFromRuntime(*branch))
}

func (h *Handlers) CreateFoundryBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.FoundryCreateBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	create := models.CreateBranchBody{Name: body.Name}
	if body.TransactionRID != nil && strings.TrimSpace(*body.TransactionRID) != "" {
		create.Source = &models.BranchSource{FromTransactionRID: body.TransactionRID}
	} else {
		dataset, err := h.Repo.GetDataset(r.Context(), datasetID)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if dataset == nil {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		if err := h.Repo.EnsureDefaultBranch(r.Context(), dataset); err != nil {
			writeBranchError(w, err)
			return
		}
		parent := strings.TrimSpace(dataset.ActiveBranch)
		if parent != "" {
			create.ParentBranch = &parent
		}
	}
	branch, err := h.Repo.CreateRuntimeBranch(r.Context(), datasetID, &create, claims.Sub)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, models.FoundryBranchFromRuntime(*branch))
}

func (h *Handlers) DeleteFoundryBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	if _, err := h.Repo.DeleteRuntimeBranch(r.Context(), datasetID, branchNameParam(r)); err != nil {
		writeBranchError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListFoundryBranchTransactions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branchName := branchNameParam(r)
	if _, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchName); err != nil {
		writeBranchError(w, err)
		return
	}
	offset, limit, err := parseFoundryPage(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	queryLimit := offset + limit + 1
	rows, err := h.Repo.ListRuntimeTransactions(r.Context(), datasetID, &branchName, nil, queryLimit)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	var next *string
	if end < len(rows) {
		value := encodeCursor(end)
		next = &value
	}
	writeJSON(w, http.StatusOK, models.FoundryListTransactionsResponse{Data: models.NewRuntimeTransactionResponses(rows[offset:end]), NextPageToken: next})
}

func (h *Handlers) ListBranchTransactions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branchName := branchNameParam(r)
	if _, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchName); err != nil {
		writeBranchError(w, err)
		return
	}
	rows, err := h.Repo.ListRuntimeTransactions(r.Context(), datasetID, &branchName, nil, 500)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	offset, limit := parsePage(r)
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	hasMore := end < len(rows)
	var next *string
	if hasMore {
		value := encodeCursor(end)
		next = &value
	}
	writeJSON(w, http.StatusOK, models.Page[models.RuntimeTransactionResponse]{Data: models.NewRuntimeTransactionResponses(rows[offset:end]), NextCursor: next, HasMore: hasMore})
}

func parseFoundryPage(r *http.Request) (offset int, limit int, err error) {
	limit = 20
	if raw := strings.TrimSpace(r.URL.Query().Get("pageSize")); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n <= 0 {
			return 0, 0, errString("pageSize must be greater than zero")
		}
		limit = n
	} else if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n <= 0 {
			return 0, 0, errString("limit must be greater than zero")
		}
		limit = n
	}
	if limit > 50 {
		limit = 50
	}
	token := strings.TrimSpace(r.URL.Query().Get("pageToken"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("cursor"))
	}
	if token != "" {
		req := r.Clone(r.Context())
		q := req.URL.Query()
		q.Set("cursor", token)
		req.URL.RawQuery = q.Encode()
		offset, _ = parsePage(req)
	}
	return offset, limit, nil
}
