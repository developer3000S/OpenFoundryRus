package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	kernelStores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/handlers/geospatial"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
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

func TestGeospatialHandlersMountedWhenWired(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	cfg := &config.Config{}
	cfg.Service.Name = "ontology-exploratory-analysis-service"
	cfg.Service.Version = "test"
	layerID := uuid.New()
	srv := httptest.NewServer(BuildRouterWithGeospatial(cfg, observability.NewMetrics(), &geospatial.AppState{DB: mock}))
	t.Cleanup(srv.Close)

	addLayerListExpectation(t, mock, layerID)
	resp, err := http.Get(srv.URL + "/api/v1/geospatial/overview")
	require.NoError(t, err)
	var overview models.GeospatialOverview
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&overview))
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, overview.LayerCount)
	assert.Equal(t, 2, overview.TotalFeatures)

	addLayerListExpectation(t, mock, layerID)
	resp, err = http.Get(srv.URL + "/api/v1/geospatial/layers")
	require.NoError(t, err)
	var layers models.ListResponse[models.LayerDefinition]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&layers))
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, layers.Items, 1)
	assert.Equal(t, "Trailheads", layers.Items[0].Name)

	addLayerListExpectation(t, mock, layerID)
	queryBody := []byte(`{
		"layer_id":"` + layerID.String() + `",
		"operation":"within",
		"bounds":{"min_lat":39.9,"min_lon":-105.4,"max_lat":40.2,"max_lon":-105.1}
	}`)
	resp, err = http.Post(srv.URL+"/api/v1/geospatial/query", "application/json", bytes.NewReader(queryBody))
	require.NoError(t, err)
	var query models.SpatialQueryResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&query))
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, query.Summary.MatchedCount)

	addLayerListExpectation(t, mock, layerID)
	clusterBody := []byte(`{"layer_id":"` + layerID.String() + `","algorithm":"kmeans","cluster_count":1}`)
	resp, err = http.Post(srv.URL+"/api/v1/geospatial/clusters", "application/json", bytes.NewReader(clusterBody))
	require.NoError(t, err)
	var clusters models.ClusterResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&clusters))
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, clusters.Clusters, 1)

	require.NoError(t, mock.ExpectationsWereMet())
}

func addLayerListExpectation(t *testing.T, mock pgxmock.PgxPoolIface, layerID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	style, err := json.Marshal(models.NewDefaultLayerStyle())
	require.NoError(t, err)
	features, err := json.Marshal([]models.MapFeature{
		{
			ID:       "mesa",
			Label:    "Mesa Trail",
			Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &models.Coordinate{Lat: 40.01, Lon: -105.25}},
		},
		{
			ID:       "betasso",
			Label:    "Betasso Preserve",
			Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &models.Coordinate{Lat: 40.03, Lon: -105.31}},
		},
	})
	require.NoError(t, err)
	tags, err := json.Marshal([]string{"trail"})
	require.NoError(t, err)
	rows := pgxmock.NewRows([]string{
		"id", "name", "description", "source_kind", "source_dataset", "geometry_type",
		"style", "features", "tags", "indexed", "created_at", "updated_at",
	}).AddRow(layerID, "Trailheads", "Boulder trailheads", "dataset", "trailheads", "point", style, features, tags, true, now, now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description, source_kind, source_dataset, geometry_type, style, features, tags, indexed, created_at, updated_at FROM geospatial_layers ORDER BY updated_at DESC")).
		WillReturnRows(rows)
}
