package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/models"
)

func requirePermission(w http.ResponseWriter, r *http.Request, resource, action string) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if !claims.HasPermission(resource, action) {
		writeJSONErr(w, http.StatusForbidden, "missing permission "+resource+":"+action)
		return nil, false
	}
	return claims, true
}

func tenantFromClaims(c *authmw.Claims) *uuid.UUID { return c.OrgID }

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, name+" must be a uuid")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "permissions", "read")
	if !ok {
		return
	}
	items, err := h.Repo.ListPermissions(r.Context(), tenantFromClaims(claims))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list permissions")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Permission]{Items: items})
}

func (h *Handlers) CreatePermission(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "permissions", "write")
	if !ok {
		return
	}
	var body models.CreatePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Resource = strings.TrimSpace(body.Resource)
	body.Action = strings.TrimSpace(body.Action)
	if body.Resource == "" || body.Action == "" {
		writeJSONErr(w, http.StatusBadRequest, "resource and action required")
		return
	}
	p, err := h.Repo.CreatePermission(r.Context(), tenantFromClaims(claims), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "read")
	if !ok {
		return
	}
	items, err := h.Repo.ListRoles(r.Context(), tenantFromClaims(claims))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.RoleResponse]{Items: items})
}
func (h *Handlers) GetRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "read")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	item, err := h.Repo.GetRole(r.Context(), tenantFromClaims(claims), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeJSONErr(w, http.StatusNotFound, "role not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "write")
	if !ok {
		return
	}
	var body models.CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "name required")
		return
	}
	item, err := h.Repo.CreateRole(r.Context(), tenantFromClaims(claims), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "write")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var body models.UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	item, err := h.Repo.UpdateRole(r.Context(), tenantFromClaims(claims), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeJSONErr(w, http.StatusNotFound, "role not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "write")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	deleted, err := h.Repo.DeleteRole(r.Context(), tenantFromClaims(claims), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "role not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) AssignRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "write")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var body models.AssignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := h.Repo.AssignRole(r.Context(), tenantFromClaims(claims), userID, body.RoleID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONErr(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handlers) RemoveRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "roles", "write")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	roleID, ok := parseUUIDParam(w, r, "role_id")
	if !ok {
		return
	}
	err := h.Repo.RemoveRole(r.Context(), tenantFromClaims(claims), userID, roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONErr(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "read")
	if !ok {
		return
	}
	items, err := h.Repo.ListGroups(r.Context(), tenantFromClaims(claims))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.GroupResponse]{Items: items})
}
func (h *Handlers) GetGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "read")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	item, err := h.Repo.GetGroup(r.Context(), tenantFromClaims(claims), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeJSONErr(w, http.StatusNotFound, "group not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "write")
	if !ok {
		return
	}
	var body models.CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "name required")
		return
	}
	item, err := h.Repo.CreateGroup(r.Context(), tenantFromClaims(claims), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "write")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var body models.UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	item, err := h.Repo.UpdateGroup(r.Context(), tenantFromClaims(claims), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeJSONErr(w, http.StatusNotFound, "group not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "write")
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	deleted, err := h.Repo.DeleteGroup(r.Context(), tenantFromClaims(claims), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "group not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handlers) AddUserToGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "write")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	var body models.UserGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := h.Repo.AddGroupMember(r.Context(), tenantFromClaims(claims), userID, body.GroupID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONErr(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handlers) RemoveUserFromGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := requirePermission(w, r, "groups", "write")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	groupID, ok := parseUUIDParam(w, r, "group_id")
	if !ok {
		return
	}
	err := h.Repo.RemoveGroupMember(r.Context(), tenantFromClaims(claims), userID, groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONErr(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
