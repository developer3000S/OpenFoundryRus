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
