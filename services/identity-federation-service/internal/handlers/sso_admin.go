// sso_admin.go: SG.3 — admin-side CRUD for SSO providers plus
// metadata-refresh, health-check, and login-troubleshoot endpoints.
//
// Read paths return the *Response shape (secrets redacted). Writes
// accept the full secret values. This file does not touch the live
// OIDC service or SAML registry — both remain seeded at boot from
// env config. The DB rows are the durable admin source-of-truth that
// a follow-up RFC will hot-load.

package handlers

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/saml"
)

// SsoAdmin owns the bearer-protected CRUD + diagnostics endpoints
// for SSO providers. The HTTP client is exposed for testing — tests
// inject a stubbed transport via NewSsoAdmin.
type SsoAdmin struct {
	Repo *repo.Repo
	HTTP *http.Client
}

// NewSsoAdmin builds an SsoAdmin. Pass nil for httpClient to use the
// default 30s-timeout client.
func NewSsoAdmin(r *repo.Repo, httpClient *http.Client) *SsoAdmin {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &SsoAdmin{Repo: r, HTTP: httpClient}
}

// ─── CRUD ──────────────────────────────────────────────────────────────

func (a *SsoAdmin) List(w http.ResponseWriter, r *http.Request) {
	items, err := a.Repo.ListSsoProviders(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]models.SsoProviderResponse, len(items))
	for i := range items {
		out[i] = items[i].IntoResponse()
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *SsoAdmin) Create(w http.ResponseWriter, r *http.Request) {
	var body models.CreateSsoProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Slug) == "" || strings.TrimSpace(body.Name) == "" {
		writeJSONErr(w, http.StatusBadRequest, "slug and name are required")
		return
	}
	pt := strings.ToLower(strings.TrimSpace(body.ProviderType))
	if pt != "oidc" && pt != "saml" {
		writeJSONErr(w, http.StatusBadRequest, "provider_type must be 'oidc' or 'saml'")
		return
	}
	if len(body.AttributeMapping) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(body.AttributeMapping, &probe); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "attribute_mapping must be a valid JSON object")
			return
		}
	}
	prov, err := a.Repo.InsertSsoProvider(r.Context(), &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, prov.IntoResponse())
}

func (a *SsoAdmin) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSsoID(w, r)
	if !ok {
		return
	}
	p, err := a.Repo.GetSsoProvider(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeJSONErr(w, http.StatusNotFound, "sso provider not found")
		return
	}
	writeJSON(w, http.StatusOK, p.IntoResponse())
}

func (a *SsoAdmin) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSsoID(w, r)
	if !ok {
		return
	}
	var body models.UpdateSsoProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.AttributeMapping) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(body.AttributeMapping, &probe); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "attribute_mapping must be a valid JSON object")
			return
		}
	}
	p, err := a.Repo.UpdateSsoProvider(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeJSONErr(w, http.StatusNotFound, "sso provider not found")
		return
	}
	writeJSON(w, http.StatusOK, p.IntoResponse())
}

func (a *SsoAdmin) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSsoID(w, r)
	if !ok {
		return
	}
	if err := a.Repo.DeleteSsoProvider(r.Context(), id); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── SAML metadata refresh ─────────────────────────────────────────────

// RefreshMetadata handles POST /api/v1/auth/sso/providers/{id}/refresh-metadata.
//
// For SAML providers with a saml_metadata_url set, fetches the
// metadata XML, extracts EntityID / SsoURL / Certificate, writes
// them back to the row, and updates metadata_last_refreshed_at /
// certificate_expires_at. For OIDC providers this is a no-op that
// returns 422.
func (a *SsoAdmin) RefreshMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSsoID(w, r)
	if !ok {
		return
	}
	prov, err := a.Repo.GetSsoProvider(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if prov == nil {
		writeJSONErr(w, http.StatusNotFound, "sso provider not found")
		return
	}
	if prov.ProviderType != "saml" {
		writeJSONErr(w, http.StatusUnprocessableEntity, "metadata refresh is only valid for saml providers")
		return
	}
	if prov.SamlMetadataURL == nil || *prov.SamlMetadataURL == "" {
		writeJSONErr(w, http.StatusUnprocessableEntity, "saml_metadata_url is empty — set it before refreshing")
		return
	}
	defaults, err := saml.ResolveMetadataDefaults(r.Context(), a.HTTP, *prov.SamlMetadataURL)
	if err != nil {
		_ = a.Repo.RecordSsoMetadataRefresh(r.Context(), id, err.Error(), nil)
		writeJSONErr(w, http.StatusBadGateway, "metadata fetch failed: "+err.Error())
		return
	}
	patch := &models.UpdateSsoProviderRequest{}
	if defaults.EntityID != nil {
		v := defaults.EntityID
		patch.SamlEntityID = &v
	}
	if defaults.SsoURL != nil {
		v := defaults.SsoURL
		patch.SamlSsoURL = &v
	}
	if defaults.Certificate != nil {
		v := defaults.Certificate
		patch.SamlCertificate = &v
	}
	if _, err := a.Repo.UpdateSsoProvider(r.Context(), id, patch); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "persist refreshed metadata: "+err.Error())
		return
	}
	var certExpiresAt *time.Time
	if defaults.Certificate != nil {
		if exp, parseErr := parseCertificateNotAfter(*defaults.Certificate); parseErr == nil {
			certExpiresAt = &exp
		}
	}
	if err := a.Repo.RecordSsoMetadataRefresh(r.Context(), id, "", certExpiresAt); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "stamp metadata refresh: "+err.Error())
		return
	}
	fresh, _ := a.Repo.GetSsoProvider(r.Context(), id)
	if fresh == nil {
		writeJSONErr(w, http.StatusInternalServerError, "refreshed provider not found")
		return
	}
	writeJSON(w, http.StatusOK, fresh.IntoResponse())
}

// ─── Health check ──────────────────────────────────────────────────────

// Health handles GET /api/v1/auth/sso/providers/{id}/health.
//
// Probes:
//   - OIDC: HEAD/GET the issuer's /.well-known/openid-configuration.
//   - SAML: HEAD/GET the metadata URL; check certificate expiry.
func (a *SsoAdmin) Health(w http.ResponseWriter, r *http.Request) {
	id, ok := parseSsoID(w, r)
	if !ok {
		return
	}
	prov, err := a.Repo.GetSsoProvider(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if prov == nil {
		writeJSONErr(w, http.StatusNotFound, "sso provider not found")
		return
	}
	health := a.healthOf(r.Context(), prov)
	writeJSON(w, http.StatusOK, health)
}

// healthOf is the pure health-evaluation helper. Exposed at package
// level so the troubleshoot endpoint can reuse it.
func (a *SsoAdmin) healthOf(ctx context.Context, prov *models.SsoProvider) models.SsoProviderHealth {
	now := time.Now().UTC()
	health := models.SsoProviderHealth{
		ProviderID:    prov.ID,
		ProviderSlug:  prov.Slug,
		ProviderType:  prov.ProviderType,
		Enabled:       prov.Enabled,
		OverallStatus: "ok",
		CheckedAt:     now,
	}
	switch prov.ProviderType {
	case "oidc":
		if prov.IssuerURL == nil || *prov.IssuerURL == "" {
			f := false
			msg := "issuer_url is not configured"
			health.IssuerReachable = &f
			health.IssuerError = &msg
			health.OverallStatus = "blocked"
			return health
		}
		discovery := strings.TrimRight(*prov.IssuerURL, "/") + "/.well-known/openid-configuration"
		if err := a.headOrGet(ctx, discovery); err != nil {
			f := false
			msg := err.Error()
			health.IssuerReachable = &f
			health.IssuerError = &msg
			health.OverallStatus = "blocked"
			return health
		}
		t := true
		health.IssuerReachable = &t
	case "saml":
		if prov.SamlMetadataURL != nil && *prov.SamlMetadataURL != "" {
			if err := a.headOrGet(ctx, *prov.SamlMetadataURL); err != nil {
				f := false
				msg := err.Error()
				health.MetadataReachable = &f
				health.MetadataError = &msg
				health.OverallStatus = "degraded"
			} else {
				t := true
				health.MetadataReachable = &t
			}
		}
		if prov.CertificateExpiresAt != nil {
			expires := *prov.CertificateExpiresAt
			health.CertificateExpiresAt = &expires
			daysLeft := int(time.Until(expires).Hours() / 24)
			health.CertificateDaysLeft = &daysLeft
			if expires.Before(now) {
				health.OverallStatus = "blocked"
			} else if daysLeft <= 7 && health.OverallStatus == "ok" {
				health.OverallStatus = "degraded"
			}
		}
	}
	if !prov.Enabled {
		health.OverallStatus = "blocked"
	}
	return health
}

// ─── Login troubleshooting ─────────────────────────────────────────────

// Troubleshoot handles POST /api/v1/auth/sso/troubleshoot.
//
// Unauthenticated — the login page calls it when the user can't get
// past the email-entry step. Returns a structured classification of
// why a sign-in might fail: unknown domain, disabled user, disabled
// provider, stale metadata, expiring/expired certificate.
//
// Mounted under /api/v1/auth (public) — does NOT leak secrets;
// providers are returned through IntoResponse() which masks
// client_secret / saml_certificate.
func (a *SsoAdmin) Troubleshoot(w http.ResponseWriter, r *http.Request) {
	var body models.LoginTroubleshootRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeJSONErr(w, http.StatusBadRequest, "email is required")
		return
	}
	domain := email[strings.Index(email, "@")+1:]
	out := models.LoginTroubleshootResponse{
		Email:            email,
		Domain:           domain,
		State:            models.LoginTroubleshootStateOK,
		MatchedProviders: []models.SsoProviderResponse{},
		Diagnostics:      []models.LoginTroubleshootIssue{},
		CheckedAt:        time.Now().UTC(),
	}

	providers, err := a.Repo.ListSsoProvidersForDomain(r.Context(), domain)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(providers) == 0 {
		out.State = models.LoginTroubleshootStateUnknownDomain
		out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
			Code:     "unknown_domain",
			Severity: "warning",
			Message:  "No identity provider is configured for this email domain.",
		})
	}

	// User-disabled inspection: an existing user with is_active=false
	// is the most common login dead-end after misconfigured SSO.
	if user, err := a.Repo.FindUserByEmail(r.Context(), email); err == nil && user != nil {
		out.UserExists = true
		if !user.IsActive {
			out.UserDisabled = true
			out.State = models.LoginTroubleshootStateUserDisabled
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "user_disabled",
				Severity: "error",
				Message:  "The user account is disabled. Ask an administrator to re-enable it.",
			})
		}
	}

	// Health-check each matched provider — surface the most severe
	// problem in the top-level state. Order: certificate_expired >
	// configuration_error > metadata_stale > certificate_expiring >
	// provider_disabled > unknown_domain.
	for _, prov := range providers {
		out.MatchedProviders = append(out.MatchedProviders, prov.IntoResponse())
		health := a.healthOf(r.Context(), &prov)
		switch {
		case !prov.Enabled:
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "provider_disabled",
				Severity: "warning",
				Message:  "Provider " + prov.Slug + " is disabled.",
			})
			if out.State == models.LoginTroubleshootStateOK || out.State == models.LoginTroubleshootStateUnknownDomain {
				out.State = models.LoginTroubleshootStateProviderDisabled
			}
		case health.CertificateExpiresAt != nil && health.CertificateExpiresAt.Before(time.Now().UTC()):
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "certificate_expired",
				Severity: "error",
				Message:  "SAML signing certificate for " + prov.Slug + " has expired.",
			})
			out.State = models.LoginTroubleshootStateCertificateExpired
		case health.IssuerError != nil || health.MetadataError != nil:
			msg := "Provider " + prov.Slug + " is unreachable."
			if health.IssuerError != nil {
				msg = "OIDC issuer for " + prov.Slug + " is unreachable: " + *health.IssuerError
			}
			if health.MetadataError != nil {
				msg = "SAML metadata for " + prov.Slug + " is unreachable: " + *health.MetadataError
			}
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "configuration_error",
				Severity: "error",
				Message:  msg,
			})
			if out.State != models.LoginTroubleshootStateCertificateExpired {
				out.State = models.LoginTroubleshootStateConfigurationError
			}
		case health.CertificateDaysLeft != nil && *health.CertificateDaysLeft <= 7:
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "certificate_expiring",
				Severity: "warning",
				Message:  "SAML signing certificate for " + prov.Slug + " expires soon.",
			})
			if out.State == models.LoginTroubleshootStateOK {
				out.State = models.LoginTroubleshootStateCertificateExpiring
			}
		case prov.MetadataLastRefreshedAt != nil && time.Since(*prov.MetadataLastRefreshedAt) > 30*24*time.Hour:
			out.Diagnostics = append(out.Diagnostics, models.LoginTroubleshootIssue{
				Code:     "metadata_stale",
				Severity: "info",
				Message:  "SAML metadata for " + prov.Slug + " has not been refreshed in 30+ days.",
			})
			if out.State == models.LoginTroubleshootStateOK {
				out.State = models.LoginTroubleshootStateMetadataStale
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ─── helpers ───────────────────────────────────────────────────────────

func parseSsoID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sso provider id")
		return uuid.Nil, false
	}
	return id, true
}

// headOrGet probes a URL — uses HEAD first, falls back to GET on
// 405 / unsupported. Returns nil when status is 2xx, an error
// otherwise.
func (a *SsoAdmin) headOrGet(ctx context.Context, target string) error {
	if _, err := url.Parse(target); err != nil {
		return errors.New("invalid url: " + err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return err
	}
	resp, err := a.HTTP.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
			return errors.New("unexpected status " + resp.Status)
		}
	}
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	getResp, err := a.HTTP.Do(getReq)
	if err != nil {
		return err
	}
	defer getResp.Body.Close()
	if getResp.StatusCode >= 200 && getResp.StatusCode < 300 {
		return nil
	}
	return errors.New("unexpected status " + getResp.Status)
}

// parseCertificateNotAfter takes a PEM or base64-DER-encoded X.509
// certificate body (as a SAML metadata file embeds it) and returns
// the cert's NotAfter time.
func parseCertificateNotAfter(body string) (time.Time, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return time.Time{}, errors.New("empty certificate body")
	}
	// Try PEM first; if no PEM block is present, treat the body as
	// base64-encoded DER (the SAML-metadata convention).
	if block, _ := pem.Decode([]byte(body)); block != nil {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		return cert.NotAfter, nil
	}
	der, err := base64.StdEncoding.DecodeString(stripBase64Whitespace(body))
	if err != nil {
		return time.Time{}, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}

func stripBase64Whitespace(in string) string {
	var b strings.Builder
	for _, r := range in {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
