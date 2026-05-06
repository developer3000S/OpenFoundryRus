// Package handlers wires the HTTP endpoints for audit-compliance-service.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/repo"
)

type Handlers struct{ Repo *repo.Repo }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseLimit(r *http.Request, fallback int) int {
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return fallback
}

func authed(r *http.Request) bool {
	_, ok := authmw.FromContext(r.Context())
	return ok
}

// ─── Audit ledger ──────────────────────────────────────────────────

func (h *Handlers) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListAuditEvents(r.Context(), parseLimit(r, 100))
	if err != nil {
		slog.Error("list audit events", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.AuditEvent]{Items: items})
}

// ─── Audit policies ────────────────────────────────────────────────

func (h *Handlers) ListAuditPolicies(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListAuditPolicies(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list audit policies")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.AuditPolicy]{Items: items})
}

// ─── Compliance reports ────────────────────────────────────────────

func (h *Handlers) ListComplianceReports(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListComplianceReports(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list reports")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.ComplianceReport]{Items: items})
}

// ─── Retention policies + jobs ─────────────────────────────────────

func (h *Handlers) ListRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListRetentionPolicies(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list retention policies")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.RetentionPolicy]{Items: items})
}

func (h *Handlers) GetRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetRetentionPolicy(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateRetentionPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.TargetKind == "" || body.PurgeMode == "" {
		writeJSONErr(w, http.StatusBadRequest, "name, target_kind, purge_mode required")
		return
	}
	if body.RetentionDays < 0 {
		writeJSONErr(w, http.StatusBadRequest, "retention_days must be ≥ 0")
		return
	}
	v, err := h.Repo.CreateRetentionPolicy(r.Context(), &body, caller.Sub.String())
	if err != nil {
		slog.Error("create retention policy", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateRetentionPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateRetentionPolicy(r.Context(), id, &body, caller.Sub.String())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) ListRetentionJobs(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var policyID *uuid.UUID
	if raw := r.URL.Query().Get("policy_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid policy_id")
			return
		}
		policyID = &id
	}
	items, err := h.Repo.ListRetentionJobs(r.Context(), policyID, parseLimit(r, 100))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list retention jobs")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.RetentionJob]{Items: items})
}

// ─── SDS ───────────────────────────────────────────────────────────

func (h *Handlers) ListSDSScanJobs(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListSDSScanJobs(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list SDS scan jobs")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.SDSScanJob]{Items: items})
}

func (h *Handlers) ListSDSIssues(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "job_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid job_id")
		return
	}
	items, err := h.Repo.ListSDSIssuesByJob(r.Context(), jobID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list SDS issues")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.SDSIssue]{Items: items})
}

func (h *Handlers) ListSDSRemediationRules(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.Repo.ListSDSRemediationRules(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list remediation rules")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.SDSRemediationRule]{Items: items})
}

// ─── Lineage deletion ──────────────────────────────────────────────

func (h *Handlers) ListLineageDeletionRequests(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var datasetID *uuid.UUID
	if raw := r.URL.Query().Get("dataset_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid dataset_id")
			return
		}
		datasetID = &id
	}
	items, err := h.Repo.ListLineageDeletionRequests(r.Context(), datasetID, parseLimit(r, 100))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list deletion requests")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.LineageDeletionRequest]{Items: items})
}

func (h *Handlers) CreateLineageDeletionRequest(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateLineageDeletionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.DatasetID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "dataset_id required")
		return
	}
	v, err := h.Repo.CreateLineageDeletionRequest(r.Context(), &body)
	if err != nil {
		slog.Error("create deletion request", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// ─── Saga audit log ────────────────────────────────────────────────

func (h *Handlers) ListSagaAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !authed(r) {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var sagaID *uuid.UUID
	if raw := r.URL.Query().Get("saga_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid saga_id")
			return
		}
		sagaID = &id
	}
	items, err := h.Repo.ListSagaAuditEvents(r.Context(), sagaID, parseLimit(r, 100))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list saga audit events")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.SagaAuditEvent]{Items: items})
}
