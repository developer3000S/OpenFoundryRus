package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/server"
)

var errReadyTest = errors.New("db unavailable")

type routerStore struct {
	owner  uuid.UUID
	source uuid.UUID
}

func newRouterStore() *routerStore {
	return &routerStore{owner: uuid.New(), source: uuid.New()}
}

func (s *routerStore) ListConnections(context.Context, *uuid.UUID) ([]models.Connection, error) {
	return []models.Connection{{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}}, nil
}

func (s *routerStore) GetConnection(context.Context, uuid.UUID) (*models.Connection, error) {
	return &models.Connection{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}

func (s *routerStore) GetConnectionForOwner(context.Context, uuid.UUID, uuid.UUID) (*models.Connection, error) {
	return &models.Connection{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}

func (s *routerStore) CreateConnection(context.Context, *models.CreateConnectionRequest, uuid.UUID) (*models.Connection, error) {
	return &models.Connection{ID: uuid.New(), Name: "created", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}

func (s *routerStore) UpdateConnection(context.Context, uuid.UUID, *models.UpdateConnectionRequest) (*models.Connection, error) {
	return &models.Connection{ID: s.source, Name: "updated", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}

func (s *routerStore) DeleteConnection(context.Context, uuid.UUID) (bool, error) { return true, nil }

func (s *routerStore) ListSyncJobs(context.Context, uuid.UUID, uuid.UUID) ([]models.SyncJob, error) {
	return []models.SyncJob{}, nil
}

func (s *routerStore) GetSyncJob(context.Context, uuid.UUID, uuid.UUID) (*models.SyncJob, error) {
	return &models.SyncJob{ID: uuid.New(), SourceID: s.source, OutputDatasetID: uuid.New()}, nil
}

func (s *routerStore) CreateSyncJob(context.Context, *models.CreateSyncJobRequest, uuid.UUID) (*models.SyncJob, error) {
	return &models.SyncJob{ID: uuid.New(), SourceID: s.source, OutputDatasetID: uuid.New()}, nil
}

func (s *routerStore) UpdateSyncJob(context.Context, uuid.UUID, *models.UpdateSyncJobRequest, uuid.UUID) (*models.SyncJob, error) {
	return &models.SyncJob{ID: uuid.New(), SourceID: s.source, OutputDatasetID: uuid.New()}, nil
}

func (s *routerStore) RunSyncJob(context.Context, uuid.UUID, uuid.UUID) (*models.SyncRun, error) {
	return &models.SyncRun{ID: uuid.New(), SyncDefID: uuid.New(), Status: "running"}, nil
}

func (s *routerStore) ListSyncRuns(context.Context, uuid.UUID, uuid.UUID) ([]models.SyncRun, error) {
	return []models.SyncRun{}, nil
}
func (s *routerStore) ListCredentials(context.Context, uuid.UUID, uuid.UUID) ([]models.CredentialResponse, error) {
	return []models.CredentialResponse{}, nil
}
func (s *routerStore) SetCredential(context.Context, uuid.UUID, uuid.UUID, string, []byte, string) (*models.CredentialResponse, error) {
	return &models.CredentialResponse{ID: uuid.New(), SourceID: s.source, Kind: "api_key", Fingerprint: "abc"}, nil
}
func (s *routerStore) ListSourcePolicies(context.Context, uuid.UUID, uuid.UUID) ([]models.SourcePolicyBindingResponse, error) {
	return []models.SourcePolicyBindingResponse{}, nil
}
func (s *routerStore) AttachPolicy(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (*models.SourcePolicyBindingResponse, error) {
	return &models.SourcePolicyBindingResponse{SourceID: s.source, PolicyID: uuid.New(), Kind: "direct"}, nil
}
func (s *routerStore) DetachPolicy(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func (s *routerStore) ListMediaSetSyncs(context.Context, uuid.UUID, uuid.UUID) ([]models.MediaSetSync, error) {
	return []models.MediaSetSync{}, nil
}

func (s *routerStore) GetMediaSetSync(context.Context, uuid.UUID, uuid.UUID) (*models.MediaSetSync, error) {
	return &models.MediaSetSync{ID: uuid.New(), SourceID: s.source, Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set.x"}, nil
}

func (s *routerStore) CreateMediaSetSync(context.Context, uuid.UUID, *models.CreateMediaSetSyncRequest, uuid.UUID) (*models.MediaSetSync, error) {
	return &models.MediaSetSync{ID: uuid.New(), SourceID: s.source, Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set.x"}, nil
}

func (s *routerStore) UpdateMediaSetSync(context.Context, uuid.UUID, *models.UpdateMediaSetSyncRequest, uuid.UUID) (*models.MediaSetSync, error) {
	return &models.MediaSetSync{ID: uuid.New(), SourceID: s.source, Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set.x"}, nil
}

func (s *routerStore) EnableVirtualTableSource(context.Context, string, *models.EnableVirtualTableSourceRequest) (*models.VirtualTableSourceLink, error) {
	return &models.VirtualTableSourceLink{SourceRID: "ri.source", Provider: "POSTGRES", VirtualTablesEnabled: true}, nil
}

func (s *routerStore) CreateVirtualTable(context.Context, string, string, *models.CreateVirtualTableRequest) (*models.VirtualTable, error) {
	return &models.VirtualTable{RID: "ri.virtual", SourceRID: "ri.source", CreatedBy: stringPtr(s.owner.String())}, nil
}

func (s *routerStore) ListVirtualTables(context.Context, string, string, string, int) ([]models.VirtualTable, error) {
	return []models.VirtualTable{}, nil
}

func (s *routerStore) GetVirtualTable(context.Context, string, string) (*models.VirtualTable, error) {
	return &models.VirtualTable{RID: "ri.virtual", SourceRID: "ri.source", CreatedBy: stringPtr(s.owner.String())}, nil
}

func (s *routerStore) ListRegistrations(context.Context, uuid.UUID) ([]models.ConnectionRegistration, error) {
	return []models.ConnectionRegistration{{ID: uuid.New(), ConnectionID: s.source, Selector: "pg", DisplayName: "pg", SourceKind: "postgres", RegistrationMode: "zero_copy", Metadata: json.RawMessage(`{"supports_zero_copy":true}`)}}, nil
}
func (s *routerStore) UpsertRegistration(context.Context, uuid.UUID, models.DiscoveredSource, string, bool, bool, *uuid.UUID, json.RawMessage) (*models.ConnectionRegistration, error) {
	return &models.ConnectionRegistration{ID: uuid.New(), ConnectionID: s.source, Selector: "pg", DisplayName: "pg", SourceKind: "postgres", RegistrationMode: "sync", Metadata: json.RawMessage(`{"supports_zero_copy":true}`)}, nil
}
func (s *routerStore) GetRegistration(context.Context, uuid.UUID, uuid.UUID) (*models.ConnectionRegistration, error) {
	return &models.ConnectionRegistration{ID: uuid.New(), ConnectionID: s.source, Selector: "pg", DisplayName: "pg", SourceKind: "postgres", RegistrationMode: "zero_copy", Metadata: json.RawMessage(`{"supports_zero_copy":true}`)}, nil
}
func (s *routerStore) DeleteRegistration(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
func (s *routerStore) UpdateConnectionConfig(context.Context, uuid.UUID, json.RawMessage) (*models.Connection, error) {
	return &models.Connection{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}
func (s *routerStore) ListIcebergNamespaces(context.Context) ([]models.Connection, error) {
	return []models.Connection{{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}}, nil
}
func (s *routerStore) GetIcebergConnection(context.Context, string) (*models.Connection, error) {
	return &models.Connection{ID: s.source, Name: "pg", ConnectorType: "postgres", OwnerID: s.owner, Config: json.RawMessage(`{}`)}, nil
}
func (s *routerStore) ListIcebergTables(context.Context, uuid.UUID) ([]models.ConnectionRegistration, error) {
	return []models.ConnectionRegistration{{ID: uuid.New(), ConnectionID: s.source, Selector: "pg", DisplayName: "pg", SourceKind: "postgres", RegistrationMode: "zero_copy", Metadata: json.RawMessage(`{"supports_zero_copy":true}`)}}, nil
}

func stringPtr(v string) *string { return &v }

func testServer(t *testing.T, devAuth bool) (*http.Server, *authmw.JWTConfig, *routerStore) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "connector-management-service"
	cfg.Service.Version = "test"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.OpenFoundryDevAuth = devAuth
	jwt := authmw.NewJWTConfig("test-secret")
	store := newRouterStore()
	return server.New(cfg, jwt, &handlers.Handlers{Repo: store}, observability.NewMetrics()), jwt, store
}

func bearer(t *testing.T, jwt *authmw.JWTConfig) string {
	t.Helper()
	now := time.Now().Unix()
	tok, err := authmw.EncodeToken(jwt, &authmw.Claims{Sub: uuid.New(), IAT: now, EXP: now + 3600, JTI: uuid.New(), Email: "test@example.com", Name: "Test", Roles: []string{"admin"}})
	require.NoError(t, err)
	return "Bearer " + tok
}

func exercise(ts http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	ts.ServeHTTP(rec, req)
	return rec
}

func TestRustRouteSurfaceIsMounted(t *testing.T) {
	srv, jwt, store := testServer(t, false)
	token := bearer(t, jwt)
	id := store.source.String()
	syncID := uuid.NewString()
	regID := uuid.NewString()

	routes := []struct {
		method string
		path   string
		token  string
		body   string
	}{
		{http.MethodGet, "/api/v1/data-connection/catalog", "", ""},
		{http.MethodGet, "/api/v1/data-connection/catalog/contracts", "", ""},
		{http.MethodGet, "/api/v1/data-connection/streaming-sources", "", ""},
		{http.MethodGet, "/api/v1/data-connection/sources", token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources", token, `{"name":"x","connector_type":"postgres"}`},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id, token, ""},
		{http.MethodDelete, "/api/v1/data-connection/sources/" + id, token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/test-connection", token, "{}"},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/capabilities", "", ""},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/credentials", token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/credentials", token, "{}"},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/egress-policies", token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/egress-policies", token, "{}"},
		{http.MethodDelete, "/api/v1/data-connection/sources/" + id + "/egress-policies/" + uuid.NewString(), token, ""},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/syncs", token, ""},
		{http.MethodPost, "/api/v1/data-connection/syncs", token, `{"source_id":"` + id + `","output_dataset_id":"` + uuid.NewString() + `"}`},
		{http.MethodPost, "/api/v1/data-connection/syncs/" + syncID + "/run", token, "{}"},
		{http.MethodGet, "/api/v1/data-connection/syncs/" + syncID + "/runs", token, ""},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/media-set-syncs", token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/media-set-syncs", token, `{"kind":"MEDIA_SET_SYNC","target_media_set_rid":"ri.foundry.main.media_set.x"}`},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/registrations", token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/discover", token, "{}"},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/bulk", token, "{}"},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/bulk/preview", token, "{}"},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/auto", token, "{}"},
		{http.MethodGet, "/api/v1/data-connection/sources/" + id + "/registrations/auto/status", token, ""},
		{http.MethodDelete, "/api/v1/data-connection/sources/" + id + "/registrations/" + regID, token, ""},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/" + regID + "/query", token, "{}"},
		{http.MethodPost, "/api/v1/data-connection/sources/" + id + "/registrations/" + regID + "/query/arrow", token, "{}"},
		{http.MethodGet, "/api/v1/connections", token, ""},
		{http.MethodPost, "/api/v1/connections", token, `{"name":"x","connector_type":"postgres"}`},
		{http.MethodGet, "/api/v1/connections/" + id, token, ""},
		{http.MethodDelete, "/api/v1/connections/" + id, token, ""},
		{http.MethodPost, "/api/v1/connections/" + id + "/test", token, "{}"},
		{http.MethodPost, "/api/v1/webhooks/" + uuid.NewString() + "/invoke", token, "{}"},
		{http.MethodGet, "/iceberg/v1/config", token, ""},
		{http.MethodGet, "/iceberg/v1/namespaces", token, ""},
		{http.MethodGet, "/iceberg/v1/namespaces/default", token, ""},
		{http.MethodGet, "/iceberg/v1/namespaces/default/tables", token, ""},
		{http.MethodGet, "/iceberg/v1/namespaces/default/tables/table", token, ""},
		{http.MethodGet, "/health", "", ""},
		{http.MethodGet, "/healthz", "", ""},
		{http.MethodGet, "/readyz", "", ""},
		{http.MethodGet, "/metrics", "", ""},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := exercise(srv.Handler, route.method, route.path, route.token, route.body)
			require.NotEqual(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.NotEqual(t, http.StatusMethodNotAllowed, rec.Code, rec.Body.String())
		})
	}
}

func TestOptionalAuthKeepsCatalogOpenButHandlersCanRequireClaims(t *testing.T) {
	srv, _, store := testServer(t, false)
	id := store.source.String()

	rec := exercise(srv.Handler, http.MethodGet, "/api/v1/data-connection/catalog", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "postgresql")

	rec = exercise(srv.Handler, http.MethodGet, "/api/v1/data-connection/sources", "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = exercise(srv.Handler, http.MethodPost, "/api/v1/data-connection/sources/"+id+"/credentials", "", "{}")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestReadyzUsesReadyChecks(t *testing.T) {
	cfg := &config.Config{}
	cfg.Service.Name = "connector-management-service"
	cfg.Service.Version = "test"
	cfg.Server.Host = "127.0.0.1"
	jwt := authmw.NewJWTConfig("test-secret")
	failing := func(context.Context) error { return errReadyTest }
	srv := server.New(cfg, jwt, &handlers.Handlers{Repo: newRouterStore()}, observability.NewMetrics(), failing)

	rec := exercise(srv.Handler, http.MethodGet, "/readyz", "", "")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "not_ready")
}

func TestDevAuthRoutesMountOnlyWhenEnabled(t *testing.T) {
	srv, _, _ := testServer(t, false)
	rec := exercise(srv.Handler, http.MethodPost, "/api/v1/auth/login", "", "{}")
	require.Equal(t, http.StatusNotFound, rec.Code)

	srv, _, _ = testServer(t, true)
	rec = exercise(srv.Handler, http.MethodPost, "/api/v1/auth/login", "", "{}")
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "dev_auth_pending")
}
