package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Service holds one *oidc.Provider per configured IdP and exposes
// the typed surface the handlers + ssorepo consume.
type Service struct {
	mu        sync.RWMutex
	providers map[string]*Provider // keyed by lowercase name
}

// Provider bundles the verified OIDC discovery + the OAuth2 config
// for one configured IdP.
type Provider struct {
	Name     string
	Cfg      ProviderConfig
	OIDC     *gooidc.Provider
	OAuth2   *oauth2.Config
	Verifier *gooidc.IDTokenVerifier
}

// NewService initialises every provider via OIDC discovery (network
// call) and returns the Service. Callers MUST pass a context with a
// reasonable deadline — discovery is the only synchronous boot step
// that depends on external infra.
func NewService(ctx context.Context, configs []ProviderConfig) (*Service, error) {
	s := &Service{providers: make(map[string]*Provider, len(configs))}
	for _, c := range configs {
		p, err := buildProvider(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("oidc provider %q: %w", c.Name, err)
		}
		s.providers[c.Name] = p
	}
	return s, nil
}

func buildProvider(ctx context.Context, c ProviderConfig) (*Provider, error) {
	prov, err := gooidc.NewProvider(ctx, c.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery (%s): %w", c.IssuerURL, err)
	}
	oauthCfg := &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  c.RedirectURL,
		Scopes:       c.Scopes,
	}
	verifier := prov.Verifier(&gooidc.Config{ClientID: c.ClientID})
	return &Provider{
		Name:     c.Name,
		Cfg:      c,
		OIDC:     prov,
		OAuth2:   oauthCfg,
		Verifier: verifier,
	}, nil
}

// ProviderNames returns the configured provider names (sorted by
// insertion order is not guaranteed — caller sorts when needed).
func (s *Service) ProviderNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.providers))
	for k := range s.providers {
		out = append(out, k)
	}
	return out
}

// Get returns (provider, ok). Caller branches on ok rather than nil-checking.
func (s *Service) Get(name string) (*Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[name]
	return p, ok
}

// AuthURLBundle is the data the start handler needs to send the user
// to the IdP and persist the matching state row.
type AuthURLBundle struct {
	URL          string
	State        string
	CodeVerifier string
	Nonce        string
}

// BuildAuthURL generates fresh state + PKCE verifier + nonce and
// returns the IdP authorize URL pre-filled with them.
//
// Persisting the state + verifier + nonce is the caller's job (see
// ssorepo). This split keeps the OIDC service stateless.
func (p *Provider) BuildAuthURL(_ context.Context) (*AuthURLBundle, error) {
	state, err := randomURLToken(32)
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		return nil, fmt.Errorf("verifier: %w", err)
	}
	nonce, err := randomURLToken(16)
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	challenge := pkceChallenge(verifier)
	url := p.OAuth2.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		gooidc.Nonce(nonce),
	)
	return &AuthURLBundle{
		URL: url, State: state, CodeVerifier: verifier, Nonce: nonce,
	}, nil
}

// Claims is the minimal slice-5a OIDC claim set we extract from the
// id_token. Slice 7's IdP mapping engine will read more (groups,
// custom attributes, etc.) — for now we map the common three:
//
//   - Subject: external_id (unique per provider)
//   - Email + EmailVerified
//   - Name (display name)
type Claims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// Exchange validates the OAuth2 code + the id_token nonce + extracts
// the claims. Mirrors the Rust crate's `domain::oauth::exchange_code`
// minus the rich IdP claim mapping.
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier, expectedNonce string) (*Claims, error) {
	tok, err := p.OAuth2.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("oidc: id_token missing from token response")
	}
	idTok, err := p.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if expectedNonce != "" && idTok.Nonce != expectedNonce {
		return nil, errors.New("oidc: id_token nonce mismatch")
	}
	var claims Claims
	if err := idTok.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	if claims.Subject == "" {
		return nil, errors.New("oidc: id_token has empty subject")
	}
	return &claims, nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// StateTTL is how long a state row remains valid. 10 minutes matches
// the Rust crate (auth_runtime.oauth_state default_time_to_live=600).
const StateTTL = 10 * time.Minute
