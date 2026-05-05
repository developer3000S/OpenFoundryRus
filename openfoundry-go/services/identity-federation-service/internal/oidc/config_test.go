package oidc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/oidc"
)

func TestLoadProvidersEmptyWhenUnset(t *testing.T) {
	t.Setenv("OIDC_PROVIDERS", "")
	got, err := oidc.LoadProvidersFromEnv("https://auth.example.com")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadProvidersGoogleWithDefaults(t *testing.T) {
	t.Setenv("OIDC_PROVIDERS", "google")
	t.Setenv("OIDC_GOOGLE_CLIENT_ID", "cid")
	t.Setenv("OIDC_GOOGLE_CLIENT_SECRET", "csec")
	got, err := oidc.LoadProvidersFromEnv("https://auth.example.com")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "google", got[0].Name)
	assert.Equal(t, "https://accounts.google.com", got[0].IssuerURL)
	assert.Equal(t, "https://auth.example.com/api/v1/auth/sso/google/callback", got[0].RedirectURL)
	assert.Equal(t, []string{"openid", "email", "profile"}, got[0].Scopes)
}

func TestLoadProvidersRequiresClientCreds(t *testing.T) {
	t.Setenv("OIDC_PROVIDERS", "google")
	t.Setenv("OIDC_GOOGLE_CLIENT_ID", "")
	t.Setenv("OIDC_GOOGLE_CLIENT_SECRET", "")
	_, err := oidc.LoadProvidersFromEnv("https://auth.example.com")
	require.Error(t, err)
}

func TestLoadProvidersHonoursOverrides(t *testing.T) {
	t.Setenv("OIDC_PROVIDERS", "okta")
	t.Setenv("OIDC_OKTA_CLIENT_ID", "cid")
	t.Setenv("OIDC_OKTA_CLIENT_SECRET", "csec")
	t.Setenv("OIDC_OKTA_ISSUER", "https://acme.okta.com/oauth2/default")
	t.Setenv("OIDC_OKTA_SCOPES", "openid,profile,custom_scope")
	got, err := oidc.LoadProvidersFromEnv("https://auth.example.com/")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "https://acme.okta.com/oauth2/default", got[0].IssuerURL)
	assert.Equal(t, []string{"openid", "profile", "custom_scope"}, got[0].Scopes)
	// Trailing slash on BASE_URL is normalised.
	assert.Equal(t, "https://auth.example.com/api/v1/auth/sso/okta/callback", got[0].RedirectURL)
}

func TestLoadProvidersOktaRequiresExplicitIssuer(t *testing.T) {
	// Okta has no built-in default — the issuer is tenant-scoped.
	t.Setenv("OIDC_PROVIDERS", "okta")
	t.Setenv("OIDC_OKTA_CLIENT_ID", "cid")
	t.Setenv("OIDC_OKTA_CLIENT_SECRET", "csec")
	t.Setenv("OIDC_OKTA_ISSUER", "")
	_, err := oidc.LoadProvidersFromEnv("https://auth.example.com")
	require.Error(t, err)
}
