// Package catalog_bridge is the Go port of the Rust
// `services/connector-management-service/src/connectors/catalog_bridge.rs`
// helper that backs the "thin" tabular adapters (Tableau, Power BI, ODBC,
// JDBC, …). It is a utility package, not a user-facing connector — each
// per-connector adapter constructs a [Bridge] with its connector_name,
// default_source_kind, and identity_fields and forwards capability
// methods here.
//
// Capability surface ported from the Rust helper:
//
//   - [Bridge.ValidateConfig]      — `validate_tabular_connector_config`
//   - [Bridge.TestConnection]      — `test_tabular_connector_connection`
//   - [Bridge.DiscoverSources]     — `discover_tabular_sources`
//   - [Bridge.QueryVirtualTable]   — `query_tabular_virtual_table`
//   - [Bridge.FetchDataset]        — `fetch_tabular_dataset`
//
// Capabilities NOT exposed by the Rust helper (and therefore expected to
// be returned as `ErrNotImplemented` by callers): Arrow IPC streaming and
// IngestSpec construction. The Rust connectors that delegate to this
// helper only expose `validate_config`, `test_connection`, `fetch_dataset`,
// `discover_sources`, and `query_virtual_table`.
//
// HTTP egress goes through [internal/domain/http_runtime] which mirrors
// Rust's `http_runtime` module (connector-agent proxy + egress validation).
package catalog_bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	httpruntime "github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/domain/http_runtime"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectionTestResult mirrors Rust's `ConnectionTestResult`
// (connectors/mod.rs).
type ConnectionTestResult struct {
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	LatencyMS int64           `json:"latency_ms"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// SyncPayload mirrors Rust's `SyncPayload` (connectors/mod.rs) — the
// bytes the sync runtime hands to dataset-versioning-service for a new
// dataset version.
type SyncPayload struct {
	Bytes      []byte          `json:"-"`
	Format     string          `json:"format"`
	RowsSynced int64           `json:"rows_synced"`
	FileName   string          `json:"file_name"`
	Metadata   json.RawMessage `json:"metadata"`
}

// Bridge is the per-connector helper. Each adapter constructs one with
// [New] and forwards its capability methods to the bridge methods below.
type Bridge struct {
	// ConnectorName matches the Rust `CONNECTOR_NAME` constant — used in
	// validation errors and metadata envelopes ("tableau", "power_bi",
	// "odbc", "jdbc", …).
	ConnectorName string
	// DefaultSourceKind is emitted on discovered sources when the inline
	// catalog entry / remote payload does not override it. Mirrors the
	// Rust `DEFAULT_SOURCE_KIND` constant.
	DefaultSourceKind string
	// IdentityFields are the per-connector configuration keys the Rust
	// helper requires when the config uses a `*_path_template` (e.g.
	// `site_id` for Tableau). [Bridge.ValidateConfig] only enforces them
	// on the remote-template branch; inline catalogs and `catalog_path`
	// configs pass without identity fields.
	IdentityFields []string
	// HTTP is the shared transport used for remote catalog / resource
	// fetches. nil falls back to [httpruntime.New] with no allowlist and
	// loopback egress permitted (only suitable for tests).
	HTTP *httpruntime.Client
}

// New returns a [Bridge] wired to the supplied [httpruntime.Client]. A
// nil client is permissible only in tests — production callers should
// pass the AppState-backed client.
func New(connectorName, defaultSourceKind string, identityFields []string, client *httpruntime.Client) *Bridge {
	return &Bridge{
		ConnectorName:     connectorName,
		DefaultSourceKind: defaultSourceKind,
		IdentityFields:    append([]string(nil), identityFields...),
		HTTP:              client,
	}
}

func (b *Bridge) httpClient() *httpruntime.Client {
	if b == nil || b.HTTP == nil {
		return httpruntime.New(nil, nil, true)
	}
	return b.HTTP
}

// ValidateConfig mirrors Rust's
// `validate_tabular_connector_config(config, connector_name, identity_fields)`.
// Accepts either an inline catalog (`tables`/`views`/`datasets`/`streams`/
// `reports`/`entities`), `base_url`+`catalog_path`, or `base_url` plus a
// resource template — in which case [Bridge.IdentityFields] become required.
func (b *Bridge) ValidateConfig(config map[string]any) error {
	entries, err := inlineCatalogEntries(config, b.ConnectorName, b.DefaultSourceKind)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}

	hasBaseURL := stringField(config, "base_url") != ""
	hasCatalogPath := stringField(config, "catalog_path") != ""
	hasResourceTemplate := firstString(config,
		"resource_path_template",
		"stream_path_template",
		"view_path_template",
		"dataset_path_template",
		"report_path_template",
		"query_path_template",
	) != ""

	if hasBaseURL && hasCatalogPath {
		return nil
	}
	if hasBaseURL && hasResourceTemplate {
		missing := make([]string, 0, len(b.IdentityFields))
		for _, field := range b.IdentityFields {
			if _, ok := config[field]; !ok {
				missing = append(missing, field)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		return fmt.Errorf("%s connector requires %s when using resource templates",
			b.ConnectorName, quotedJoin(missing))
	}

	return fmt.Errorf("%s connector requires an inline catalog in 'tables', 'views', "+
		"'datasets', 'streams' or 'reports', or 'base_url' plus "+
		"'catalog_path'/'resource_path_template'", b.ConnectorName)
}

// TestConnection mirrors Rust's
// `test_tabular_connector_connection(state, &config, agent_url, …)`.
// When the config exposes a health URL the bridge issues a GET; otherwise
// it returns a `validated <connector> catalog` message keyed off the
// inline entries.
func (b *Bridge) TestConnection(
	ctx context.Context,
	config map[string]any,
	agentURL string,
) (ConnectionTestResult, error) {
	entries, err := inlineCatalogEntries(config, b.ConnectorName, b.DefaultSourceKind)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	var firstEntry *catalogEntry
	if len(entries) > 0 {
		firstEntry = &entries[0]
	}
	u, err := healthURL(config, firstEntry)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	if u != nil {
		started := time.Now()
		headers, err := httpruntime.BuildHeaders(config)
		if err != nil {
			return ConnectionTestResult{}, err
		}
		resp, err := b.httpClient().Get(ctx, config, u, headers, bearerToken(config), agentURL)
		if err != nil {
			return ConnectionTestResult{}, err
		}
		if resp.Status < 200 || resp.Status >= 300 {
			return ConnectionTestResult{}, fmt.Errorf(
				"%s bridge returned HTTP %d", b.ConnectorName, resp.Status,
			)
		}
		details, _ := json.Marshal(map[string]any{
			"url":             u.String(),
			"catalog_sources": len(entries),
			"agent_url":       optionalString(agentURL),
		})
		return ConnectionTestResult{
			Success:   true,
			Message:   fmt.Sprintf("%s bridge responded with HTTP %d", b.ConnectorName, resp.Status),
			LatencyMS: time.Since(started).Milliseconds(),
			Details:   details,
		}, nil
	}

	details, _ := json.Marshal(map[string]any{
		"catalog_sources": len(entries),
		"mode":            "inline_catalog",
	})
	return ConnectionTestResult{
		Success:   true,
		Message:   fmt.Sprintf("validated %s catalog with %d source(s)", b.ConnectorName, len(entries)),
		LatencyMS: 0,
		Details:   details,
	}, nil
}

// DiscoverSources mirrors Rust's
// `discover_tabular_sources(state, &config, agent_url, …)`. Inline catalogs
// short-circuit the HTTP path; otherwise the configured catalog URL is
// fetched and the response is normalised through [normalizeRecords].
func (b *Bridge) DiscoverSources(
	ctx context.Context,
	config map[string]any,
	agentURL string,
) ([]models.DiscoveredSource, error) {
	entries, err := inlineCatalogEntries(config, b.ConnectorName, b.DefaultSourceKind)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return entriesToSources(entries), nil
	}

	u, err := catalogURL(config)
	if err != nil {
		return nil, err
	}
	headers, err := httpruntime.BuildHeaders(config)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient().Get(ctx, config, u, headers, bearerToken(config), agentURL)
	if err != nil {
		return nil, err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, fmt.Errorf("%s catalog returned HTTP %d", b.ConnectorName, resp.Status)
	}

	body, err := httpruntime.JSONBody(resp)
	if err != nil {
		return nil, err
	}
	parsed, err := catalogEntriesFromJSON(body, b.ConnectorName, b.DefaultSourceKind)
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("%s catalog did not expose any sources", b.ConnectorName)
	}
	return entriesToSources(parsed), nil
}

// QueryVirtualTable mirrors Rust's
// `query_tabular_virtual_table(state, &config, request, …)`. Inline
// `sample_rows`/`preview_rows` short-circuit the HTTP path; the row count
// is clamped to `[1, 500]` to match the Rust helper.
func (b *Bridge) QueryVirtualTable(
	ctx context.Context,
	config map[string]any,
	request models.VirtualTableQueryRequest,
	agentURL string,
) (models.VirtualTableQueryResponse, error) {
	rows, metadata, err := b.resolveRows(ctx, config, request.Selector, request.Limit, agentURL)
	if err != nil {
		return models.VirtualTableQueryResponse{}, err
	}
	return virtualTableResponse(request.Selector, rows, metadata), nil
}

// FetchDataset mirrors Rust's
// `fetch_tabular_dataset(state, &config, selector, …)`. The rows are
// JSON-encoded into [SyncPayload.Bytes] and a sha256 source signature is
// spliced into the payload's metadata via [addSourceSignature].
func (b *Bridge) FetchDataset(
	ctx context.Context,
	config map[string]any,
	selector string,
	agentURL string,
) (SyncPayload, error) {
	rows, metadata, err := b.resolveRows(ctx, config, selector, nil, agentURL)
	if err != nil {
		return SyncPayload{}, err
	}
	payloadBytes, err := json.Marshal(rows)
	if err != nil {
		return SyncPayload{}, err
	}
	payload := SyncPayload{
		Bytes:      payloadBytes,
		Format:     "json",
		RowsSynced: int64(len(rows)),
		FileName:   sanitizeFileStem(selector, b.ConnectorName) + ".json",
		Metadata:   metadata,
	}
	addSourceSignature(&payload)
	return payload, nil
}

func (b *Bridge) resolveRows(
	ctx context.Context,
	config map[string]any,
	selector string,
	limit *int,
	agentURL string,
) ([]json.RawMessage, json.RawMessage, error) {
	resolved := 50
	if limit != nil {
		resolved = *limit
	}
	if resolved < 1 {
		resolved = 1
	}
	if resolved > 500 {
		resolved = 500
	}

	entries, err := inlineCatalogEntries(config, b.ConnectorName, b.DefaultSourceKind)
	if err != nil {
		return nil, nil, err
	}
	var selected *catalogEntry
	for i := range entries {
		if entries[i].Selector == selector {
			selected = &entries[i]
			break
		}
	}

	if selected != nil && len(selected.SampleRows) > 0 {
		rows := selected.SampleRows
		if len(rows) > resolved {
			rows = rows[:resolved]
		}
		meta, err := json.Marshal(map[string]any{
			"connector": b.ConnectorName,
			"selector":  selector,
			"mode":      "inline_catalog",
			"entry":     selected.Metadata,
		})
		if err != nil {
			return nil, nil, err
		}
		return rows, meta, nil
	}

	u, err := sourceURL(config, selector, selected)
	if err != nil {
		return nil, nil, err
	}
	headers, err := httpruntime.BuildHeaders(config)
	if err != nil {
		return nil, nil, err
	}
	resp, err := b.httpClient().Get(ctx, config, u, headers, bearerToken(config), agentURL)
	if err != nil {
		return nil, nil, err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, nil, fmt.Errorf(
			"%s source '%s' returned HTTP %d", b.ConnectorName, selector, resp.Status,
		)
	}

	body, err := httpruntime.JSONBody(resp)
	if err != nil {
		return nil, nil, err
	}
	rows, err := normalizeRecordsFromJSON(body)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) > resolved {
		rows = rows[:resolved]
	}

	metaMap := map[string]any{
		"connector": b.ConnectorName,
		"selector":  selector,
		"mode":      "bridge_fetch",
		"url":       u.String(),
		"agent_url": optionalString(agentURL),
	}
	if selected != nil {
		metaMap["entry"] = selected.Metadata
	} else {
		metaMap["entry"] = nil
	}
	meta, err := json.Marshal(metaMap)
	if err != nil {
		return nil, nil, err
	}
	return rows, meta, nil
}

type catalogEntry struct {
	Selector         string
	DisplayName      string
	SourceKind       string
	Path             string
	SampleRows       []json.RawMessage
	SupportsSync     bool
	SupportsZeroCopy bool
	Metadata         map[string]any
}

func inlineCatalogEntries(config map[string]any, connectorName, defaultSourceKind string) ([]catalogEntry, error) {
	raw, ok := catalogEntriesValue(config)
	if !ok {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s connector expects its inline catalog to be an array", connectorName)
	}
	parsed := make([]catalogEntry, 0, len(list))
	for index, item := range list {
		entry, ok := parseCatalogEntry(item, defaultSourceKind)
		if !ok {
			return nil, fmt.Errorf("%s connector tables[%d] requires 'selector', 'name' or 'table'",
				connectorName, index)
		}
		parsed = append(parsed, entry)
	}
	return parsed, nil
}

func catalogEntriesFromJSON(payload json.RawMessage, connectorName, defaultSourceKind string) ([]catalogEntry, error) {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("%s: decode catalog response: %w", connectorName, err)
	}
	rows, err := normalizeRecords(decoded)
	if err != nil {
		return nil, err
	}
	parsed := make([]catalogEntry, 0, len(rows))
	for index, row := range rows {
		var obj map[string]any
		if err := json.Unmarshal(row, &obj); err != nil {
			return nil, fmt.Errorf("%s catalog row %d is not a JSON object: %w",
				connectorName, index, err)
		}
		entry, ok := parseCatalogEntry(obj, defaultSourceKind)
		if !ok {
			return nil, fmt.Errorf("%s catalog row %d requires 'selector', 'name' or 'table'",
				connectorName, index)
		}
		parsed = append(parsed, entry)
	}
	return parsed, nil
}

func parseCatalogEntry(raw any, defaultSourceKind string) (catalogEntry, bool) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return catalogEntry{}, false
	}
	selector := strings.TrimSpace(firstStringFromObj(obj,
		"selector", "name", "table", "view", "dataset", "stream", "report", "asset",
	))
	if selector == "" {
		return catalogEntry{}, false
	}
	displayName := firstStringFromObj(obj, "display_name", "title", "name")
	if displayName == "" {
		displayName = selector
	}
	sourceKind := firstStringFromObj(obj, "source_kind", "kind")
	if sourceKind == "" {
		sourceKind = defaultSourceKind
	}
	path := firstStringFromObj(obj,
		"path", "resource_path", "stream_path", "view_path",
		"dataset_path", "report_path", "query_path",
	)
	sampleRows := rawSliceFromObj(obj, "sample_rows", "preview_rows")
	supportsSync := boolFromObj(obj, "supports_sync", true)
	supportsZeroCopy := boolFromObj(obj, "supports_zero_copy", true)

	metadata := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "sample_rows" || k == "preview_rows" {
			continue
		}
		metadata[k] = v
	}

	return catalogEntry{
		Selector:         selector,
		DisplayName:      displayName,
		SourceKind:       sourceKind,
		Path:             path,
		SampleRows:       sampleRows,
		SupportsSync:     supportsSync,
		SupportsZeroCopy: supportsZeroCopy,
		Metadata:         metadata,
	}, true
}

func entriesToSources(entries []catalogEntry) []models.DiscoveredSource {
	out := make([]models.DiscoveredSource, 0, len(entries))
	for _, entry := range entries {
		meta, _ := json.Marshal(entry.Metadata)
		out = append(out, models.DiscoveredSource{
			Selector:         entry.Selector,
			DisplayName:      entry.DisplayName,
			SourceKind:       entry.SourceKind,
			SupportsSync:     entry.SupportsSync,
			SupportsZeroCopy: entry.SupportsZeroCopy,
			Metadata:         meta,
		})
	}
	return out
}

func catalogEntriesValue(config map[string]any) (any, bool) {
	for _, key := range []string{"tables", "views", "datasets", "streams", "reports", "entities"} {
		if v, ok := config[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func healthURL(config map[string]any, firstEntry *catalogEntry) (*url.URL, error) {
	base := stringField(config, "base_url")
	if base == "" {
		return nil, nil
	}
	template := firstString(config, "health_path", "catalog_path")
	if template == "" && firstEntry != nil {
		template = firstEntry.Path
	}
	if template == "" {
		return nil, nil
	}
	selector := ""
	if firstEntry != nil {
		selector = firstEntry.Selector
	}
	return joinURL(base, interpolateTemplate(template, config, selector))
}

func catalogURL(config map[string]any) (*url.URL, error) {
	base := stringField(config, "base_url")
	if base == "" {
		return nil, errors.New("connector bridge requires 'base_url'")
	}
	path := stringField(config, "catalog_path")
	if path == "" {
		return nil, errors.New("connector bridge requires 'catalog_path' when tables are not inlined")
	}
	return joinURL(base, interpolateTemplate(path, config, ""))
}

func sourceURL(config map[string]any, selector string, entry *catalogEntry) (*url.URL, error) {
	base := stringField(config, "base_url")
	if base == "" {
		return nil, errors.New("connector bridge requires 'base_url' for remote table access")
	}
	template := ""
	if entry != nil && entry.Path != "" {
		template = entry.Path
	}
	if template == "" {
		template = firstString(config,
			"resource_path_template",
			"stream_path_template",
			"view_path_template",
			"dataset_path_template",
			"report_path_template",
			"query_path_template",
		)
	}
	if template == "" {
		template = selector
	}
	return joinURL(base, interpolateTemplate(template, config, selector))
}

func interpolateTemplate(template string, config map[string]any, selector string) string {
	rendered := strings.ReplaceAll(template, "{selector}", selector)
	for _, field := range []string{
		"project_id", "dataset_id", "account", "database", "schema",
		"warehouse", "region", "site_id", "project_name", "workspace_id",
		"tenant_id", "report_id", "workbook_id", "dsn", "driver",
		"connection_string", "jdbc_url", "driver_class", "stream_name",
		"consumer_name", "catalog", "workgroup",
	} {
		if value := stringField(config, field); value != "" {
			rendered = strings.ReplaceAll(rendered, "{"+field+"}", value)
		}
	}
	return rendered
}

func joinURL(base, path string) (*url.URL, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse base_url %q: %w", base, err)
	}
	rel, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse path %q: %w", path, err)
	}
	return baseURL.ResolveReference(rel), nil
}

func bearerToken(config map[string]any) string {
	return stringField(config, "bearer_token")
}

func stringField(config map[string]any, key string) string {
	value, ok := config[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func firstString(config map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := stringField(config, key); v != "" {
			return v
		}
	}
	return ""
}

func firstStringFromObj(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func rawSliceFromObj(obj map[string]any, keys ...string) []json.RawMessage {
	for _, key := range keys {
		v, ok := obj[key]
		if !ok {
			continue
		}
		list, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]json.RawMessage, 0, len(list))
		for _, item := range list {
			buf, err := json.Marshal(item)
			if err != nil {
				continue
			}
			out = append(out, buf)
		}
		return out
	}
	return nil
}

func boolFromObj(obj map[string]any, key string, defaultValue bool) bool {
	if v, ok := obj[key].(bool); ok {
		return v
	}
	return defaultValue
}

func quotedJoin(fields []string) string {
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, "'"+f+"'")
	}
	return strings.Join(quoted, ", ")
}

// normalizeRecords mirrors Rust's `normalize_records` — peels off the
// common envelope keys (`data`, `items`, `records`, `value`) and returns
// the row list as `[]json.RawMessage`.
func normalizeRecords(payload any) ([]json.RawMessage, error) {
	switch v := payload.(type) {
	case []any:
		return marshalSlice(v), nil
	case map[string]any:
		for _, key := range []string{"data", "items", "records", "value"} {
			if list, ok := v[key].([]any); ok {
				delete(v, key)
				return marshalSlice(list), nil
			}
		}
		buf, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return []json.RawMessage{buf}, nil
	default:
		buf, err := json.Marshal(map[string]any{"value": v})
		if err != nil {
			return nil, err
		}
		return []json.RawMessage{buf}, nil
	}
}

func normalizeRecordsFromJSON(payload json.RawMessage) ([]json.RawMessage, error) {
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return normalizeRecords(decoded)
}

func marshalSlice(list []any) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(list))
	for _, item := range list {
		buf, err := json.Marshal(item)
		if err != nil {
			continue
		}
		out = append(out, buf)
	}
	return out
}

// virtualTableResponse mirrors Rust's `virtual_table_response`
// (connectors/mod.rs). Mode is hardcoded to "zero_copy" — per-call mode
// (inline_catalog vs bridge_fetch) is carried inside `metadata`.
func virtualTableResponse(selector string, rows []json.RawMessage, metadata json.RawMessage) models.VirtualTableQueryResponse {
	if rows == nil {
		rows = []json.RawMessage{}
	}
	columns := firstObjectKeys(rows)
	signature := signatureFromRows(rows)
	if metadata == nil {
		metadata = json.RawMessage("null")
	}
	return models.VirtualTableQueryResponse{
		Selector:        selector,
		Mode:            "zero_copy",
		Columns:         columns,
		RowCount:        len(rows),
		Rows:            rows,
		SourceSignature: signature,
		Metadata:        metadata,
	}
}

// addSourceSignature mirrors Rust's `add_source_signature` — splices a
// `source_signature` field into the payload's metadata object. When
// metadata isn't a JSON object it is left alone (Rust's `as_object_mut`
// returns None in that case).
func addSourceSignature(p *SyncPayload) {
	signature := sourceSignature(p.Bytes)
	var obj map[string]json.RawMessage
	if len(p.Metadata) == 0 {
		obj = map[string]json.RawMessage{}
	} else if err := json.Unmarshal(p.Metadata, &obj); err != nil {
		return
	}
	sigBytes, err := json.Marshal(signature)
	if err != nil {
		return
	}
	obj["source_signature"] = sigBytes
	merged, err := json.Marshal(obj)
	if err != nil {
		return
	}
	p.Metadata = merged
}

func sourceSignature(b []byte) string {
	digest := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func signatureFromRows(rows []json.RawMessage) *string {
	encoded, err := json.Marshal(rows)
	if err != nil {
		return nil
	}
	sig := sourceSignature(encoded)
	return &sig
}

// firstObjectKeys returns the keys of the first row that decodes as a
// JSON object, preserving insertion order.
func firstObjectKeys(rows []json.RawMessage) []string {
	for _, row := range rows {
		keys, ok := orderedObjectKeys(row)
		if ok {
			return keys
		}
	}
	return []string{}
}

func orderedObjectKeys(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, false
	}
	keys := []string{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := t.(string)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, false
		}
	}
	return keys, true
}

// sanitizeFileStem mirrors Rust's `sanitize_file_stem` — keeps ASCII
// alphanumerics, converts everything else to `_`, trims leading/trailing
// underscores, and clamps to 64 chars. Falls back to `fallback` when the
// sanitised stem is empty.
func sanitizeFileStem(selector, fallback string) string {
	var b strings.Builder
	for _, ch := range selector {
		if ch < 128 && (unicode.IsLetter(ch) || unicode.IsDigit(ch)) {
			b.WriteRune(ch)
		} else {
			b.WriteByte('_')
		}
	}
	stem := strings.Trim(b.String(), "_")
	if stem == "" {
		return fallback
	}
	if len(stem) > 64 {
		stem = stem[:64]
	}
	return stem
}

func optionalString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
