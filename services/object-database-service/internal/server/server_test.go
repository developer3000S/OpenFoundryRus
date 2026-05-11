package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/storage"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "object-database-service"
	cfg.Service.Version = "test"
	h := &handlers.Handlers{
		Objects: storage.NewInMemoryObjectStore(),
		Links:   storage.NewInMemoryLinkStore(),
		Backend: config.BackendInMemory,
	}
	return httptest.NewServer(BuildRouter(cfg, h, observability.NewMetrics()))
}

func TestStatusReportsBackend(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["ready"])
	assert.Equal(t, "in_memory", body["backend"], "wire token must match Rust BackendMode::InMemory")
	assert.Equal(t, "object-database-service", body["service"])
}

func TestHealthAndHealthzCoexist(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	// Plain text /health (Rust legacy probe)
	resp, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))

	// JSON /healthz (openfoundry-go convention)
	resp2, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp2.Body.Close()
	var hz map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&hz))
	assert.Equal(t, "object-database-service", hz["service"])
}

func TestPutGetDeleteRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	body := `{
		"type_id": "aircraft",
		"version": 0,
		"payload": {"tail":"N123OF"},
		"owner": "owner-1",
		"markings": ["public"],
		"created_at_ms": 1,
		"updated_at_ms": 2
	}`
	req, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPut, srv.URL+"/api/v1/object-database/objects/tenant-a/object-1",
		strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var put map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&put))
	assert.Equal(t, "inserted", put["outcome"])

	// GET
	resp2, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/object-1")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&got))
	assert.Equal(t, "tenant-a", got["tenant"])
	assert.Equal(t, "object-1", got["id"])
	assert.Equal(t, "aircraft", got["type_id"])
	assert.Equal(t, float64(1), got["version"])

	// Version conflict
	conflict := `{"type_id":"aircraft","version":0,"payload":{},"expected_version":99,"markings":[]}`
	req2, _ := http.NewRequest(http.MethodPut,
		srv.URL+"/api/v1/object-database/objects/tenant-a/object-1",
		strings.NewReader(conflict))
	resp3, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp3.Body.Close()
	var conflictBody map[string]any
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&conflictBody))
	assert.Equal(t, "version_conflict", conflictBody["outcome"])
	assert.Equal(t, float64(99), conflictBody["expected_version"])
	assert.Equal(t, float64(1), conflictBody["actual_version"])

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/api/v1/object-database/objects/tenant-a/object-1", nil)
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	defer delResp.Body.Close()
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// 404 after delete
	resp4, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/object-1")
	require.NoError(t, err)
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp4.StatusCode)
}

func TestListByOwnerAndMarkingAndLinks(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	// seed two objects
	put := func(id, owner, marking string) {
		body := `{"type_id":"aircraft","version":0,"payload":{},"owner":"` + owner +
			`","markings":["` + marking + `"],"updated_at_ms":1}`
		req, _ := http.NewRequest(http.MethodPut,
			srv.URL+"/api/v1/object-database/objects/tenant-a/"+id, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	put("obj-1", "owner-1", "public")
	put("obj-2", "owner-2", "secret")

	resp, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/by-owner/owner-1?size=10")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var byOwner map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&byOwner))
	items, _ := byOwner["items"].([]any)
	assert.Len(t, items, 1)

	resp2, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/by-marking/secret?size=10")
	require.NoError(t, err)
	defer resp2.Body.Close()
	var byMark map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&byMark))
	items2, _ := byMark["items"].([]any)
	assert.Len(t, items2, 1)

	linkBody := `{"from":"obj-1","to":"obj-2","payload":{"kind":"primary"},"created_at_ms":3}`
	linkReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/object-database/links/tenant-a/related_to", strings.NewReader(linkBody))
	linkResp, err := http.DefaultClient.Do(linkReq)
	require.NoError(t, err)
	defer linkResp.Body.Close()
	assert.Equal(t, http.StatusCreated, linkResp.StatusCode)

	outgoing, err := http.Get(srv.URL + "/api/v1/object-database/links/tenant-a/related_to/outgoing/obj-1?size=10")
	require.NoError(t, err)
	defer outgoing.Body.Close()
	assert.Equal(t, http.StatusOK, outgoing.StatusCode)
	var links map[string]any
	require.NoError(t, json.NewDecoder(outgoing.Body).Decode(&links))
	linkItems, _ := links["items"].([]any)
	assert.Len(t, linkItems, 1)
	assert.Equal(t, "obj-2", linkItems[0].(map[string]any)["to"])
}
