package handlers_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/models"
)

func TestRoleCRUDWireShape(t *testing.T) {
	t.Parallel()
	desc := "data admins"
	permID := uuid.New()
	role := models.RoleResponse{ID: uuid.New(), Name: "data-admin", Description: &desc, CreatedAt: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), PermissionIDs: []uuid.UUID{permID}, Permissions: []string{"datasets:read"}}
	out, err := json.Marshal(role)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, key := range []string{"id", "name", "description", "created_at", "permission_ids", "permissions"} {
		assert.Contains(t, view, key)
	}
}

func TestPermissionCatalogWireShape(t *testing.T) {
	t.Parallel()
	desc := "read policies"
	permission := models.Permission{ID: uuid.New(), Resource: "policies", Action: "read", Description: &desc, CreatedAt: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)}
	out, err := json.Marshal(permission)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, key := range []string{"id", "resource", "action", "description", "created_at"} {
		assert.Contains(t, view, key)
	}
}

func TestGroupMembershipAndRoleGrantWireShape(t *testing.T) {
	t.Parallel()
	roleID := uuid.New()
	group := models.GroupResponse{ID: uuid.New(), Name: "analytics", CreatedAt: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), MemberCount: 3, RoleIDs: []uuid.UUID{roleID}, Roles: []string{"data-reader"}}
	out, err := json.Marshal(group)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, key := range []string{"id", "name", "created_at", "member_count", "role_ids", "roles"} {
		assert.Contains(t, view, key)
	}
}

func TestRoleCRUDRequiresPermission(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	claims := &authmw.Claims{Sub: uuid.New(), Permissions: []string{"roles:read"}}
	req := httptest.NewRequest("POST", "/roles", strings.NewReader(`{"name":"admin"}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), claims))
	rec := httptest.NewRecorder()
	h.CreateRole(rec, req)
	assert.Equal(t, 403, rec.Code)
	assert.Contains(t, rec.Body.String(), "roles:write")
}

func TestPermissionCatalogRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("GET", "/permissions", nil)
	rec := httptest.NewRecorder()
	h.ListPermissions(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestGroupMembershipRequiresWritePermission(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	claims := &authmw.Claims{Sub: uuid.New(), Permissions: []string{"groups:read"}}
	req := httptest.NewRequest("POST", "/users/"+uuid.NewString()+"/groups", strings.NewReader(`{"group_id":"`+uuid.NewString()+`"}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), claims))
	rec := httptest.NewRecorder()
	h.AddUserToGroup(rec, req)
	assert.Equal(t, 403, rec.Code)
}

func TestTenantIDSerializedOnRBACResponses(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	role := models.RoleResponse{ID: uuid.New(), TenantID: &tenantID, Name: "tenant-admin", CreatedAt: time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)}
	out, err := json.Marshal(role)
	require.NoError(t, err)
	assert.Contains(t, string(out), tenantID.String())
}
