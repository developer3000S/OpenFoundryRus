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

// SG.3 wire-format: SsoProvider gains domains, metadata refresh state
// and a typed AttributeMapping. The test pins the snake_case keys so a
// future schema change fails loudly.
func TestSsoProviderSG3WireShape(t *testing.T) {
	t.Parallel()
	issuer := "https://idp.example.com"
	now := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	exp := now.Add(180 * 24 * time.Hour)
	lastErr := "tls: handshake failed"
	clientID := "client-abc"
	p := models.SsoProvider{
		ID: uuid.New(), Slug: "acme-okta", Name: "Acme Okta",
		ProviderType: "oidc", Enabled: true,
		ClientID: &clientID, IssuerURL: &issuer,
		Scopes:                  []string{"openid", "email", "profile"},
		AttributeMapping:        json.RawMessage(`{"email":"email","groups":{"claim":"groups"}}`),
		Domains:                 []string{"acme.example.com"},
		MetadataLastRefreshedAt: &now,
		MetadataLastError:       &lastErr,
		CertificateExpiresAt:    &exp,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	out, err := json.Marshal(p.IntoResponse())
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "slug", "name", "provider_type", "enabled",
		"client_secret_configured", "saml_certificate_configured",
		"scopes", "attribute_mapping", "domains",
		"metadata_last_refreshed_at", "metadata_last_error", "certificate_expires_at",
		"created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
	assert.Equal(t, false, view["client_secret_configured"]) // ClientSecret left nil
	domains, ok := view["domains"].([]any)
	require.True(t, ok)
	assert.Equal(t, "acme.example.com", domains[0])
}

func TestSsoProviderResponseMasksSecret(t *testing.T) {
	t.Parallel()
	secret := "super-secret"
	cert := "ABCDEF..."
	p := models.SsoProvider{
		ID:              uuid.New(),
		Slug:            "okta",
		Name:            "Okta",
		ProviderType:    "saml",
		Enabled:         true,
		ClientSecret:    &secret,
		SamlCertificate: &cert,
		Scopes:          []string{},
		Domains:         []string{},
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	resp := p.IntoResponse()
	assert.True(t, resp.ClientSecretConfigured)
	assert.True(t, resp.SamlCertificateConfigured)
	// Marshalling the response must not surface the raw secret.
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "super-secret")
	assert.NotContains(t, string(out), "ABCDEF")
}

// Validation paths that don't touch the DB.

func TestSsoAdminCreateRejectsEmptyBody(t *testing.T) {
	t.Parallel()
	a := handlers.NewSsoAdmin(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/providers", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	a.Create(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "slug")
}

func TestSsoAdminCreateRejectsInvalidProviderType(t *testing.T) {
	t.Parallel()
	a := handlers.NewSsoAdmin(nil, nil)
	body := `{"slug":"x","name":"X","provider_type":"ldap"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/providers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.Create(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "provider_type")
}

func TestSsoAdminCreateRejectsInvalidAttributeMapping(t *testing.T) {
	t.Parallel()
	a := handlers.NewSsoAdmin(nil, nil)
	body := `{"slug":"x","name":"X","provider_type":"oidc","attribute_mapping":"oops"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/providers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.Create(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "attribute_mapping")
}

func TestSsoAdminTroubleshootRequiresEmail(t *testing.T) {
	t.Parallel()
	a := handlers.NewSsoAdmin(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/troubleshoot", strings.NewReader(`{"email":""}`))
	rec := httptest.NewRecorder()
	a.Troubleshoot(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "email")
}

func TestSsoAdminTroubleshootRejectsBareString(t *testing.T) {
	t.Parallel()
	a := handlers.NewSsoAdmin(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/auth/sso/troubleshoot", strings.NewReader(`{"email":"not-an-email"}`))
	rec := httptest.NewRecorder()
	a.Troubleshoot(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLoginTroubleshootStateConstants(t *testing.T) {
	t.Parallel()
	// Pin the wire vocabulary — the login UI keys translations off
	// these strings. Renames are wire-breaking.
	assert.Equal(t, "ok", models.LoginTroubleshootStateOK)
	assert.Equal(t, "unknown_domain", models.LoginTroubleshootStateUnknownDomain)
	assert.Equal(t, "user_disabled", models.LoginTroubleshootStateUserDisabled)
	assert.Equal(t, "provider_disabled", models.LoginTroubleshootStateProviderDisabled)
	assert.Equal(t, "metadata_stale", models.LoginTroubleshootStateMetadataStale)
	assert.Equal(t, "certificate_expired", models.LoginTroubleshootStateCertificateExpired)
	assert.Equal(t, "certificate_expiring", models.LoginTroubleshootStateCertificateExpiring)
	assert.Equal(t, "configuration_error", models.LoginTroubleshootStateConfigurationError)
}
