package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
)

// RBAC wires the management endpoints (slice 6).
//
// All handlers are bearer-JWT-protected at the router; admin-role
// enforcement is the gateway's job (`x-openfoundry-tenant-tier=enterprise`
// + role checks). Slice 8 layers Cedar policy enforcement on top.
type RBAC struct{ Repo *repo.Repo }

// ─── Users ──────────────────────────────────────────────────────────────

func (h *RBAC) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Repo.ListUsers(r.Context())
	if err != nil {
		slog.Error("list users", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *RBAC) GetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
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

func (h *RBAC) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body models.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, err := h.Repo.UpdateUser(r.Context(), id, &body)
	if err != nil {
		slog.Error("update user", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if u == nil {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *RBAC) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteUser(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) ListUserRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	roles, err := h.Repo.ListUserRoles(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *RBAC) AssignUserRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseID(w, r)
	if !ok {
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "role_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid role id")
		return
	}
	if err := h.Repo.AssignRoleToUser(r.Context(), userID, roleID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) RevokeUserRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseID(w, r)
	if !ok {
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "role_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid role id")
		return
	}
	if err := h.Repo.RevokeRoleFromUser(r.Context(), userID, roleID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Roles ──────────────────────────────────────────────────────────────

func (h *RBAC) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.Repo.ListRoles(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *RBAC) GetRole(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	role, err := h.Repo.GetRole(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if role == nil {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *RBAC) CreateRole(w http.ResponseWriter, r *http.Request) {
	var body models.CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "name required")
		return
	}
	role, err := h.Repo.CreateRole(r.Context(), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *RBAC) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body models.UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	role, err := h.Repo.UpdateRole(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *RBAC) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteRole(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Permissions ────────────────────────────────────────────────────────

func (h *RBAC) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.Repo.ListPermissions(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, perms)
}

func (h *RBAC) CreatePermission(w http.ResponseWriter, r *http.Request) {
	var body models.CreatePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Resource == "" || body.Action == "" {
		writeJSONErr(w, http.StatusBadRequest, "resource and action required")
		return
	}
	p, err := h.Repo.CreatePermission(r.Context(), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *RBAC) DeletePermission(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeletePermission(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) AssignRolePermission(w http.ResponseWriter, r *http.Request) {
	roleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid role id")
		return
	}
	permID, err := uuid.Parse(chi.URLParam(r, "permission_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid permission id")
		return
	}
	if err := h.Repo.AssignPermissionToRole(r.Context(), roleID, permID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) RevokeRolePermission(w http.ResponseWriter, r *http.Request) {
	roleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid role id")
		return
	}
	permID, err := uuid.Parse(chi.URLParam(r, "permission_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid permission id")
		return
	}
	if err := h.Repo.RevokePermissionFromRole(r.Context(), roleID, permID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Groups ─────────────────────────────────────────────────────────────

func (h *RBAC) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.Repo.ListGroups(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *RBAC) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	g, err := h.Repo.GetGroup(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if g == nil {
		writeJSONErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *RBAC) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body models.CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "name required")
		return
	}
	g, err := h.Repo.CreateGroup(r.Context(), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *RBAC) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var body models.UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := h.Repo.UpdateGroup(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *RBAC) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.DeleteGroup(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	groupID, ok := parseID(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.Repo.AddGroupMember(r.Context(), groupID, userID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RBAC) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	groupID, ok := parseID(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.Repo.RemoveGroupMember(r.Context(), groupID, userID); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── API keys ───────────────────────────────────────────────────────────

func (h *RBAC) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	c := authCallerID(r)
	keys, err := h.Repo.ListAPIKeys(r.Context(), c)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (h *RBAC) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	c := authCallerID(r)
	var body models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "name required")
		return
	}
	plaintext, err := newAPIKeyPlaintext()
	if err != nil {
		slog.Error("api key mint", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	hash := hashAPIKey(plaintext)
	k, err := h.Repo.CreateAPIKey(r.Context(), c, body.Name, hash, body.ExpiresAt)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, models.CreateAPIKeyResponse{APIKey: *k, Token: plaintext})
}

func (h *RBAC) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	c := authCallerID(r)
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Repo.RevokeAPIKey(r.Context(), c, id, time.Now().UTC()); err != nil {
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

// authCallerID extracts the JWT subject from the bearer middleware.
func authCallerID(r *http.Request) uuid.UUID {
	if c, ok := authmw.FromContext(r.Context()); ok {
		return c.Sub
	}
	return uuid.Nil
}

// newAPIKeyPlaintext returns a 32-byte URL-safe base64 token.
func newAPIKeyPlaintext() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ofapikey_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
