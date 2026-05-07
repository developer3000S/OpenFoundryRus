package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

func branchNameParam(r *http.Request) string {
	return chi.URLParam(r, "branch")
}

func (h *Handlers) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	out, err := h.Repo.DeleteRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) BranchAction(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	branchAction := branchNameParam(r)
	branch, action, ok := strings.Cut(branchAction, ":")
	if !ok {
		writeJSONErr(w, http.StatusMethodNotAllowed, "POST on /branches/{branch} requires a ':reparent' action suffix")
		return
	}
	if action != "reparent" {
		writeJSONErr(w, http.StatusBadRequest, "unsupported branch action; only ':reparent' is supported")
		return
	}
	var body models.ReparentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.ReparentRuntimeBranch(r.Context(), datasetID, branch, body.NewParentBranch)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) CheckoutBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	out, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) BranchAncestry(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	chain, err := h.Repo.BranchAncestry(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	payload := make([]map[string]any, 0, len(chain))
	for _, b := range chain {
		payload = append(payload, map[string]any{"rid": b.RID, "name": b.Name, "is_root": b.ParentBranchID == nil})
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handlers) PreviewDeleteBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	out, err := h.Repo.PreviewDeleteBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) UpdateRetention(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.UpdateRetentionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Policy == models.RetentionPolicyTTLDays && (body.TTLDays == nil || *body.TTLDays <= 0) {
		writeJSONErr(w, http.StatusBadRequest, "ttl_days must be > 0 when policy = TTL_DAYS")
		return
	}
	if body.Policy != models.RetentionPolicyInherited && body.Policy != models.RetentionPolicyForever && body.Policy != models.RetentionPolicyTTLDays {
		writeJSONErr(w, http.StatusBadRequest, "invalid retention policy")
		return
	}
	_, err := h.Repo.UpdateBranchRetention(r.Context(), datasetID, branchNameParam(r), body.Policy, body.TTLDays)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": branchNameParam(r), "policy": body.Policy, "ttl_days": body.TTLDays})
}

func (h *Handlers) GetBranchMarkings(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	rows, err := h.Repo.ListBranchMarkings(r.Context(), branch.ID)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	view := models.BranchMarkingsView{Effective: []uuid.UUID{}, Explicit: []uuid.UUID{}, InheritedFromParent: []uuid.UUID{}}
	for _, row := range rows {
		view.Effective = append(view.Effective, row.MarkingID)
		if row.Source == "EXPLICIT" {
			view.Explicit = append(view.Explicit, row.MarkingID)
		} else {
			view.InheritedFromParent = append(view.InheritedFromParent, row.MarkingID)
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handlers) RestoreBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	out, err := h.Repo.RestoreBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": out.Name, "restored_at": out.UpdatedAt})
}

func (h *Handlers) CompareBranches(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	base := r.URL.Query().Get("base")
	if base == "" {
		base = r.URL.Query().Get("base_branch")
	}
	compare := r.URL.Query().Get("compare")
	if compare == "" {
		compare = r.URL.Query().Get("target_branch")
	}
	if base == "" || compare == "" {
		writeJSONErr(w, http.StatusBadRequest, "base and compare are required")
		return
	}
	out, err := h.Repo.CompareBranches(r.Context(), datasetID, base, compare)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) RollbackBranch(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.RollbackBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Repo.RollbackBranch(r.Context(), datasetID, branchNameParam(r), &body, claims.Sub)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) ListFallbacks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	out, err := h.Repo.ListFallbacks(r.Context(), branch.ID)
	if err != nil {
		writeBranchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) PutFallbacks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), datasetID, branchNameParam(r))
	if err != nil {
		writeBranchError(w, err)
		return
	}
	var body models.PutFallbacksRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Repo.ReplaceFallbacks(r.Context(), branch.ID, body.Fallbacks); err != nil {
		writeBranchError(w, err)
		return
	}
	h.ListFallbacks(w, r)
}
