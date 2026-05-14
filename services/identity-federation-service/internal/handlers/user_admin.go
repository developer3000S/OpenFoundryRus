// user_admin.go: SG.4 — user-administration endpoints layered on top
// of the existing RBAC user CRUD.
//
// Adds:
//   - GET    /api/v1/users (with q / organization_id / realm / status /
//                          include_deleted / limit / offset query params,
//                          envelope response with `items` + `total`)
//   - POST   /api/v1/users/preregister
//   - POST   /api/v1/users/{id}/restore (undelete)
//   - POST   /api/v1/users/{id}/revoke-tokens
//   - GET    /api/v1/users/{id}/inspect (combined view)
//
// The original GET /users (bare-array response) stays mounted on
// the existing route surface; the SG.4 envelope is mounted under
// /api/v1/users with the same path but consults the query string to
// decide which response shape to return.

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

// SearchUsers implements GET /api/v1/users with the SG.4 filter +
// pagination envelope. Returns 200 with `{items, total}`. The filter
// shape is parsed from the query string:
//
//	q                — case-insensitive substring on email, username, name.
//	organization_id  — UUID; exact match.
//	realm            — exact match.
//	status           — "active" | "inactive".
//	include_deleted  — "true" to surface soft-deleted rows.
//	limit            — 1..200 (default 50).
//	offset           — non-negative integer (default 0).
func (h *RBAC) SearchUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := &models.ListUsersFilter{
		Query:          strings.TrimSpace(q.Get("q")),
		Realm:          strings.TrimSpace(q.Get("realm")),
		Status:         strings.TrimSpace(q.Get("status")),
		IncludeDeleted: q.Get("include_deleted") == "true",
		Limit:          parseIntDefault(q.Get("limit"), 50),
		Offset:         parseIntDefault(q.Get("offset"), 0),
	}
	if raw := strings.TrimSpace(q.Get("organization_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid organization_id")
			return
		}
		filter.OrganizationID = &parsed
	}
	if filter.Status != "" && filter.Status != "active" && filter.Status != "inactive" {
		writeJSONErr(w, http.StatusBadRequest, "status must be 'active' or 'inactive'")
		return
	}

	users, total, err := h.Repo.ListUsersFiltered(r.Context(), filter)
	if err != nil {
		slog.Error("search users", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, models.ListUsersResponse{Items: users, Total: total})
}

// PreregisterUser implements POST /api/v1/users/preregister. The
// caller must hold the admin role (router-side `RequireAdmin`).
//
// SG.4: an admin can seed a user before they have a password / SSO
// binding. The row starts with `preregistered=true`, an empty
// password hash, and the admin's UUID as invited_by. When the user
// completes self-service registration or an SSO callback, the
// existing user-resolution policy promotes the row.
func (h *RBAC) PreregisterUser(w http.ResponseWriter, r *http.Request) {
	var body models.PreregisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Email) == "" || strings.TrimSpace(body.Name) == "" {
		writeJSONErr(w, http.StatusBadRequest, "email and name are required")
		return
	}
	invitedBy := authCallerID(r)
	if invitedBy == uuid.Nil {
		writeJSONErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.Repo.PreregisterUser(r.Context(), invitedBy, &body)
	if err != nil {
		slog.Error("preregister user", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// RestoreUser implements POST /api/v1/users/{id}/restore. Clears the
// soft-delete tombstone. The user stays inactive until an admin
// explicitly re-activates via PATCH /users/{id}.
func (h *RBAC) RestoreUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.UndeleteUser(r.Context(), id); err != nil {
		slog.Error("undelete user", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	u, err := h.Repo.FindUserByID(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if u == nil {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// RevokeUserTokens implements POST /api/v1/users/{id}/revoke-tokens.
// Explicit admin action: marks every active refresh token revoked.
// Useful when an account is compromised but soft-delete is too heavy.
func (h *RBAC) RevokeUserTokens(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	count, err := h.Repo.RevokeAllUserRefreshTokens(r.Context(), id, time.Now().UTC())
	if err != nil {
		slog.Error("revoke user tokens", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": id,
		"revoked": count,
	})
}

// InspectUser implements GET /api/v1/users/{id}/inspect. SG.4: returns
// the combined view used by the admin UI — user core + role names +
// group projection + token summary + external IdP bindings.
func (h *RBAC) InspectUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	user, err := h.Repo.FindUserByID(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	roles, err := h.Repo.ListUserRoles(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "list roles: "+err.Error())
		return
	}
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}
	groups, err := h.Repo.ListUserGroups(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "list groups: "+err.Error())
		return
	}
	groupBriefs := make([]models.GroupBrief, 0, len(groups))
	for _, g := range groups {
		groupBriefs = append(groupBriefs, models.GroupBrief{ID: g.ID, Name: g.Name})
	}
	tokens, err := h.Repo.SummarizeUserTokens(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "token summary: "+err.Error())
		return
	}
	apiKeys, err := h.Repo.CountActiveAPIKeys(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "api keys: "+err.Error())
		return
	}
	idents, err := h.Repo.ListUserExternalIdentities(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "list external identities: "+err.Error())
		return
	}
	externals := make([]models.ExternalBinding, 0, len(idents))
	for _, ei := range idents {
		externals = append(externals, models.ExternalBinding{
			Provider:    ei.Provider,
			ExternalID:  ei.ExternalID,
			Email:       ei.Email,
			LastLoginAt: ei.LastLoginAt,
			CreatedAt:   ei.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, models.UserInspection{
		User:               *user,
		Roles:              roleNames,
		Groups:             groupBriefs,
		Tokens:             models.TokenSummary{
			ActiveCount:   tokens.ActiveCount,
			RevokedCount:  tokens.RevokedCount,
			NextExpiresAt: tokens.NextExpiresAt,
			APIKeysActive: apiKeys,
		},
		ExternalIdentities: externals,
	})
}

func parseIntDefault(v string, def int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
