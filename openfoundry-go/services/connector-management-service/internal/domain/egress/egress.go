// Package egress mirrors the Rust connector-management egress policy helpers in
// `services/connector-management-service/src/domain/egress.rs`.
//
// The helpers only validate URLs against policy metadata owned by the connector
// management plane. They do not open sockets, resolve DNS, or enforce an
// external network boundary; Rust delegates that boundary to the HTTP runtime /
// connector-agent / network-boundary services, and Go keeps the same ownership
// split.
package egress

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Policy mirrors Rust's EgressPolicy. Host lists are normalized to trimmed,
// lower-case values by FromStateAndConfig.
type Policy struct {
	AllowedHosts         []string
	BlockedHosts         []string
	AllowPrivateNetworks bool
	AllowInsecureHTTP    bool
}

// FromStateAndConfig mirrors Rust's EgressPolicy::from_state_and_config.
//
// The state inputs correspond to AppState.allowed_egress_hosts and
// AppState.allow_private_network_egress. Per-connection overrides live under
// config.egress.{allowed_hosts, blocked_hosts, allow_private_networks,
// allow_insecure_http}. Non-string entries in host lists are ignored, matching
// Rust's serde_json::Value::as_str filter_map behavior.
func FromStateAndConfig(allowedHosts []string, allowPrivateNetworks bool, config map[string]any) Policy {
	policy := Policy{
		AllowedHosts:         normalizeHostList(allowedHosts),
		BlockedHosts:         nil,
		AllowPrivateNetworks: allowPrivateNetworks,
		AllowInsecureHTTP:    false,
	}
	egressConfig, _ := config["egress"].(map[string]any)
	if egressConfig == nil {
		return policy
	}
	if hosts := stringList(egressConfig["allowed_hosts"]); hosts != nil {
		policy.AllowedHosts = hosts
	}
	if hosts := stringList(egressConfig["blocked_hosts"]); hosts != nil {
		policy.BlockedHosts = hosts
	}
	if allow, ok := egressConfig["allow_private_networks"].(bool); ok {
		policy.AllowPrivateNetworks = allow
	}
	if allow, ok := egressConfig["allow_insecure_http"].(bool); ok {
		policy.AllowInsecureHTTP = allow
	}
	return policy
}

// ValidateURL mirrors Rust's validate_url.
func ValidateURL(u *url.URL, policy Policy) error {
	if u == nil {
		return fmt.Errorf("URL '<nil>' has no host")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("URL '%s' has no host", u.String())
	}

	scheme := strings.ToLower(u.Scheme)
	if !policy.AllowInsecureHTTP && scheme == "http" && !IsLocalhost(host) && !IsPrivateIP(host) {
		return fmt.Errorf("egress policy blocks insecure HTTP to host '%s'", host)
	}
	if !policy.AllowPrivateNetworks && IsPrivateIP(host) && !IsLocalhost(host) {
		return fmt.Errorf("egress policy blocks private-network host '%s'", host)
	}
	if HostMatchesAny(host, policy.BlockedHosts) {
		return fmt.Errorf("egress policy blocks host '%s'", host)
	}
	if len(policy.AllowedHosts) > 0 && !HostMatchesAny(host, policy.AllowedHosts) {
		return fmt.Errorf("egress policy does not allow host '%s'", host)
	}
	return nil
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeHostList(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			out = append(out, host)
		}
	}
	return out
}

// HostMatchesAny returns true when host matches any allow/block pattern.
func HostMatchesAny(host string, patterns []string) bool {
	for _, pattern := range patterns {
		if HostMatches(host, pattern) {
			return true
		}
	}
	return false
}

// HostMatches mirrors Rust's host_matches helper.
func HostMatches(host, pattern string) bool {
	host = strings.ToLower(host)
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

// IsLocalhost mirrors Rust's is_localhost helper.
func IsLocalhost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// IsPrivateIP mirrors Rust's is_private_ip helper: 10/8, 172.16/12,
// 192.168/16, 127/8, 169.254/16, IPv6 loopback, and IPv6 unique-local
// addresses (fc00::/7) are private. Hostnames are not resolved here.
func IsPrivateIP(host string) bool {
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
	return len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc
}
