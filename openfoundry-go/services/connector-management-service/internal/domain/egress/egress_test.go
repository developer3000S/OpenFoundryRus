package egress

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestValidateURLBlocksHostsOutsideAllowlist(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"api.example.com"}}
	err := ValidateURL(mustURL(t, "https://internal.example.net/v1/data"), policy)
	require.ErrorContains(t, err, "egress policy does not allow host 'internal.example.net'")
}

func TestValidateURLAllowsWildcardSubdomainsAndRoot(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"*.example.com"}}
	require.NoError(t, ValidateURL(mustURL(t, "https://sales.eu.example.com/odata"), policy))
	require.NoError(t, ValidateURL(mustURL(t, "https://example.com/odata"), policy))
}

func TestValidateURLBlocksPrivateNetworkWhenDisabled(t *testing.T) {
	policy := Policy{AllowInsecureHTTP: true}
	err := ValidateURL(mustURL(t, "http://10.0.0.4/feed"), policy)
	require.ErrorContains(t, err, "egress policy blocks private-network host '10.0.0.4'")
}

func TestValidateURLBlocksInsecureHTTPToPublicHost(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"api.example.com"}}
	err := ValidateURL(mustURL(t, "http://api.example.com/v1"), policy)
	require.ErrorContains(t, err, "egress policy blocks insecure HTTP to host 'api.example.com'")
}

func TestValidateURLAllowsLocalhostHTTPWithoutPrivateNetworkFlag(t *testing.T) {
	policy := Policy{}
	require.NoError(t, ValidateURL(mustURL(t, "http://127.0.0.1:8080/probe"), policy))
	require.NoError(t, ValidateURL(mustURL(t, "http://localhost:8080/probe"), policy))
}

func TestValidateURLBlockedHostsOverrideAllowlist(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"*.example.com"}, BlockedHosts: []string{"sales.eu.example.com"}}
	err := ValidateURL(mustURL(t, "https://sales.eu.example.com/odata"), policy)
	require.ErrorContains(t, err, "egress policy blocks host 'sales.eu.example.com'")
}

func TestValidateURLRejectsMissingHost(t *testing.T) {
	policy := Policy{}
	err := ValidateURL(mustURL(t, "mailto:admin@example.com"), policy)
	require.ErrorContains(t, err, "has no host")
	err = ValidateURL(nil, policy)
	require.ErrorContains(t, err, "has no host")
}

func TestFromStateAndConfigNormalizesAndOverrides(t *testing.T) {
	policy := FromStateAndConfig(
		[]string{" API.Upstream.example.com ", ""},
		false,
		map[string]any{
			"egress": map[string]any{
				"allowed_hosts":          []any{" *.Override.Example.com ", 17, ""},
				"blocked_hosts":          []any{" Bad.Override.Example.com "},
				"allow_private_networks": true,
				"allow_insecure_http":    true,
			},
		},
	)
	require.Equal(t, []string{"*.override.example.com"}, policy.AllowedHosts)
	require.Equal(t, []string{"bad.override.example.com"}, policy.BlockedHosts)
	require.True(t, policy.AllowPrivateNetworks)
	require.True(t, policy.AllowInsecureHTTP)
}

func TestFromStateAndConfigFallsBackToStateDefaults(t *testing.T) {
	policy := FromStateAndConfig([]string{" API.Upstream.example.com "}, true, nil)
	require.Equal(t, []string{"api.upstream.example.com"}, policy.AllowedHosts)
	require.Empty(t, policy.BlockedHosts)
	require.True(t, policy.AllowPrivateNetworks)
	require.False(t, policy.AllowInsecureHTTP)
}

func TestPrivateNetworkClassificationMatchesRust(t *testing.T) {
	privateHosts := []string{"10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.10", "127.0.0.2", "169.254.1.1", "::1", "fc00::1", "fd12:3456::1"}
	for _, host := range privateHosts {
		require.Truef(t, IsPrivateIP(host), "%s should be private", host)
	}
	publicHosts := []string{"8.8.8.8", "172.15.255.255", "172.32.0.1", "2001:4860:4860::8888", "internal.example.com"}
	for _, host := range publicHosts {
		require.Falsef(t, IsPrivateIP(host), "%s should not be private", host)
	}
}
