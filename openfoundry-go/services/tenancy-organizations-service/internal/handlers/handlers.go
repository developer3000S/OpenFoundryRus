// Package handlers wires the foundation HTTP endpoints for tenancy-organizations-service.
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/repo"
)

// Handlers groups every endpoint so server.go has a single dependency.
type Handlers struct{ Repo *repo.Repo }

// ─── Organizations ──────────────────────────────────────────────────────

func (h *Handlers) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	items, err := h.Repo.ListOrganizations(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Organization]{Items: items})
}

func (h *Handlers) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	o, err := h.Repo.GetOrganization(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if o == nil {
		writeJSONErr(w, http.StatusNotFound, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handlers) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var body models.CreateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Slug == "" || body.DisplayName == "" {
		writeJSONErr(w, http.StatusBadRequest, "organization slug and display name are required")
		return
	}
	o, err := h.Repo.CreateOrganization(r.Context(), &body)
	if err != nil {
		slog.Error("create organization", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *Handlers) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body models.UpdateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	o, err := h.Repo.UpdateOrganization(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if o == nil {
		writeJSONErr(w, http.StatusNotFound, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *Handlers) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteOrganization(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Enrollments ────────────────────────────────────────────────────────

func (h *Handlers) ListEnrollments(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	items, err := h.Repo.ListEnrollmentsByOrg(r.Context(), orgID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Enrollment]{Items: items})
}

func (h *Handlers) CreateEnrollment(w http.ResponseWriter, r *http.Request) {
	var body models.CreateEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.OrganizationID == uuid.Nil || body.UserID == uuid.Nil || body.RoleSlug == "" {
		writeJSONErr(w, http.StatusBadRequest, "organization_id, user_id and role_slug required")
		return
	}
	e, err := h.Repo.CreateEnrollment(r.Context(), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handlers) DeleteEnrollment(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteEnrollment(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── helpers ────────────────────────────────────────────────────────────

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// silence unused errors import in build configurations that strip it
var _ = errors.New
