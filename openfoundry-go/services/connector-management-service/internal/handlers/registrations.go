package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/domain"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/workers"
)

func normalizeRegistrationMode(mode *string) (string, error) {
	if mode == nil || strings.TrimSpace(*mode) == "" {
		return "sync", nil
	}
	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "sync", "zero_copy":
		return strings.ToLower(strings.TrimSpace(*mode)), nil
	default:
		return "", fmt.Errorf("registration_mode must be sync or zero_copy")
	}
}

func sourceBoolMetadata(raw json.RawMessage, key string, fallback bool) bool {
	var m map[string]any
	if len(raw) > 0 && json.Unmarshal(raw, &m) == nil {
		if v, ok := m[key].(bool); ok {
			return v
		}
	}
	return fallback
}

func discoverConnectionSources(c *models.Connection) []models.DiscoveredSource {
	zeroCopyTypes := map[string]bool{"adls": true, "azure_blob": true, "bigquery": true, "csv": true, "databricks": true, "gcs": true, "generic": true, "google_cloud_storage": true, "json": true, "mysql": true, "open_table_catalog": true, "postgresql": true, "s3": true, "snowflake": true}
	var cfg map[string]json.RawMessage
	_ = json.Unmarshal(c.Config, &cfg)
	for _, key := range []string{"tables", "datasets", "iceberg_tables", "delta_tables", "topics", "streams", "entities", "objects"} {
		if raw, ok := cfg[key]; ok {
			if out := discoveredFromConfigArray(raw, key, c.ConnectorType, zeroCopyTypes[c.ConnectorType]); len(out) > 0 {
				return out
			}
		}
	}
	meta, _ := json.Marshal(map[string]any{"connection_type": c.ConnectorType, "supports_zero_copy": zeroCopyTypes[c.ConnectorType]})
	return []models.DiscoveredSource{{Selector: c.Name, DisplayName: c.Name, SourceKind: c.ConnectorType, SupportsSync: true, SupportsZeroCopy: zeroCopyTypes[c.ConnectorType], Metadata: meta}}
}

func discoveredFromConfigArray(raw json.RawMessage, collection, connectorType string, zeroCopy bool) []models.DiscoveredSource {
	var entries []map[string]any
	if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 {
		return nil
	}
	out := make([]models.DiscoveredSource, 0, len(entries))
	for _, t := range entries {
		selector := stringValue(t, "selector", stringValue(t, "name", stringValue(t, "table", stringValue(t, "dataset", stringValue(t, "topic", stringValue(t, "stream", stringValue(t, "entity", "")))))))
		if selector == "" {
			continue
		}
		display := stringValue(t, "display_name", selector)
		kind := stringValue(t, "source_kind", defaultSourceKind(connectorType, collection))
		metadata, _ := json.Marshal(t)
		out = append(out, models.DiscoveredSource{Selector: selector, DisplayName: display, SourceKind: kind, SupportsSync: true, SupportsZeroCopy: boolValue(t, "supports_zero_copy", zeroCopy), Metadata: metadata})
	}
	return out
}

func defaultSourceKind(connectorType, collection string) string {
	switch collection {
	case "topics":
		return "topic"
	case "streams":
		return "stream"
	case "iceberg_tables":
		return "iceberg_table"
	case "delta_tables":
		return "delta_table"
	case "datasets":
		if connectorType == "parquet" {
			return "parquet_file"
		}
	}
	return connectorType
}

func stringValue(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
func boolValue(m map[string]any, key string, fallback bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

func (h *Handlers) sourceForClaims(w http.ResponseWriter, r *http.Request, id uuid.UUID) (*models.Connection, bool) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return nil, false
	}
	c, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "connection lookup failed")
		return nil, false
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return nil, false
	}
	if c.OwnerID != claims.Sub && !claims.HasRole("admin") && !claims.HasPermission("connections", "write") {
		writeJSONErr(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return c, true
}

func (h *Handlers) ListRegistrations(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok := h.sourceForClaims(w, r, id); !ok {
		return
	}
	items, err := h.Repo.ListRegistrations(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list registrations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registrations": items})
}

func (h *Handlers) DiscoverRegistrations(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": discoverConnectionSources(c)})
}

func (h *Handlers) BulkRegisterPreview(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	var body models.RegistrationBulkRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Registrations) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "registrations array is empty")
		return
	}
	discovered := discoverConnectionSources(c)
	bySelector := map[string]models.DiscoveredSource{}
	for _, d := range discovered {
		bySelector[d.Selector] = d
	}
	matched, unmatched, invalid := []any{}, []any{}, []any{}
	for _, item := range body.Registrations {
		mode, err := normalizeRegistrationMode(item.RegistrationMode)
		if err != nil {
			invalid = append(invalid, map[string]any{"selector": item.Selector, "error": err.Error()})
			continue
		}
		if d, ok := bySelector[item.Selector]; ok {
			matched = append(matched, map[string]any{"selector": item.Selector, "source_kind": d.SourceKind, "supports_zero_copy": d.SupportsZeroCopy, "supports_sync": d.SupportsSync, "registration_mode": mode})
		} else {
			unmatched = append(unmatched, map[string]any{"selector": item.Selector})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"discovered_count": len(discovered), "matched": matched, "unmatched": unmatched, "invalid": invalid})
}

func (h *Handlers) BulkRegister(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	var body models.RegistrationBulkRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Registrations) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "registrations array is empty")
		return
	}
	discovered := discoverConnectionSources(c)
	bySelector := map[string]models.DiscoveredSource{}
	for _, d := range discovered {
		bySelector[d.Selector] = d
	}
	created, errs := []models.ConnectionRegistration{}, []any{}
	for _, item := range body.Registrations {
		mode, err := normalizeRegistrationMode(item.RegistrationMode)
		if err != nil {
			errs = append(errs, map[string]any{"selector": item.Selector, "error": err.Error()})
			continue
		}
		src, ok := bySelector[item.Selector]
		if !ok {
			src = models.DiscoveredSource{Selector: item.Selector, DisplayName: item.Selector, SourceKind: c.ConnectorType, SupportsSync: true, Metadata: []byte(`{}`)}
			if item.DisplayName != nil {
				src.DisplayName = *item.DisplayName
			}
			if item.SourceKind != nil {
				src.SourceKind = *item.SourceKind
			}
		}
		autoSync := item.AutoSync != nil && *item.AutoSync
		updateDetection := item.UpdateDetection == nil || *item.UpdateDetection
		meta := item.Metadata
		if len(meta) == 0 || string(meta) == "null" {
			meta = src.Metadata
		}
		reg, err := h.Repo.UpsertRegistration(r.Context(), id, src, mode, autoSync, updateDetection, item.TargetDatasetID, meta)
		if err != nil {
			errs = append(errs, map[string]any{"selector": item.Selector, "error": err.Error()})
			continue
		}
		created = append(created, *reg)
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "errors": errs})
}

func (h *Handlers) AutoRegister(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	var body models.AutoRegisterRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	mode, err := normalizeRegistrationMode(body.RegistrationMode)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	selectorSet := map[string]bool{}
	for _, s := range body.Selectors {
		selectorSet[s] = true
	}
	autoSync := body.AutoSync != nil && *body.AutoSync
	updateDetection := body.UpdateDetection == nil || *body.UpdateDetection
	created := []models.ConnectionRegistration{}
	errs := []any{}
	discovered := discoverConnectionSources(c)
	for _, src := range discovered {
		if len(selectorSet) > 0 && !selectorSet[src.Selector] {
			continue
		}
		reg, err := h.Repo.UpsertRegistration(r.Context(), id, src, mode, autoSync, updateDetection, body.DefaultTargetDatasetID, []byte(`{"origin":"auto_register"}`))
		if err != nil {
			errs = append(errs, map[string]any{"selector": src.Selector, "error": err.Error()})
			continue
		}
		created = append(created, *reg)
	}
	writeJSON(w, http.StatusOK, map[string]any{"discovered_count": len(discovered), "created": created, "errors": errs})
}

func (h *Handlers) AutoRegisterStatus(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	settings := any(workers.AutoRegistrationSettingsViewFromConfig(c.Config))
	if settings == nil {
		settings = map[string]any{"enabled": false}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection_id": id, "settings": settings, "last_run": workers.DefaultAutoRegistrationRecorder.LastRun(id)})
}

func (h *Handlers) UpdateAutoRegistration(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, ok := h.sourceForClaims(w, r, id)
	if !ok {
		return
	}
	var body models.UpdateAutoRegistrationBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if _, err := normalizeRegistrationMode(body.RegistrationMode); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var cfg map[string]any
	_ = json.Unmarshal(c.Config, &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	block, _ := cfg["auto_registration"].(map[string]any)
	if block == nil {
		block = map[string]any{}
		cfg["auto_registration"] = block
	}
	if body.Enabled != nil {
		block["enabled"] = *body.Enabled
	}
	if body.RegistrationMode != nil {
		block["registration_mode"] = *body.RegistrationMode
	}
	if body.AutoSync != nil {
		block["auto_sync"] = *body.AutoSync
	}
	if body.UpdateDetection != nil {
		block["update_detection"] = *body.UpdateDetection
	}
	if body.Selectors != nil {
		block["selectors"] = body.Selectors
	}
	if body.IntervalSeconds != nil {
		block["interval_secs"] = *body.IntervalSeconds
	}
	if body.TagFilters != nil {
		block["tag_filters"] = body.TagFilters
	}
	b, _ := json.Marshal(cfg)
	updated, err := h.Repo.UpdateConnectionConfig(r.Context(), id, b)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to update auto registration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection_id": id, "settings": block, "connection": updated})
}

func (h *Handlers) DeleteRegistration(w http.ResponseWriter, r *http.Request) {
	sid, _, err := routeUUIDParam(r, "source_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	rid, _, err := routeUUIDParam(r, "registration_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid registration_id")
		return
	}
	if _, ok := h.sourceForClaims(w, r, sid); !ok {
		return
	}
	deleted, err := h.Repo.DeleteRegistration(r.Context(), sid, rid)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to delete registration")
		return
	}
	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) QueryRegistration(w http.ResponseWriter, r *http.Request) {
	sid, _, err := routeUUIDParam(r, "source_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	rid, _, err := routeUUIDParam(r, "registration_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid registration_id")
		return
	}
	c, ok := h.sourceForClaims(w, r, sid)
	if !ok {
		return
	}
	reg, err := h.Repo.GetRegistration(r.Context(), sid, rid)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "registration lookup failed")
		return
	}
	if reg == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	limit := 10
	var body models.QueryRegistrationBody
	if r.Body != nil && json.NewDecoder(r.Body).Decode(&body) == nil && body.Limit != nil {
		limit = *body.Limit
	}
	response := virtualTableQueryFromConfig(c, reg, limit)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) QueryRegistrationArrow(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/json" {
		h.QueryRegistration(w, r)
		return
	}
	sid, _, err := routeUUIDParam(r, "source_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	rid, _, err := routeUUIDParam(r, "registration_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid registration_id")
		return
	}
	if _, ok := h.sourceForClaims(w, r, sid); !ok {
		return
	}
	reg, err := h.Repo.GetRegistration(r.Context(), sid, rid)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "registration lookup failed")
		return
	}
	if reg == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	limit := 10
	var body models.QueryRegistrationBody
	if r.Body != nil && json.NewDecoder(r.Body).Decode(&body) == nil && body.Limit != nil {
		limit = *body.Limit
	}
	c, err := h.Repo.GetConnection(r.Context(), sid)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "connection lookup failed")
		return
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	query := virtualTableQueryFromConfig(c, reg, limit)
	stream, err := materializeArrowStream(query.Columns, query.Rows)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to encode arrow stream")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(stream)
}

func virtualTableQueryFromConfig(c *models.Connection, reg *models.ConnectionRegistration, limit int) models.VirtualTableQueryResponse {
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	rows := rowsForSelector(c.Config, reg.Selector, limit)
	if len(rows) == 0 {
		rows = make([]json.RawMessage, 0, limit)
		for i := 0; i < limit; i++ {
			b, _ := json.Marshal(map[string]any{"selector": reg.Selector, "row_number": i + 1})
			rows = append(rows, b)
		}
	}
	columns := columnsForRows(rows)
	sig := reg.LastSourceSignature
	if sig == nil {
		b, _ := json.Marshal(rows)
		digest := sha256.Sum256(b)
		value := fmt.Sprintf("sha256:%x", digest[:])
		sig = &value
	}
	meta := json.RawMessage(fmt.Sprintf(`{"connection_type":%q,"adapter":"go_inline"}`, c.ConnectorType))
	return models.VirtualTableQueryResponse{Selector: reg.Selector, Mode: reg.RegistrationMode, Columns: columns, RowCount: len(rows), Rows: rows, SourceSignature: sig, Metadata: meta}
}

func rowsForSelector(config json.RawMessage, selector string, limit int) []json.RawMessage {
	var cfg map[string]json.RawMessage
	_ = json.Unmarshal(config, &cfg)
	for _, key := range []string{"tables", "datasets", "iceberg_tables", "delta_tables", "topics", "streams", "entities", "objects"} {
		raw, ok := cfg[key]
		if !ok {
			continue
		}
		var entries []map[string]any
		if json.Unmarshal(raw, &entries) != nil {
			continue
		}
		for _, entry := range entries {
			name := stringValue(entry, "selector", stringValue(entry, "name", stringValue(entry, "table", stringValue(entry, "dataset", stringValue(entry, "topic", stringValue(entry, "stream", stringValue(entry, "entity", "")))))))
			if name != selector {
				continue
			}
			for _, rowKey := range []string{"sample_rows", "rows", "records", "messages"} {
				if rows, ok := rawRows(entry[rowKey], limit); ok {
					return rows
				}
			}
		}
	}
	return nil
}

func rawRows(value any, limit int) ([]json.RawMessage, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		b, _ := json.Marshal(item)
		out = append(out, b)
	}
	return out, true
}

func columnsForRows(rows []json.RawMessage) []string {
	seen := map[string]bool{}
	cols := []string{}
	for _, raw := range rows {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		for key := range obj {
			if !seen[key] {
				seen[key] = true
				cols = append(cols, key)
			}
		}
	}
	if len(cols) == 0 {
		return []string{"selector", "row_number"}
	}
	return cols
}

func materializeArrowStream(columns []string, rows []json.RawMessage) ([]byte, error) {
	mem := memory.NewGoAllocator()
	fields := make([]arrow.Field, 0, len(columns))
	arrays := make([]arrow.Array, 0, len(columns))
	for _, name := range columns {
		fields = append(fields, arrow.Field{Name: name, Type: arrow.BinaryTypes.String, Nullable: true})
		builder := array.NewStringBuilder(mem)
		defer builder.Release()
		for _, raw := range rows {
			var obj map[string]any
			_ = json.Unmarshal(raw, &obj)
			value, ok := obj[name]
			if !ok || value == nil {
				builder.AppendNull()
				continue
			}
			switch v := value.(type) {
			case string:
				builder.Append(v)
			default:
				builder.Append(fmt.Sprint(v))
			}
		}
		arr := builder.NewArray()
		defer arr.Release()
		arrays = append(arrays, arr)
	}
	schema := arrow.NewSchema(fields, nil)
	rec := array.NewRecord(schema, arrays, int64(len(rows)))
	defer rec.Release()
	var buf bytes.Buffer
	writer := ipc.NewWriter(&buf, ipc.WithSchema(schema), ipc.WithAllocator(mem))
	if err := writer.Write(rec); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type connectionTestAdapter interface {
	TestConnection(ctx context.Context, raw json.RawMessage) (adapters.ConnectionTestResult, error)
}

func (h *Handlers) TestConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "connection lookup failed")
		return
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}

	result := adapters.ConnectionTestResult{
		Success:   false,
		Message:   fmt.Sprintf("unsupported connector type: %s", c.ConnectorType),
		LatencyMS: 0,
	}
	if h.AdapterRegistry != nil {
		adapter, err := h.AdapterRegistry.Lookup(c.ConnectorType)
		if err != nil {
			result.Message = err.Error()
		} else if tester, ok := adapter.(connectionTestAdapter); ok {
			if tested, err := tester.TestConnection(r.Context(), c.Config); err != nil {
				result.Message = err.Error()
			} else {
				result = tested
			}
		} else {
			result.Message = fmt.Sprintf("test_connection is not supported for connector type: %s", c.ConnectorType)
		}
	}

	status := "error"
	if result.Success {
		status = "connected"
	}
	_, _ = h.Repo.UpdateConnection(r.Context(), id, &models.UpdateConnectionRequest{Status: &status})
	var latency any
	if result.Success || result.LatencyMS != 0 {
		latency = result.LatencyMS
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": result.Success, "message": result.Message, "latency_ms": latency, "details": result.Details})
}

func (h *Handlers) InvokeWebhook(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	id, _, err := routeUUIDParam(r, "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load webhook")
		return
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "webhook not found")
		return
	}
	var body models.InvokeWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var def models.WebhookDefinition
	if err := json.Unmarshal(c.Config, &def); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid webhook config")
		return
	}
	method := strings.ToUpper(strings.TrimSpace(def.Method))
	if method == "" {
		method = http.MethodPost
	}
	u, err := url.Parse(def.URL)
	if err != nil || u.Scheme == "" {
		writeJSONErr(w, http.StatusBadRequest, "invalid webhook url")
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), method, def.URL, bytes.NewReader(body.Inputs))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid webhook request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range def.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		writeJSONErr(w, http.StatusBadGateway, "webhook upstream error: "+err.Error())
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var val json.RawMessage = []byte(`null`)
	if len(strings.TrimSpace(string(b))) > 0 {
		val = b
	}
	var decoded map[string]json.RawMessage
	_ = json.Unmarshal(val, &decoded)
	out := json.RawMessage(`{}`)
	if decoded != nil && decoded["output_parameters"] != nil {
		out = decoded["output_parameters"]
	}
	writeJSON(w, http.StatusOK, models.InvokeWebhookResponse{Status: uint16(resp.StatusCode), Response: val, OutputParameters: out})
}

func sanitizeIceberg(value string) string {
	return regexp.MustCompile(`[^A-Za-z0-9_-]`).ReplaceAllString(value, "_")
}
func icebergNotFound(w http.ResponseWriter, kind, value string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": fmt.Sprintf("%s '%s' not found", kind, value), "type": "NoSuchNamespaceException", "code": 404}})
}

func (h *Handlers) IcebergGetConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, models.IcebergConfigResponse{Defaults: models.IcebergConfigValues{Warehouse: "openfoundry"}, Overrides: models.IcebergConfigValues{}})
}
func (h *Handlers) IcebergListNamespaces(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	conns, err := h.Repo.ListIcebergNamespaces(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list namespaces")
		return
	}
	ns := [][]string{}
	for _, c := range conns {
		ns = append(ns, []string{sanitizeIceberg(c.Name)})
	}
	writeJSON(w, http.StatusOK, models.IcebergListNamespacesResponse{Namespaces: ns})
}
func (h *Handlers) IcebergGetNamespace(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	ns := chi.URLParam(r, "namespace")
	c, err := h.Repo.GetIcebergConnection(r.Context(), ns)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load namespace")
		return
	}
	if c == nil {
		icebergNotFound(w, "namespace", ns)
		return
	}
	writeJSON(w, http.StatusOK, models.IcebergNamespaceResponse{Namespace: []string{sanitizeIceberg(c.Name)}, Properties: map[string]string{"connection_id": c.ID.String(), "connector_type": c.ConnectorType, "owner": c.OwnerID.String()}})
}
func (h *Handlers) IcebergListTables(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	ns := chi.URLParam(r, "namespace")
	c, err := h.Repo.GetIcebergConnection(r.Context(), ns)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load namespace")
		return
	}
	if c == nil {
		icebergNotFound(w, "namespace", ns)
		return
	}
	regs, err := h.Repo.ListIcebergTables(r.Context(), c.ID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list tables")
		return
	}
	ids := []models.IcebergTableIdentifier{}
	for _, r := range regs {
		ids = append(ids, models.IcebergTableIdentifier{Namespace: []string{sanitizeIceberg(c.Name)}, Name: r.Selector})
	}
	writeJSON(w, http.StatusOK, models.IcebergListTablesResponse{Identifiers: ids})
}
func (h *Handlers) IcebergLoadTable(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	ns, table := chi.URLParam(r, "namespace"), chi.URLParam(r, "table")
	c, err := h.Repo.GetIcebergConnection(r.Context(), ns)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load namespace")
		return
	}
	if c == nil {
		icebergNotFound(w, "namespace", ns)
		return
	}
	regs, err := h.Repo.ListIcebergTables(r.Context(), c.ID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load table")
		return
	}
	for i, reg := range regs {
		if reg.Selector == table || (i == 0 && len(regs) == 1) {
			loc := fmt.Sprintf("openfoundry://catalog/%s/%s/v0.metadata.json", c.ID, reg.ID)
			var meta map[string]any
			_ = json.Unmarshal(reg.Metadata, &meta)
			upstreamMetadata := false
			if u, ok := meta["upstream"].(map[string]any); ok {
				if s, ok := u["metadata_location"].(string); ok {
					loc = s
					upstreamMetadata = true
				}
			}
			if d, ok := meta["discovery"].(map[string]any); ok {
				if u, ok := d["upstream"].(map[string]any); ok {
					if s, ok := u["metadata_location"].(string); ok {
						loc = s
						upstreamMetadata = true
					}
				}
			}
			cfgMap := map[string]any{"connection_id": c.ID.String(), "registration_id": reg.ID.String(), "connector_type": c.ConnectorType, "source_kind": reg.SourceKind}
			if !upstreamMetadata {
				cfgMap["foundry-vended"] = fmt.Sprintf("/api/v1/data-connection/sources/%s/registrations/%s/query", c.ID, reg.ID)
			}
			ttl := h.Config.VendedCredentialsTTLSeconds
			if ttl <= 0 {
				ttl = 900
			}
			for k, v := range domain.VendCredentials(c, ttl, time.Now()).Entries {
				cfgMap[k] = v
			}
			cfg, _ := json.Marshal(cfgMap)
			md, _ := json.Marshal(map[string]any{"format-version": 2, "table-uuid": reg.ID.String(), "location": loc, "last-updated-ms": time.Now().UnixMilli(), "current-schema-id": 0, "schemas": []any{map[string]any{"schema-id": 0, "type": "struct", "fields": []any{}}}, "properties": map[string]any{"openfoundry.selector": reg.Selector}})
			writeJSON(w, http.StatusOK, models.IcebergLoadTableResponse{MetadataLocation: loc, Metadata: md, Config: cfg})
			return
		}
	}
	icebergNotFound(w, "table", table)
}
