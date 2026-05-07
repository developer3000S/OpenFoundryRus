// Package http_runtime is the Go port of the Rust
// `services/connector-management-service/src/connectors/http_runtime.rs`
// helper. It is the shared HTTP transport that backs the bridge-style
// connectors (Tableau, Power BI, ODBC, JDBC, REST, …) and is consumed by
// the [internal/domain/catalog_bridge] package and any per-connector
// adapter that needs to delegate egress through the same gate.
//
// Surface ported (CMA-13 contract):
//
//   - [Client.Get]      — `http_runtime::get`
//   - [Client.PostJSON] — `http_runtime::post_json`
//   - [Client.PostForm] — `http_runtime::post_form`
//   - [JSONBody]        — `http_runtime::json_body`
//   - [BuildHeaders]    — `http_runtime::header_map`
//
// Each call returns a [ResponseEnvelope] mirroring Rust's
// `HttpResponseEnvelope { status, headers, bytes }`. Non-2xx responses do
// NOT produce errors so callers can format the connector-specific
// "<connector> bridge returned HTTP <code>" envelopes the catalog bridge
// expects.
//
// Connector-agent proxying mirrors Rust: when `agentURL` is non-empty the
// outbound call is wrapped as a JSON `agentProxyRequest` POST to
// `<agentURL>/api/v1/connector-agent/http` and the response body is
// reconstructed from the agent's `body_base64` / `text` / `json` fields
// (in that priority order).
//
// Egress policy mirrors `src/domain/egress.rs` — non-loopback HTTP requires
// `allow_insecure_http`, RFC1918 / link-local require `allow_private_networks`,
// the optional allowlist suffix-matches hostnames. Per-request overrides
// live under `config.egress.{allowed_hosts, blocked_hosts,
// allow_private_networks, allow_insecure_http}`.
package http_runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ResponseEnvelope mirrors Rust's `HttpResponseEnvelope`. Headers are kept
// as a lower-cased single-value map to match the Rust `BTreeMap<String,
// String>` shape that downstream code marshalls back to JSON.
type ResponseEnvelope struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Bytes   []byte            `json:"-"`
}

// Client holds the shared HTTP transport plus the default egress posture.
// It mirrors the AppState fields the Rust helper reads
// (`http_client`, `allowed_egress_hosts`, `allow_private_network_egress`).
type Client struct {
	HTTPClient                *http.Client
	AllowedEgressHosts        []string
	AllowPrivateNetworkEgress bool
}

// New returns a [Client] wired with the supplied transport (or
// [http.DefaultClient] when nil) and the egress defaults that came from
// AppState.
func New(httpClient *http.Client, allowedHosts []string, allowPrivateNetwork bool) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		HTTPClient:                httpClient,
		AllowedEgressHosts:        append([]string(nil), allowedHosts...),
		AllowPrivateNetworkEgress: allowPrivateNetwork,
	}
}

func (c *Client) httpClient() *http.Client {
	if c == nil || c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// Get mirrors Rust's `http_runtime::get`. The supplied config feeds the
// per-request [EgressPolicy] overrides.
func (c *Client) Get(
	ctx context.Context,
	config map[string]any,
	u *url.URL,
	headers http.Header,
	bearerToken string,
	agentURL string,
) (*ResponseEnvelope, error) {
	policy := c.policyForConfig(config)
	if err := ValidateURL(u, policy); err != nil {
		return nil, err
	}
	if agentURL != "" {
		return c.proxyViaAgent(ctx, agentURL, http.MethodGet, u, headers, bearerToken, nil)
	}
	return c.do(ctx, http.MethodGet, u, headers, bearerToken, nil, "")
}

// PostJSON mirrors Rust's `http_runtime::post_json`. The body is
// JSON-marshalled before it goes on the wire.
func (c *Client) PostJSON(
	ctx context.Context,
	config map[string]any,
	u *url.URL,
	headers http.Header,
	bearerToken string,
	body any,
	agentURL string,
) (*ResponseEnvelope, error) {
	policy := c.policyForConfig(config)
	if err := ValidateURL(u, policy); err != nil {
		return nil, err
	}
	if agentURL != "" {
		return c.proxyViaAgent(ctx, agentURL, http.MethodPost, u, headers, bearerToken, body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}
	return c.do(ctx, http.MethodPost, u, headers, bearerToken, encoded, "application/json")
}

// PostForm mirrors Rust's `http_runtime::post_form`. The form values are
// urlencoded with `application/x-www-form-urlencoded`. The connector-agent
// proxy is intentionally bypassed because the Rust helper's agent contract
// only supports JSON bodies — `agentURL` is accepted for signature parity
// but ignored.
func (c *Client) PostForm(
	ctx context.Context,
	config map[string]any,
	u *url.URL,
	headers http.Header,
	form [][2]string,
	agentURL string,
) (*ResponseEnvelope, error) {
	_ = agentURL
	policy := c.policyForConfig(config)
	if err := ValidateURL(u, policy); err != nil {
		return nil, err
	}
	values := url.Values{}
	for _, kv := range form {
		values.Add(kv[0], kv[1])
	}
	encoded := []byte(values.Encode())
	return c.do(ctx, http.MethodPost, u, headers, "", encoded, "application/x-www-form-urlencoded")
}

func (c *Client) do(
	ctx context.Context,
	method string,
	u *url.URL,
	headers http.Header,
	bearerToken string,
	body []byte,
	contentType string,
) (*ResponseEnvelope, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("build %s %s: %w", method, u.String(), err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &ResponseEnvelope{
		Status:  resp.StatusCode,
		Headers: headerMapToStrings(resp.Header),
		Bytes:   respBody,
	}, nil
}

type agentProxyRequest struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	BearerToken string            `json:"bearer_token,omitempty"`
	JSONBody    json.RawMessage   `json:"json_body,omitempty"`
}

type agentProxyResponse struct {
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyBase64 string            `json:"body_base64,omitempty"`
	Text       string            `json:"text,omitempty"`
	JSON       json.RawMessage   `json:"json,omitempty"`
}

func (c *Client) proxyViaAgent(
	ctx context.Context,
	agentURL string,
	method string,
	u *url.URL,
	headers http.Header,
	bearerToken string,
	body any,
) (*ResponseEnvelope, error) {
	base, err := url.Parse(agentURL)
	if err != nil {
		return nil, fmt.Errorf("parse agent URL %q: %w", agentURL, err)
	}
	rel, err := url.Parse("/api/v1/connector-agent/http")
	if err != nil {
		return nil, err
	}
	proxyURL := base.ResolveReference(rel)

	var jsonBody json.RawMessage
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal proxy body: %w", err)
		}
		jsonBody = encoded
	}
	envelope := agentProxyRequest{
		Method:      method,
		URL:         u.String(),
		Headers:     headerMapToStrings(headers),
		BearerToken: bearerToken,
		JSONBody:    jsonBody,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal agent envelope: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyURL.String(), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("connector agent returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var decoded agentProxyResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode agent response: %w", err)
	}
	bytesOut, err := decodeAgentBody(decoded)
	if err != nil {
		return nil, err
	}
	return &ResponseEnvelope{
		Status:  decoded.Status,
		Headers: decoded.Headers,
		Bytes:   bytesOut,
	}, nil
}

func decodeAgentBody(resp agentProxyResponse) ([]byte, error) {
	if resp.BodyBase64 != "" {
		return base64.StdEncoding.DecodeString(resp.BodyBase64)
	}
	if resp.Text != "" {
		return []byte(resp.Text), nil
	}
	if len(resp.JSON) > 0 {
		return []byte(resp.JSON), nil
	}
	return nil, nil
}

// JSONBody mirrors Rust's `http_runtime::json_body`. Returns the response
// body as a [json.RawMessage] when the bytes parse as valid JSON.
func JSONBody(env *ResponseEnvelope) (json.RawMessage, error) {
	if env == nil {
		return nil, fmt.Errorf("response envelope is nil")
	}
	if !json.Valid(env.Bytes) {
		return nil, fmt.Errorf("response body is not valid JSON")
	}
	return json.RawMessage(env.Bytes), nil
}

// BuildHeaders mirrors Rust's `http_runtime::header_map`. Reads the
// optional `headers` map from `config` and renders it as an [http.Header].
// Non-string values produce an error matching Rust's "header '<name>' must
// be a string" envelope.
func BuildHeaders(config map[string]any) (http.Header, error) {
	headers := http.Header{}
	raw, ok := config["headers"]
	if !ok {
		return headers, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return headers, nil
	}
	for name, value := range obj {
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("header %q must be a string", name)
		}
		headers.Add(name, s)
	}
	return headers, nil
}

func (c *Client) policyForConfig(config map[string]any) *EgressPolicy {
	allowed := c.AllowedEgressHosts
	allowPrivate := false
	if c != nil {
		allowPrivate = c.AllowPrivateNetworkEgress
	}
	return EgressPolicyFromState(allowed, allowPrivate, config)
}

func headerMapToStrings(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		values := h.Values(k)
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(k)] = values[0]
	}
	return out
}
