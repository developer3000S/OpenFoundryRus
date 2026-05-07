package spatial

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func makeFeature(id string, lat, lon float64) models.MapFeature {
	pt := models.Coordinate{Lat: lat, Lon: lon}
	return models.MapFeature{
		ID:       id,
		Label:    id,
		Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &pt},
	}
}

func TestHexAggregateEmptyLayer(t *testing.T) {
	t.Parallel()
	out := HexAggregate(models.LayerDefinition{ID: uuid.New()})
	assert.Empty(t, out)
}

func TestHexAggregateBucketsOnTenth(t *testing.T) {
	t.Parallel()
	// Rust rounds centroid coords to 0.1 — these three points all
	// collapse onto the same cell `1.2:3.4` and the fourth lands on
	// `5:5` (1-bucket cell with intensity = 1/3).
	layer := models.LayerDefinition{
		Features: []models.MapFeature{
			makeFeature("a", 1.18, 3.41),
			makeFeature("b", 1.22, 3.36),
			makeFeature("c", 1.16, 3.42),
			makeFeature("d", 5.00, 5.00),
		},
	}
	out := HexAggregate(layer)
	require.Len(t, out, 2)
	// Sort: count desc, then cell_id asc.
	assert.Equal(t, 3, out[0].Count)
	assert.Equal(t, "1.2:3.4", out[0].CellID)
	assert.Equal(t, 1.0, out[0].Intensity)
	assert.Equal(t, 1, out[1].Count)
	assert.Equal(t, "5:5", out[1].CellID)
	assert.InDelta(t, 1.0/3.0, out[1].Intensity, 1e-9)
}

func TestHexAggregateCentroidIsAverage(t *testing.T) {
	t.Parallel()
	layer := models.LayerDefinition{
		Features: []models.MapFeature{
			makeFeature("a", 1.0, 2.0),
			makeFeature("b", 1.0, 2.0),
		},
	}
	out := HexAggregate(layer)
	require.Len(t, out, 1)
	assert.Equal(t, 2, out[0].Count)
	assert.Equal(t, "1:2", out[0].CellID)
	assert.Equal(t, 1.0, out[0].Centroid.Lat)
	assert.Equal(t, 2.0, out[0].Centroid.Lon)
	assert.Equal(t, 1.0, out[0].Intensity)
}

func TestHexAggregateNegativeCoordsKeyShape(t *testing.T) {
	t.Parallel()
	// Rust formats negative floats verbatim ("-1.2"), no leading
	// zero stripping; Go's strconv FormatFloat 'g' -1 64 matches.
	layer := models.LayerDefinition{
		Features: []models.MapFeature{makeFeature("a", -1.18, -3.42)},
	}
	out := HexAggregate(layer)
	require.Len(t, out, 1)
	assert.Equal(t, "-1.2:-3.4", out[0].CellID)
}

func TestHexAggregateSortedByCountDesc(t *testing.T) {
	t.Parallel()
	feats := []models.MapFeature{}
	// three buckets with 5, 3, 1 features
	for i := 0; i < 5; i++ {
		feats = append(feats, makeFeature("a", 1.0, 1.0))
	}
	for i := 0; i < 3; i++ {
		feats = append(feats, makeFeature("b", 2.0, 2.0))
	}
	feats = append(feats, makeFeature("c", 3.0, 3.0))

	out := HexAggregate(models.LayerDefinition{Features: feats})
	require.Len(t, out, 3)
	assert.Equal(t, 5, out[0].Count)
	assert.Equal(t, 3, out[1].Count)
	assert.Equal(t, 1, out[2].Count)
}
