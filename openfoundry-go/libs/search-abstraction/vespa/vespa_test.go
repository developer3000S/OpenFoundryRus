package vespa

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

func TestVespaIndexAndDeleteWireFormatAndAuth(t *testing.T) {
	var sawIndex, sawSearch, sawDelete bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Basic dXNlcjpwYXNz", r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/document/v1/of/aircraft/group/acme/obj-1":
			sawIndex = true
			assert.Equal(t, "aircraft.version < 7", r.URL.Query().Get("condition"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			var body struct {
				Fields map[string]any `json:"fields"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "obj-1", body.Fields["id"])
			assert.Equal(t, "acme", body.Fields["tenant"])
			assert.Equal(t, "Aircraft", body.Fields["type_id"])
			assert.Equal(t, float64(7), body.Fields["version"])
			assert.Equal(t, "EC-123", body.Fields["tail_number"])
			_, _ = w.Write([]byte(`{"id":"id:of:aircraft::obj-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/search/":
			sawSearch = true
			_, _ = w.Write([]byte(`{"root":{"children":[{"relevance":1.0,"fields":{"id":"obj-1","type_id":"Aircraft"}}]}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/document/v1/of/aircraft/group/acme/obj-1":
			sawDelete = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithAuthHeader("Basic dXNlcjpwYXNz"), WithHTTPClient(ts.Client()))
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
	assert.True(t, sawSearch)
	assert.True(t, sawDelete)
}

func TestVespaStaleIndexPreconditionIsSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/document/v1/of/aircraft/group/acme/obj-1", r.URL.Path)
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithHTTPClient(ts.Client()))
	err := backend.Index(context.Background(), searchabstraction.IndexDoc{
		Tenant: repos.TenantId("acme"), ID: repos.ObjectId("obj-1"), TypeID: repos.TypeId("Aircraft"), Version: 6, Payload: json.RawMessage(`{}`),
	})
	assert.NoError(t, err)
}

func TestVespaErrorMapping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	backend := NewWithOptions(ts.URL, WithHTTPClient(ts.Client()))
	err := backend.Index(context.Background(), searchabstraction.IndexDoc{
		Tenant: repos.TenantId("acme"), ID: repos.ObjectId("obj-1"), TypeID: repos.TypeId("Aircraft"), Version: 7, Payload: json.RawMessage(`{}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vespa index HTTP 503")
}
