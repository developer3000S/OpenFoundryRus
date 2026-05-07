package handlers_test

// Schema validation + history endpoint coverage for IRF-9. Mirrors
// the behavioural assertions from the Rust handler and the registry
// crate; all cases run against fakes so the build invariant stays
// green without a Postgres or schema-registry instance.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/repo"
)

// ─── fakeSchemaStore ───────────────────────────────────────────────────

type fakeSchemaStore struct {
	streams map[uuid.UUID]struct {
		schema []byte
		mode   string
	}
	history map[uuid.UUID][]models.StreamSchemaVersion
	errOn   string
}

func (f *fakeSchemaStore) StreamExists(_ context.Context, id uuid.UUID) (bool, error) {
	if f.errOn == "exists" {
		return false, errors.New("boom")
	}
	_, ok := f.streams[id]
	return ok, nil
}

func (f *fakeSchemaStore) CurrentSchema(_ context.Context, id uuid.UUID) ([]byte, string, error) {
	if f.errOn == "current" {
		return nil, "", errors.New("boom")
	}
	v, ok := f.streams[id]
	if !ok {
		return nil, "", repo.ErrStreamNotFound
	}
	return v.schema, v.mode, nil
}

func (f *fakeSchemaStore) ListSchemaHistory(_ context.Context, id uuid.UUID) ([]models.StreamSchemaVersion, error) {
	if f.errOn == "history" {
		return nil, errors.New("boom")
	}
	return f.history[id], nil
}

// withClaims attaches a synthetic Claims to the request context so the
// requireClaims gate passes.
func withClaims(req *http.Request) *http.Request {
	c := &authmw.Claims{Sub: uuid.New()}
	return req.WithContext(authmw.ContextWithClaims(context.Background(), c))
}

// withChiID injects {id} as a chi URL param so chi.URLParam(r, "id")
// resolves correctly when the handler is exercised in isolation.
func withChiID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// ─── ValidateSchema ────────────────────────────────────────────────────

func TestValidateSchemaRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.SchemasHandler{
		Store:    &fakeSchemaStore{},
		Registry: handlers.NoopSchemaRegistry{},
	}
	req := httptest.NewRequest("POST", "/streams/x/schema:validate",
		strings.NewReader(`{"schema_avro":{}}`))
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestValidateSchemaRejectsBadStreamID(t *testing.T) {
	t.Parallel()
	h := &handlers.SchemasHandler{
		Store:    &fakeSchemaStore{},
		Registry: handlers.NoopSchemaRegistry{},
	}
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{"schema_avro":{}}`)))
	req = withChiID(req, "not-a-uuid")
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestValidateSchemaRejectsMissingSchemaAvro(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	h := &handlers.SchemasHandler{
		Store:    &fakeSchemaStore{},
		Registry: handlers.NoopSchemaRegistry{},
	}
	// schema_avro is absent → empty json.RawMessage → 400 before we
	// even touch the store.
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{}`)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestValidateSchemaReturns404WhenStreamMissing(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	h := &handlers.SchemasHandler{
		Store:    &fakeSchemaStore{streams: map[uuid.UUID]struct {
			schema []byte
			mode   string
		}{}},
		Registry: handlers.NoopSchemaRegistry{},
	}
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{"schema_avro":{"type":"record","name":"X","fields":[]}}`)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestValidateSchemaWithoutCurrentSkipsCompatAndWarns(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	store := &fakeSchemaStore{streams: map[uuid.UUID]struct {
		schema []byte
		mode   string
	}{
		// Nil schema → the skip-compat warning path.
		id: {schema: nil, mode: "BACKWARD"},
	}}
	h := &handlers.SchemasHandler{Store: store, Registry: handlers.NoopSchemaRegistry{}}
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{"schema_avro":{"type":"record","name":"X","fields":[]}}`)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.ValidateSchemaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Valid)
	assert.NotNil(t, resp.Fingerprint)
	assert.Nil(t, resp.Compatibility)
	require.Len(t, resp.Warnings, 1)
	assert.Contains(t, resp.Warnings[0], "compatibility check skipped")
}

func TestValidateSchemaWithIdenticalCurrentReportsCompatible(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	current := []byte(`{"type":"record","name":"X","fields":[]}`)
	store := &fakeSchemaStore{streams: map[uuid.UUID]struct {
		schema []byte
		mode   string
	}{id: {schema: current, mode: "BACKWARD"}}}
	h := &handlers.SchemasHandler{Store: store, Registry: handlers.NoopSchemaRegistry{}}
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{"schema_avro":{"type":"record","name":"X","fields":[]}}`)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.ValidateSchemaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Compatibility)
	assert.Equal(t, "BACKWARD", resp.Compatibility.Mode)
	assert.True(t, resp.Compatibility.Compatible)
	assert.True(t, resp.Valid)
}

func TestValidateSchemaWithDriftReportsIncompatible(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	current := []byte(`{"type":"record","name":"X","fields":[{"name":"a","type":"string"}]}`)
	store := &fakeSchemaStore{streams: map[uuid.UUID]struct {
		schema []byte
		mode   string
	}{id: {schema: current, mode: "BACKWARD"}}}
	h := &handlers.SchemasHandler{Store: store, Registry: handlers.NoopSchemaRegistry{}}
	req := withClaims(httptest.NewRequest("POST", "/v",
		strings.NewReader(`{"schema_avro":{"type":"record","name":"X","fields":[{"name":"b","type":"string"}]}}`)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.ValidateSchemaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Compatibility)
	assert.False(t, resp.Compatibility.Compatible)
	assert.False(t, resp.Valid)
	assert.NotEmpty(t, resp.Errors)
}

func TestValidateSchemaRejectsInvalidCompatibilityMode(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	store := &fakeSchemaStore{streams: map[uuid.UUID]struct {
		schema []byte
		mode   string
	}{id: {schema: []byte(`{"type":"record","name":"X","fields":[]}`), mode: "BACKWARD"}}}
	h := &handlers.SchemasHandler{Store: store, Registry: handlers.NoopSchemaRegistry{}}
	body := `{"schema_avro":{"type":"record","name":"X","fields":[]},"compatibility":"WAT"}`
	req := withClaims(httptest.NewRequest("POST", "/v", strings.NewReader(body)))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid compatibility mode")
}

// ─── ListSchemaHistory ─────────────────────────────────────────────────

func TestListSchemaHistoryRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.SchemasHandler{Store: &fakeSchemaStore{}}
	req := httptest.NewRequest("GET", "/streams/x/schema/history", nil)
	rec := httptest.NewRecorder()
	h.ListSchemaHistory(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListSchemaHistoryReturns404WhenStreamMissing(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	store := &fakeSchemaStore{streams: map[uuid.UUID]struct {
		schema []byte
		mode   string
	}{}}
	h := &handlers.SchemasHandler{Store: store}
	req := withClaims(httptest.NewRequest("GET", "/h", nil))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ListSchemaHistory(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListSchemaHistoryReturnsDataArray(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	versions := []models.StreamSchemaVersion{
		{ID: uuid.New(), StreamID: id, Version: 2, Fingerprint: "sha256:b", Compatibility: "BACKWARD", CreatedBy: "test"},
		{ID: uuid.New(), StreamID: id, Version: 1, Fingerprint: "sha256:a", Compatibility: "BACKWARD", CreatedBy: "test"},
	}
	store := &fakeSchemaStore{
		streams: map[uuid.UUID]struct {
			schema []byte
			mode   string
		}{id: {schema: nil, mode: "BACKWARD"}},
		history: map[uuid.UUID][]models.StreamSchemaVersion{id: versions},
	}
	h := &handlers.SchemasHandler{Store: store}
	req := withClaims(httptest.NewRequest("GET", "/h", nil))
	req = withChiID(req, id.String())
	rec := httptest.NewRecorder()
	h.ListSchemaHistory(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var view struct {
		Data []models.StreamSchemaVersion `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.Data, 2)
	assert.Equal(t, int32(2), view.Data[0].Version)
	assert.Equal(t, int32(1), view.Data[1].Version)
}

// ─── BusControlSchemaRegistry ──────────────────────────────────────────

func TestBusControlRegistryFingerprintIsCanonicalAndStable(t *testing.T) {
	t.Parallel()
	registry := handlers.BusControlSchemaRegistry{}
	schema := []byte(`{
		"type": "record",
		"name": "Order",
		"fields": [
			{ "name": "order_id", "type": "string" },
			{ "name": "amount", "type": "long" }
		]
	}`)
	pretty := []byte("{\n  \"type\": \"record\",\n  \"name\": \"Order\",\n  \"fields\": [\n    { \"name\": \"order_id\", \"type\": \"string\" },\n    { \"name\": \"amount\", \"type\": \"long\" }\n  ]\n}")
	a, err := registry.Fingerprint(schema)
	require.NoError(t, err)
	b, err := registry.Fingerprint(pretty)
	require.NoError(t, err)
	assert.Equal(t, a, b, "fingerprint must be whitespace-insensitive")
	assert.True(t, strings.HasPrefix(a, "sha256:"))
}

func TestBusControlRegistryFingerprintRejectsBadJSON(t *testing.T) {
	t.Parallel()
	registry := handlers.BusControlSchemaRegistry{}
	_, err := registry.Fingerprint([]byte(`not json`))
	assert.Error(t, err)
}

func TestBusControlRegistryBackwardCompatibleSchemaPasses(t *testing.T) {
	t.Parallel()
	registry := handlers.BusControlSchemaRegistry{}
	v1 := []byte(`{
		"type": "record",
		"name": "Order",
		"fields": [
			{ "name": "order_id", "type": "string" },
			{ "name": "amount", "type": "long" }
		]
	}`)
	// Adds a defaulted field — non-breaking under BACKWARD.
	v2 := []byte(`{
		"type": "record",
		"name": "Order",
		"fields": [
			{ "name": "order_id", "type": "string" },
			{ "name": "amount", "type": "long" },
			{ "name": "currency", "type": "string", "default": "USD" }
		]
	}`)
	require.NoError(t, registry.CheckCompatibility(v1, v2, "BACKWARD"))
}

func TestBusControlRegistryRejectsBreakingChange(t *testing.T) {
	t.Parallel()
	registry := handlers.BusControlSchemaRegistry{}
	v1 := []byte(`{
		"type": "record",
		"name": "Order",
		"fields": [
			{ "name": "order_id", "type": "string" },
			{ "name": "amount", "type": "long" }
		]
	}`)
	// New required field with no default — breaks BACKWARD.
	v2 := []byte(`{
		"type": "record",
		"name": "Order",
		"fields": [
			{ "name": "order_id", "type": "string" },
			{ "name": "amount", "type": "long" },
			{ "name": "currency", "type": "string" }
		]
	}`)
	err := registry.CheckCompatibility(v1, v2, "BACKWARD")
	assert.Error(t, err)
}

func TestBusControlRegistryCompatRejectsBadMode(t *testing.T) {
	t.Parallel()
	registry := handlers.BusControlSchemaRegistry{}
	err := registry.CheckCompatibility([]byte(`{}`), []byte(`{}`), "WAT")
	assert.Error(t, err)
}

// ─── NoopSchemaRegistry ────────────────────────────────────────────────

func TestNoopFingerprintHashesValidJSON(t *testing.T) {
	t.Parallel()
	registry := handlers.NoopSchemaRegistry{}
	fp, err := registry.Fingerprint([]byte(`{"type":"record","name":"X","fields":[]}`))
	require.NoError(t, err)
	assert.NotEmpty(t, fp)
}

func TestNoopFingerprintRejectsBadJSON(t *testing.T) {
	t.Parallel()
	registry := handlers.NoopSchemaRegistry{}
	_, err := registry.Fingerprint([]byte(`not json`))
	assert.Error(t, err)
}

func TestNoopCheckCompatibilityNoneAlwaysSucceeds(t *testing.T) {
	t.Parallel()
	registry := handlers.NoopSchemaRegistry{}
	assert.NoError(t, registry.CheckCompatibility([]byte(`{"a":1}`), []byte(`{"b":2}`), "NONE"))
	assert.NoError(t, registry.CheckCompatibility([]byte(`{"a":1}`), []byte(`{"b":2}`), "none"))
	assert.NoError(t, registry.CheckCompatibility([]byte(`{"a":1}`), []byte(`{"b":2}`), ""))
}

func TestNoopCheckCompatibilityFlagsDriftUnderStrictModes(t *testing.T) {
	t.Parallel()
	registry := handlers.NoopSchemaRegistry{}
	err := registry.CheckCompatibility([]byte(`{"a":1}`), []byte(`{"a":2}`), "BACKWARD")
	assert.Error(t, err)
}
