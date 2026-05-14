package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

// SG.4 wire-format: User gains username, realm, last_login_at /
// last_login_ip, preregistered/invited_by and deleted_at. Pinned so
// a future schema change fails loudly.
func TestUserSG4WireShape(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	username := "alice"
	ip := "10.0.0.5"
	invitedBy := uuid.New()
	u := models.User{
		ID: uuid.New(), Email: "alice@example.com", Username: &username, Name: "Alice",
		IsActive:    true,
		AuthSource:  "local",
		Realm:       "local",
		LastLoginAt: &now,
		LastLoginIP: &ip,
		Preregistered: true,
		InvitedBy:   &invitedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	out, err := json.Marshal(u)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "email", "username", "name", "is_active", "auth_source", "realm",
		"last_login_at", "last_login_ip", "preregistered", "invited_by",
		"created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
	// password_hash must NEVER serialise.
	assert.NotContains(t, view, "password_hash")
}

func TestUserInspectionWireShape(t *testing.T) {
	t.Parallel()
	u := models.User{ID: uuid.New(), Email: "a@example.com", Name: "A", IsActive: true, Realm: "local"}
	insp := models.UserInspection{
		User:  u,
		Roles: []string{"viewer"},
		Groups: []models.GroupBrief{
			{ID: uuid.New(), Name: "eng"},
		},
		Tokens: models.TokenSummary{
			ActiveCount:   2,
			RevokedCount:  3,
			APIKeysActive: 1,
		},
		ExternalIdentities: []models.ExternalBinding{},
	}
	out, err := json.Marshal(insp)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{"user", "roles", "groups", "tokens", "external_identities"} {
		assert.Contains(t, view, k)
	}
	tokens := view["tokens"].(map[string]any)
	assert.Equal(t, float64(2), tokens["active_count"])
	assert.Equal(t, float64(3), tokens["revoked_count"])
	assert.Equal(t, float64(1), tokens["api_keys_active"])
}

func TestListUsersResponseEnvelope(t *testing.T) {
	t.Parallel()
	resp := models.ListUsersResponse{Items: []models.User{}, Total: 0}
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	assert.Contains(t, view, "items")
	assert.Contains(t, view, "total")
}

// SG.4 validation paths that don't touch the DB.

func TestSearchUsersRejectsBadStatus(t *testing.T) {
	t.Parallel()
	h := &handlers.RBAC{}
	req := httptest.NewRequest(http.MethodGet, "/users/search?status=banned", nil)
	rec := httptest.NewRecorder()
	h.SearchUsers(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "status")
}

func TestSearchUsersRejectsBadOrgID(t *testing.T) {
	t.Parallel()
	h := &handlers.RBAC{}
	req := httptest.NewRequest(http.MethodGet, "/users/search?organization_id=not-a-uuid", nil)
	rec := httptest.NewRecorder()
	h.SearchUsers(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "organization_id")
}

func TestPreregisterUserRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	h := &handlers.RBAC{}
	req := httptest.NewRequest(http.MethodPost, "/users/preregister", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.PreregisterUser(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "email")
}

func TestPreregisterUserRequiresAuthenticatedAdmin(t *testing.T) {
	t.Parallel()
	h := &handlers.RBAC{}
	body, _ := json.Marshal(models.PreregisterUserRequest{
		Email: "new@example.com",
		Name:  "New User",
	})
	req := httptest.NewRequest(http.MethodPost, "/users/preregister", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h.PreregisterUser(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
