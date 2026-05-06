package handlers_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
)

// Wire-format: the Organization JSON shape must match the Rust crate
// (snake_case, NO HTML camelCase). Tests below are pinned so a future
// change to JSON tags fails loudly.
func TestOrganizationJSONShape(t *testing.T) {
	t.Parallel()
	dw := "default-ws"
	tier := "enterprise"
	o := models.Organization{
		ID: uuid.New(), Slug: "acme", DisplayName: "Acme",
		OrganizationType: "enterprise", DefaultWorkspace: &dw, TenantTier: &tier,
		Status: "active",
		CreatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(o)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "slug", "display_name", "organization_type",
		"default_workspace", "tenant_tier", "status", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

func TestEnrollmentJSONShape(t *testing.T) {
	t.Parallel()
	ws := "team-a"
	e := models.Enrollment{
		ID: uuid.New(), OrganizationID: uuid.New(), UserID: uuid.New(),
		WorkspaceSlug: &ws, RoleSlug: "viewer", Status: "active",
		CreatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(e)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "organization_id", "user_id", "workspace_slug",
		"role_slug", "status", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

// ListResponse[T] envelope must always carry an "items" field, never
// "data" or "results", to keep wire-compat with the Rust crate.
func TestListResponseEnvelope(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(models.ListResponse[models.Organization]{Items: []models.Organization{}})
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	assert.Contains(t, view, "items")
}

// Validation paths that don't touch the DB.
func TestCreateOrganizationRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.CreateOrganization(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "slug")
}

func TestCreateEnrollmentRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/enrollments", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.CreateEnrollment(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "role_slug")
}

func TestCreateOrganizationRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(`not-json`))
	h.CreateOrganization(rec, req)
	assert.Equal(t, 400, rec.Code)
}
