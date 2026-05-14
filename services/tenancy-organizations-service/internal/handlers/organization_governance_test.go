package handlers_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
)

// SG.2 wire-format: Organization now exposes description, contact_email,
// metadata, settings and quotas alongside the existing fields. The test
// pins the snake_case keys so a future schema change fails loudly.
func TestOrganizationSG2WireShape(t *testing.T) {
	t.Parallel()
	desc := "Acme tenancy."
	contact := "ops@acme.example"
	dw := "default-ws"
	tier := "enterprise"
	o := models.Organization{
		ID: uuid.New(), Slug: "acme", DisplayName: "Acme",
		Description: desc, ContactEmail: &contact,
		OrganizationType: "enterprise", DefaultWorkspace: &dw, TenantTier: &tier,
		Status:    "active",
		Metadata:  map[string]any{"region": "eu-west-1"},
		Settings:  map[string]any{"scoped_sessions_enabled": true},
		Quotas:    map[string]any{"max_users": 250.0},
		CreatedAt: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(o)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "slug", "display_name", "description", "contact_email",
		"organization_type", "default_workspace", "tenant_tier", "status",
		"metadata", "settings", "quotas", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
	// Nested object keys round-trip correctly.
	meta, ok := view["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", meta["region"])
	settings, ok := view["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, settings["scoped_sessions_enabled"])
}

func TestOrganizationAdminWireShape(t *testing.T) {
	t.Parallel()
	granter := uuid.New()
	a := models.OrganizationAdmin{
		OrganizationID: uuid.New(), UserID: uuid.New(),
		Scope: "enrollment_admin", GrantedBy: &granter,
		CreatedAt: time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(a)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{"organization_id", "user_id", "scope", "granted_by", "created_at"} {
		assert.Contains(t, view, k)
	}
}

func TestOrganizationGuestWireShape(t *testing.T) {
	t.Parallel()
	g := models.OrganizationGuest{
		OrganizationID: uuid.New(), UserID: uuid.New(),
		PrimaryOrganizationID: uuid.New(),
		Status:                "active",
		CreatedAt:             time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
		UpdatedAt:             time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(g)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"organization_id", "user_id", "primary_organization_id",
		"status", "invited_by", "expires_at", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

func TestTenancySpaceWireShape(t *testing.T) {
	t.Parallel()
	s := models.TenancySpace{
		ID: uuid.New(), OrganizationID: uuid.New(),
		Slug: "internal", DisplayName: "Internal",
		Description: "Default internal space",
		Settings:    map[string]any{"retention_default_days": 90.0},
		Quotas:      map[string]any{"max_projects": 100.0},
		Status:      "active",
		CreatedAt:   time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(s)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "organization_id", "slug", "display_name", "description",
		"settings", "quotas", "status", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

// Validation paths that don't touch the DB.

func TestCreateOrganizationAdminRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("POST", "/organizations/00000000-0000-0000-0000-000000000001/admins", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(chiCtxWithParam("id", "00000000-0000-0000-0000-000000000001"))
	rec := httptest.NewRecorder()
	h.CreateOrganizationAdmin(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "user_id")
}

func TestCreateOrganizationGuestRejectsSamePrimaryOrg(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	orgID := uuid.New()
	body, _ := json.Marshal(models.CreateOrganizationGuestRequest{
		UserID:                uuid.New(),
		PrimaryOrganizationID: orgID,
	})
	req := httptest.NewRequest("POST", "/organizations/"+orgID.String()+"/guests", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(chiCtxWithParam("id", orgID.String()))
	rec := httptest.NewRecorder()
	h.CreateOrganizationGuest(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "primary_organization_id must differ")
}

func TestCreateTenancySpaceRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	orgID := uuid.New()
	req := httptest.NewRequest("POST", "/organizations/"+orgID.String()+"/spaces", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(chiCtxWithParam("id", orgID.String()))
	rec := httptest.NewRecorder()
	h.CreateTenancySpace(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "slug")
}

// chiCtxWithParam wraps a request context with a chi RouteContext that
// has the given URL parameter set — needed because we call handlers
// directly without a chi mux in these unit tests.
func chiCtxWithParam(key, value string) ctxWith {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return ctxWith{rctx: rctx}
}

type ctxWith struct {
	rctx *chi.Context
}

func (c ctxWith) Deadline() (time.Time, bool)              { return time.Time{}, false }
func (c ctxWith) Done() <-chan struct{}                    { return nil }
func (c ctxWith) Err() error                               { return nil }
func (c ctxWith) Value(key any) any {
	if key == chi.RouteCtxKey {
		return c.rctx
	}
	return nil
}
