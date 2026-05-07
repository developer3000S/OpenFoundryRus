package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kernelStores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-exploratory-analysis-service"
	cfg.Service.Version = "test"
	return httptest.NewServer(BuildRouter(cfg, observability.NewMetrics()))
}

func TestSubstrateProbesAreMounted(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	for _, path := range []string{"/health", "/readiness", "/healthz"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			resp, err := http.Get(srv.URL + path)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Equal(t, "ontology-exploratory-analysis-service", body["service"])
			assert.Equal(t, "ok", body["status"])
		})
	}
}

func TestNoDomainHandlersMounted(t *testing.T) {
	// Wire-compat with Rust: the binary deliberately mounts no
	// domain routes until the four consolidation merges land. The
	// substrate-only constructor (BuildRouter, no handlers) keeps the
	// 404 envelope.
	t.Parallel()
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/api/v1/views",
		"/api/v1/maps",
		"/api/v1/writeback-proposals",
		"/v1/views",
	} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "path %s should not be mounted yet", path)
	}
}

func TestDomainHandlersMountedWhenWired(t *testing.T) {
	// When callers thread a *Handlers value through
	// BuildRouterWithHandlers, the saved-view / saved-map routes are
	// mounted alongside the substrate probes. Mirrors the Rust code
	// path the four consolidation merges will eventually take.
	t.Parallel()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-exploratory-analysis-service"
	cfg.Service.Version = "test"
	h := &handlers.Handlers{
		Definitions: kernelStores.NewInMemoryDefinitionStore(),
		Actions:     kernelStores.NewInMemoryActionLogStore(),
		Tenant:      storageabstraction.TenantId("tenant-a"),
		Subject:     "analyst-1",
	}
	srv := httptest.NewServer(BuildRouterWithHandlers(cfg, observability.NewMetrics(), h))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/views")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/api/v1/maps")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Substrate probe still works.
	resp, err = http.Get(srv.URL + "/health")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
