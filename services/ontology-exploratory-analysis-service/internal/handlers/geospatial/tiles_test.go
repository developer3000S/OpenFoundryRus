package geospatial

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func TestGetVectorTileRejectsInvalidUUID(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	r := chi.NewRouter()
	r.Get("/tiles/{id}", state.GetVectorTile)

	req := httptest.NewRequest(http.MethodGet, "/tiles/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid layer id")
}

func TestGetViewportTileFeaturesRejectsMissingBounds(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	r := chi.NewRouter()
	r.Get("/tiles/{id}/features", state.GetViewportTileFeatures)

	req := httptest.NewRequest(http.MethodGet, "/tiles/11111111-1111-4111-8111-111111111111/features", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "min_lat is required")
}

func TestGetViewportTileFeaturesReturnsBoundedPage(t *testing.T) {
	t.Parallel()
	layerID := uuid.New()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	addLayerRowExpectation(t, mock, layerID)

	state := &AppState{DB: mock}
	r := chi.NewRouter()
	r.Get("/tiles/{id}/features", state.GetViewportTileFeatures)
	req := httptest.NewRequest(http.MethodGet, "/tiles/"+layerID.String()+"/features?min_lat=39.99&min_lon=-105.32&max_lat=40.02&max_lon=-105.28&limit=1&offset=0&zoom=12&simplify_tolerance=0.001", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var page models.ViewportTileFeaturePage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&page))
	assert.Equal(t, layerID, page.LayerID)
	assert.Equal(t, "Tile test layer", page.LayerName)
	assert.Equal(t, 2, page.TotalMatchingCount)
	assert.Equal(t, 1, page.ReturnedCount)
	require.NotNil(t, page.NextOffset)
	assert.Equal(t, 1, *page.NextOffset)
	require.Len(t, page.Features, 1)
	assert.Equal(t, "inside-line", page.Features[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRoutesIncludesTilesAndGeocode(t *testing.T) {
	t.Parallel()
	router := (&AppState{}).Routes()
	seen := map[string]bool{}
	walker := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen[method+" "+route] = true
		return nil
	}
	if err := chi.Walk(router, walker); err != nil {
		t.Fatalf("walk: %v", err)
	}
	assert.True(t, seen["GET /tiles/{id}"], "GET /tiles/{id} missing: %v", seen)
	assert.True(t, seen["GET /tiles/{id}/features"], "GET /tiles/{id}/features missing: %v", seen)
	assert.True(t, seen["POST /geocode"], "POST /geocode missing: %v", seen)
	assert.True(t, seen["POST /geocode/reverse"], "POST /geocode/reverse missing: %v", seen)
}

func addLayerRowExpectation(t *testing.T, mock pgxmock.PgxPoolIface, layerID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	style, err := json.Marshal(models.NewDefaultLayerStyle())
	require.NoError(t, err)
	features, err := json.Marshal([]models.MapFeature{
		{
			ID:    "inside-line",
			Label: "Inside line",
			Geometry: models.Geometry{Type: models.GeometryTypeLineString, LineString: []models.Coordinate{
				{Lat: 40.0000, Lon: -105.3000},
				{Lat: 40.0005, Lon: -105.2950},
				{Lat: 40.0010, Lon: -105.2900},
			}},
		},
		{
			ID:       "inside-point",
			Label:    "Inside point",
			Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &models.Coordinate{Lat: 40.01, Lon: -105.30}},
		},
		{
			ID:       "outside-point",
			Label:    "Outside point",
			Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &models.Coordinate{Lat: 41.01, Lon: -106.30}},
		},
	})
	require.NoError(t, err)
	tags, err := json.Marshal([]string{"tile"})
	require.NoError(t, err)
	rows := pgxmock.NewRows([]string{
		"id", "name", "description", "source_kind", "source_dataset", "geometry_type",
		"style", "features", "tags", "indexed", "created_at", "updated_at",
	}).AddRow(layerID, "Tile test layer", "Large layer", "dataset", "large-layer", "line_string", style, features, tags, true, now, now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description, source_kind, source_dataset, geometry_type, style, features, tags, indexed, created_at, updated_at FROM geospatial_layers WHERE id = $1")).
		WithArgs(layerID).
		WillReturnRows(rows)
}
