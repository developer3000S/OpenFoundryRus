package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const (
	RESTAPIAuthNone         = "none"
	RESTAPIAuthBasic        = "basic"
	RESTAPIAuthBearer       = "bearer"
	RESTAPIAuthAPIKey       = "api_key"
	RESTAPIAuthCustomHeader = "custom_header"

	RESTAPIWorkerFoundry = "foundry"
	RESTAPIWorkerAgent   = "agent"
)

type RESTAPISourceConfig struct {
	SourceKind   string                      `json:"source_kind,omitempty"`
	Domain       string                      `json:"domain"`
	BaseURL      string                      `json:"base_url"`
	HealthPath   string                      `json:"health_path,omitempty"`
	ResourcePath string                      `json:"resource_path,omitempty"`
	ResourceName string                      `json:"resource_name,omitempty"`
	CatalogPath  string                      `json:"catalog_path,omitempty"`
	Headers      map[string]string           `json:"headers,omitempty"`
	QueryParams  map[string]string           `json:"query_params,omitempty"`
	Resources    []json.RawMessage           `json:"resources,omitempty"`
	Auth         RESTAPIAuthConfig           `json:"auth"`
	Secrets      map[string]string           `json:"secrets,omitempty"`
	Runtime      RESTAPIRuntimePolicy        `json:"runtime"`
	Permissions  RESTAPIPermissions          `json:"permissions"`
	Webhook      *RESTAPIWebhookConfig       `json:"webhook,omitempty"`
	Listener     *InboundListenerDefinition  `json:"listener,omitempty"`
	Listeners    []InboundListenerDefinition `json:"listeners,omitempty"`

	// BearerToken keeps the pre-DC.1 local/dev contract working. New configs
	// should prefer auth.secret_ref plus the source credential endpoints.
	BearerToken string `json:"bearer_token,omitempty"`
}

type RESTAPIAuthConfig struct {
	Type              string `json:"type"`
	Scheme            string `json:"scheme,omitempty"`
	SecretRef         string `json:"secret_ref,omitempty"`
	HeaderName        string `json:"header_name,omitempty"`
	QueryParam        string `json:"query_param,omitempty"`
	Token             string `json:"token,omitempty"`
	Value             string `json:"value,omitempty"`
	UsernameSecretRef string `json:"username_secret_ref,omitempty"`
	PasswordSecretRef string `json:"password_secret_ref,omitempty"`
}

type RESTAPIRuntimePolicy struct {
	Worker               string   `json:"worker"`
	TimeoutMS            int      `json:"timeout_ms"`
	RetryCount           int      `json:"retry_count"`
	AllowedMethods       []string `json:"allowed_methods"`
	AllowPrivateNetworks bool     `json:"allow_private_networks"`
}

type RESTAPIPermissions struct {
	Discoverable       bool     `json:"discoverable"`
	Syncable           bool     `json:"syncable"`
	Invokable          bool     `json:"invokable"`
	UsableInCode       bool     `json:"usable_in_code"`
	AllowedEgressHosts []string `json:"allowed_egress_hosts,omitempty"`
	PolicyIDs          []string `json:"policy_ids,omitempty"`
}

type RESTAPIWebhookConfig struct {
	Path             string                   `json:"path,omitempty"`
	Method           string                   `json:"method,omitempty"`
	Headers          map[string]string        `json:"headers,omitempty"`
	QueryParams      map[string]string        `json:"query_params,omitempty"`
	Body             json.RawMessage          `json:"body,omitempty"`
	BodyTemplate     string                   `json:"body_template,omitempty"`
	Inputs           []WebhookParameter       `json:"inputs,omitempty"`
	Calls            []WebhookCall            `json:"calls,omitempty"`
	Outputs          []WebhookOutputParameter `json:"outputs,omitempty"`
	TimeoutMS        int                      `json:"timeout_ms,omitempty"`
	ConcurrencyLimit int                      `json:"concurrency_limit,omitempty"`
	RateLimit        *WebhookRateLimit        `json:"rate_limit,omitempty"`
	Limits           WebhookInvocationLimits  `json:"limits,omitempty"`
	History          WebhookHistorySettings   `json:"history,omitempty"`
}

func NormalizeRESTAPISourceConfig(raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := ParseRESTAPISourceConfig(raw)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("rest_api source: marshal config: %w", err)
	}
	return out, nil
}

func ParseRESTAPISourceConfig(raw json.RawMessage) (*RESTAPISourceConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, errors.New("rest_api source requires config")
	}
	var rawObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawObject); err != nil {
		return nil, fmt.Errorf("rest_api source: invalid config: %w", err)
	}
	_, hasPermissions := rawObject["permissions"]
	var cfg RESTAPISourceConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("rest_api source: invalid config: %w", err)
	}
	if err := normalizeRESTAPISourceConfig(&cfg, hasPermissions); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func normalizeRESTAPISourceConfig(cfg *RESTAPISourceConfig, hasPermissions bool) error {
	cfg.SourceKind = strings.TrimSpace(cfg.SourceKind)
	if cfg.SourceKind == "" {
		cfg.SourceKind = "rest_api"
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Domain = strings.TrimSpace(cfg.Domain)
	if cfg.BaseURL == "" && cfg.Domain != "" {
		cfg.BaseURL = baseURLFromDomain(cfg.Domain)
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("rest_api source requires an http(s) base_url or domain")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("rest_api source base_url must use http or https")
	}
	cfg.BaseURL = strings.TrimRight(parsed.String(), "/")
	cfg.Domain = parsed.Host
	cfg.Auth = normalizeRESTAPIAuth(cfg.Auth, cfg.BearerToken)
	if err := validateRESTAPIAuth(cfg.Auth); err != nil {
		return err
	}
	cfg.Runtime = normalizeRESTAPIRuntime(cfg.Runtime)
	if err := validateRESTAPIRuntime(cfg.Runtime); err != nil {
		return err
	}
	cfg.Permissions = normalizeRESTAPIPermissions(cfg.Permissions, cfg.Domain, hasPermissions)
	if cfg.Webhook != nil {
		cfg.Webhook.Method = strings.ToUpper(strings.TrimSpace(cfg.Webhook.Method))
		if cfg.Webhook.Method == "" {
			cfg.Webhook.Method = "GET"
		}
		if !allowedRESTAPIMethod(cfg.Webhook.Method) {
			return fmt.Errorf("rest_api source webhook method %q is not supported", cfg.Webhook.Method)
		}
		cfg.Webhook.Path = strings.TrimSpace(cfg.Webhook.Path)
	}
	return nil
}

func baseURLFromDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if strings.Contains(domain, "://") {
		return strings.TrimRight(domain, "/")
	}
	return "https://" + strings.TrimRight(domain, "/")
}

func normalizeRESTAPIAuth(auth RESTAPIAuthConfig, legacyBearer string) RESTAPIAuthConfig {
	auth.Type = normalizeRESTAPIAuthType(auth.Type)
	if auth.Type == "" {
		if strings.TrimSpace(legacyBearer) != "" {
			auth.Type = RESTAPIAuthBearer
			auth.Token = strings.TrimSpace(legacyBearer)
		} else {
			auth.Type = RESTAPIAuthNone
		}
	}
	auth.Scheme = strings.TrimSpace(auth.Scheme)
	auth.SecretRef = strings.TrimSpace(auth.SecretRef)
	auth.HeaderName = strings.TrimSpace(auth.HeaderName)
	auth.QueryParam = strings.TrimSpace(auth.QueryParam)
	auth.Token = strings.TrimSpace(auth.Token)
	auth.Value = strings.TrimSpace(auth.Value)
	auth.UsernameSecretRef = strings.TrimSpace(auth.UsernameSecretRef)
	auth.PasswordSecretRef = strings.TrimSpace(auth.PasswordSecretRef)
	if auth.Type == RESTAPIAuthBearer && auth.Scheme == "" {
		auth.Scheme = "Bearer"
	}
	if auth.Type == RESTAPIAuthAPIKey && auth.HeaderName == "" && auth.QueryParam == "" {
		auth.HeaderName = "X-API-Key"
	}
	return auth
}

func normalizeRESTAPIAuthType(v string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(v, "-", "_"))) {
	case "", RESTAPIAuthNone:
		return strings.ToLower(strings.TrimSpace(v))
	case "bearer_token", "bearer token", "token":
		return RESTAPIAuthBearer
	case "apikey", "api key":
		return RESTAPIAuthAPIKey
	case RESTAPIAuthBasic, RESTAPIAuthBearer, RESTAPIAuthAPIKey, RESTAPIAuthCustomHeader:
		return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(v, "-", "_")))
	default:
		return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(v, "-", "_")))
	}
}

func validateRESTAPIAuth(auth RESTAPIAuthConfig) error {
	switch auth.Type {
	case RESTAPIAuthNone, RESTAPIAuthBasic, RESTAPIAuthBearer, RESTAPIAuthAPIKey, RESTAPIAuthCustomHeader:
	default:
		return fmt.Errorf("rest_api source auth.type %q is not supported", auth.Type)
	}
	if auth.Type == RESTAPIAuthCustomHeader && auth.HeaderName == "" {
		return errors.New("rest_api source custom_header auth requires header_name")
	}
	return nil
}

func normalizeRESTAPIRuntime(runtime RESTAPIRuntimePolicy) RESTAPIRuntimePolicy {
	runtime.Worker = strings.ToLower(strings.TrimSpace(runtime.Worker))
	if runtime.Worker == "" {
		runtime.Worker = RESTAPIWorkerFoundry
	}
	if runtime.TimeoutMS == 0 {
		runtime.TimeoutMS = 10000
	}
	if runtime.RetryCount < 0 {
		runtime.RetryCount = 0
	}
	if len(runtime.AllowedMethods) == 0 {
		runtime.AllowedMethods = []string{"GET", "POST"}
	} else {
		methods := make([]string, 0, len(runtime.AllowedMethods))
		seen := map[string]bool{}
		for _, method := range runtime.AllowedMethods {
			normalized := strings.ToUpper(strings.TrimSpace(method))
			if normalized == "" || seen[normalized] {
				continue
			}
			methods = append(methods, normalized)
			seen[normalized] = true
		}
		sort.Strings(methods)
		runtime.AllowedMethods = methods
	}
	return runtime
}

func validateRESTAPIRuntime(runtime RESTAPIRuntimePolicy) error {
	if runtime.Worker != RESTAPIWorkerFoundry && runtime.Worker != RESTAPIWorkerAgent {
		return fmt.Errorf("rest_api source runtime.worker must be %q or %q", RESTAPIWorkerFoundry, RESTAPIWorkerAgent)
	}
	if runtime.TimeoutMS < 100 || runtime.TimeoutMS > 120000 {
		return errors.New("rest_api source runtime.timeout_ms must be between 100 and 120000")
	}
	for _, method := range runtime.AllowedMethods {
		if !allowedRESTAPIMethod(method) {
			return fmt.Errorf("rest_api source runtime.allowed_methods contains unsupported method %q", method)
		}
	}
	return nil
}

func allowedRESTAPIMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
		return true
	default:
		return false
	}
}

func normalizeRESTAPIPermissions(permissions RESTAPIPermissions, domain string, hasPermissions bool) RESTAPIPermissions {
	if !hasPermissions && restAPIPermissionsEmpty(permissions) {
		permissions = RESTAPIPermissions{
			Discoverable: true,
			Syncable:     false,
			Invokable:    true,
			UsableInCode: true,
		}
	}
	if len(permissions.AllowedEgressHosts) == 0 && strings.TrimSpace(domain) != "" {
		permissions.AllowedEgressHosts = []string{domain}
	}
	permissions.AllowedEgressHosts = normalizeStringSet(permissions.AllowedEgressHosts)
	permissions.PolicyIDs = normalizeStringSet(permissions.PolicyIDs)
	return permissions
}

func restAPIPermissionsEmpty(permissions RESTAPIPermissions) bool {
	return !permissions.Discoverable &&
		!permissions.Syncable &&
		!permissions.Invokable &&
		!permissions.UsableInCode &&
		len(permissions.AllowedEgressHosts) == 0 &&
		len(permissions.PolicyIDs) == 0
}

func normalizeStringSet(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		out = append(out, trimmed)
		seen[trimmed] = true
	}
	sort.Strings(out)
	return out
}
