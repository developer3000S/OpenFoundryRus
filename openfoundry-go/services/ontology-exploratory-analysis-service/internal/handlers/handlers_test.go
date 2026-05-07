package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kernelStores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers"
)

// newTestHandlers mirrors the Rust `state()` helper in handlers.rs:
// it wires an InMemoryDefinitionStore + InMemoryActionLogStore against
// a fixed tenant/subject. Callers that don't need writeback pass nil
// to skip wiring it.
func newTestHandlers(t *testing.T) (*handlers.Handlers, *kernelStores.InMemoryDefinitionStore, *kernelStores.InMemoryActionLogStore) {
	t.Helper()
	defs := kernelStores.NewInMemoryDefinitionStore()
	actions := kernelStores.NewInMemoryActionLogStore()
	h := &handlers.Handlers{
		Definitions: defs,
		Actions:     actions,
		Tenant:      storageabstraction.TenantId("tenant-a"),
		Subject:     "analyst-1",
	}
	return h, defs, actions
}

func newRouter(h *handlers.Handlers) chi.Router {
	r := chi.NewRouter()
	h.MountViews(r)
	h.MountMaps(r)
	return r
}

func doJSON(t *testing.T, r http.Handler, method, path string, body any) *http.Response {
	t.Helper()
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		buf = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Result()
}

// TestViewAndMapHandlersUseDefinitionStore is the Go port of Rust
// `view_and_map_handlers_use_definition_store` in handlers.rs.
func TestViewAndMapHandlersUseDefinitionStore(t *testing.T) {
	t.Parallel()
	h, defs, _ := newTestHandlers(t)
	r := newRouter(h)

	// Create view.
	resp := doJSON(t, r, http.MethodPost, "/api/v1/views", map[string]any{
		"slug":        "fleet",
		"name":        "Fleet",
		"object_type": "aircraft",
		"filter_spec": map[string]any{"where": "active"},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	viewIDStr, ok := created["id"].(string)
	require.True(t, ok, "id must be a string")
	viewID, err := uuid.Parse(viewIDStr)
	require.NoError(t, err)

	stored, err := defs.Get(
		t.Context(),
		storageabstraction.DefinitionKind("exploratory_view"),
		storageabstraction.DefinitionId(viewID.String()),
		storageabstraction.Strong(),
	)
	require.NoError(t, err)
	require.NotNil(t, stored, "definition should be stored")

	// Duplicate slug → 409.
	resp = doJSON(t, r, http.MethodPost, "/api/v1/views", map[string]any{
		"slug":        "fleet",
		"name":        "Fleet duplicate",
		"object_type": "aircraft",
		"filter_spec": map[string]any{},
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	// Get by id.
	resp = doJSON(t, r, http.MethodGet, "/api/v1/views/"+viewID.String(), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Create map referencing the view.
	resp = doJSON(t, r, http.MethodPost, "/api/v1/maps", map[string]any{
		"view_id":  viewID,
		"name":     "Map",
		"map_kind": "geo",
		"config":   map[string]any{"projection": "mercator"},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// List maps returns the freshly-created entry.
	resp = doJSON(t, r, http.MethodGet, "/api/v1/maps", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var maps []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&maps))
	resp.Body.Close()
	require.Len(t, maps, 1)
	assert.Equal(t, viewID.String(), maps[0]["view_id"])
}

// TestCreateViewRejectsBlankSlug guards the Rust
// `if body.slug.trim().is_empty()` check.
func TestCreateViewRejectsBlankSlug(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandlers(t)
	r := newRouter(h)

	resp := doJSON(t, r, http.MethodPost, "/api/v1/views", map[string]any{
		"slug":        "   ",
		"name":        "x",
		"object_type": "aircraft",
		"filter_spec": map[string]any{},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "slug is required", string(body))
}

// TestGetViewMissingReturns404 guards the Rust `Ok(None)` branch.
func TestGetViewMissingReturns404(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandlers(t)
	r := newRouter(h)

	id, _ := uuid.NewV7()
	resp := doJSON(t, r, http.MethodGet, "/api/v1/views/"+id.String(), nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "view not found", string(body))
}

// TestCreateMapWithUnknownViewReturns404 guards the Rust
// `Ok(None) => StatusCode::NOT_FOUND` branch in create_map.
func TestCreateMapWithUnknownViewReturns404(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandlers(t)
	r := newRouter(h)

	id, _ := uuid.NewV7()
	resp := doJSON(t, r, http.MethodPost, "/api/v1/maps", map[string]any{
		"view_id":  id,
		"name":     "Map",
		"map_kind": "geo",
		"config":   map[string]any{},
	})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "view not found", string(body))
}

// TestCreateMapWithoutViewIDPersists exercises the Option<view_id> =
// None code path — Rust passes the create through without a parent
// pointer, the response keeps view_id null, and a list call returns
// the row.
func TestCreateMapWithoutViewIDPersists(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandlers(t)
	r := newRouter(h)

	resp := doJSON(t, r, http.MethodPost, "/api/v1/maps", map[string]any{
		"name":     "Standalone",
		"map_kind": "heatmap",
		"config":   map[string]any{"radius": 5},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	resp.Body.Close()
	assert.Nil(t, got["view_id"], "view_id should be null when absent")

	resp = doJSON(t, r, http.MethodGet, "/api/v1/maps", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listed []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	resp.Body.Close()
	require.Len(t, listed, 1)
}

// TestListViewsEmpty exercises the empty-list path: handler MUST
// return `[]` not `null`.
func TestListViewsEmpty(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestHandlers(t)
	r := newRouter(h)

	resp := doJSON(t, r, http.MethodGet, "/api/v1/views", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "[]\n", string(body))
}
