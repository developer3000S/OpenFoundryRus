package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/oidc"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/service"
)

// SSO wires the OAuth/OIDC SSO endpoints (slice 5a).
//
// SAML lands in slice 5b. Per-org IdP rows + claim-mapping rules in
// slice 7.
type SSO struct {
	Repo   *repo.Repo
	OIDC   *oidc.Service
	Issuer *service.Issuer
}

// ListProviders handles GET /api/v1/auth/sso/providers.
//
// Public endpoint — the login page calls this to render the
// "Sign in with X" buttons.
func (s *SSO) ListProviders(w http.ResponseWriter, _ *http.Request) {
	names := s.OIDC.ProviderNames()
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"name": n, "kind": "oidc"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// Start handles GET /api/v1/auth/sso/{provider}/start.
//
// Generates state + PKCE verifier + nonce, persists them, and 302s
// the caller to the IdP authorize URL.
func (s *SSO) Start(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "provider")
	prov, ok := s.OIDC.Get(name)
	if !ok {
		writeJSONErr(w, http.StatusNotFound, "unknown provider")
		return
	}
	bundle, err := prov.BuildAuthURL(r.Context())
	if err != nil {
		slog.Error("sso start: build auth url", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	redirectAfter := r.URL.Query().Get("redirect_after")
	if redirectAfter == "" {
		redirectAfter = "/"
	}
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

	target, _ := url.Parse(st.RedirectAfter)
	q := url.Values{}
	q.Set("access_token", access)
	q.Set("refresh_token", refresh)
	q.Set("token_type", "Bearer")
	target.Fragment = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
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
