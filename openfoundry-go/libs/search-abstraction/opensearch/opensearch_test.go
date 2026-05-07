package opensearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	searchabstraction "github.com/openfoundry/openfoundry-go/libs/search-abstraction"
	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestOpenSearchIndexAndDeleteWireFormatAndAuth(t *testing.T) {
	var sawIndex, sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/of-acme-aircraft/_doc/obj-1":
			sawIndex = true
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "7", r.URL.Query().Get("version"))
			assert.Equal(t, "external", r.URL.Query().Get("version_type"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "obj-1", body["id"])
			assert.Equal(t, "acme", body["tenant"])
			assert.Equal(t, "Aircraft", body["type_id"])
			assert.Equal(t, float64(7), body["version"])
			assert.Equal(t, "EC-123", body["tail_number"])
			_, _ = w.Write([]byte(`{"result":"created"}`))
		case "/of-acme-*/_delete_by_query":
			sawDelete = true
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Contains(t, body, "query")
			_, _ = w.Write([]byte(`{"deleted":1}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithAuthHeader("Bearer test-token"), WithHTTPClient(ts.Client()))
	require.NoError(t, backend.Index(context.Background(), searchabstraction.IndexDoc{
		Tenant:  repos.TenantId("acme"),
		ID:      repos.ObjectId("obj-1"),
		TypeID:  repos.TypeId("Aircraft"),
		Version: 7,
		Payload: json.RawMessage(`{"tail_number":"EC-123"}`),
	}))
	deleted, err := backend.Delete(context.Background(), repos.TenantId("acme"), repos.ObjectId("obj-1"))
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.True(t, sawIndex)
	assert.True(t, sawDelete)
}

func TestOpenSearchStaleIndexConflictIsSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/of-acme-aircraft/_doc/obj-1", r.URL.Path)
		w.WriteHeader(http.StatusConflict)
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithHTTPClient(ts.Client()))
	err := backend.Index(context.Background(), searchabstraction.IndexDoc{
		Tenant: repos.TenantId("acme"), ID: repos.ObjectId("obj-1"), TypeID: repos.TypeId("Aircraft"), Version: 6, Payload: json.RawMessage(`{}`),
	})
	assert.NoError(t, err)
}

func TestOpenSearchErrorMapping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithHTTPClient(ts.Client()))
	err := backend.Index(context.Background(), searchabstraction.IndexDoc{
		Tenant: repos.TenantId("acme"), ID: repos.ObjectId("obj-1"), TypeID: repos.TypeId("Aircraft"), Version: 7, Payload: json.RawMessage(`{}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opensearch index HTTP 503")
}
