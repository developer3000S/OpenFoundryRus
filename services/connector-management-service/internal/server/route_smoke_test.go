package server_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestRouteSmokeMountsDataConnectionRoutes(t *testing.T) {
	t.Parallel()
	srv, _, _ := testServer(t, false)

	assertRoutesMounted(t, srv.Handler, []routeSmokeCase{
		{http.MethodGet, "/api/v1/data-connection/sources"},
		{http.MethodPost, "/api/v1/data-connection/sources"},
		{http.MethodPost, "/api/v1/webhooks/{id}/invoke"},
		{http.MethodGet, "/api/v1/webhooks/{id}/history"},
		{http.MethodPost, "/api/v1/listeners/{id}/events"},
		{http.MethodGet, "/api/v1/data-connection/catalog"},
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
