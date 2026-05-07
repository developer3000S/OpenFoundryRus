// Shared helpers for the saved-view / saved-map / writeback handlers.
//
// These mirror the small free functions in src/handlers.rs
// (`now_ms`, `datetime_from_ms`, `required_string`, plain-text error
// envelope, JSON encoder).

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// nowMs returns the current wall-clock millisecond, preferring an
// injected clock when set. Mirrors Rust `now_ms()`.
func (h *Handlers) nowMs() int64 {
	if h != nil && h.Now != nil {
		return h.Now()
	}
	return time.Now().UTC().UnixMilli()
}

// datetimeFromMs mirrors Rust `datetime_from_ms`: epoch-ms → UTC
// time.Time, falling back to time.Now when the input is invalid.
func datetimeFromMs(ms int64) time.Time {
	if ms < 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(ms).UTC()
}

// requiredString mirrors Rust `required_string`: pulls a string field
// from a JSON payload, erroring out when missing or non-string.
func requiredString(payload json.RawMessage, field string) (string, error) {
	var m map[string]json.RawMessage
	if len(payload) == 0 {
		return "", fmt.Errorf("stored record is missing %s", field)
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return "", fmt.Errorf("stored record is missing %s", field)
	}
	raw, ok := m[field]
	if !ok {
		return "", fmt.Errorf("stored record is missing %s", field)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("stored record is missing %s", field)
	}
	return s, nil
}

// payloadField returns the raw JSON value for a top-level key, or nil
// when missing. Mirrors `record.payload.get(field).cloned()`.
func payloadField(payload json.RawMessage, field string) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil
	}
	return m[field]
}

// pickTimestamp returns *primary or, when nil, *fallback, or 0.
// Mirrors `record.created_at_ms.or(record.updated_at_ms).unwrap_or(0)`.
func pickTimestamp(primary, fallback *int64) int64 {
	if primary != nil {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return 0
}

// nullableRaw replaces an empty json.RawMessage with `null` so JSON
// marshaling produces a stable shape (matches Rust serde_json::Value
// which serialises Null when the source is None / absent).
func nullableRaw(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
}

// writeJSON encodes payload as JSON and writes it to w with the given
// status. Mirrors axum `(status, Json(value))`.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// plainText writes a `text/plain` body matching axum's default for
// `(StatusCode, &str)` / `(StatusCode, String)` tuples.
func plainText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
