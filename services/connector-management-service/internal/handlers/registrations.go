package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

type webhookHistoryAppender interface {
	AppendWebhookHistory(ctx context.Context, body *models.CreateWebhookHistoryEntry) (*models.WebhookHistoryEntry, error)
}

type webhookHistoryLister interface {
	ListWebhookHistory(ctx context.Context, sourceID uuid.UUID, limit int) ([]models.WebhookHistoryEntry, error)
}

type inboundListenerEventAppender interface {
	AppendInboundListenerEvent(ctx context.Context, body *models.CreateInboundListenerEvent) (*models.InboundListenerEvent, error)
}

type inboundListenerEventLister interface {
	ListInboundListenerEvents(ctx context.Context, sourceID uuid.UUID, limit int) ([]models.InboundListenerEvent, error)
}

func (h *Handlers) InvokeWebhook(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
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
	if c.OwnerID != claims.Sub && !claims.HasRole("admin") && !claims.HasPermission("connections", "write") && !claims.HasPermission("webhooks", "invoke") {
		writeJSONErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var body models.InvokeWebhookRequest
	r.Body = http.MaxBytesReader(w, r.Body, int64(models.DefaultWebhookMaxInputBytes))
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	def, source, err := webhookDefinitionForConnection(c)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid webhook config")
		return
	}
	startedAt := time.Now().UTC()
	recordFailure := func(status int, msg string, callCount int, upstreamStatus *uint16) {
		_ = h.recordWebhookHistory(r.Context(), id, claims.Sub, def, body.Inputs, "failed", upstreamStatus, nil, msg, startedAt, callCount)
		writeJSONErr(w, status, msg)
	}
	if source != nil && !source.Permissions.Invokable {
		recordFailure(http.StatusForbidden, "source is not invokable", 0, nil)
		return
	}
	if err := models.ValidateWebhookInvocation(def, body.Inputs); err != nil {
		recordFailure(http.StatusBadRequest, models.SanitizeWebhookDiagnostic(err.Error()), 0, nil)
		return
	}
	release, retryAfter, ok := defaultWebhookInvocationLimiter.acquire(id, def)
	if !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		recordFailure(http.StatusTooManyRequests, "webhook invocation limit exceeded", 0, nil)
		return
	}
	defer release()
	timeout := time.Duration(def.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	state := map[string]any{}
	results := make([]models.WebhookCallResult, 0, len(def.Calls))
	for _, call := range def.Calls {
		built, err := models.BuildWebhookRequest(def, source, call, body.Inputs, state)
		if err != nil {
			recordFailure(http.StatusBadRequest, "invalid webhook request: "+models.SanitizeWebhookDiagnostic(err.Error()), len(results), nil)
			return
		}
		if err := validateWebhookBuiltRequest(source, built); err != nil {
			recordFailure(http.StatusForbidden, models.SanitizeWebhookDiagnostic(err.Error()), len(results), nil)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), built.Method, built.URL, bytes.NewReader(built.Body))
		if err != nil {
			recordFailure(http.StatusBadRequest, "invalid webhook request", len(results), nil)
			return
		}
		for k, v := range built.Headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			recordFailure(http.StatusBadGateway, "webhook upstream error: "+models.SanitizeWebhookDiagnostic(err.Error()), len(results)+1, nil)
			return
		}
		b, tooLarge := readWebhookResponseBody(resp.Body, def.Limits.MaxResponseBytes)
		_ = resp.Body.Close()
		if tooLarge {
			upstreamStatus := uint16(resp.StatusCode)
			recordFailure(http.StatusBadGateway, "webhook response exceeded max_response_bytes", len(results)+1, &upstreamStatus)
			return
		}
		var val json.RawMessage = []byte(`null`)
		if len(strings.TrimSpace(string(b))) > 0 {
			val = b
		}
		result := models.WebhookCallResult{CallID: call.ID, Status: uint16(resp.StatusCode), Response: val}
		results = append(results, result)
		if err := models.CaptureWebhookCallResult(def, call, result, state); err != nil {
			recordFailure(http.StatusBadGateway, "webhook response extraction failed: "+models.SanitizeWebhookDiagnostic(err.Error()), len(results), &result.Status)
			return
		}
	}
	out, err := models.ExtractWebhookOutputs(def, results)
	if err != nil {
		var upstreamStatus *uint16
		if len(results) > 0 {
			upstreamStatus = &results[len(results)-1].Status
		}
		recordFailure(http.StatusBadGateway, "webhook output extraction failed: "+models.SanitizeWebhookDiagnostic(err.Error()), len(results), upstreamStatus)
		return
	}
	final := json.RawMessage(`null`)
	status := uint16(0)
	if len(results) > 0 {
		final = results[len(results)-1].Response
		status = results[len(results)-1].Status
	}
	var upstreamStatus *uint16
	if status > 0 {
		upstreamStatus = &status
	}
	history := h.recordWebhookHistory(r.Context(), id, claims.Sub, def, body.Inputs, "succeeded", upstreamStatus, out, "", startedAt, len(results))
	writeJSON(w, http.StatusOK, models.InvokeWebhookResponse{Status: status, Response: final, OutputParameters: out, History: history})
}

func (h *Handlers) ListWebhookHistory(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.Repo == nil {
		writeRoutePending(w, http.StatusServiceUnavailable, "repository_unavailable", "connection repository is not configured")
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
	if c.OwnerID != claims.Sub && !claims.HasRole("admin") && !claims.HasPermission("connections", "read") && !claims.HasPermission("webhooks", "read") {
		writeJSONErr(w, http.StatusForbidden, "forbidden")
		return
	}
	lister, ok := h.Repo.(webhookHistoryLister)
	if !ok {
		writeRoutePending(w, http.StatusServiceUnavailable, "webhook_history_repository_unavailable", "webhook history repository is not configured")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeJSONErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	items, err := lister.ListWebhookHistory(r.Context(), id, limit)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list webhook history")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.WebhookHistoryEntry]{Items: items})
}

func (h *Handlers) ReceiveInboundListener(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.Repo == nil {
		writeRoutePending(w, http.StatusServiceUnavailable, "repository_unavailable", "connection repository is not configured")
		return
	}
	appender, ok := h.Repo.(inboundListenerEventAppender)
	if !ok {
		writeRoutePending(w, http.StatusServiceUnavailable, "inbound_listener_repository_unavailable", "inbound listener repository is not configured")
		return
	}
	c, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load listener source")
		return
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "listener source not found")
		return
	}
	listenerID := strings.TrimSpace(chi.URLParam(r, "listener_id"))
	def, err := inboundListenerDefinitionForConnection(c, listenerID)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := def.Limits.MaxPayloadBytes
	if limit <= 0 {
		limit = models.DefaultInboundListenerMaxPayloadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(limit))
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "listener payload exceeded max_payload_bytes")
		return
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "listener payload is empty")
		return
	}
	if !json.Valid(payload) {
		writeJSONErr(w, http.StatusBadRequest, "listener payload must be valid JSON")
		return
	}
	signatureVerified, authErr := verifyInboundListenerAuth(def, r.Header, payload)
	if authErr != nil {
		status := http.StatusUnauthorized
		var typed inboundListenerAuthError
		if errors.As(authErr, &typed) {
			status = typed.status
		}
		writeJSONErr(w, status, authErr.Error())
		return
	}
	headers := sanitizeInboundListenerHeaders(r.Header, def.Auth.Header)
	event, err := appender.AppendInboundListenerEvent(r.Context(), &models.CreateInboundListenerEvent{
		SourceID:          id,
		ListenerID:        def.ID,
		EventID:           inboundListenerExternalEventID(r.Header, payload),
		Status:            "accepted",
		SignatureVerified: signatureVerified,
		Payload:           append(json.RawMessage(nil), payload...),
		Headers:           headers,
		Destination:       def.Destination,
	})
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to store inbound listener event")
		return
	}
	writeJSON(w, http.StatusAccepted, models.ReceiveInboundListenerResponse{
		EventID:           event.ID,
		SourceID:          event.SourceID,
		ListenerID:        event.ListenerID,
		Status:            event.Status,
		SignatureVerified: event.SignatureVerified,
		Destination:       event.Destination,
	})
}

func (h *Handlers) ListInboundListenerEvents(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.Repo == nil {
		writeRoutePending(w, http.StatusServiceUnavailable, "repository_unavailable", "connection repository is not configured")
		return
	}
	c, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load listener source")
		return
	}
	if c == nil {
		writeJSONErr(w, http.StatusNotFound, "listener source not found")
		return
	}
	if c.OwnerID != claims.Sub && !claims.HasRole("admin") && !claims.HasPermission("connections", "read") && !claims.HasPermission("listeners", "read") {
		writeJSONErr(w, http.StatusForbidden, "forbidden")
		return
	}
	lister, ok := h.Repo.(inboundListenerEventLister)
	if !ok {
		writeRoutePending(w, http.StatusServiceUnavailable, "inbound_listener_repository_unavailable", "inbound listener repository is not configured")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeJSONErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	items, err := lister.ListInboundListenerEvents(r.Context(), id, limit)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list inbound listener events")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.InboundListenerEvent]{Items: items})
}

func (h *Handlers) recordWebhookHistory(ctx context.Context, sourceID uuid.UUID, userID uuid.UUID, def *models.WebhookDefinition, inputs json.RawMessage, status string, httpStatus *uint16, outputs json.RawMessage, errorMessage string, startedAt time.Time, callCount int) json.RawMessage {
	if def == nil {
		return json.RawMessage(`{"enabled":false,"stored":false}`)
	}
	retentionDays := def.History.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}
	summary := map[string]any{
		"enabled":        def.History.Enabled,
		"retention_days": retentionDays,
		"stored":         false,
		"stored_inputs":  def.History.Enabled && def.History.StoreInputs,
		"stored_outputs": def.History.Enabled && def.History.StoreOutputs,
		"call_count":     callCount,
		"status":         status,
	}
	if !def.History.Enabled {
		return marshalWebhookHistorySummary(summary)
	}
	appender, ok := h.Repo.(webhookHistoryAppender)
	if !ok {
		summary["error"] = "history repository unavailable"
		return marshalWebhookHistorySummary(summary)
	}
	finishedAt := time.Now().UTC()
	if startedAt.IsZero() {
		startedAt = finishedAt
	}
	var storedInputs json.RawMessage
	if def.History.StoreInputs && len(inputs) > 0 {
		storedInputs = append(json.RawMessage(nil), inputs...)
	}
	var storedOutputs json.RawMessage
	if def.History.StoreOutputs && len(outputs) > 0 {
		storedOutputs = append(json.RawMessage(nil), outputs...)
	}
	visibility := "hidden"
	if def.History.StoreInputs {
		visibility = "stored"
	}
	var errPtr *string
	if strings.TrimSpace(errorMessage) != "" {
		sanitized := models.SanitizeWebhookDiagnostic(errorMessage)
		errPtr = &sanitized
	}
	entry, err := appender.AppendWebhookHistory(ctx, &models.CreateWebhookHistoryEntry{
		SourceID:   sourceID,
		UserID:     userID,
		Status:     status,
		HTTPStatus: httpStatus,
		InputPolicy: models.WebhookHistoryInputPolicy{
			StoreInputs:  def.History.StoreInputs,
			StoreOutputs: def.History.StoreOutputs,
			Visibility:   visibility,
		},
		Inputs:             storedInputs,
		OutputParameters:   storedOutputs,
		Error:              errPtr,
		CallCount:          callCount,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		RetentionExpiresAt: finishedAt.Add(time.Duration(retentionDays) * 24 * time.Hour),
	})
	if err != nil {
		summary["error"] = "history persist failed"
		return marshalWebhookHistorySummary(summary)
	}
	summary["stored"] = true
	summary["entry_id"] = entry.ID
	summary["created_at"] = entry.CreatedAt
	summary["retention_expires_at"] = entry.RetentionExpiresAt
	return marshalWebhookHistorySummary(summary)
}

func marshalWebhookHistorySummary(summary map[string]any) json.RawMessage {
	body, err := json.Marshal(summary)
	if err != nil {
		return json.RawMessage(`{"enabled":false,"stored":false}`)
	}
	return body
}

func webhookDefinitionForConnection(c *models.Connection) (*models.WebhookDefinition, *models.RESTAPISourceConfig, error) {
	if c == nil {
		return nil, nil, errors.New("connection is nil")
	}
	switch strings.ToLower(strings.TrimSpace(c.ConnectorType)) {
	case "rest_api", "rest-api":
		source, err := models.ParseRESTAPISourceConfig(c.Config)
		if err != nil {
			return nil, nil, err
		}
		def, err := models.WebhookDefinitionFromRESTAPISource(source)
		return def, source, err
	default:
		def, err := models.NormalizeWebhookDefinition(c.Config)
		return def, nil, err
	}
}

func inboundListenerDefinitionForConnection(c *models.Connection, requestedID string) (*models.InboundListenerDefinition, error) {
	if c == nil {
		return nil, errors.New("connection is nil")
	}
	var envelope struct {
		Listener  *models.InboundListenerDefinition  `json:"listener"`
		Listeners []models.InboundListenerDefinition `json:"listeners"`
	}
	if err := json.Unmarshal(c.Config, &envelope); err != nil {
		return nil, fmt.Errorf("invalid listener config: %w", err)
	}
	requestedID = strings.TrimSpace(requestedID)
	if envelope.Listener != nil {
		def := *envelope.Listener
		if requestedID == "" || strings.EqualFold(strings.TrimSpace(def.ID), requestedID) || strings.TrimSpace(def.ID) == "" {
			if err := normalizeInboundListenerDefinition(&def, requestedID); err != nil {
				return nil, err
			}
			return &def, nil
		}
	}
	for _, listener := range envelope.Listeners {
		if requestedID == "" || strings.EqualFold(strings.TrimSpace(listener.ID), requestedID) {
			def := listener
			if err := normalizeInboundListenerDefinition(&def, requestedID); err != nil {
				return nil, err
			}
			return &def, nil
		}
	}
	if requestedID != "" {
		return nil, fmt.Errorf("listener %q is not configured", requestedID)
	}
	return nil, errors.New("listener is not configured for source")
}

func normalizeInboundListenerDefinition(def *models.InboundListenerDefinition, requestedID string) error {
	def.ID = strings.TrimSpace(def.ID)
	if def.ID == "" {
		def.ID = strings.TrimSpace(requestedID)
	}
	if def.ID == "" {
		def.ID = "default"
	}
	def.Type = strings.ToLower(strings.TrimSpace(def.Type))
	if def.Type == "" {
		def.Type = "https"
	}
	if def.Type != "https" {
		return fmt.Errorf("listener type %q is not supported", def.Type)
	}
	if !def.Enabled {
		return errors.New("listener is disabled")
	}
	def.Auth.Type = strings.ToLower(strings.TrimSpace(def.Auth.Type))
	if def.Auth.Type == "" {
		def.Auth.Type = "none"
	}
	def.Destination.Mode = strings.ToLower(strings.TrimSpace(def.Destination.Mode))
	if def.Destination.Mode == "" {
		def.Destination.Mode = "event_log"
	}
	switch def.Destination.Mode {
	case "event_log", "dataset", "object":
	default:
		return fmt.Errorf("listener destination mode %q is not supported", def.Destination.Mode)
	}
	if def.Limits.MaxPayloadBytes <= 0 {
		def.Limits.MaxPayloadBytes = models.DefaultInboundListenerMaxPayloadBytes
	}
	return nil
}

type inboundListenerAuthError struct {
	status  int
	message string
}

func (e inboundListenerAuthError) Error() string { return e.message }

func verifyInboundListenerAuth(def *models.InboundListenerDefinition, headers http.Header, payload []byte) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(def.Auth.Type)) {
	case "", "none":
		return false, nil
	case "shared_secret":
		secret := strings.TrimSpace(def.Auth.Secret)
		if secret == "" {
			return false, inboundListenerAuthError{status: http.StatusServiceUnavailable, message: "listener shared secret is not configured"}
		}
		headerName := strings.TrimSpace(def.Auth.Header)
		if headerName == "" {
			headerName = "X-OpenFoundry-Token"
		}
		got := headers.Get(headerName)
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			return false, inboundListenerAuthError{status: http.StatusUnauthorized, message: "listener token is invalid"}
		}
		return true, nil
	case "hmac_sha256", "hmac-sha256", "sha256":
		secret := strings.TrimSpace(def.Auth.Secret)
		if secret == "" {
			return false, inboundListenerAuthError{status: http.StatusServiceUnavailable, message: "listener HMAC secret is not configured"}
		}
		headerName := strings.TrimSpace(def.Auth.Header)
		if headerName == "" {
			headerName = "X-OpenFoundry-Signature"
		}
		got := strings.TrimSpace(headers.Get(headerName))
		if got == "" {
			return false, inboundListenerAuthError{status: http.StatusUnauthorized, message: "listener signature is missing"}
		}
		got = strings.TrimPrefix(got, "sha256=")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(payload)
		expected := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(got)), []byte(expected)) != 1 {
			return false, inboundListenerAuthError{status: http.StatusUnauthorized, message: "listener signature is invalid"}
		}
		return true, nil
	default:
		return false, inboundListenerAuthError{status: http.StatusBadRequest, message: "listener auth type is not supported"}
	}
}

func sanitizeInboundListenerHeaders(headers http.Header, authHeader string) json.RawMessage {
	redact := strings.ToLower(strings.TrimSpace(authHeader))
	if redact == "" {
		redact = "x-openfoundry-signature"
	}
	out := map[string][]string{}
	for key, values := range headers {
		if strings.EqualFold(key, redact) || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "x-openfoundry-token") {
			out[key] = []string{"[redacted]"}
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return body
}

func inboundListenerExternalEventID(headers http.Header, payload json.RawMessage) string {
	for _, name := range []string{"X-OpenFoundry-Event-Id", "X-Event-Id", "Idempotency-Key"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	var obj map[string]any
	if json.Unmarshal(payload, &obj) == nil {
		for _, key := range []string{"event_id", "eventId", "id"} {
			if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func validateWebhookBuiltRequest(source *models.RESTAPISourceConfig, built *models.BuiltWebhookRequest) error {
	if built == nil {
		return errors.New("webhook request is nil")
	}
	if source == nil {
		return nil
	}
	methodAllowed := len(source.Runtime.AllowedMethods) == 0
	for _, method := range source.Runtime.AllowedMethods {
		if strings.EqualFold(method, built.Method) {
			methodAllowed = true
			break
		}
	}
	if !methodAllowed {
		return fmt.Errorf("webhook method %s is not allowed by source runtime policy", built.Method)
	}
	u, err := url.Parse(built.URL)
	if err != nil || u.Host == "" {
		return errors.New("webhook URL is invalid")
	}
	if len(source.Permissions.AllowedEgressHosts) > 0 && !webhookHostAllowed(u.Host, source.Permissions.AllowedEgressHosts) {
		return fmt.Errorf("webhook egress host %s is not allowed by source permissions", u.Host)
	}
	return nil
}

func webhookHostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	hostname := host
	if parsedHost := strings.Split(host, ":"); len(parsedHost) > 0 {
		hostname = parsedHost[0]
	}
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if candidate == host || candidate == hostname {
			return true
		}
		if strings.HasPrefix(candidate, "*.") && strings.HasSuffix(hostname, strings.TrimPrefix(candidate, "*")) {
			return true
		}
	}
	return false
}

func readWebhookResponseBody(body io.Reader, maxBytes int) ([]byte, bool) {
	if maxBytes <= 0 {
		maxBytes = models.DefaultWebhookMaxResponseBytes
	}
	limited := io.LimitReader(body, int64(maxBytes)+1)
	b, _ := io.ReadAll(limited)
	if len(b) > maxBytes {
		return b[:maxBytes], true
	}
	return b, false
}

type webhookInvocationLimiter struct {
	mu     sync.Mutex
	active map[uuid.UUID]int
	recent map[uuid.UUID][]time.Time
}

var defaultWebhookInvocationLimiter = &webhookInvocationLimiter{
	active: map[uuid.UUID]int{},
	recent: map[uuid.UUID][]time.Time{},
}

func (l *webhookInvocationLimiter) acquire(id uuid.UUID, def *models.WebhookDefinition) (func(), int, bool) {
	if l == nil || def == nil {
		return func() {}, 0, true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if def.ConcurrencyLimit > 0 && l.active[id] >= def.ConcurrencyLimit {
		return func() {}, 1, false
	}
	if def.RateLimit != nil && def.RateLimit.MaxRequests > 0 && def.RateLimit.PerSeconds > 0 {
		window := time.Duration(def.RateLimit.PerSeconds) * time.Second
		cutoff := now.Add(-window)
		recent := l.recent[id][:0]
		for _, ts := range l.recent[id] {
			if ts.After(cutoff) {
				recent = append(recent, ts)
			}
		}
		if len(recent) >= def.RateLimit.MaxRequests {
			l.recent[id] = recent
			retryAfter := int(window.Seconds())
			if len(recent) > 0 {
				retryAfter = int(recent[0].Add(window).Sub(now).Seconds()) + 1
			}
			return func() {}, retryAfter, false
		}
		recent = append(recent, now)
		l.recent[id] = recent
	}
	l.active[id]++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.active[id] <= 1 {
			delete(l.active, id)
			return
		}
		l.active[id]--
	}, 0, true
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
