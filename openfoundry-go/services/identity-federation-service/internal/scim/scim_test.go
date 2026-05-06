package scim_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/scim"
)

const baseURL = "https://id.example.test"

// --- hardening/scim.rs round-trip parity tests --------------------------

func TestScimUserRoundTripsScimFieldNames(t *testing.T) {
	t.Parallel()
	given := "Alice"
	family := "Doe"
	primary := true
	emailType := "work"
	active := true
	user := scim.ScimUser{
		Schemas: []string{scim.SchemaUser},
		UserName: "alice@acme.com",
		Name: &scim.ScimName{GivenName: &given, FamilyName: &family},
		Emails: []scim.ScimEmail{
			{Value: "alice@acme.com", Primary: &primary, Type: &emailType},
		},
		Active: &active,
	}
	raw, err := json.Marshal(user)
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, "alice@acme.com", obj["userName"])
	assert.Equal(t, "Alice", obj["name"].(map[string]any)["givenName"])
	emails := obj["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "work", emails[0].(map[string]any)["type"])

	var back scim.ScimUser
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "alice@acme.com", back.UserName)
}

func TestScimUserExtensionsFlatten(t *testing.T) {
	t.Parallel()
	ext := json.RawMessage(`{"organizationId":"org-123"}`)
	user := scim.ScimUser{
		Schemas:  []string{scim.SchemaUser},
		UserName: "bob",
		Extensions: map[string]json.RawMessage{
			scim.SchemaOpenfoundryUserExtension: ext,
		},
	}
	raw, err := json.Marshal(user)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	require.Contains(t, obj, scim.SchemaOpenfoundryUserExtension)

	// And round-trip back.
	var back scim.ScimUser
	require.NoError(t, json.Unmarshal(raw, &back))
	require.NotNil(t, back.Extensions)
	assert.JSONEq(t, string(ext), string(back.Extensions[scim.SchemaOpenfoundryUserExtension]))
}

func TestScimErrorContractIncludesScimType(t *testing.T) {
	t.Parallel()
	scimType := "invalidFilter"
	err := scim.NewScimError(400, "unsupported filter", &scimType)
	raw, e := json.Marshal(err)
	require.NoError(t, e)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, scim.SchemaError, obj["schemas"].([]any)[0])
	assert.Equal(t, "400", obj["status"])
	assert.Equal(t, "invalidFilter", obj["scimType"])
}

func TestUnsupportedErrorIsMutability(t *testing.T) {
	t.Parallel()
	err := scim.NewUnsupportedError("custom field is read-only")
	require.NotNil(t, err.ScimType)
	assert.Equal(t, "mutability", *err.ScimType)
	assert.Equal(t, "400", err.Status)
}

func TestServiceProviderConfigAdvertisesContract(t *testing.T) {
	t.Parallel()
	cfg := scim.ServiceProviderConfigPayload(baseURL)
	assert.True(t, cfg.Patch.Supported)
	assert.True(t, cfg.Filter.Supported)
	assert.Equal(t, 500, cfg.Filter.MaxResults)
	assert.False(t, cfg.Bulk.Supported)
	assert.False(t, cfg.ChangePassword.Supported)
	require.Len(t, cfg.AuthenticationSchemes, 1)
	assert.Equal(t, "oauthbearertoken", cfg.AuthenticationSchemes[0].Type)
	assert.Equal(t, baseURL+scim.RouteServiceProviderConfig, cfg.Meta.Location)
}

func TestSchemaAndResourceTypesMetadata(t *testing.T) {
	t.Parallel()
	schemas := scim.SchemaResources(baseURL)
	hasUser := false
	hasGroup := false
	for _, s := range schemas {
		if s.ID == scim.SchemaUser {
			hasUser = true
		}
		if s.ID == scim.SchemaGroup {
			hasGroup = true
		}
	}
	assert.True(t, hasUser && hasGroup)

	rts := scim.ResourceTypes(baseURL)
	require.Len(t, rts, 2)
	assert.Equal(t, scim.RouteUsers, rts[0].Endpoint)
	assert.Equal(t, scim.RouteGroups, rts[1].Endpoint)
}

func TestListResponseUsesResourcesKey(t *testing.T) {
	t.Parallel()
	resp := scim.NewScimList([]scim.ScimResourceType{{
		Schemas:  []string{scim.SchemaResourceType},
		ID:       "User",
		Name:     "User",
		Endpoint: scim.RouteUsers,
		Schema:   scim.SchemaUser,
		Meta:     scim.ScimMeta{ResourceType: "ResourceType", Location: "/scim/v2/ResourceTypes/User"},
	}}, 1, 1)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(raw, &obj))
	assert.Equal(t, scim.SchemaListResponse, obj["schemas"].([]any)[0])
	assert.Equal(t, float64(1), obj["totalResults"])
	assert.Equal(t, float64(1), obj["itemsPerPage"])
	resources := obj["Resources"].([]any)
	require.Len(t, resources, 1)
	assert.Equal(t, scim.SchemaUser, resources[0].(map[string]any)["schema"])
}

// --- Filter parser ------------------------------------------------------

func TestParseEqFilterUserNameMatches(t *testing.T) {
	t.Parallel()
	expr := `userName eq "alice@acme.com"`
	got, err := scim.ParseEqFilter(&expr, []string{"userName", "externalId"})
	require.Nil(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scim.FilterUserName, got.Attribute)
	assert.Equal(t, "alice@acme.com", got.Value)
}

func TestParseEqFilterExternalIDMatches(t *testing.T) {
	t.Parallel()
	expr := `EXTERNALID eq "scim-42"`
	got, err := scim.ParseEqFilter(&expr, []string{"userName", "externalId"})
	require.Nil(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scim.FilterExternalID, got.Attribute)
	assert.Equal(t, "scim-42", got.Value)
}

func TestParseEqFilterUnsupportedAttributeRejected(t *testing.T) {
	t.Parallel()
	expr := `email eq "alice@acme.com"`
	_, err := scim.ParseEqFilter(&expr, []string{"userName", "externalId"})
	require.NotNil(t, err)
	assert.Equal(t, "400", err.Status)
	require.NotNil(t, err.ScimType)
	assert.Equal(t, "invalidFilter", *err.ScimType)
	assert.Contains(t, err.Detail, "unsupported SCIM filter")
}

func TestParseEqFilterUnquotedRejected(t *testing.T) {
	t.Parallel()
	expr := `userName eq alice`
	_, err := scim.ParseEqFilter(&expr, []string{"userName"})
	require.NotNil(t, err)
	assert.Contains(t, err.Detail, "must be quoted")
}

func TestParseEqFilterEmptyReturnsNil(t *testing.T) {
	t.Parallel()
	for _, expr := range []*string{nil, ptr(""), ptr("   ")} {
		got, err := scim.ParseEqFilter(expr, []string{"userName"})
		require.Nil(t, err)
		assert.Nil(t, got)
	}
}

// --- UserToScim conversion ---------------------------------------------

func TestUserToScimBasicShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	rec := scim.UserRecord{
		ID:        id,
		Email:     "alice@acme.com",
		Name:      "Alice Doe",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}
	got := scim.UserToScim(rec, baseURL)
	require.NotNil(t, got.ID)
	assert.Equal(t, id.String(), *got.ID)
	assert.Equal(t, "alice@acme.com", got.UserName)
	require.NotNil(t, got.Active)
	assert.True(t, *got.Active)
	require.Len(t, got.Emails, 1)
	require.NotNil(t, got.Meta)
	assert.Equal(t, "User", got.Meta.ResourceType)
	assert.Equal(t, baseURL+scim.RouteUsers+"/"+id.String(), got.Meta.Location)
}

func TestUserToScimPullsStructuredNameFromAttributes(t *testing.T) {
	t.Parallel()
	rec := scim.UserRecord{
		ID:    uuid.New(),
		Email: "alice@acme.com",
		Name:  "Alice Doe",
		Attributes: json.RawMessage(`{
            "scim": {
                "name": {"givenName": "Alice", "familyName": "Doe"}
            }
        }`),
	}
	got := scim.UserToScim(rec, baseURL)
	require.NotNil(t, got.Name)
	require.NotNil(t, got.Name.GivenName)
	assert.Equal(t, "Alice", *got.Name.GivenName)
	require.NotNil(t, got.Name.FamilyName)
	assert.Equal(t, "Doe", *got.Name.FamilyName)
}

func TestUserToScimFallsBackToFormattedName(t *testing.T) {
	t.Parallel()
	rec := scim.UserRecord{
		ID:    uuid.New(),
		Email: "alice@acme.com",
		Name:  "Alice Doe",
	}
	got := scim.UserToScim(rec, baseURL)
	require.NotNil(t, got.Name)
	require.NotNil(t, got.Name.Formatted)
	assert.Equal(t, "Alice Doe", *got.Name.Formatted)
	assert.Nil(t, got.Name.GivenName)
}

func TestUserToScimSynthesisesOpenfoundryExtensionFromOrg(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	rec := scim.UserRecord{
		ID:             uuid.New(),
		Email:          "alice@acme.com",
		Name:           "Alice",
		OrganizationID: &org,
	}
	got := scim.UserToScim(rec, baseURL)
	ext, ok := got.Extensions[scim.SchemaOpenfoundryUserExtension]
	require.True(t, ok, "extension must be synthesised when org_id is present")
	var obj map[string]string
	require.NoError(t, json.Unmarshal(ext, &obj))
	assert.Equal(t, org.String(), obj["organizationId"])
}

func TestUserToScimRespectsExistingExtensionInAttributes(t *testing.T) {
	t.Parallel()
	rec := scim.UserRecord{
		ID:    uuid.New(),
		Email: "alice@acme.com",
		Name:  "Alice",
		Attributes: json.RawMessage(`{
            "scim": {
                "openfoundry": {"organizationId":"explicit-org","department":"Eng"}
            }
        }`),
	}
	got := scim.UserToScim(rec, baseURL)
	ext, ok := got.Extensions[scim.SchemaOpenfoundryUserExtension]
	require.True(t, ok)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(ext, &obj))
	assert.Equal(t, "explicit-org", obj["organizationId"])
	assert.Equal(t, "Eng", obj["department"])
}

func TestUserToScimReadsExternalIDFromAttributes(t *testing.T) {
	t.Parallel()
	rec := scim.UserRecord{
		ID:    uuid.New(),
		Email: "alice@acme.com",
		Name:  "Alice",
		Attributes: json.RawMessage(`{"scim": {"externalId": "ext-7"}}`),
	}
	got := scim.UserToScim(rec, baseURL)
	require.NotNil(t, got.ExternalID)
	assert.Equal(t, "ext-7", *got.ExternalID)
}

// --- InMemoryUserStore --------------------------------------------------

func TestInMemoryStoreGetMissReturnsNil(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	got, err := s.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestInMemoryStoreListSortsCreatedAtDesc(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "first@x", Name: "First", CreatedAt: now.Add(-2 * time.Hour)})
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "newer@x", Name: "Newer", CreatedAt: now})
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "middle@x", Name: "Middle", CreatedAt: now.Add(-1 * time.Hour)})
	rows, total, err := s.List(context.Background(), nil, 1, 100)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, rows, 3)
	assert.Equal(t, "newer@x", rows[0].Email)
	assert.Equal(t, "middle@x", rows[1].Email)
	assert.Equal(t, "first@x", rows[2].Email)
}

func TestInMemoryStoreListPaginates(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		s.Insert(scim.UserRecord{
			ID:        uuid.New(),
			Email:     "u" + string(rune('a'+i)) + "@x",
			Name:      "u",
			CreatedAt: now.Add(time.Duration(-i) * time.Minute),
		})
	}
	page2, total, err := s.List(context.Background(), nil, 3, 2)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	require.Len(t, page2, 2)
}

func TestInMemoryStoreListAppliesUserNameFilter(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "alice@x", Name: "A"})
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "bob@x", Name: "B"})
	filter := &scim.EqFilter{Attribute: scim.FilterUserName, Value: "alice@x"}
	rows, total, err := s.List(context.Background(), filter, 1, 100)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "alice@x", rows[0].Email)
}

func TestInMemoryStoreListAppliesExternalIDFilter(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	ext := "scim-42"
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "alice@x", ScimExternalID: &ext})
	s.Insert(scim.UserRecord{ID: uuid.New(), Email: "bob@x"})
	rows, total, err := s.List(context.Background(),
		&scim.EqFilter{Attribute: scim.FilterExternalID, Value: ext}, 1, 100)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, "alice@x", rows[0].Email)
}

func TestInMemoryStoreGetByExternalID(t *testing.T) {
	t.Parallel()
	s := scim.NewInMemoryUserStore()
	ext := "ext-7"
	rec := scim.UserRecord{ID: uuid.New(), Email: "alice@x", ScimExternalID: &ext}
	s.Insert(rec)
	got, err := s.GetByExternalID(context.Background(), ext)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, rec.ID, got.ID)

	got, err = s.GetByExternalID(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// --- Discovery handlers -------------------------------------------------

func newRouter(h *scim.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Get(scim.RouteServiceProviderConfig, h.ServiceProviderConfigHandler)
	r.Get(scim.RouteSchemas, h.ListSchemas)
	r.Get(scim.RouteSchemas+"/{id}", h.GetSchema)
	r.Get(scim.RouteResourceTypes, h.ListResourceTypes)
	r.Get(scim.RouteResourceTypes+"/{id}", h.GetResourceType)
	r.Get(scim.RouteUsers+"/{id}", h.GetUser)
	r.Get(scim.RouteUsers, h.ListUsers)
	return r
}

func TestServiceProviderConfigHandler(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, scim.RouteServiceProviderConfig, nil)
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/scim+json")
	var cfg scim.ServiceProviderConfig
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&cfg))
	assert.True(t, cfg.Patch.Supported)
}

func TestListSchemasReturnsBoth(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, scim.RouteSchemas, nil)
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp scim.ScimListResponse[scim.ScimSchemaResource]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.TotalResults)
	assert.Equal(t, scim.SchemaListResponse, resp.Schemas[0])
}

func TestGetSchemaUserReturnsDescriptor(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, scim.RouteSchemas+"/"+scim.SchemaUser, nil)
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var schema scim.ScimSchemaResource
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&schema))
	assert.Equal(t, scim.SchemaUser, schema.ID)
	assert.Equal(t, "User", schema.Name)
}

func TestGetSchemaUnknownIs404(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, scim.RouteSchemas+"/urn:something:else", nil)
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListResourceTypesReturnsBoth(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, scim.RouteResourceTypes, nil)
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp scim.ScimListResponse[scim.ScimResourceType]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Resources, 2)
}

// --- GetUser ------------------------------------------------------------

func ctxAdmin(t *testing.T, req *http.Request) *http.Request {
	t.Helper()
	claims := &authmw.Claims{Sub: uuid.New(), Roles: []string{"admin"}}
	return req.WithContext(authmw.ContextWithClaims(req.Context(), claims))
}

func ctxNoPerms(t *testing.T, req *http.Request) *http.Request {
	t.Helper()
	claims := &authmw.Claims{Sub: uuid.New(), Roles: []string{"viewer"}}
	return req.WithContext(authmw.ContextWithClaims(req.Context(), claims))
}

func TestGetUserReturnsScimShape(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	user := scim.UserRecord{ID: uuid.New(), Email: "alice@x", Name: "Alice", IsActive: true}
	store.Insert(user)
	h := &scim.Handlers{BaseURL: baseURL, Users: store}

	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"/"+user.ID.String(), nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "got body=%s", rec.Body.String())
	var got scim.ScimUser
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.NotNil(t, got.ID)
	assert.Equal(t, user.ID.String(), *got.ID)
	assert.Equal(t, "alice@x", got.UserName)
}

func TestGetUserNotFoundIs404(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"/"+uuid.New().String(), nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	var err scim.ScimError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&err))
	assert.Contains(t, err.Detail, "not found")
}

func TestGetUserInvalidUUIDIs400(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"/not-a-uuid", nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetUserMissingClaimsIs401(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetUserNoPermissionsIs403(t *testing.T) {
	t.Parallel()
	h := &scim.Handlers{BaseURL: baseURL, Users: scim.NewInMemoryUserStore()}
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"/"+uuid.New().String(), nil)
	req = ctxNoPerms(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// --- ListUsers ----------------------------------------------------------

func TestListUsersHappyPath(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	store.Insert(scim.UserRecord{ID: uuid.New(), Email: "alice@x", Name: "Alice", IsActive: true})
	store.Insert(scim.UserRecord{ID: uuid.New(), Email: "bob@x", Name: "Bob", IsActive: true})
	h := &scim.Handlers{BaseURL: baseURL, Users: store}

	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers, nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp scim.ScimListResponse[scim.ScimUser]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.TotalResults)
	assert.Equal(t, 1, resp.StartIndex)
	assert.Equal(t, 2, resp.ItemsPerPage)
	require.Len(t, resp.Resources, 2)
}

func TestListUsersFilterByUserName(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	store.Insert(scim.UserRecord{ID: uuid.New(), Email: "alice@x", Name: "Alice"})
	store.Insert(scim.UserRecord{ID: uuid.New(), Email: "bob@x", Name: "Bob"})
	h := &scim.Handlers{BaseURL: baseURL, Users: store}

	req := httptest.NewRequest(http.MethodGet,
		scim.RouteUsers+`?filter=userName+eq+%22alice%40x%22`, nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "got body=%s", rec.Body.String())
	var resp scim.ScimListResponse[scim.ScimUser]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, 1, resp.TotalResults)
	assert.Equal(t, "alice@x", resp.Resources[0].UserName)
}

func TestListUsersUnsupportedFilterIs400(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	h := &scim.Handlers{BaseURL: baseURL, Users: store}
	req := httptest.NewRequest(http.MethodGet,
		scim.RouteUsers+`?filter=email+eq+%22alice%40x%22`, nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var err scim.ScimError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&err))
	require.NotNil(t, err.ScimType)
	assert.Equal(t, "invalidFilter", *err.ScimType)
}

func TestListUsersClampsCount(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	for i := 0; i < 3; i++ {
		store.Insert(scim.UserRecord{ID: uuid.New(), Email: "u" + string(rune('a'+i)) + "@x"})
	}
	h := &scim.Handlers{BaseURL: baseURL, Users: store}

	// count=9999 → clamped to 500 max; we only have 3 rows so we
	// expect 3.
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"?count=9999", nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp scim.ScimListResponse[scim.ScimUser]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 3, resp.TotalResults)
	assert.Equal(t, 3, resp.ItemsPerPage)
}

func TestListUsersStartIndexHonoursOneBased(t *testing.T) {
	t.Parallel()
	store := scim.NewInMemoryUserStore()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		store.Insert(scim.UserRecord{
			ID:        uuid.New(),
			Email:     "u" + string(rune('a'+i)) + "@x",
			CreatedAt: now.Add(time.Duration(-i) * time.Minute),
		})
	}
	h := &scim.Handlers{BaseURL: baseURL, Users: store}
	req := httptest.NewRequest(http.MethodGet, scim.RouteUsers+"?startIndex=2&count=10", nil)
	req = ctxAdmin(t, req)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp scim.ScimListResponse[scim.ScimUser]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 3, resp.TotalResults)
	assert.Equal(t, 2, resp.StartIndex)
	assert.Equal(t, 2, resp.ItemsPerPage, "startIndex=2 with count=10 yields rows [2,3]")
}

// --- Service-account claim helper --------------------------------------

func TestIsScimServiceAccount(t *testing.T) {
	t.Parallel()
	withRoleAndKind := &authmw.Claims{
		Sub:   uuid.New(),
		Roles: []string{"scim_writer"},
		Attributes: json.RawMessage(`{"kind":"service_account"}`),
	}
	assert.True(t, scim.IsScimServiceAccount(withRoleAndKind))

	noRole := &authmw.Claims{Sub: uuid.New(), Attributes: json.RawMessage(`{"kind":"service_account"}`)}
	assert.False(t, scim.IsScimServiceAccount(noRole))

	humanRole := &authmw.Claims{
		Sub:   uuid.New(),
		Roles: []string{"scim_writer"},
		Attributes: json.RawMessage(`{"kind":"human"}`),
	}
	assert.False(t, scim.IsScimServiceAccount(humanRole))
}

// --- Helpers -----------------------------------------------------------

func ptr(s string) *string { return &s }

// silence unused-imports-on-some-paths.
var _ = strings.HasPrefix
