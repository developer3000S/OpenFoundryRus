package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/server"
)

func TestRustParityRoutesRequireAuth(t *testing.T) {
	srv := newTestServer(t)

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/datasets"},
		{http.MethodPost, "/api/v1/datasets"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/api/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodDelete, "/api/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/quality"},
		{http.MethodPost, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/quality/profile"},
		{http.MethodPost, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/quality/rules"},
		{http.MethodPatch, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/quality/rules/rule-1"},
		{http.MethodDelete, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/quality/rules/rule-1"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/lint"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/branches"},
		{http.MethodPost, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/branches"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/branches/master"},
		{http.MethodDelete, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/branches/master"},
		{http.MethodGet, "/api/v1/datasets/00000000-0000-0000-0000-000000000001/branches/master/transactions"},
		{http.MethodGet, "/api/v1/datasets/ri.foundry.main.dataset.example/health"},
		{http.MethodGet, "/api/v2/datasets"},
		{http.MethodPost, "/api/v2/datasets"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example"},
		{http.MethodPatch, "/api/v2/datasets/ri.foundry.main.dataset.example"},
		{http.MethodDelete, "/api/v2/datasets/ri.foundry.main.dataset.example"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example:restore"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/branches"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/branches"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master"},
		{http.MethodDelete, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master/transactions"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master/transactions"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:commit"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:abort"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/transactions"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/transactions:batchGet"},
		{http.MethodPost, "/api/v2/datasets/getSchemaBatch"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/readTable"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/preview"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/schema"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/getSchema"},
		{http.MethodPut, "/api/v2/datasets/ri.foundry.main.dataset.example/schema"},
		{http.MethodPut, "/api/v2/datasets/ri.foundry.main.dataset.example/putSchema"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/schema:infer"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/schema:validate"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/schema/history"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/files"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/files/metadata"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/files/content"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/files/00000000-0000-0000-0000-000000000003"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/files/00000000-0000-0000-0000-000000000003/download"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/transactions/00000000-0000-0000-0000-000000000002/files"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/transactions/00000000-0000-0000-0000-000000000002/files/content"},
		{http.MethodDelete, "/api/v2/datasets/ri.foundry.main.dataset.example/transactions/00000000-0000-0000-0000-000000000002/files"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/compare"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/views"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/current"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/at"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1/files"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1/schema"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1/schema"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1/data"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1/preview"},
		{http.MethodGet, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1"},
		{http.MethodPost, "/api/v2/datasets/ri.foundry.main.dataset.example/views/view-1:refresh"},
		{http.MethodGet, "/internal/datasets/ri.foundry.main.dataset.example/metadata"},
		{http.MethodGet, "/v1/datasets"},
		{http.MethodPost, "/v1/datasets"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodPatch, "/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodDelete, "/v1/datasets/00000000-0000-0000-0000-000000000001"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/model"},
		{http.MethodPatch, "/v1/datasets/ri.foundry.main.dataset.example/metadata"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/markings"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/markings"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/permissions"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/permissions"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/lineage-links"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/lineage-links"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/files/index"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/files/index"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001/versions"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001/branches"},
		{http.MethodPost, "/v1/datasets/00000000-0000-0000-0000-000000000001/branches"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/compare"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001/branches/master"},
		{http.MethodDelete, "/v1/datasets/ri.foundry.main.dataset.example/branches/master"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/checkout"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/ancestry"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/preview-delete"},
		{http.MethodPatch, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/retention"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/markings"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master:restore"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/rollback"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/fallbacks"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/fallbacks"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:commit"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:abort"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/transactions"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/transactions:batchGet"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/compare"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/views"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/current"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/at"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1/files"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1/schema"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1/schema"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1/data"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1/preview"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/views/view-1"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/views/:refresh"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001/files"},
		{http.MethodGet, "/v1/datasets/00000000-0000-0000-0000-000000000001/files/00000000-0000-0000-0000-000000000003/download"},
		{http.MethodPost, "/v1/datasets/00000000-0000-0000-0000-000000000001/transactions/00000000-0000-0000-0000-000000000002/files"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/storage-details"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/upload"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/preview"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/readTable"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/schema"},
		{http.MethodPut, "/v1/datasets/ri.foundry.main.dataset.example/schema"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/schema:infer"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/schema/history"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/schema:validate"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/health"},
	}

	for _, tc := range protected {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			srv.Handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestDatasetAPIV2CompatibilityScopes(t *testing.T) {
	jwt := authmw.NewJWTConfig("dataset-versioning-router-test-secret")
	srv := newTestServerWithJWT(t, jwt)

	for _, tc := range []struct {
		name          string
		claims        *authmw.Claims
		method        string
		path          string
		requiredScope string
	}{
		{
			name:          "read requires dataset read scope",
			claims:        testClaims(),
			method:        http.MethodGet,
			path:          "/api/v2/datasets",
			requiredScope: "datasets:read",
		},
		{
			name:          "write requires dataset write scope",
			claims:        testClaimsWithPermissions("datasets:read"),
			method:        http.MethodPost,
			path:          "/api/v2/datasets",
			requiredScope: "datasets:write",
		},
		{
			name: "local token method scope is enforced",
			claims: testClaimsWithSessionScope(&authmw.SessionScope{
				AllowedMethods:      []string{http.MethodGet},
				AllowedPathPrefixes: []string{"/api/v2/datasets"},
			}, "datasets:write"),
			method:        http.MethodPost,
			path:          "/api/v2/datasets",
			requiredScope: "datasets:write",
		},
		{
			name: "local token path scope is enforced",
			claims: testClaimsWithSessionScope(&authmw.SessionScope{
				AllowedMethods:      []string{http.MethodGet},
				AllowedPathPrefixes: []string{"/api/v2/datasets/allowed"},
			}, "datasets:read"),
			method:        http.MethodGet,
			path:          "/api/v2/datasets",
			requiredScope: "datasets:read",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+tokenForClaims(t, jwt, tc.claims))

			srv.Handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "PERMISSION_DENIED", body["code"])
			require.Equal(t, tc.requiredScope, body["required_scope"])
		})
	}
}

func TestRustParityPublicRoutesStayPublic(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/healthz", "/health", "/metrics"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, path)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/_internal/local-fs/some/path.parquet?expires=1&signature=todo", nil)
	srv.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestNoTransactionRoutesRemainPlaceholder501AfterAuth(t *testing.T) {
	jwt := authmw.NewJWTConfig("dataset-versioning-router-test-secret")
	srv := newTestServerWithJWT(t, jwt)
	tok := tokenFor(t, jwt)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:commit"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/branches/master/transactions/00000000-0000-0000-0000-000000000002:abort"},
		{http.MethodGet, "/v1/datasets/ri.foundry.main.dataset.example/transactions"},
		{http.MethodPost, "/v1/datasets/ri.foundry.main.dataset.example/transactions:batchGet"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		srv.Handler.ServeHTTP(rec, req)
		require.NotEqual(t, http.StatusNotImplemented, rec.Code, tc.path)
	}
}

func newTestServer(t *testing.T) *http.Server {
	t.Helper()
	return newTestServerWithJWT(t, authmw.NewJWTConfig("dataset-versioning-router-test-secret"))
}

func newTestServerWithJWT(t *testing.T, jwt *authmw.JWTConfig) *http.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "dataset-versioning-service"
	cfg.Service.Version = "test"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	return server.New(cfg, jwt, &handlers.Handlers{}, observability.NewMetrics())
}

func tokenFor(t *testing.T, cfg *authmw.JWTConfig) string {
	t.Helper()
	return tokenForClaims(t, cfg, testAdminClaims())
}

func tokenForClaims(t *testing.T, cfg *authmw.JWTConfig, claims *authmw.Claims) string {
	t.Helper()
	tok, err := authmw.EncodeToken(cfg, claims)
	require.NoError(t, err)
	return tok
}

func testAdminClaims() *authmw.Claims {
	now := time.Now()
	return &authmw.Claims{
		Sub:   uuid.New(),
		IAT:   now.Unix(),
		EXP:   now.Add(time.Hour).Unix(),
		JTI:   uuid.New(),
		Email: "route-test@example.com",
		Name:  "Route Test",
		Roles: []string{"admin"},
	}
}

func testClaimsWithPermissions(permissions ...string) *authmw.Claims {
	claims := testClaims()
	claims.Permissions = permissions
	return claims
}

func testClaimsWithSessionScope(scope *authmw.SessionScope, permissions ...string) *authmw.Claims {
	claims := testClaimsWithPermissions(permissions...)
	claims.SessionScope = scope
	return claims
}

func testClaims() *authmw.Claims {
	now := time.Now()
	return &authmw.Claims{
		Sub:         uuid.New(),
		IAT:         now.Unix(),
		EXP:         now.Add(time.Hour).Unix(),
		JTI:         uuid.New(),
		Email:       "route-test@example.com",
		Name:        "Route Test",
		Roles:       []string{},
		Permissions: []string{},
	}
}
