package server_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/server"
)

func TestRestrictedViewCRUDRoutesAreConsolidatedInIdentityFederationService(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.Service.Name = "identity-federation-service"
	srv := server.New(cfg, nil, nil, nil, nil, nil, nil, &handlers.RBAC{}, observability.NewMetrics())
	router, ok := srv.Handler.(chi.Routes)
	require.True(t, ok)

	mounted := map[string]bool{}
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}))

	for _, route := range []string{
		"GET /api/v1/restricted-views",
		"POST /api/v1/restricted-views",
		"GET /api/v1/restricted-views/{id}",
		"PUT /api/v1/restricted-views/{id}",
		"PATCH /api/v1/restricted-views/{id}",
		"DELETE /api/v1/restricted-views/{id}",
	} {
		require.Truef(t, mounted[route], "missing route %s", route)
	}
}
