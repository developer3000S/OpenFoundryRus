package repo

// sso_providers.go — SG.3 persistence for SAML/OIDC provider rows.
//
// Slice 5a/5b loaded providers from env into the OIDC service and SAML
// registry at boot. Slice 5c stores them in Postgres so an admin can
// add / disable / update providers without a service restart. The
// boot-time loader continues to seed the in-memory services; a
// follow-up RFC will hot-load from the DB.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

const ssoProviderColumns = `id, slug, name, provider_type, enabled,
	client_id, client_secret, issuer_url, authorization_url, token_url, userinfo_url, scopes,
	saml_metadata_url, saml_entity_id, saml_sso_url, saml_certificate,
	attribute_mapping, domains, metadata_last_refreshed_at, metadata_last_error, certificate_expires_at,
	created_at, updated_at`

// ListSsoProviders returns every row in sso_providers, newest first.
func (r *Repo) ListSsoProviders(ctx context.Context) ([]models.SsoProvider, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+ssoProviderColumns+` FROM sso_providers ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SsoProvider, 0)
	for rows.Next() {
		p, err := scanSsoProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetSsoProvider returns the row for `id` or nil when missing.
func (r *Repo) GetSsoProvider(ctx context.Context, id uuid.UUID) (*models.SsoProvider, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM sso_providers WHERE id = $1`, id)
	p, err := scanSsoProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// GetSsoProviderBySlug returns the row keyed by `slug` (the
// URL-friendly identifier on the SSO routes) or nil.
func (r *Repo) GetSsoProviderBySlug(ctx context.Context, slug string) (*models.SsoProvider, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM sso_providers WHERE slug = $1`, slug)
	p, err := scanSsoProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// ListSsoProvidersForDomain returns enabled providers that claim the
// given email domain. The match is exact, lower-case.
func (r *Repo) ListSsoProvidersForDomain(ctx context.Context, domain string) ([]models.SsoProvider, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+ssoProviderColumns+` FROM sso_providers
		 WHERE enabled = TRUE AND domains @> $1::jsonb
		 ORDER BY created_at DESC`,
		toJSONArray([]string{strings.ToLower(domain)}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SsoProvider, 0)
	for rows.Next() {
		p, err := scanSsoProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// InsertSsoProvider creates a new provider row with a freshly-minted
// id. Returns the row as persisted (including server-side defaults).
func (r *Repo) InsertSsoProvider(ctx context.Context, body *models.CreateSsoProviderRequest) (*models.SsoProvider, error) {
	id := uuid.New()
	now := time.Now().UTC()
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	providerType := strings.ToLower(strings.TrimSpace(body.ProviderType))
	if providerType != "oidc" && providerType != "saml" {
		return nil, fmt.Errorf("provider_type must be 'oidc' or 'saml', got %q", body.ProviderType)
	}
	scopes := body.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	domains := normalizeDomains(body.Domains)
	attributeMapping := body.AttributeMapping
	if len(attributeMapping) == 0 {
		attributeMapping = []byte(`{}`)
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("encode scopes: %w", err)
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, fmt.Errorf("encode domains: %w", err)
	}
	_, err = r.Pool.Exec(ctx,
		`INSERT INTO sso_providers
		 (id, slug, name, provider_type, enabled,
		  client_id, client_secret, issuer_url, authorization_url, token_url, userinfo_url, scopes,
		  saml_metadata_url, saml_entity_id, saml_sso_url, saml_certificate,
		  attribute_mapping, domains,
		  created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5,
		         $6, $7, $8, $9, $10, $11, $12::jsonb,
		         $13, $14, $15, $16,
		         $17::jsonb, $18::jsonb,
		         $19, $19)`,
		id, strings.TrimSpace(body.Slug), strings.TrimSpace(body.Name), providerType, enabled,
		body.ClientID, body.ClientSecret, body.IssuerURL, body.AuthorizationURL, body.TokenURL, body.UserinfoURL, scopesJSON,
		body.SamlMetadataURL, body.SamlEntityID, body.SamlSsoURL, body.SamlCertificate,
		attributeMapping, domainsJSON, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert sso_provider: %w", err)
	}
	return r.GetSsoProvider(ctx, id)
}

// UpdateSsoProvider applies an UpdateSsoProviderRequest PATCH and
// returns the new row.
func (r *Repo) UpdateSsoProvider(ctx context.Context, id uuid.UUID, body *models.UpdateSsoProviderRequest) (*models.SsoProvider, error) {
	current, err := r.GetSsoProvider(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	name := current.Name
	if body.Name != nil {
		name = *body.Name
	}
	enabled := current.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	clientID := current.ClientID
	if body.ClientID != nil {
		clientID = *body.ClientID
	}
	clientSecret := current.ClientSecret
	if body.ClientSecret != nil {
		clientSecret = *body.ClientSecret
	}
	issuerURL := current.IssuerURL
	if body.IssuerURL != nil {
		issuerURL = *body.IssuerURL
	}
	authorizationURL := current.AuthorizationURL
	if body.AuthorizationURL != nil {
		authorizationURL = *body.AuthorizationURL
	}
	tokenURL := current.TokenURL
	if body.TokenURL != nil {
		tokenURL = *body.TokenURL
	}
	userinfoURL := current.UserinfoURL
	if body.UserinfoURL != nil {
		userinfoURL = *body.UserinfoURL
	}
	scopes := current.Scopes
	if body.Scopes != nil {
		scopes = *body.Scopes
	}
	if scopes == nil {
		scopes = []string{}
	}
	samlMetadataURL := current.SamlMetadataURL
	if body.SamlMetadataURL != nil {
		samlMetadataURL = *body.SamlMetadataURL
	}
	samlEntityID := current.SamlEntityID
	if body.SamlEntityID != nil {
		samlEntityID = *body.SamlEntityID
	}
	samlSsoURL := current.SamlSsoURL
	if body.SamlSsoURL != nil {
		samlSsoURL = *body.SamlSsoURL
	}
	samlCertificate := current.SamlCertificate
	if body.SamlCertificate != nil {
		samlCertificate = *body.SamlCertificate
	}
	attributeMapping := current.AttributeMapping
	if len(body.AttributeMapping) > 0 {
		attributeMapping = body.AttributeMapping
	}
	if len(attributeMapping) == 0 {
		attributeMapping = []byte(`{}`)
	}
	domains := current.Domains
	if body.Domains != nil {
		domains = normalizeDomains(*body.Domains)
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("encode scopes: %w", err)
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, fmt.Errorf("encode domains: %w", err)
	}
	_, err = r.Pool.Exec(ctx,
		`UPDATE sso_providers SET
		 name = $2, enabled = $3,
		 client_id = $4, client_secret = $5,
		 issuer_url = $6, authorization_url = $7, token_url = $8, userinfo_url = $9, scopes = $10::jsonb,
		 saml_metadata_url = $11, saml_entity_id = $12, saml_sso_url = $13, saml_certificate = $14,
		 attribute_mapping = $15::jsonb, domains = $16::jsonb,
		 updated_at = $17
		 WHERE id = $1`,
		id, name, enabled, clientID, clientSecret,
		issuerURL, authorizationURL, tokenURL, userinfoURL, scopesJSON,
		samlMetadataURL, samlEntityID, samlSsoURL, samlCertificate,
		attributeMapping, domainsJSON, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("update sso_provider: %w", err)
	}
	return r.GetSsoProvider(ctx, id)
}

// DeleteSsoProvider removes the row. Cascading clean-up of in-memory
// OIDC service / SAML registry is the handler's responsibility (the
// row is the durable source-of-truth; the registries are advisory
// caches seeded at boot).
func (r *Repo) DeleteSsoProvider(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM sso_providers WHERE id = $1`, id)
	return err
}

// RecordSsoMetadataRefresh stamps the metadata-refresh diagnostics
// after a SAML metadata-fetch attempt. `err` should be non-empty when
// the fetch failed; the certificate-expiry update only happens on a
// successful refresh.
func (r *Repo) RecordSsoMetadataRefresh(ctx context.Context, id uuid.UUID, err string, certExpiresAt *time.Time) error {
	now := time.Now().UTC()
	var errPtr *string
	if err != "" {
		errPtr = &err
	}
	_, dbErr := r.Pool.Exec(ctx,
		`UPDATE sso_providers SET
		 metadata_last_refreshed_at = $2,
		 metadata_last_error = $3,
		 certificate_expires_at = COALESCE($4, certificate_expires_at),
		 updated_at = $2
		 WHERE id = $1`,
		id, now, errPtr, certExpiresAt,
	)
	return dbErr
}

// ─── helpers ────────────────────────────────────────────────────────────

// ssoRowLike is the minimal Scan-only contract shared by *pgx.Row and
// pgx.Rows so the same scanner works for QueryRow and Query.
type ssoRowLike interface{ Scan(...any) error }

func scanSsoProvider(r ssoRowLike) (*models.SsoProvider, error) {
	p := &models.SsoProvider{}
	var scopes, domains []byte
	if err := r.Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProviderType, &p.Enabled,
		&p.ClientID, &p.ClientSecret, &p.IssuerURL, &p.AuthorizationURL, &p.TokenURL, &p.UserinfoURL, &scopes,
		&p.SamlMetadataURL, &p.SamlEntityID, &p.SamlSsoURL, &p.SamlCertificate,
		&p.AttributeMapping, &domains, &p.MetadataLastRefreshedAt, &p.MetadataLastError, &p.CertificateExpiresAt,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(scopes, &p.Scopes); err != nil {
		return nil, fmt.Errorf("decode scopes: %w", err)
	}
	if p.Scopes == nil {
		p.Scopes = []string{}
	}
	if err := json.Unmarshal(domains, &p.Domains); err != nil {
		return nil, fmt.Errorf("decode domains: %w", err)
	}
	if p.Domains == nil {
		p.Domains = []string{}
	}
	if len(p.AttributeMapping) == 0 {
		p.AttributeMapping = []byte(`{}`)
	}
	return p, nil
}

// normalizeDomains lower-cases and trims every entry, drops empties,
// and deduplicates in insertion order.
func normalizeDomains(in []string) []string {
	if in == nil {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		d := strings.ToLower(strings.TrimSpace(v))
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// toJSONArray serialises a []string to a JSON array. Helper for
// `@> $1::jsonb` containment queries — pgx won't auto-encode a
// []string into a jsonb-compatible parameter.
func toJSONArray(values []string) []byte {
	out, _ := json.Marshal(values)
	if len(out) == 0 {
		return []byte("[]")
	}
	return out
}
