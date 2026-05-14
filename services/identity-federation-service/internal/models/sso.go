package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SsoProvider mirrors `models::sso::SsoProvider`. Persistence-shaped
// row used by the OIDC + SAML SSO flows. Slice 5a wired the OIDC
// surface; slice 5b added SAML configuration columns
// (`saml_metadata_url`, `saml_entity_id`, `saml_sso_url`,
// `saml_certificate`); slice 5c (SG.3, 2026-05-14) added the
// persistent admin row with `domains`, metadata-refresh diagnostics
// and a typed AttributeMapping shape.
//
// `AttributeMapping` is the JSON column that maps SAML/OIDC claim
// names → canonical OpenFoundry claim slots. The SAML domain package
// reads it via `gjson`-style lookups against the raw bytes — empty /
// missing keys fall back to defaults (`NameID`, `email`, `name`).
// The typed shape in [AttributeMapping] documents what an admin UI
// can collect; it is also valid JSON for the column.
type SsoProvider struct {
	ID                       uuid.UUID       `json:"id"`
	Slug                     string          `json:"slug"`
	Name                     string          `json:"name"`
	ProviderType             string          `json:"provider_type"`
	Enabled                  bool            `json:"enabled"`
	ClientID                 *string         `json:"client_id,omitempty"`
	ClientSecret             *string         `json:"client_secret,omitempty"`
	IssuerURL                *string         `json:"issuer_url,omitempty"`
	AuthorizationURL         *string         `json:"authorization_url,omitempty"`
	TokenURL                 *string         `json:"token_url,omitempty"`
	UserinfoURL              *string         `json:"userinfo_url,omitempty"`
	Scopes                   []string        `json:"scopes"`
	SamlMetadataURL          *string         `json:"saml_metadata_url,omitempty"`
	SamlEntityID             *string         `json:"saml_entity_id,omitempty"`
	SamlSsoURL               *string         `json:"saml_sso_url,omitempty"`
	SamlCertificate          *string         `json:"saml_certificate,omitempty"`
	AttributeMapping         json.RawMessage `json:"attribute_mapping,omitempty"`
	Domains                  []string        `json:"domains"`
	MetadataLastRefreshedAt  *time.Time      `json:"metadata_last_refreshed_at,omitempty"`
	MetadataLastError        *string         `json:"metadata_last_error,omitempty"`
	CertificateExpiresAt     *time.Time      `json:"certificate_expires_at,omitempty"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

// AttributeMapping is the typed shape of the SsoProvider's
// `attribute_mapping` JSON column. The columns are documented in
// docs/security-governance/identity-and-access.md.
//
//   - Subject / Email / Name override the default claim names used to
//     populate the corresponding fields on a fresh user (OIDC defaults
//     are `sub` / `email` / `name`; SAML defaults are `NameID` /
//     `email` / `name`).
//   - Attributes is an arbitrary key→IdP-claim mapping that is copied
//     into the OpenFoundry user-attributes blob on every login.
//   - Groups names the IdP claim that carries the user's group
//     membership list. Each value in the claim is looked up against
//     `Groups`.IdPToGroup to find an OpenFoundry group slug.
type AttributeMapping struct {
	Subject    string            `json:"subject,omitempty"`
	Email      string            `json:"email,omitempty"`
	Name       string            `json:"name,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Groups     *GroupMapping     `json:"groups,omitempty"`
}

// GroupMapping is the IdP-group → OpenFoundry-group translation rule.
type GroupMapping struct {
	Claim       string            `json:"claim"`
	IdPToGroup  map[string]string `json:"idp_to_group,omitempty"`
	DefaultRole string            `json:"default_role,omitempty"`
}

// SsoProviderResponse mirrors `models::sso::SsoProviderResponse` — the
// public-safe view that drops `client_secret` and replaces it with
// `client_secret_configured`.
type SsoProviderResponse struct {
	ID                      uuid.UUID       `json:"id"`
	Slug                    string          `json:"slug"`
	Name                    string          `json:"name"`
	ProviderType            string          `json:"provider_type"`
	Enabled                 bool            `json:"enabled"`
	ClientID                *string         `json:"client_id,omitempty"`
	ClientSecretConfigured  bool            `json:"client_secret_configured"`
	IssuerURL               *string         `json:"issuer_url,omitempty"`
	AuthorizationURL        *string         `json:"authorization_url,omitempty"`
	TokenURL                *string         `json:"token_url,omitempty"`
	UserinfoURL             *string         `json:"userinfo_url,omitempty"`
	Scopes                  []string        `json:"scopes"`
	SamlMetadataURL         *string         `json:"saml_metadata_url,omitempty"`
	SamlEntityID            *string         `json:"saml_entity_id,omitempty"`
	SamlSsoURL              *string         `json:"saml_sso_url,omitempty"`
	SamlCertificateConfigured bool          `json:"saml_certificate_configured"`
	AttributeMapping        json.RawMessage `json:"attribute_mapping,omitempty"`
	Domains                 []string        `json:"domains"`
	MetadataLastRefreshedAt *time.Time      `json:"metadata_last_refreshed_at,omitempty"`
	MetadataLastError       *string         `json:"metadata_last_error,omitempty"`
	CertificateExpiresAt    *time.Time      `json:"certificate_expires_at,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

// IntoResponse mirrors `SsoProvider::into_response` — strips the
// secret while preserving every other field.
func (p *SsoProvider) IntoResponse() SsoProviderResponse {
	domains := p.Domains
	if domains == nil {
		domains = []string{}
	}
	return SsoProviderResponse{
		ID:                        p.ID,
		Slug:                      p.Slug,
		Name:                      p.Name,
		ProviderType:              p.ProviderType,
		Enabled:                   p.Enabled,
		ClientID:                  p.ClientID,
		ClientSecretConfigured:    p.ClientSecret != nil && *p.ClientSecret != "",
		IssuerURL:                 p.IssuerURL,
		AuthorizationURL:          p.AuthorizationURL,
		TokenURL:                  p.TokenURL,
		UserinfoURL:               p.UserinfoURL,
		Scopes:                    p.Scopes,
		SamlMetadataURL:           p.SamlMetadataURL,
		SamlEntityID:              p.SamlEntityID,
		SamlSsoURL:                p.SamlSsoURL,
		SamlCertificateConfigured: p.SamlCertificate != nil && *p.SamlCertificate != "",
		AttributeMapping:          p.AttributeMapping,
		Domains:                   domains,
		MetadataLastRefreshedAt:   p.MetadataLastRefreshedAt,
		MetadataLastError:         p.MetadataLastError,
		CertificateExpiresAt:      p.CertificateExpiresAt,
		CreatedAt:                 p.CreatedAt,
		UpdatedAt:                 p.UpdatedAt,
	}
}

// CreateSsoProviderRequest is the body of POST /api/v1/auth/sso/providers.
//
// SG.3: admin-side payload. Slug must be unique and ProviderType ∈
// {"oidc","saml"}. AttributeMapping accepts free-form JSON for
// backwards compatibility — the typed [AttributeMapping] above is
// the recommended shape.
type CreateSsoProviderRequest struct {
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	ProviderType     string          `json:"provider_type"`
	Enabled          *bool           `json:"enabled,omitempty"`
	ClientID         *string         `json:"client_id,omitempty"`
	ClientSecret     *string         `json:"client_secret,omitempty"`
	IssuerURL        *string         `json:"issuer_url,omitempty"`
	AuthorizationURL *string         `json:"authorization_url,omitempty"`
	TokenURL         *string         `json:"token_url,omitempty"`
	UserinfoURL      *string         `json:"userinfo_url,omitempty"`
	Scopes           []string        `json:"scopes,omitempty"`
	SamlMetadataURL  *string         `json:"saml_metadata_url,omitempty"`
	SamlEntityID     *string         `json:"saml_entity_id,omitempty"`
	SamlSsoURL       *string         `json:"saml_sso_url,omitempty"`
	SamlCertificate  *string         `json:"saml_certificate,omitempty"`
	AttributeMapping json.RawMessage `json:"attribute_mapping,omitempty"`
	Domains          []string        `json:"domains,omitempty"`
}

// UpdateSsoProviderRequest is the body of PATCH
// /api/v1/auth/sso/providers/{id}. Every field optional; missing
// fields preserve the current value. Pass an explicit null for
// pointer fields to clear them.
type UpdateSsoProviderRequest struct {
	Name             *string         `json:"name,omitempty"`
	Enabled          *bool           `json:"enabled,omitempty"`
	ClientID         **string        `json:"client_id,omitempty"`
	ClientSecret     **string        `json:"client_secret,omitempty"`
	IssuerURL        **string        `json:"issuer_url,omitempty"`
	AuthorizationURL **string        `json:"authorization_url,omitempty"`
	TokenURL         **string        `json:"token_url,omitempty"`
	UserinfoURL      **string        `json:"userinfo_url,omitempty"`
	Scopes           *[]string       `json:"scopes,omitempty"`
	SamlMetadataURL  **string        `json:"saml_metadata_url,omitempty"`
	SamlEntityID     **string        `json:"saml_entity_id,omitempty"`
	SamlSsoURL       **string        `json:"saml_sso_url,omitempty"`
	SamlCertificate  **string        `json:"saml_certificate,omitempty"`
	AttributeMapping json.RawMessage `json:"attribute_mapping,omitempty"`
	Domains          *[]string       `json:"domains,omitempty"`
}

// SsoProviderHealth is the response of GET
// /api/v1/auth/sso/providers/{id}/health. SG.3 troubleshooting.
type SsoProviderHealth struct {
	ProviderID          uuid.UUID  `json:"provider_id"`
	ProviderSlug        string     `json:"provider_slug"`
	ProviderType        string     `json:"provider_type"`
	Enabled             bool       `json:"enabled"`
	OverallStatus       string     `json:"overall_status"` // "ok", "degraded", "blocked"
	IssuerReachable     *bool      `json:"issuer_reachable,omitempty"`
	IssuerError         *string    `json:"issuer_error,omitempty"`
	MetadataReachable   *bool      `json:"metadata_reachable,omitempty"`
	MetadataError       *string    `json:"metadata_error,omitempty"`
	CertificateExpiresAt *time.Time `json:"certificate_expires_at,omitempty"`
	CertificateDaysLeft *int       `json:"certificate_days_left,omitempty"`
	CheckedAt           time.Time  `json:"checked_at"`
}

// LoginTroubleshootRequest is the body of POST
// /api/v1/auth/sso/troubleshoot. Unauthenticated — used by the login
// page to explain "why can I not sign in with my email?".
type LoginTroubleshootRequest struct {
	Email string `json:"email"`
}

// LoginTroubleshootResponse classifies a sign-in attempt. SG.3:
// "Provide login troubleshooting states for unknown domains, disabled
// users, certificate/metadata failures, and stale group mappings."
type LoginTroubleshootResponse struct {
	Email             string                    `json:"email"`
	Domain            string                    `json:"domain"`
	State             string                    `json:"state"` // see consts below
	MatchedProviders  []SsoProviderResponse     `json:"matched_providers"`
	UserExists        bool                      `json:"user_exists"`
	UserDisabled      bool                      `json:"user_disabled"`
	Diagnostics       []LoginTroubleshootIssue  `json:"diagnostics"`
	CheckedAt         time.Time                 `json:"checked_at"`
}

// LoginTroubleshootIssue is a single structured diagnostic.
type LoginTroubleshootIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // "info" | "warning" | "error"
	Message  string `json:"message"`
}

// Login-troubleshoot state vocabulary. Stable wire constants — the
// login UI keys translations off these.
const (
	LoginTroubleshootStateOK                  = "ok"
	LoginTroubleshootStateUnknownDomain       = "unknown_domain"
	LoginTroubleshootStateUserDisabled        = "user_disabled"
	LoginTroubleshootStateProviderDisabled    = "provider_disabled"
	LoginTroubleshootStateMetadataStale       = "metadata_stale"
	LoginTroubleshootStateCertificateExpired  = "certificate_expired"
	LoginTroubleshootStateCertificateExpiring = "certificate_expiring"
	LoginTroubleshootStateConfigurationError  = "configuration_error"
)
