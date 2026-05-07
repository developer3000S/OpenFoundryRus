package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) StartTransaction(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.StartTransactionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validTransactionType(body.Type) {
		writeJSONErr(w, http.StatusBadRequest, "invalid transaction type")
		return
	}
	branchName := branchNameParam(r)
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchName)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	summary := ""
	if body.Summary != nil {
		summary = *body.Summary
	}
	out, err := h.Repo.StartTransaction(r.Context(), datasetID, branch.ID, branch.Name, body.Type, summary, body.Providence, claims.Sub)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) || repo.IsConflict(err) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "BRANCH_HAS_OPEN_TRANSACTION", "message": "branch already has an OPEN transaction", "branch": branch.Name})
			return
		}
		writeTransactionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handlers) GetTransaction(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	txnID, err := uuid.Parse(transactionIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "transaction id is not a valid UUID")
		return
	}
	out, err := h.Repo.GetRuntimeTransaction(r.Context(), datasetID, txnID)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	if out == nil || out.BranchName != branchNameParam(r) {
		writeJSONErr(w, http.StatusNotFound, "transaction not found")
		return
	}
	writeJSONWithETag(w, r, http.StatusOK, out)
}

func (h *Handlers) TransactionAction(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST on /transactions/{txn} requires ':commit' or ':abort' action suffix"})
}

func (h *Handlers) CommitTransaction(w http.ResponseWriter, r *http.Request) {
	h.finishTransaction(w, r, "commit")
}

func (h *Handlers) AbortTransaction(w http.ResponseWriter, r *http.Request) {
	h.finishTransaction(w, r, "abort")
}

func (h *Handlers) finishTransaction(w http.ResponseWriter, r *http.Request, action string) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	txnID, err := uuid.Parse(transactionIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "transaction id is not a valid UUID")
		return
	}
	before, err := h.Repo.GetRuntimeTransaction(r.Context(), datasetID, txnID)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	if before == nil || before.BranchName != branchNameParam(r) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found", "txn": txnID})
		return
	}
	if action == "commit" {
		err = h.Repo.CommitTransaction(r.Context(), datasetID, txnID)
	} else {
		err = h.Repo.AbortTransaction(r.Context(), datasetID, txnID)
	}
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	after, err := h.Repo.GetRuntimeTransaction(r.Context(), datasetID, txnID)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	if after == nil || after.BranchName != branchNameParam(r) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found after action", "txn": txnID})
		return
	}
	writeJSON(w, http.StatusOK, after)
}

func (h *Handlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	var branch *string
	if raw := strings.TrimSpace(r.URL.Query().Get("branch")); raw != "" {
		branch = &raw
	}
	var before *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "'before' must be RFC3339")
			return
		}
		parsed = parsed.UTC()
		before = &parsed
	}
	rows, err := h.Repo.ListRuntimeTransactions(r.Context(), datasetID, branch, before, 200)
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
		v := encodeCursor(offset + limit)
		next = &v
	}
	writeJSON(w, http.StatusOK, models.Page[models.RuntimeTransaction]{Data: rows[offset:end], NextCursor: next, HasMore: hasMore})
}

func (h *Handlers) BatchGetTransactions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	var body models.BatchGetTransactionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	items := make([]models.BatchItemResult[models.RuntimeTransaction], 0, len(body.IDs))
	for _, raw := range body.IDs {
		txnID, err := uuid.Parse(raw)
		if err != nil {
			msg := "transaction id is not a valid UUID"
			items = append(items, models.BatchItemResult[models.RuntimeTransaction]{ID: raw, Status: http.StatusBadRequest, Error: &msg})
			continue
		}
		row, err := h.Repo.GetRuntimeTransaction(r.Context(), datasetID, txnID)
		if err != nil {
			writeTransactionError(w, err)
			return
		}
		if row == nil {
			msg := "transaction not found"
			items = append(items, models.BatchItemResult[models.RuntimeTransaction]{ID: raw, Status: http.StatusNotFound, Error: &msg})
			continue
		}
		items = append(items, models.BatchItemResult[models.RuntimeTransaction]{ID: raw, Status: http.StatusOK, Data: row})
	}
	writeJSON(w, http.StatusMultiStatus, items)
}

func validTransactionType(v models.TransactionType) bool {
	switch v {
	case models.TransactionTypeSnapshot, models.TransactionTypeAppend, models.TransactionTypeUpdate, models.TransactionTypeDelete:
		return true
	default:
		return false
	}
}

func writeTransactionError(w http.ResponseWriter, err error) {
	if errors.Is(err, repo.ErrNotFound) {
		writeJSONErr(w, http.StatusNotFound, "transaction not found")
		return
	}
	if errors.Is(err, repo.ErrInvalidTransition) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "transaction is not OPEN"})
		return
	}
	if errors.Is(err, repo.ErrValidation) {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, repo.ErrConflict) || repo.IsConflict(err) {
		writeJSONErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONErr(w, http.StatusInternalServerError, err.Error())
}

func writeJSONWithETag(w http.ResponseWriter, r *http.Request, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to encode response")
		return
	}
	sum := sha256.Sum256(data)
	etag := `"sha256:` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n"))
}
