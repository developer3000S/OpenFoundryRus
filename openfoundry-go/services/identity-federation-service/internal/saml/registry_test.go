package saml

import (
	"sort"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	defaultSP := ServiceProviderConfig{
		EntityID:                    "http://sp/meta",
		AssertionConsumerServiceURL: "http://sp/acs",
		AllowedClockSkewSecs:        60,
	}
	reg := NewRegistry(defaultSP)
	provider := fixtureProvider()
	provider.Slug = "okta"
	reg.Register(provider, ServiceProviderConfig{})

	entry, ok := reg.Get("okta")
	if !ok {
		t.Fatalf("Get missed")
	}
	if entry.Provider != provider {
		t.Errorf("entry.Provider mismatch")
	}
	if entry.SP != defaultSP {
		t.Errorf("default SP not applied: %+v", entry.SP)
	}

	if _, ok := reg.Get("unknown"); ok {
		t.Errorf("unknown slug should miss")
	}
}

func TestRegistryRegisterOverridesSP(t *testing.T) {
	reg := NewRegistry(ServiceProviderConfig{EntityID: "default"})
	provider := fixtureProvider()
	provider.Slug = "azure"
	custom := ServiceProviderConfig{
		EntityID:                    "tenant-specific",
		AssertionConsumerServiceURL: "http://sp/acs/azure",
	}
	reg.Register(provider, custom)
	entry, _ := reg.Get("azure")
	if entry.SP != custom {
		t.Errorf("override not applied: %+v", entry.SP)
	}
}

func TestRegistryRegisterReplacesByslug(t *testing.T) {
	reg := NewRegistry(ServiceProviderConfig{})
	first := fixtureProvider()
	first.Slug = "saml"
	first.Name = "First"
	reg.Register(first, ServiceProviderConfig{})

	second := fixtureProvider()
	second.Slug = "saml"
	second.Name = "Second"
	reg.Register(second, ServiceProviderConfig{})

	entry, _ := reg.Get("saml")
	if entry.Provider.Name != "Second" {
		t.Errorf("re-registration did not replace entry")
	}
}

func TestRegistryRegisterNilProviderNoop(t *testing.T) {
	reg := NewRegistry(ServiceProviderConfig{})
	reg.Register(nil, ServiceProviderConfig{})
	if len(reg.Names()) != 0 {
		t.Errorf("nil register should not add an entry")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry(ServiceProviderConfig{})
	for _, slug := range []string{"okta", "azure", "auth0"} {
		p := fixtureProvider()
		p.Slug = slug
		reg.Register(p, ServiceProviderConfig{})
	}
	names := reg.Names()
	sort.Strings(names)
	want := []string{"auth0", "azure", "okta"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range names {
		if names[i] != want[i] {
			t.Errorf("names[%d]: got %q, want %q", i, names[i], want[i])
		}
	}
}
