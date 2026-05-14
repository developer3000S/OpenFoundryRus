package handlers

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/oidc"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/saml"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/service"
)

// SSO wires the OAuth/OIDC + SAML SSO endpoints. OIDC is slice 5a;
// SAML is slice 5b. Per-org IdP rows + claim-mapping rules in slice 7.
//
// SAML is opt-in via the SAML field — a nil registry means the
// service runs OIDC-only and any /sso/{slug}/start lookup that
// misses OIDC returns 404 unchanged.
type SSO struct {
	Repo   *repo.Repo
	OIDC   *oidc.Service
	SAML   *saml.Registry
	Issuer *service.Issuer
}

// ListProviders handles GET /api/v1/auth/sso/providers.
//
// Public endpoint — the login page calls this to render the
// "Sign in with X" buttons. Each entry carries the provider's
// kind ("oidc" or "saml") so the UI can route the click to the
// right Start handler.
func (s *SSO) ListProviders(w http.ResponseWriter, _ *http.Request) {
	out := make([]map[string]any, 0)
	oidcNames := s.OIDC.ProviderNames()
	sort.Strings(oidcNames)
	for _, n := range oidcNames {
		out = append(out, map[string]any{"name": n, "kind": "oidc"})
	}
	if s.SAML != nil {
		samlNames := s.SAML.Names()
		sort.Strings(samlNames)
		for _, n := range samlNames {
			out = append(out, map[string]any{"name": n, "kind": "saml"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// Start handles GET /api/v1/auth/sso/{provider}/start.
//
// OIDC: generates state + PKCE verifier + nonce, persists them,
// and 302s the caller to the IdP authorize URL.
//
// SAML: builds an AuthnRequest, persists the request_id alongside
// a freshly-minted state token, and 302s to the IdP SSO URL with
// SAMLRequest + RelayState query parameters.
func (s *SSO) Start(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	if prov, ok := s.OIDC.Get(name); ok {
		s.startOIDC(w, r, name, prov)
		return
	}
	if s.SAML != nil {
		if entry, ok := s.SAML.Get(name); ok {
			s.startSAML(w, r, name, entry)
			return
		}
	}
	writeJSONErr(w, http.StatusNotFound, "unknown provider")
}

func (s *SSO) startOIDC(w http.ResponseWriter, r *http.Request, name string, prov *oidc.Provider) {
	bundle, err := prov.BuildAuthURL(r.Context())
	if err != nil {
		slog.Error("sso start: build auth url", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	redirectAfter := normalizeRedirect(r.URL.Query().Get("redirect_after"))
	now := time.Now().UTC()
	if err := s.Repo.InsertOAuthState(r.Context(), &repo.OAuthState{
		State: bundle.State, CodeVerifier: bundle.CodeVerifier, Provider: name,
		RedirectAfter: redirectAfter, Nonce: bundle.Nonce,
		IssuedAt: now, ExpiresAt: now.Add(oidc.StateTTL),
	}); err != nil {
		slog.Error("sso start: persist state", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.Redirect(w, r, bundle.URL, http.StatusFound)
}

func (s *SSO) startSAML(w http.ResponseWriter, r *http.Request, name string, entry *saml.Entry) {
	authn, err := saml.BuildAuthnRequest(entry.Provider, entry.SP)
	if err != nil {
		slog.Error("sso start: build authn request", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	state := uuid.Must(uuid.NewV7()).String()
	redirectAfter := normalizeRedirect(r.URL.Query().Get("redirect_after"))
	now := time.Now().UTC()
	if err := s.Repo.InsertOAuthState(r.Context(), &repo.OAuthState{
		State: state, Provider: name, RedirectAfter: redirectAfter,
		SamlRequestID: &authn.RequestID,
		IssuedAt:      now,
		ExpiresAt:     now.Add(oidc.StateTTL),
	}); err != nil {
		slog.Error("sso start: persist state", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(authn.XML))
	destination := *entry.Provider.SamlSsoURL
	target, err := saml.AuthorizationURL(destination, encoded, state)
	if err != nil {
		slog.Error("sso start: build redirect URL", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func normalizeRedirect(v string) string {
	if v == "" {
		return "/"
	}
	return v
}

// Callback handles GET /api/v1/auth/sso/{provider}/callback.
//
// Flow:
//  1. consume state row (one-shot — DELETE … RETURNING)
//  2. exchange code, verify id_token, extract claims
//  3. resolve user: existing binding → existing email → create new
//  4. link the binding, issue tokens, redirect with the access token
//     in the URL fragment (slice 7 swaps this for a Set-Cookie handoff)
func (s *SSO) Callback(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	prov, ok := s.OIDC.Get(name)
	if !ok {
		writeJSONErr(w, http.StatusNotFound, "unknown provider")
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeJSONErr(w, http.StatusBadRequest, "missing state or code")
		return
	}

	st, err := s.Repo.ConsumeOAuthState(r.Context(), state)
	if err != nil {
		slog.Error("sso callback: consume state", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if st == nil || st.Provider != name {
		writeJSONErr(w, http.StatusUnauthorized, "invalid state")
		return
	}

	claims, err := prov.Exchange(r.Context(), code, st.CodeVerifier, st.Nonce)
	if err != nil {
		slog.Warn("sso callback: exchange failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusUnauthorized, "exchange failed")
		return
	}

	user, err := s.resolveUser(r.Context(), name, claims)
	if err != nil {
		slog.Error("sso callback: resolve user", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.Repo.LinkExternalIdentity(r.Context(), &repo.ExternalIdentity{
		ID: ids.New(), UserID: user.ID, Provider: name,
		ExternalID: claims.Subject, Email: claims.Email,
	}); err != nil {
		slog.Error("sso callback: link identity", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	access, refresh, err := s.Issuer.IssueTokens(r.Context(), user, []string{"sso", name})
	if err != nil {
		slog.Error("sso callback: issue tokens", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	// SG.4: stamp last_login_at + last_login_ip on every SSO sign-in.
	if err := s.Repo.StampLogin(r.Context(), user.ID, time.Now().UTC(), clientIP(r)); err != nil {
		slog.Warn("sso callback: stamp login", slog.String("user_id", user.ID.String()), slog.String("error", err.Error()))
	}

	target, _ := url.Parse(st.RedirectAfter)
	q := url.Values{}
	q.Set("access_token", access)
	q.Set("refresh_token", refresh)
	q.Set("token_type", "Bearer")
	target.Fragment = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// AssertionConsumerService handles POST
// /api/v1/auth/sso/{provider}/acs.
//
// Per SAML 2.0 HTTP-POST binding (RFC 7522 §3.5): the IdP renders
// a self-submitting form to the browser that POSTs SAMLResponse +
// RelayState to this URL. We:
//  1. Read SAMLResponse + RelayState from the form body.
//  2. Consume the matching oauth_state row by RelayState (one-shot).
//  3. Look up the SAML provider entry by the persisted slug.
//  4. ParseSamlResponse with the row's saml_request_id pinned for
//     InResponseTo validation.
//  5. Resolve the user (re-using the slice-5a policy).
//  6. Issue tokens + 302 to the redirect_after target with the
//     access/refresh tokens in the URL fragment.
func (s *SSO) AssertionConsumerService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	if s.SAML == nil {
		writeJSONErr(w, http.StatusNotFound, "saml not configured")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid form body")
		return
	}
	samlResponse := r.PostForm.Get("SAMLResponse")
	relayState := r.PostForm.Get("RelayState")
	if samlResponse == "" || relayState == "" {
		writeJSONErr(w, http.StatusBadRequest, "missing SAMLResponse or RelayState")
		return
	}

	st, err := s.Repo.ConsumeOAuthState(r.Context(), relayState)
	if err != nil {
		slog.Error("sso acs: consume state", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if st == nil || st.Provider != name {
		writeJSONErr(w, http.StatusUnauthorized, "invalid relay state")
		return
	}

	entry, ok := s.SAML.Get(name)
	if !ok {
		writeJSONErr(w, http.StatusNotFound, "unknown saml provider")
		return
	}

	identity, err := saml.ParseSamlResponse(entry.Provider, samlResponse, saml.ValidationContext{
		ServiceProvider: entry.SP,
		RequestID:       st.SamlRequestID,
	})
	if err != nil {
		slog.Warn("sso acs: parse response failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusUnauthorized, "saml validation failed")
		return
	}

	user, err := s.resolveSamlUser(r.Context(), name, identity)
	if err != nil {
		slog.Error("sso acs: resolve user", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := s.Repo.LinkExternalIdentity(r.Context(), &repo.ExternalIdentity{
		ID: ids.New(), UserID: user.ID, Provider: name,
		ExternalID: identity.Subject, Email: identity.Email,
	}); err != nil {
		slog.Error("sso acs: link identity", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	access, refresh, err := s.Issuer.IssueTokens(r.Context(), user, []string{"sso", name})
	if err != nil {
		slog.Error("sso acs: issue tokens", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	// SG.4: stamp last_login_at + last_login_ip on every SAML sign-in.
	if err := s.Repo.StampLogin(r.Context(), user.ID, time.Now().UTC(), clientIP(r)); err != nil {
		slog.Warn("sso acs: stamp login", slog.String("user_id", user.ID.String()), slog.String("error", err.Error()))
	}

	target, _ := url.Parse(st.RedirectAfter)
	q := url.Values{}
	q.Set("access_token", access)
	q.Set("refresh_token", refresh)
	q.Set("token_type", "Bearer")
	target.Fragment = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

// resolveSamlUser is the SAML-flavoured twin of resolveUser. The
// Identity already carries the canonical subject/email/name shape
// the slice-5a policy needs, so the body is identical to its OIDC
// sibling — kept separate so a future change can diverge cleanly.
func (s *SSO) resolveSamlUser(ctx context.Context, provider string, identity *saml.Identity) (*models.User, error) {
	bind, err := s.Repo.FindExternalIdentity(ctx, provider, identity.Subject)
	if err != nil {
		return nil, err
	}
	if bind != nil {
		u, err := s.Repo.FindUserByID(ctx, bind.UserID)
		if err != nil {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}
	if identity.Email != "" {
		u, err := s.Repo.FindUserByEmail(ctx, identity.Email)
		if err != nil {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}
	id := ids.New()
	if err := s.Repo.CreateUserForSSO(ctx, id, identity.Email, identity.Name, provider); err != nil {
		return nil, err
	}
	return s.Repo.FindUserByID(ctx, id)
}

// resolveUser implements the slice-5a SSO user-resolution policy:
//
//  1. (provider, external_id) already binds → that user.
//  2. claims.email matches an existing user → that user.
//  3. otherwise → fresh user with auth_source=<provider>, no role.
func (s *SSO) resolveUser(ctx context.Context, provider string, claims *oidc.Claims) (*models.User, error) {
	bind, err := s.Repo.FindExternalIdentity(ctx, provider, claims.Subject)
	if err != nil {
		return nil, err
	}
	if bind != nil {
		u, err := s.Repo.FindUserByID(ctx, bind.UserID)
		if err != nil {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}

	if claims.Email != "" {
		u, err := s.Repo.FindUserByEmail(ctx, claims.Email)
		if err != nil {
			return nil, err
		}
		if u != nil {
			return u, nil
		}
	}

	id := ids.New()
	if err := s.Repo.CreateUserForSSO(ctx, id, claims.Email, claims.Name, provider); err != nil {
		return nil, err
	}
	return s.Repo.FindUserByID(ctx, id)
}
