package spatial

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func TestVectorTileShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	layer := models.LayerDefinition{
		ID:       id,
		Name:     "Roads",
		Features: []models.MapFeature{makeFeature("a", 1.0, 1.0), makeFeature("b", 1.0, 1.0)},
	}
	got := VectorTile(layer)
	assert.Equal(t, id, got.LayerID)
	assert.Equal(t, "Roads", got.LayerName)
	assert.Equal(t, "mvt", got.Format)
	assert.Equal(t, [2]uint8{4, 14}, got.ZoomRange)
	assert.Equal(t, 2, got.FeatureCount)
	require.Len(t, got.H3Bins, 1)
	assert.Equal(t, "1:1", got.H3Bins[0].CellID)
	expected := "/api/v1/geospatial/tiles/" + id.String() + "?z={z}&x={x}&y={y}"
	assert.Equal(t, expected, got.TileURLTemplate)
}

func TestVectorTileWireShape(t *testing.T) {
	t.Parallel()
	layer := models.LayerDefinition{
		ID:       uuid.New(),
		Name:     "L",
		Features: []models.MapFeature{makeFeature("a", 1.0, 1.0)},
	}
	raw, err := json.Marshal(VectorTile(layer))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Contains(t, got, "layer_id")
	assert.Contains(t, got, "layer_name")
	assert.Contains(t, got, "tile_url_template")
	assert.Equal(t, "mvt", got["format"])
	assert.Contains(t, got, "zoom_range")
	assert.Contains(t, got, "h3_bins")
	assert.Equal(t, float64(1), got["feature_count"])
	zoom, ok := got["zoom_range"].([]any)
	require.True(t, ok)
	require.Len(t, zoom, 2)
	assert.Equal(t, float64(4), zoom[0])
	assert.Equal(t, float64(14), zoom[1])
}

func TestViewportTileFeaturesFiltersPaginatesAndSimplifies(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	layer := models.LayerDefinition{
		ID:   id,
		Name: "Large trails",
		Features: []models.MapFeature{
			longLineFeature("trail-1", []models.Coordinate{
				{Lat: 40.0000, Lon: -105.3000},
				{Lat: 40.0005, Lon: -105.2950},
				{Lat: 40.0010, Lon: -105.2900},
				{Lat: 40.0015, Lon: -105.2850},
			}),
			longLineFeature("trail-2", []models.Coordinate{
				{Lat: 40.0100, Lon: -105.3100},
				{Lat: 40.0105, Lon: -105.3050},
				{Lat: 40.0110, Lon: -105.3000},
			}),
			longLineFeature("outside", []models.Coordinate{
				{Lat: 41.0000, Lon: -106.0000},
				{Lat: 41.1000, Lon: -106.1000},
			}),
		},
	}

	page := ViewportTileFeatures(layer, ViewportTileOptions{
		Bounds:            models.Bounds{MinLat: 39.99, MinLon: -105.32, MaxLat: 40.02, MaxLon: -105.28},
		Zoom:              11.5,
		Limit:             1,
		Offset:            0,
		SimplifyTolerance: 0.0007,
	})

	assert.Equal(t, id, page.LayerID)
	assert.Equal(t, "Large trails", page.LayerName)
	assert.Equal(t, 11.5, page.Zoom)
	assert.Equal(t, 1, page.Limit)
	assert.Equal(t, 0, page.Offset)
	assert.Equal(t, 2, page.TotalMatchingCount)
	assert.Equal(t, 1, page.ReturnedCount)
	require.NotNil(t, page.NextOffset)
	assert.Equal(t, 1, *page.NextOffset)
	require.Len(t, page.Features, 1)
	assert.Equal(t, "trail-1", page.Features[0].ID)
	assert.Less(t, len(page.Features[0].Geometry.LineString), 4)

	second := ViewportTileFeatures(layer, ViewportTileOptions{
		Bounds: models.Bounds{MinLat: 39.99, MinLon: -105.32, MaxLat: 40.02, MaxLon: -105.28},
		Limit:  1,
		Offset: 1,
	})
	require.Len(t, second.Features, 1)
	assert.Equal(t, "trail-2", second.Features[0].ID)
	assert.Nil(t, second.NextOffset)
}

func TestViewportTileFeaturesClampsLimitAndOffset(t *testing.T) {
	t.Parallel()
	layer := models.LayerDefinition{
		ID:       uuid.New(),
		Name:     "Points",
		Features: []models.MapFeature{makeFeature("a", 1, 1), makeFeature("b", 1.1, 1.1)},
	}

	page := ViewportTileFeatures(layer, ViewportTileOptions{
		Bounds: models.Bounds{MinLat: 0, MinLon: 0, MaxLat: 2, MaxLon: 2},
		Limit:  100_000,
		Offset: -20,
	})

	assert.Equal(t, MaxViewportTileLimit, page.Limit)
	assert.Equal(t, 0, page.Offset)
	assert.Equal(t, 2, page.ReturnedCount)
}

func TestViewportTileFeaturesLargeLayerReturnsOnlyRequestedPage(t *testing.T) {
	t.Parallel()
	features := make([]models.MapFeature, 10_000)
	for i := range features {
		features[i] = makeFeature("trail", 40+float64(i%1000)*0.00001, -105+float64(i%1000)*0.00001)
	}
	layer := models.LayerDefinition{
		ID:       uuid.New(),
		Name:     "Large points",
		Features: features,
	}

	page := ViewportTileFeatures(layer, ViewportTileOptions{
		Bounds: models.Bounds{MinLat: 39.99, MinLon: -105.01, MaxLat: 40.02, MaxLon: -104.98},
		Limit:  25,
		Offset: 50,
	})

	assert.Equal(t, 10_000, page.TotalMatchingCount)
	assert.Equal(t, 25, page.ReturnedCount)
	assert.Equal(t, 25, len(page.Features))
	require.NotNil(t, page.NextOffset)
	assert.Equal(t, 75, *page.NextOffset)
}

func TestSimplifyGeometryPreservesClosedPolygonRing(t *testing.T) {
	t.Parallel()
	polygon := models.Geometry{Type: models.GeometryTypePolygon, Polygon: []models.Coordinate{
		{Lat: 0, Lon: 0},
		{Lat: 0.001, Lon: 0.2},
		{Lat: 0, Lon: 1},
		{Lat: 1, Lon: 1},
		{Lat: 1, Lon: 0},
		{Lat: 0, Lon: 0},
	}}

	got := SimplifyGeometry(polygon, 0.01)

	require.Equal(t, models.GeometryTypePolygon, got.Type)
	require.GreaterOrEqual(t, len(got.Polygon), 4)
	assert.Equal(t, got.Polygon[0], got.Polygon[len(got.Polygon)-1])
	assert.Less(t, len(got.Polygon), len(polygon.Polygon))
}

func longLineFeature(id string, points []models.Coordinate) models.MapFeature {
	return models.MapFeature{
		ID:       id,
		Label:    id,
		Geometry: models.Geometry{Type: models.GeometryTypeLineString, LineString: points},
	}
}
