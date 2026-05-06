package saml

import (
	"sync"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

// Registry is the in-memory SAML provider catalog the SSO router
// looks up at request time. It mirrors the OIDC service's lookup
// surface (Get / ProviderNames) so handlers/sso.go can dispatch on
// `provider.ProviderType` without caring about the storage layer.
//
// A future slice can swap this for a Postgres-backed store
// (mirroring the Rust crate's `sso_providers` table) without
// changing handler code — the lookup signature is the same.
type Registry struct {
	mu        sync.RWMutex
	bySlug    map[string]*Entry
	spDefault ServiceProviderConfig
}

// Entry pairs the persisted SsoProvider row with the SP-side config
// the validators need at every callback. SP config is identical
// across providers (same SP entity_id + ACS URL) so the registry
// stamps `spDefault` onto every entry registered without an
// override.
type Entry struct {
	Provider *models.SsoProvider
	SP       ServiceProviderConfig
}

// NewRegistry returns an empty registry seeded with the default SP
// config. Use Register to add providers at boot.
func NewRegistry(spDefault ServiceProviderConfig) *Registry {
	return &Registry{
		bySlug:    map[string]*Entry{},
		spDefault: spDefault,
	}
}

// Register adds a provider to the registry. If `sp` is the zero
// value, the registry's default SP config is used. Replaces any
// existing entry under the same slug.
func (r *Registry) Register(provider *models.SsoProvider, sp ServiceProviderConfig) {
	if provider == nil {
		return
	}
	if sp == (ServiceProviderConfig{}) {
		sp = r.spDefault
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySlug[provider.Slug] = &Entry{Provider: provider, SP: sp}
}

// Get returns the entry registered for `slug` or nil when missing.
// The `ok` second return mirrors the map-style API the OIDC
// service exposes.
func (r *Registry) Get(slug string) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.bySlug[slug]
	return e, ok
}

// Names returns every registered slug. Order is not guaranteed —
// callers that need stable ordering should sort the result.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.bySlug))
	for k := range r.bySlug {
		out = append(out, k)
	}
	return out
}
