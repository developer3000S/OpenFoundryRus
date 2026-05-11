package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultWebhookTimeoutMS        = 10000
	defaultWebhookHistoryRetention = 30
	DefaultWebhookMaxCalls         = 10
	DefaultWebhookMaxInputBytes    = 256 * 1024
	DefaultWebhookMaxResponseBytes = 2 * 1024 * 1024
)

type WebhookParameter struct {
	ID          string          `json:"id,omitempty"`
	Name        string          `json:"name,omitempty"`
	Type        string          `json:"type,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Description string          `json:"description,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
}

type WebhookCall struct {
	ID           string                     `json:"id,omitempty"`
	Method       string                     `json:"method,omitempty"`
	URL          string                     `json:"url,omitempty"`
	Path         string                     `json:"path,omitempty"`
	QueryParams  map[string]string          `json:"query_params,omitempty"`
	Headers      map[string]string          `json:"headers,omitempty"`
	Body         json.RawMessage            `json:"body,omitempty"`
	BodyTemplate string                     `json:"body_template,omitempty"`
	ContentType  string                     `json:"content_type,omitempty"`
	Extractors   []WebhookResponseExtractor `json:"extractors,omitempty"`
}

type WebhookResponseExtractor struct {
	Name     string `json:"name"`
	FromCall string `json:"from_call,omitempty"`
	Path     string `json:"path"`
	Type     string `json:"type,omitempty"`
}

type WebhookOutputParameter struct {
	ID          string                `json:"id,omitempty"`
	Name        string                `json:"name,omitempty"`
	Type        string                `json:"type,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Description string                `json:"description,omitempty"`
	Extractor   WebhookValueReference `json:"extractor,omitempty"`
}

type WebhookValueReference struct {
	FromCall string `json:"from_call,omitempty"`
	Path     string `json:"path,omitempty"`
}

type WebhookRateLimit struct {
	MaxRequests int `json:"max_requests,omitempty"`
	PerSeconds  int `json:"per_seconds,omitempty"`
}

type WebhookInvocationLimits struct {
	MaxCalls         int `json:"max_calls,omitempty"`
	MaxInputBytes    int `json:"max_input_bytes,omitempty"`
	MaxResponseBytes int `json:"max_response_bytes,omitempty"`
}

type WebhookHistorySettings struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retention_days,omitempty"`
	StoreInputs   bool `json:"store_inputs,omitempty"`
	StoreOutputs  bool `json:"store_outputs,omitempty"`
}

type BuiltWebhookRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"-"`
}

type WebhookCallResult struct {
	CallID   string          `json:"call_id"`
	Status   uint16          `json:"status"`
	Response json.RawMessage `json:"response"`
}

func NormalizeWebhookDefinition(raw json.RawMessage) (*WebhookDefinition, error) {
	if len(strings.TrimSpace(string(raw))) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, errors.New("webhook definition requires config")
	}
	var def WebhookDefinition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, fmt.Errorf("webhook definition: invalid config: %w", err)
	}
	return normalizeWebhookDefinition(&def)
}

func WebhookDefinitionFromRESTAPISource(source *RESTAPISourceConfig) (*WebhookDefinition, error) {
	if source == nil || source.Webhook == nil {
		return nil, errors.New("rest_api source has no webhook definition")
	}
	w := source.Webhook
	def := &WebhookDefinition{
		Name:             "REST API webhook",
		Method:           w.Method,
		Path:             w.Path,
		QueryParams:      cloneStringMap(w.QueryParams),
		Headers:          cloneStringMap(w.Headers),
		Body:             cloneRawMessage(w.Body),
		BodyTemplate:     w.BodyTemplate,
		Inputs:           append([]WebhookParameter(nil), w.Inputs...),
		Calls:            append([]WebhookCall(nil), w.Calls...),
		Outputs:          append([]WebhookOutputParameter(nil), w.Outputs...),
		TimeoutMS:        w.TimeoutMS,
		ConcurrencyLimit: w.ConcurrencyLimit,
		RateLimit:        w.RateLimit,
		Limits:           w.Limits,
		History:          w.History,
	}
	return normalizeWebhookDefinition(def)
}

func normalizeWebhookDefinition(def *WebhookDefinition) (*WebhookDefinition, error) {
	def.Method = strings.ToUpper(strings.TrimSpace(def.Method))
	def.URL = strings.TrimSpace(def.URL)
	def.Path = strings.TrimSpace(def.Path)
	if def.TimeoutMS == 0 {
		def.TimeoutMS = defaultWebhookTimeoutMS
	}
	if def.TimeoutMS < 100 || def.TimeoutMS > 180000 {
		return nil, errors.New("webhook timeout_ms must be between 100 and 180000")
	}
	if def.ConcurrencyLimit < 0 {
		return nil, errors.New("webhook concurrency_limit must be >= 0")
	}
	if def.RateLimit != nil && (def.RateLimit.MaxRequests < 0 || def.RateLimit.PerSeconds < 0) {
		return nil, errors.New("webhook rate_limit values must be >= 0")
	}
	def.Limits = normalizeWebhookInvocationLimits(def.Limits)
	if !def.History.Enabled && def.History.RetentionDays == 0 && !def.History.StoreInputs && !def.History.StoreOutputs {
		def.History = WebhookHistorySettings{Enabled: true, RetentionDays: defaultWebhookHistoryRetention, StoreOutputs: true}
	}
	if def.History.Enabled && def.History.RetentionDays == 0 {
		def.History.RetentionDays = defaultWebhookHistoryRetention
	}
	for i := range def.Inputs {
		normalizeWebhookParameter(&def.Inputs[i])
		if def.Inputs[i].ID == "" {
			return nil, fmt.Errorf("webhook input at index %d requires id or name", i)
		}
	}
	if len(def.Calls) == 0 {
		def.Calls = []WebhookCall{{
			ID:           "call_1",
			Method:       def.Method,
			URL:          def.URL,
			Path:         def.Path,
			QueryParams:  cloneStringMap(def.QueryParams),
			Headers:      cloneStringMap(def.Headers),
			Body:         cloneRawMessage(def.Body),
			BodyTemplate: def.BodyTemplate,
		}}
	}
	for i := range def.Calls {
		call := &def.Calls[i]
		call.ID = strings.TrimSpace(call.ID)
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", i+1)
		}
		call.Method = strings.ToUpper(strings.TrimSpace(call.Method))
		if call.Method == "" {
			call.Method = def.Method
		}
		if call.Method == "" {
			call.Method = "POST"
		}
		if !allowedRESTAPIMethod(call.Method) {
			return nil, fmt.Errorf("webhook call %q method %q is not supported", call.ID, call.Method)
		}
		call.URL = strings.TrimSpace(call.URL)
		call.Path = strings.TrimSpace(call.Path)
		if call.URL == "" && call.Path == "" && def.URL == "" && def.Path == "" {
			return nil, fmt.Errorf("webhook call %q requires url or relative path", call.ID)
		}
		for j := range call.Extractors {
			call.Extractors[j].Name = strings.TrimSpace(call.Extractors[j].Name)
			call.Extractors[j].FromCall = strings.TrimSpace(call.Extractors[j].FromCall)
			call.Extractors[j].Path = strings.TrimSpace(call.Extractors[j].Path)
			call.Extractors[j].Type = strings.ToLower(strings.TrimSpace(call.Extractors[j].Type))
			if call.Extractors[j].Name == "" || call.Extractors[j].Path == "" {
				return nil, fmt.Errorf("webhook call %q extractor at index %d requires name and path", call.ID, j)
			}
		}
	}
	for i := range def.Outputs {
		normalizeWebhookOutput(&def.Outputs[i])
		if def.Outputs[i].ID == "" {
			return nil, fmt.Errorf("webhook output at index %d requires id or name", i)
		}
	}
	return def, nil
}

func normalizeWebhookInvocationLimits(limits WebhookInvocationLimits) WebhookInvocationLimits {
	if limits.MaxCalls == 0 {
		limits.MaxCalls = DefaultWebhookMaxCalls
	}
	if limits.MaxInputBytes == 0 {
		limits.MaxInputBytes = DefaultWebhookMaxInputBytes
	}
	if limits.MaxResponseBytes == 0 {
		limits.MaxResponseBytes = DefaultWebhookMaxResponseBytes
	}
	return limits
}

func ValidateWebhookInvocation(def *WebhookDefinition, inputs json.RawMessage) error {
	if def == nil {
		return errors.New("webhook definition is nil")
	}
	limits := normalizeWebhookInvocationLimits(def.Limits)
	if len(def.Calls) > limits.MaxCalls {
		return fmt.Errorf("webhook has %d calls but max_calls is %d", len(def.Calls), limits.MaxCalls)
	}
	if len(inputs) > limits.MaxInputBytes {
		return fmt.Errorf("webhook inputs exceed max_input_bytes %d", limits.MaxInputBytes)
	}
	inputValues := map[string]any{}
	if len(strings.TrimSpace(string(inputs))) > 0 {
		if err := json.Unmarshal(inputs, &inputValues); err != nil {
			return fmt.Errorf("webhook inputs must be a JSON object: %w", err)
		}
	}
	for _, input := range def.Inputs {
		if !input.Required {
			continue
		}
		value, ok := inputValues[input.ID]
		if !ok || value == nil {
			return fmt.Errorf("required webhook input %q is missing", input.ID)
		}
		if err := validateWebhookInputType(input, value); err != nil {
			return err
		}
	}
	return nil
}

func validateWebhookInputType(input WebhookParameter, value any) error {
	switch input.Type {
	case "", "any", "json", "object":
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("webhook input %q must be a string", input.ID)
		}
	case "number", "double", "float", "integer", "long", "int":
		if _, err := webhookNumber(value); err != nil {
			return fmt.Errorf("webhook input %q must be numeric", input.ID)
		}
	case "boolean", "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("webhook input %q must be boolean", input.ID)
		}
	}
	return nil
}

func normalizeWebhookParameter(p *WebhookParameter) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	if p.ID == "" {
		p.ID = p.Name
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	if p.Type == "" {
		p.Type = "string"
	}
}

func normalizeWebhookOutput(p *WebhookOutputParameter) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	if p.ID == "" {
		p.ID = p.Name
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	if p.Type == "" {
		p.Type = "any"
	}
	p.Extractor.FromCall = strings.TrimSpace(p.Extractor.FromCall)
	p.Extractor.Path = strings.TrimSpace(p.Extractor.Path)
}

func BuildWebhookRequest(def *WebhookDefinition, source *RESTAPISourceConfig, call WebhookCall, inputs json.RawMessage, state map[string]any) (*BuiltWebhookRequest, error) {
	if def == nil {
		return nil, errors.New("webhook definition is nil")
	}
	state = webhookTemplateState(inputs, state)
	method := call.Method
	if method == "" {
		method = def.Method
	}
	if method == "" && source != nil && source.Webhook != nil {
		method = source.Webhook.Method
	}
	if method == "" {
		method = "POST"
	}
	method = strings.ToUpper(method)
	u, err := resolveWebhookURL(def, source, call)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	mergeTemplatedQuery(q, sourceQueryParams(source), state)
	mergeTemplatedQuery(q, def.QueryParams, state)
	mergeTemplatedQuery(q, call.QueryParams, state)
	u.RawQuery = q.Encode()
	headers := map[string]string{}
	mergeTemplatedHeaders(headers, sourceHeaders(source), state)
	mergeTemplatedHeaders(headers, def.Headers, state)
	mergeTemplatedHeaders(headers, call.Headers, state)
	u, headers = applySourceAuthForWebhook(u, headers, source, state)
	body := webhookCallBody(def, call, state)
	contentType := strings.TrimSpace(call.ContentType)
	if contentType == "" && len(body) > 0 {
		contentType = "application/json"
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return &BuiltWebhookRequest{Method: method, URL: u.String(), Headers: headers, Body: body}, nil
}

func resolveWebhookURL(def *WebhookDefinition, source *RESTAPISourceConfig, call WebhookCall) (*url.URL, error) {
	if call.URL != "" {
		return parseWebhookURL(call.URL)
	}
	baseURL := def.URL
	path := call.Path
	if path == "" {
		path = def.Path
	}
	if baseURL == "" && source != nil {
		baseURL = source.BaseURL
	}
	if baseURL == "" {
		return nil, errors.New("webhook call requires absolute url or REST API source base_url")
	}
	base, err := parseWebhookURL(baseURL)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return base, nil
	}
	rel, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("webhook path %q is invalid: %w", path, err)
	}
	return base.ResolveReference(rel), nil
}

func parseWebhookURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, errors.New("invalid webhook url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("webhook url must use http or https")
	}
	return u, nil
}

func webhookCallBody(def *WebhookDefinition, call WebhookCall, state map[string]any) []byte {
	if strings.TrimSpace(call.BodyTemplate) != "" {
		return []byte(interpolateWebhookTemplate(call.BodyTemplate, state))
	}
	if len(call.Body) > 0 {
		return []byte(interpolateWebhookTemplate(string(call.Body), state))
	}
	if strings.TrimSpace(def.BodyTemplate) != "" {
		return []byte(interpolateWebhookTemplate(def.BodyTemplate, state))
	}
	if len(def.Body) > 0 {
		return []byte(interpolateWebhookTemplate(string(def.Body), state))
	}
	return nil
}

func sourceQueryParams(source *RESTAPISourceConfig) map[string]string {
	if source == nil {
		return nil
	}
	out := cloneStringMap(source.QueryParams)
	if source.Webhook != nil {
		for k, v := range source.Webhook.QueryParams {
			out[k] = v
		}
	}
	return out
}

func sourceHeaders(source *RESTAPISourceConfig) map[string]string {
	if source == nil {
		return nil
	}
	out := cloneStringMap(source.Headers)
	if source.Webhook != nil {
		for k, v := range source.Webhook.Headers {
			out[k] = v
		}
	}
	return out
}

func mergeTemplatedQuery(q url.Values, values map[string]string, state map[string]any) {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		q.Set(k, interpolateWebhookTemplate(v, state))
	}
}

func mergeTemplatedHeaders(headers map[string]string, values map[string]string, state map[string]any) {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headers[k] = interpolateWebhookTemplate(v, state)
	}
}

func applySourceAuthForWebhook(u *url.URL, headers map[string]string, source *RESTAPISourceConfig, state map[string]any) (*url.URL, map[string]string) {
	if source == nil {
		return u, headers
	}
	auth := source.Auth
	switch auth.Type {
	case RESTAPIAuthBearer:
		token := interpolateWebhookTemplate(auth.Token, state)
		if token == "" {
			token = source.BearerToken
		}
		if token != "" {
			scheme := auth.Scheme
			if scheme == "" {
				scheme = "Bearer"
			}
			headers["Authorization"] = scheme + " " + token
		}
	case RESTAPIAuthAPIKey:
		value := interpolateWebhookTemplate(auth.Value, state)
		if value == "" {
			return u, headers
		}
		if auth.HeaderName != "" {
			headers[auth.HeaderName] = value
		}
		if auth.QueryParam != "" {
			q := u.Query()
			q.Set(auth.QueryParam, value)
			u.RawQuery = q.Encode()
		}
	case RESTAPIAuthCustomHeader:
		if auth.HeaderName != "" && auth.Value != "" {
			headers[auth.HeaderName] = interpolateWebhookTemplate(auth.Value, state)
		}
	}
	return u, headers
}

func CaptureWebhookCallResult(def *WebhookDefinition, call WebhookCall, result WebhookCallResult, state map[string]any) error {
	if state == nil {
		return errors.New("webhook state is nil")
	}
	decoded, err := decodeWebhookJSON(result.Response)
	if err != nil {
		return err
	}
	callState := map[string]any{"status": int(result.Status), "response": decoded}
	state[call.ID] = callState
	for _, extractor := range call.Extractors {
		value, ok := lookupWebhookPath(decoded, extractor.Path)
		if !ok {
			return fmt.Errorf("webhook extractor %q path %q not found", extractor.Name, extractor.Path)
		}
		coerced, err := coerceWebhookValue(value, extractor.Type)
		if err != nil {
			return fmt.Errorf("webhook extractor %q: %w", extractor.Name, err)
		}
		state[extractor.Name] = coerced
		callState[extractor.Name] = coerced
	}
	return nil
}

func ExtractWebhookOutputs(def *WebhookDefinition, results []WebhookCallResult) (json.RawMessage, error) {
	if def == nil {
		return nil, errors.New("webhook definition is nil")
	}
	if len(results) == 0 {
		return json.RawMessage(`{}`), nil
	}
	byCall := map[string]any{}
	var final any
	for _, result := range results {
		decoded, err := decodeWebhookJSON(result.Response)
		if err != nil {
			return nil, err
		}
		byCall[result.CallID] = decoded
		final = decoded
	}
	if len(def.Outputs) == 0 {
		if obj, ok := final.(map[string]any); ok {
			if out, ok := obj["output_parameters"]; ok {
				return json.Marshal(out)
			}
		}
		return json.RawMessage(`{}`), nil
	}
	out := map[string]any{}
	for _, param := range def.Outputs {
		key := param.ID
		if key == "" {
			key = param.Name
		}
		path := param.Extractor.Path
		if path == "" {
			path = "/" + key
		}
		source := final
		if param.Extractor.FromCall != "" {
			if selected, ok := byCall[param.Extractor.FromCall]; ok {
				source = selected
			} else if param.Required {
				return nil, fmt.Errorf("webhook output %q references unknown call %q", key, param.Extractor.FromCall)
			}
		}
		value, ok := lookupWebhookPath(source, path)
		if !ok {
			if param.Required {
				return nil, fmt.Errorf("webhook output %q path %q not found", key, path)
			}
			out[key] = nil
			continue
		}
		coerced, err := coerceWebhookValue(value, param.Type)
		if err != nil {
			return nil, fmt.Errorf("webhook output %q: %w", key, err)
		}
		out[key] = coerced
	}
	return json.Marshal(out)
}

func decodeWebhookJSON(raw json.RawMessage) (any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("webhook response is not valid JSON: %w", err)
	}
	return decoded, nil
}

func lookupWebhookPath(value any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return value, true
	}
	if strings.HasPrefix(path, "$.") {
		path = strings.TrimPrefix(path, "$.")
	} else if strings.HasPrefix(path, ".") {
		path = strings.TrimPrefix(path, ".")
	}
	if strings.HasPrefix(path, "/") {
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		current := value
		for _, rawPart := range parts {
			part := strings.ReplaceAll(strings.ReplaceAll(rawPart, "~1", "/"), "~0", "~")
			next, ok := webhookChild(current, part)
			if !ok {
				return nil, false
			}
			current = next
		}
		return current, true
	}
	current := value
	for _, part := range strings.Split(path, ".") {
		next, ok := webhookChild(current, part)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func webhookChild(current any, part string) (any, bool) {
	switch typed := current.(type) {
	case map[string]any:
		v, ok := typed[part]
		return v, ok
	case []any:
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 0 || idx >= len(typed) {
			return nil, false
		}
		return typed[idx], true
	default:
		return nil, false
	}
}

func coerceWebhookValue(value any, typ string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "", "any", "json", "object":
		return value, nil
	case "string":
		switch v := value.(type) {
		case string:
			return v, nil
		default:
			buf, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			return string(buf), nil
		}
	case "number", "double", "float":
		return webhookNumber(value)
	case "integer", "long", "int":
		n, err := webhookNumber(value)
		if err != nil {
			return nil, err
		}
		return int64(n), nil
	case "boolean", "bool":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			return strconv.ParseBool(v)
		default:
			return nil, fmt.Errorf("cannot coerce %T to boolean", value)
		}
	default:
		return value, nil
	}
}

func webhookNumber(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot coerce %T to number", value)
	}
}

var webhookTemplatePattern = regexp.MustCompile(`\{\{\s*(json\s+)?([A-Za-z0-9_.-]+)\s*\}\}`)

func interpolateWebhookTemplate(template string, state map[string]any) string {
	return webhookTemplatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := webhookTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return ""
		}
		value, ok := lookupWebhookTemplateValue(state, parts[2])
		if !ok {
			return ""
		}
		if strings.TrimSpace(parts[1]) != "" {
			buf, err := json.Marshal(value)
			if err != nil {
				return "null"
			}
			return string(buf)
		}
		return fmt.Sprint(value)
	})
}

func webhookTemplateState(inputs json.RawMessage, state map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range state {
		out[k] = v
	}
	var decoded any
	if len(strings.TrimSpace(string(inputs))) > 0 {
		_ = json.Unmarshal(inputs, &decoded)
	}
	inputObj, ok := decoded.(map[string]any)
	if !ok {
		inputObj = map[string]any{}
	}
	out["inputs"] = inputObj
	for k, v := range inputObj {
		out[k] = v
	}
	return out
}

func lookupWebhookTemplateValue(state map[string]any, path string) (any, bool) {
	if value, ok := state[path]; ok {
		return value, true
	}
	return lookupWebhookPath(state, path)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return json.RawMessage(out)
}

var webhookURLPattern = regexp.MustCompile(`https?://[^\s"']+`)
var webhookSecretAssignmentPattern = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+|authorization=\s*bearer%20|api[_-]?key=|token=|secret=|password=)([^&\s"']+)`)

func SanitizeWebhookDiagnostic(msg string) string {
	sanitized := webhookURLPattern.ReplaceAllStringFunc(msg, func(candidate string) string {
		u, err := url.Parse(candidate)
		if err != nil || u.Host == "" {
			return candidate
		}
		q := u.Query()
		changed := false
		for key := range q {
			if webhookSensitiveKey(key) {
				q.Set(key, "REDACTED")
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
		return u.String()
	})
	return webhookSecretAssignmentPattern.ReplaceAllString(sanitized, "${1}REDACTED")
}

func webhookSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		key == "key"
}
