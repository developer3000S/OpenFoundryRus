// Egress policy ported from
// `services/connector-management-service/src/domain/egress.rs`.
//
// The policy gates outbound HTTP from bridge connectors. It enforces:
//
//   - Insecure HTTP is blocked unless `allow_insecure_http` is set OR the
//     target is loopback / private (127/8, ::1, RFC1918, link-local).
//   - Private-network targets are blocked unless `allow_private_networks`
//     is true (loopback always gets a free pass).
//   - `blocked_hosts` overrides everything.
//   - A non-empty `allowed_hosts` enforces an allowlist; wildcards and
//     suffix matches mirror Rust's `host_matches` rules.
//
// Per-request overrides live under `connection.config.egress.{allowed_hosts,
// blocked_hosts, allow_private_networks, allow_insecure_http}`. Absent keys
// inherit the [EgressPolicyFromState] inputs.
package http_runtime

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// EgressPolicy mirrors Rust's `EgressPolicy`. Host slices are stored
// lower-cased and trimmed so [ValidateURL] can compare hosts directly.
type EgressPolicy struct {
	AllowedHosts         []string
	BlockedHosts         []string
	AllowPrivateNetworks bool
	AllowInsecureHTTP    bool
}

// EgressPolicyFromState mirrors Rust's `EgressPolicy::from_state_and_config`.
// State inputs come from AppState (`allowed_egress_hosts`,
// `allow_private_network_egress`); per-request overrides come from the
// connection config's `egress` block.
func EgressPolicyFromState(
	allowedHosts []string,
	allowPrivateNetworks bool,
	config map[string]any,
) *EgressPolicy {
	policy := &EgressPolicy{
		AllowedHosts:         normaliseHostList(allowedHosts),
		BlockedHosts:         nil,
		AllowPrivateNetworks: allowPrivateNetworks,
		AllowInsecureHTTP:    false,
	}
	egress := mapField(config, "egress")
	if egress == nil {
		return policy
	}
	if list := stringList(egress["allowed_hosts"]); list != nil {
		policy.AllowedHosts = list
	}
	if list := stringList(egress["blocked_hosts"]); list != nil {
		policy.BlockedHosts = list
	}
	if v, ok := egress["allow_private_networks"].(bool); ok {
		policy.AllowPrivateNetworks = v
	}
	if v, ok := egress["allow_insecure_http"].(bool); ok {
		policy.AllowInsecureHTTP = v
	}
	return policy
}

// ValidateURL mirrors Rust's `validate_url`.
func ValidateURL(u *url.URL, policy *EgressPolicy) error {
	if u == nil {
		return fmt.Errorf("URL '<nil>' has no host")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("URL '%s' has no host", u.String())
	}

	scheme := strings.ToLower(u.Scheme)
	if !policy.AllowInsecureHTTP && scheme == "http" && !isLocalhost(host) && !isPrivateIP(host) {
		return fmt.Errorf("egress policy blocks insecure HTTP to host '%s'", host)
	}

	if !policy.AllowPrivateNetworks && isPrivateIP(host) && !isLocalhost(host) {
		return fmt.Errorf("egress policy blocks private-network host '%s'", host)
	}

	if hostMatchesAny(host, policy.BlockedHosts) {
		return fmt.Errorf("egress policy blocks host '%s'", host)
	}

	if len(policy.AllowedHosts) > 0 && !hostMatchesAny(host, policy.AllowedHosts) {
		return fmt.Errorf("egress policy does not allow host '%s'", host)
	}

	return nil
}

func mapField(config map[string]any, key string) map[string]any {
	if config == nil {
		return nil
	}
	v, ok := config[key].(map[string]any)
	if !ok {
		return nil
	}
	return v
}

func stringList(value any) []string {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func normaliseHostList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func hostMatchesAny(host string, patterns []string) bool {
	for _, p := range patterns {
		if hostMatches(host, p) {
			return true
		}
	}
	return false
}

func hostMatches(host, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern || strings.HasSuffix(host, "."+pattern)
}

func isLocalhost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// isPrivateIP mirrors Rust's `is_private_ip`: matches 10/8, 172.16-31/12,
// 192.168/16, 127/8, 169.254/16 plus IPv6 loopback and unique-local
// addresses (fc00::/7).
func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 10:
			return true
		case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
			return true
		case v4[0] == 192 && v4[1] == 168:
			return true
		case v4[0] == 127:
			return true
		case v4[0] == 169 && v4[1] == 254:
			return true
		default:
			return false
		}
	}
	if ip.IsLoopback() {
		return true
	}
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}
	return false
}
