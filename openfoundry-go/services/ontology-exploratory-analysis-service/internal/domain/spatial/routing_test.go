package spatial

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func TestRouteDriveDuration(t *testing.T) {
	t.Parallel()
	got := Route(models.RouteRequest{
		Origin:      models.Coordinate{Lat: 40.0, Lon: -3.0},
		Destination: models.Coordinate{Lat: 41.0, Lon: -3.0},
		Mode:        models.RouteModeDrive,
	})
	// 1 deg lat ≈ 111 km, drive speed = 58 km/h → 114.83… min, ceil = 115.
	assert.Equal(t, models.RouteModeDrive, got.Mode)
	assert.True(t, math.Abs(got.DistanceKm-111.0) < 1e-6)
	assert.Equal(t, uint32(115), got.DurationMin)
	assert.Len(t, got.Polyline, 3)
	assert.Equal(t, got.Polyline[0], models.Coordinate{Lat: 40.0, Lon: -3.0})
	assert.Equal(t, got.Polyline[2], models.Coordinate{Lat: 41.0, Lon: -3.0})
	require.Len(t, got.Isochrone, 3)
}

func TestRouteWalkClampsTo1Min(t *testing.T) {
	t.Parallel()
	// origin == destination → distance 0 → duration_min = max(ceil(0), 1)
	// = 1, max_minutes fallback → max(1, 20) = 20, step → max(20/3=6, 5) = 6.
	got := Route(models.RouteRequest{
		Origin:      models.Coordinate{Lat: 1.0, Lon: 2.0},
		Destination: models.Coordinate{Lat: 1.0, Lon: 2.0},
		Mode:        models.RouteModeWalk,
	})
	assert.Equal(t, uint32(1), got.DurationMin)
	assert.Equal(t, uint32(0), uint32(got.DistanceKm))
	require.Len(t, got.Isochrone, 3)
	assert.Equal(t, "10 min", got.Isochrone[0].Label)
	assert.Equal(t, uint32(6), got.Isochrone[0].EtaMinutes)
	assert.Equal(t, "20 min", got.Isochrone[1].Label)
	assert.Equal(t, uint32(12), got.Isochrone[1].EtaMinutes)
	assert.Equal(t, "18 min", got.Isochrone[2].Label)
	assert.Equal(t, uint32(18), got.Isochrone[2].EtaMinutes)
}

func TestRouteRespectsMaxMinutes(t *testing.T) {
	t.Parallel()
	max := uint32(45)
	got := Route(models.RouteRequest{
		Origin:      models.Coordinate{Lat: 0, Lon: 0},
		Destination: models.Coordinate{Lat: 0, Lon: 0},
		Mode:        models.RouteModeBike,
		MaxMinutes:  &max,
	})
	// step = max(45/3=15, 5) = 15.
	assert.Equal(t, uint32(15), got.Isochrone[0].EtaMinutes)
	assert.Equal(t, uint32(30), got.Isochrone[1].EtaMinutes)
	assert.Equal(t, uint32(45), got.Isochrone[2].EtaMinutes)
	assert.Equal(t, "45 min", got.Isochrone[2].Label)
}

func TestRouteMidpointOffsets(t *testing.T) {
	t.Parallel()
	got := Route(models.RouteRequest{
		Origin:      models.Coordinate{Lat: 40.0, Lon: -3.0},
		Destination: models.Coordinate{Lat: 42.0, Lon: -1.0},
		Mode:        models.RouteModeDrive,
	})
	// midpoint = ((40+42)/2 + 0.04, (-3 + -1)/2 - 0.03) = (41.04, -2.03)
	assert.True(t, math.Abs(got.Polyline[1].Lat-41.04) < 1e-9)
	assert.True(t, math.Abs(got.Polyline[1].Lon-(-2.03)) < 1e-9)
}

func TestRouteJSONShape(t *testing.T) {
	t.Parallel()
	got := Route(models.RouteRequest{
		Origin:      models.Coordinate{Lat: 0, Lon: 0},
		Destination: models.Coordinate{Lat: 0, Lon: 0},
		Mode:        models.RouteModeWalk,
	})
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))
	assert.Equal(t, "walk", generic["mode"])
	assert.Contains(t, generic, "distance_km")
	assert.Contains(t, generic, "duration_min")
	assert.Contains(t, generic, "polyline")
	assert.Contains(t, generic, "isochrone")
	iso, ok := generic["isochrone"].([]any)
	require.True(t, ok)
	require.Len(t, iso, 3)
}

func TestRouteModeUnmarshalsFromJSON(t *testing.T) {
	t.Parallel()
	var m models.RouteMode
	require.NoError(t, json.Unmarshal([]byte(`"bike"`), &m))
	assert.Equal(t, models.RouteModeBike, m)
	err := json.Unmarshal([]byte(`"hover"`), &m)
	assert.Error(t, err)
}
