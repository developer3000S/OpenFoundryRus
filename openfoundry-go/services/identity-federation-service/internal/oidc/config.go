// Package oidc is the OAuth 2.0 + OpenID Connect SSO surface for
// identity-federation-service slice 5a.
//
// Out of scope for slice 5a (lands in slice 5b / 7):
//   - SAML 2.0 SSO (slice 5b — needs XML signing).
//   - Per-org dynamic IdP configuration via the control-panel
//     (slice 7 — DB-backed provider rows + UI to manage them).
//   - Rich IdP claim → role/group mapping rules (Rust
//     domain/idp_mapping.rs, 546 LOC). Slice 5a does the basic claim
//     pass-through (sub, email, name) only.
package oidc

import (
	"fmt"
	"os"
	"strings"
)

// ProviderConfig is one OIDC issuer configured via env.
//
// Multiple providers share one binary. Variables follow the pattern
// `OIDC_<UPPER_NAME>_*` so adding `google` means setting
// OIDC_GOOGLE_CLIENT_ID, OIDC_GOOGLE_CLIENT_SECRET,
// OIDC_GOOGLE_ISSUER (defaults to https://accounts.google.com), and
// OIDC_GOOGLE_SCOPES (CSV; defaults to "openid,email,profile").
//
// The slice-5a env contract intentionally stays terse — slice 7 adds
// the DB-backed admin surface that supersedes env-based configuration.
type ProviderConfig struct {
	Name         string   // path key: /api/v1/auth/sso/<name>/...
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectURL  string // computed: <BASE_URL>/api/v1/auth/sso/<name>/callback
}

// LoadProvidersFromEnv resolves every provider declared via the
// `OIDC_PROVIDERS` env var (CSV of provider names). Missing client
// id / client secret for any declared provider is a fatal config error.
//
// `baseURL` is the externally-visible base URL of identity-federation
// (e.g. https://auth.openfoundry.io). The callback URL each provider
// is registered with at the IdP MUST match `<baseURL>/api/v1/auth/sso/<name>/callback`.
func LoadProvidersFromEnv(baseURL string) ([]ProviderConfig, error) {
	raw := os.Getenv("OIDC_PROVIDERS")
	if raw == "" {
		return nil, nil
	}
	if baseURL == "" {
		return nil, fmt.Errorf("OIDC_PROVIDERS set but BASE_URL is empty")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	out := make([]ProviderConfig, 0)
	for _, name := range splitCSV(raw) {
		key := strings.ToUpper(name)
		clientID := os.Getenv("OIDC_" + key + "_CLIENT_ID")
		clientSecret := os.Getenv("OIDC_" + key + "_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf(
				"oidc provider %q: OIDC_%s_CLIENT_ID and OIDC_%s_CLIENT_SECRET required",
				name, key, key,
			)
		}
		issuer := os.Getenv("OIDC_" + key + "_ISSUER")
		if issuer == "" {
			issuer = defaultIssuer(strings.ToLower(name))
			if issuer == "" {
				return nil, fmt.Errorf(
					"oidc provider %q: OIDC_%s_ISSUER required (no built-in default)",
					name, key,
				)
			}
		}
		scopes := defaultScopes
		if v := os.Getenv("OIDC_" + key + "_SCOPES"); v != "" {
			scopes = splitCSV(v)
		}
		out = append(out, ProviderConfig{
			Name:         strings.ToLower(name),
			IssuerURL:    issuer,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       scopes,
			RedirectURL:  baseURL + "/api/v1/auth/sso/" + strings.ToLower(name) + "/callback",
		})
	}
	return out, nil
}

// defaultScopes is the OIDC minimum: openid + the email + profile
// access. Override per-provider via OIDC_<NAME>_SCOPES.
var defaultScopes = []string{"openid", "email", "profile"}

// defaultIssuer maps the well-known provider names to their canonical
// issuer URLs. Avoids the operator hand-typing them.
func defaultIssuer(name string) string {
	switch name {
	case "google":
		return "https://accounts.google.com"
	case "microsoft", "azure", "azuread":
		return "https://login.microsoftonline.com/common/v2.0"
	case "github":
		// GitHub doesn't ship OIDC discovery — the operator must use
		// a generic OAuth flow or set OIDC_GITHUB_ISSUER manually.
		return ""
	case "gitlab":
		return "https://gitlab.com"
	case "okta":
		return "" // tenant-scoped, no built-in default
	default:
		return ""
	}
}

func splitCSV(v string) []string {
	out := make([]string, 0)
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
