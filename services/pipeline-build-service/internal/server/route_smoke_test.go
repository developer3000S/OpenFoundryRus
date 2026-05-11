package server_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/server"
)

func TestRouteSmokeMountsPipelineBuilderRoutes(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Service.Name = "pipeline-build-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = "test-secret-32bytes-test-secret-3"

	assertRoutesMounted(t, server.BuildRouter(cfg, nil), []routeSmokeCase{
		{http.MethodGet, "/api/v1/pipelines/transforms/catalog"},
		{http.MethodPost, "/api/v1/pipelines/_validate"},
		{http.MethodPost, "/api/v1/pipelines/_schema-guidance"},
		{http.MethodPost, "/api/v1/pipelines/geospatial/gpx/parse"},
		{http.MethodGet, "/api/v1/pipelines/{id}/nodes/{node_id}/preview"},
		{http.MethodPost, "/api/v1/pipelines/{id}/runs"},
	})
}

type routeSmokeCase struct {
	method string
	path   string
}

func assertRoutesMounted(t *testing.T, handler http.Handler, expected []routeSmokeCase) {
	t.Helper()
	routes, ok := handler.(chi.Routes)
	require.True(t, ok, "handler should expose chi routes")

	seen := map[routeSmokeCase]bool{}
	require.NoError(t, chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen[routeSmokeCase{method: method, path: route}] = true
		return nil
	}))

	for _, want := range expected {
		require.True(t, seen[want], "%s %s is not mounted", want.method, want.path)
	}
}
