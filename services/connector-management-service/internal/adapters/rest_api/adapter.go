// Package rest_api is the Go port of the Rust REST API connector that
// lives in `services/connector-management-service/src/connectors/rest_api.rs`.
//
// Capabilities mirrored from the Rust module:
//
//   - DiscoverSources    — inline `resources[]` short-circuit;
//     otherwise GET against an optional `catalog_path`, normalising
//     `{data|items|records|value}[]` envelopes; if neither inline nor
//     catalog yields entries, a single `rest_resource` fallback is
//     synthesised from `resource_path` / `resource_name`.
//   - QueryVirtualTable  — bounded GET against the resource path,
//     JSON rows clamped to [1, 500].
//   - StreamArrow        — not exposed by the Rust connector; returns
//     [adapters.ErrNotImplemented].
//   - BuildIngestSpec    — placeholder spec the bridge forwards to
//     ingestion-replication-service; the selector is the resource path.
//   - TestConnection     — validates the modeled REST API source by issuing
//     a bounded GET to the configured health/resource path.
//
// HTTP egress: stdlib http.Client, bearer/API-key/custom-header auth. The
// Rust `http_runtime` module that supports connector-agent proxying
// and EgressPolicy validation is not yet ported here; per-instance
// overrides via [Adapter.SetHTTPClient] keep the test surface
// symmetric with the salesforce / sap adapters.
package rest_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

const (
	// ConnectorType is the `connections.connector_type` value the registry
	// binds this adapter under. Mirrors the Rust dispatch arm.
	ConnectorType = "rest_api"

	defaultSourceKind   = "rest_resource"
	defaultPreviewLimit = 50
)

// Adapter implements [adapters.ConnectorAdapter] for generic REST APIs.
// It is safe for concurrent use; the embedded HTTP client is reused
// across requests.
type Adapter struct {
	httpClient *http.Client
}

// New returns a ready-to-use [Adapter] backed by [http.DefaultClient].
func New() *Adapter {
	return &Adapter{httpClient: http.DefaultClient}
}

// Factory returns an [adapters.Factory] that constructs fresh REST
// adapters; the registry stores the factory and asks for an instance
// per request so per-connection state stays scoped.
func Factory() adapters.Factory {
	return adapters.FactoryFunc(func() adapters.ConnectorAdapter { return New() })
}

// SetHTTPClient overrides the embedded [http.Client]. Intended for
// tests pointing at an httptest server.
func (a *Adapter) SetHTTPClient(client *http.Client) {
	if client != nil {
		a.httpClient = client
	}
}

type restConfig struct {
	SourceKind   string                       `json:"source_kind"`
	Domain       string                       `json:"domain"`
	BaseURL      string                       `json:"base_url"`
	HealthPath   string                       `json:"health_path"`
	ResourcePath string                       `json:"resource_path"`
	ResourceName string                       `json:"resource_name"`
	CatalogPath  string                       `json:"catalog_path"`
	BearerToken  string                       `json:"bearer_token"`
	Headers      map[string]string            `json:"headers"`
	QueryParams  map[string]string            `json:"query_params"`
	Resources    []json.RawMessage            `json:"resources"`
	Auth         models.RESTAPIAuthConfig     `json:"auth"`
	Runtime      models.RESTAPIRuntimePolicy  `json:"runtime"`
	Permissions  models.RESTAPIPermissions    `json:"permissions"`
	Webhook      *models.RESTAPIWebhookConfig `json:"webhook"`
}

func parseConfig(raw json.RawMessage) (*restConfig, error) {
	cfg := &restConfig{}
	if len(raw) == 0 {
		return cfg, nil
	}
	normalized, err := models.NormalizeRESTAPISourceConfig(raw)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(normalized, cfg); err != nil {
		return nil, fmt.Errorf("rest_api: invalid config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig mirrors Rust's `validate_config`: a non-empty
// `base_url` is the only required identity field.
func ValidateConfig(raw json.RawMessage) error {
	_, err := models.NormalizeRESTAPISourceConfig(raw)
	return err
}

// DiscoverSources mirrors Rust's `discover_sources`. Inline
// `resources[]` short-circuits the HTTP path; otherwise a GET against
// `catalog_path` is normalised through the same `{data|items|records|
// value}[]` envelope rules; if neither inline nor catalog yields
// entries, a single `rest_resource` fallback is synthesised from
// `resource_path` / `resource_name`.
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	cfg, err := a.cfg(c)
	if err != nil {
		return nil, err
	}
	if len(cfg.Resources) > 0 {
		out := make([]adapters.Source, 0, len(cfg.Resources))
		for _, raw := range cfg.Resources {
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				continue
			}
			if src, ok := discoveredFromConfig(v, cfg.Permissions); ok {
				out = append(out, src)
			}
		}
		return out, nil
	}

	if cfg.CatalogPath != "" {
		u, err := buildURL(cfg, cfg.CatalogPath, false)
		if err != nil {
			return nil, err
		}
		body, err := a.fetch(ctx, cfg, u, "REST catalog returned HTTP")
		if err != nil {
			return nil, err
		}
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("rest_api: decode catalog payload: %w", err)
		}
		records := normalizeRecords(payload)
		entries := make([]adapters.Source, 0, len(records))
		for _, rec := range records {
			if src, ok := discoveredFromConfig(rec, cfg.Permissions); ok {
				entries = append(entries, src)
			}
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}

	// Fallback: synthesise a single resource from `resource_path` /
	// `resource_name`, matching Rust's last-resort path.
	resourcePath := strings.TrimSpace(cfg.ResourcePath)
	if resourcePath == "" {
		resourcePath = "/"
	}
	resourceName := strings.TrimSpace(cfg.ResourceName)
	if resourceName == "" {
		resourceName = "REST resource"
	}
	meta, _ := json.Marshal(map[string]any{
		"base_url":    cfg.BaseURL,
		"domain":      cfg.Domain,
		"runtime":     cfg.Runtime,
		"permissions": cfg.Permissions,
	})
	return []adapters.Source{{
		Selector:         resourcePath,
		DisplayName:      resourceName,
		SourceKind:       defaultSourceKind,
		SupportsSync:     cfg.Permissions.Syncable,
		SupportsZeroCopy: cfg.Permissions.Discoverable || cfg.Permissions.UsableInCode,
		Metadata:         meta,
	}}, nil
}

func (a *Adapter) TestConnection(ctx context.Context, raw json.RawMessage) (adapters.ConnectionTestResult, error) {
	start := time.Now()
	cfg, err := parseConfig(raw)
	if err != nil {
		return adapters.ConnectionTestResult{}, err
	}
	u, err := buildURL(cfg, "", true)
	if err != nil {
		return adapters.ConnectionTestResult{}, err
	}
	body, err := a.fetch(ctx, cfg, u, "REST API source test returned HTTP")
	if err != nil {
		return adapters.ConnectionTestResult{}, err
	}
	details, _ := json.Marshal(map[string]any{
		"base_url":    cfg.BaseURL,
		"domain":      cfg.Domain,
		"auth_type":   cfg.Auth.Type,
		"worker":      cfg.Runtime.Worker,
		"status_code": http.StatusOK,
		"bytes":       len(body),
	})
	return adapters.ConnectionTestResult{
		Success:   true,
		Message:   "validated REST API source",
		LatencyMS: time.Since(start).Milliseconds(),
		Details:   details,
	}, nil
}

// QueryVirtualTable mirrors Rust's `query_virtual_table`: GETs the
// resource at `selector`, normalises through the same envelope rules,
// and returns rows clamped to [1, 500].
func (a *Adapter) QueryVirtualTable(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (*adapters.Result, error) {
	if q == nil {
		return nil, errors.New("rest_api: query request is nil")
	}
	cfg, err := a.cfg(c)
	if err != nil {
		return nil, err
	}
	limit := defaultPreviewLimit
	if q.Limit != nil {
		limit = *q.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	u, err := buildURL(cfg, q.Selector, false)
	if err != nil {
		return nil, err
	}
	body, err := a.fetch(ctx, cfg, u, "REST source returned HTTP")
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("rest_api: decode resource payload: %w", err)
	}
	records := normalizeRecords(payload)
	if len(records) > limit {
		records = records[:limit]
	}
	rawRows := make([]json.RawMessage, 0, len(records))
	for _, rec := range records {
		buf, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("rest_api: marshal preview row: %w", err)
		}
		rawRows = append(rawRows, buf)
	}
	meta, _ := json.Marshal(map[string]any{
		"selector": q.Selector,
		"url":      u.String(),
		"rows":     int64(len(rawRows)),
	})
	return &adapters.Result{
		Selector: q.Selector,
		Mode:     "zero_copy",
		Columns:  columnsFromRows(rawRows),
		RowCount: len(rawRows),
		Rows:     rawRows,
		Metadata: meta,
	}, nil
}

// StreamArrow returns [adapters.ErrNotImplemented]: the Rust REST
// connector does not expose `stream_arrow_ipc`; sync rows go through
// the JSON `fetch_dataset` path.
func (a *Adapter) StreamArrow(_ context.Context, _ *models.Connection, _ *adapters.Query, _ string) (adapters.ArrowStream, error) {
	return nil, fmt.Errorf("%w: rest_api arrow streaming", adapters.ErrNotImplemented)
}

// BuildIngestSpec emits the `rest_api` source variant the bridge
// forwards to ingestion-replication-service. The selector becomes the
// `resource_path`; base URL and authorisation are passed through so
// the bridge can re-fetch the resource at sync time.
func (a *Adapter) BuildIngestSpec(_ context.Context, c *models.Connection, src *adapters.Source) (*adapters.IngestSpec, error) {
	if c == nil {
		return nil, errors.New("rest_api: connection is nil")
	}
	if src == nil {
		return nil, errors.New("rest_api: source is nil")
	}
	cfg, err := parseConfig(c.Config)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("rest_api: connection config missing 'base_url'")
	}
	specCfg := map[string]any{
		"base_url":      cfg.BaseURL,
		"domain":        cfg.Domain,
		"resource_path": src.Selector,
		"auth":          cfg.Auth,
		"runtime":       cfg.Runtime,
		"permissions":   cfg.Permissions,
	}
	raw, err := json.Marshal(specCfg)
	if err != nil {
		return nil, fmt.Errorf("rest_api: marshal ingest spec: %w", err)
	}
	return &adapters.IngestSpec{
		Name:      c.Name,
		Namespace: "default",
		Source:    ConnectorType,
		Config:    raw,
	}, nil
}

func (a *Adapter) cfg(c *models.Connection) (*restConfig, error) {
	if c == nil {
		return nil, errors.New("rest_api: connection is nil")
	}
	cfg, err := parseConfig(c.Config)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("rest_api connector requires 'base_url'")
	}
	return cfg, nil
}

func (a *Adapter) fetch(ctx context.Context, cfg *restConfig, u *url.URL, errorPrefix string) ([]byte, error) {
	reqURL := *u
	query := reqURL.Query()
	for k, v := range cfg.QueryParams {
		if _, exists := query[k]; !exists {
			query.Set(k, v)
		}
	}
	reqURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("rest_api: build %s: %w", reqURL.String(), err)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	applyAuth(req, cfg)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rest_api: transport error: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %d", errorPrefix, resp.StatusCode)
	}
	return body, nil
}

func applyAuth(req *http.Request, cfg *restConfig) {
	switch cfg.Auth.Type {
	case models.RESTAPIAuthBearer:
		token := cfg.Auth.Token
		if token == "" {
			token = cfg.BearerToken
		}
		if token != "" {
			scheme := strings.TrimSpace(cfg.Auth.Scheme)
			if scheme == "" {
				scheme = "Bearer"
			}
			req.Header.Set("Authorization", scheme+" "+token)
		}
	case models.RESTAPIAuthAPIKey:
		value := cfg.Auth.Value
		if value == "" {
			return
		}
		if cfg.Auth.HeaderName != "" {
			req.Header.Set(cfg.Auth.HeaderName, value)
		}
		if cfg.Auth.QueryParam != "" {
			q := req.URL.Query()
			q.Set(cfg.Auth.QueryParam, value)
			req.URL.RawQuery = q.Encode()
		}
	case models.RESTAPIAuthCustomHeader:
		if cfg.Auth.HeaderName != "" && cfg.Auth.Value != "" {
			req.Header.Set(cfg.Auth.HeaderName, cfg.Auth.Value)
		}
	}
}

func buildURL(cfg *restConfig, selector string, forHealth bool) (*url.URL, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("rest_api: parse base_url: %w", err)
	}
	var path string
	if forHealth {
		path = strings.TrimSpace(cfg.HealthPath)
		if path == "" {
			path = strings.TrimSpace(cfg.ResourcePath)
		}
		if path == "" {
			path = strings.TrimSpace(selector)
		}
		if path == "" {
			path = "/health"
		}
	} else {
		path = strings.TrimSpace(selector)
		if path == "" {
			path = strings.TrimSpace(cfg.ResourcePath)
		}
		if path == "" {
			path = "/"
		}
	}
	rel, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("rest_api: parse path %q: %w", path, err)
	}
	return base.ResolveReference(rel), nil
}

// normalizeRecords mirrors Rust's `normalize_records`: peels off the
// `{data|items|records|value}[]` wrappers, returns an array as-is,
// wraps a single object into a one-element array, and wraps any other
// scalar as `[{ "value": <scalar> }]`.
func normalizeRecords(payload any) []any {
	switch v := payload.(type) {
	case []any:
		return v
	case map[string]any:
		for _, key := range []string{"data", "items", "records", "value"} {
			if list, ok := v[key].([]any); ok {
				return list
			}
		}
		return []any{v}
	default:
		return []any{map[string]any{"value": v}}
	}
}

func discoveredFromConfig(value any, permissions models.RESTAPIPermissions) (adapters.Source, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return adapters.Source{}, false
	}
	selector := stringOrEmpty(obj, "selector", "path")
	if selector == "" {
		return adapters.Source{}, false
	}
	display := stringOrEmpty(obj, "display_name", "name")
	if display == "" {
		display = selector
	}
	meta, _ := json.Marshal(obj)
	return adapters.Source{
		Selector:         selector,
		DisplayName:      display,
		SourceKind:       defaultSourceKind,
		SupportsSync:     permissions.Syncable,
		SupportsZeroCopy: permissions.Discoverable || permissions.UsableInCode,
		Metadata:         meta,
	}, true
}

func stringOrEmpty(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func columnsFromRows(rows []json.RawMessage) []string {
	for _, row := range rows {
		dec := json.NewDecoder(strings.NewReader(string(row)))
		tok, err := dec.Token()
		if err != nil {
			continue
		}
		delim, ok := tok.(json.Delim)
		if !ok || delim != '{' {
			continue
		}
		out := []string{}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			key, ok := tok.(string)
			if !ok {
				break
			}
			out = append(out, key)
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				break
			}
		}
		return out
	}
	return []string{}
}
