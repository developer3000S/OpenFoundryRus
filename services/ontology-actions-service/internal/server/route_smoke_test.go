package server_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/server"
)

func TestRouteSmokeMountsOntologyActionRoutes(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = testJWTSecret
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}

	assertRoutesMounted(t, server.BuildRouter(cfg, state, nil), []routeSmokeCase{
		{http.MethodGet, "/api/v1/ontology/actions"},
		{http.MethodPost, "/api/v1/ontology/actions/{id}/validate"},
		{http.MethodPost, "/api/v1/ontology/actions/{id}/execute"},
		{http.MethodPost, "/api/v1/ontology/actions/{id}/execute-batch"},
		{http.MethodGet, "/api/v1/ontology/functions"},
		{http.MethodGet, "/api/v1/ontology/funnel/sources"},
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
