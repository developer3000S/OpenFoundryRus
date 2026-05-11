package server

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestRouteSmokeMountsWorkshopAppBuilderRoutes(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, _ := testWorkshopRouter(mock)
	assertRoutesMounted(t, router, []routeSmokeCase{
		{http.MethodGet, "/api/v1/widgets/catalog"},
		{http.MethodGet, "/api/v1/apps/templates"},
		{http.MethodPost, "/api/v1/apps/from-template"},
		{http.MethodGet, "/api/v1/apps/{id}/preview"},
		{http.MethodPost, "/api/v1/apps/{id}/pages"},
		{http.MethodGet, "/api/v1/apps/public/{slug}"},
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
