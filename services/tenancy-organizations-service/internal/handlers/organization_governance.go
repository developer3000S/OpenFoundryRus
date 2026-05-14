// organization_governance.go: SG.2 — organization administrators,
// guest memberships, and Foundry-style spaces.
//
// All three resources are administrative surfaces required by the
// public Foundry security model and the SG.2 checklist line in
// docs/migration/foundry-security-governance-1to1-checklist.md.
//
// The handlers reuse the foundation `Repo` for SQL access and the
// existing `parseID` / `writeJSON` helpers in handlers.go. They are
// declared in this file (rather than appended to handlers.go) so the
// foundation file stays focused on the seed resources and the new
// SG.2 surface is easy to locate.

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
)

// ─── Administrators ────────────────────────────────────────────────────

func (h *Handlers) ListOrganizationAdmins(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListOrganizationAdmins(r.Context(), orgID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.OrganizationAdmin]{Items: items})
}

func (h *Handlers) CreateOrganizationAdmin(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	var body models.CreateOrganizationAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.UserID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "user_id is required")
		return
	}
	admin, err := h.Repo.UpsertOrganizationAdmin(r.Context(), orgID, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, admin)
}

func (h *Handlers) DeleteOrganizationAdmin(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = "enrollment_admin"
	}
	if err := h.Repo.DeleteOrganizationAdmin(r.Context(), orgID, userID, scope); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Guests ────────────────────────────────────────────────────────────

func (h *Handlers) ListOrganizationGuests(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListOrganizationGuests(r.Context(), orgID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.OrganizationGuest]{Items: items})
}

func (h *Handlers) CreateOrganizationGuest(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	var body models.CreateOrganizationGuestRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.UserID == uuid.Nil || body.PrimaryOrganizationID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "user_id and primary_organization_id are required")
		return
	}
	if body.PrimaryOrganizationID == orgID {
		writeJSONErr(w, http.StatusBadRequest, "guest primary_organization_id must differ from the host organization")
		return
	}
	guest, err := h.Repo.UpsertOrganizationGuest(r.Context(), orgID, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, guest)
}

func (h *Handlers) DeleteOrganizationGuest(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	if err := h.Repo.DeleteOrganizationGuest(r.Context(), orgID, userID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Tenancy spaces (Foundry-style) ────────────────────────────────────

func (h *Handlers) ListTenancySpaces(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListTenancySpaces(r.Context(), orgID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.TenancySpace]{Items: items})
}

func (h *Handlers) CreateTenancySpace(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	var body models.CreateTenancySpaceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Slug) == "" || strings.TrimSpace(body.DisplayName) == "" {
		writeJSONErr(w, http.StatusBadRequest, "space slug and display name are required")
		return
	}
	space, err := h.Repo.CreateTenancySpace(r.Context(), orgID, &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, space)
}

func (h *Handlers) GetTenancySpace(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	s, err := h.Repo.GetTenancySpace(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeJSONErr(w, http.StatusNotFound, "space not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) UpdateTenancySpace(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body models.UpdateTenancySpaceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	s, err := h.Repo.UpdateTenancySpace(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeJSONErr(w, http.StatusNotFound, "space not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) DeleteTenancySpace(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteTenancySpace(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Organization membership probe ─────────────────────────────────────

// CheckOrganizationMembership lets clients ask whether the
// authenticated caller (or an explicit user_id query param) is a
// member of `{id}`. Returns 200 with a small JSON envelope.
//
// Wire shape:
//
//	{
//	  "organization_id": "…",
//	  "user_id": "…",
//	  "is_member": true,
//	  "is_admin": false
//	}
//
// SG.2 requires "enforce organization membership before resource
// discovery and access" — this endpoint is the lookup other services
// can call when their own claims-based check (claims.AllowsOrgID) is
// not enough because they need to consult the persistent admin/guest
// tables.
func (h *Handlers) CheckOrganizationMembership(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseOrgID(w, r)
	if !ok {
		return
	}
	userID, err := resolveUserID(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	isMember, err := h.Repo.IsOrganizationMember(r.Context(), orgID, userID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	isAdmin, err := h.Repo.IsOrganizationAdmin(r.Context(), orgID, userID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"organization_id": orgID,
		"user_id":         userID,
		"is_member":       isMember,
		"is_admin":        isAdmin,
	})
}

// ─── helpers ───────────────────────────────────────────────────────────

// parseOrgID is the chi-aware id parser for the {id} URL param that
// the organizations.* routes use. It exists separately from parseID
// for readability — the body of the route is interpreted as an
// organization, not a generic id.
func parseOrgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid organization id")
		return uuid.Nil, false
	}
	return id, true
}

// resolveUserID picks the user id to check membership for. Either:
//   - ?user_id=… (admins probing on behalf of someone else), or
//   - the authenticated subject in the JWT claims.
func resolveUserID(r *http.Request) (uuid.UUID, error) {
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, errors.New("invalid user_id")
		}
		return id, nil
	}
	claims, ok := authmw.FromContext(r.Context())
	if !ok || claims == nil {
		return uuid.Nil, errors.New("user_id query parameter required for anonymous calls")
	}
	return claims.Sub, nil
}
