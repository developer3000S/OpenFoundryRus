package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/config"
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
	// domain routes until the four consolidation merges land.
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
