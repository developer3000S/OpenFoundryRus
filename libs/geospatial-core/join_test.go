package geospatialcore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeoJSONIntersectsPointPolygonAndLine(t *testing.T) {
	polygon := mustGeoJSON(t, `{"type":"Polygon","coordinates":[[[-105.36,40.0],[-105.30,40.0],[-105.30,40.05],[-105.36,40.05],[-105.36,40.0]]]}`)
	point := mustGeoJSON(t, `{"type":"Point","coordinates":[-105.34458,40.016353]}`)
	line := mustGeoJSON(t, `{"type":"LineString","coordinates":[[-105.40,40.02],[-105.32,40.02]]}`)
	outside := mustGeoJSON(t, `{"type":"Point","coordinates":[-105.20,40.20]}`)

	ok, err := GeoJSONIntersects(point, polygon)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = GeoJSONIntersects(line, polygon)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = GeoJSONIntersects(outside, polygon)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestGeoJSONDistanceUsesSupportedUnits(t *testing.T) {
	left := mustGeoJSON(t, `{"type":"Point","coordinates":[0,0]}`)
	right := mustGeoJSON(t, `{"type":"Point","coordinates":[1,0]}`)

	miles, err := GeoJSONDistance(left, right, "miles")
	require.NoError(t, err)
	require.InDelta(t, 69.09341957563636, miles, 1e-9)

	meters, err := GeoJSONDistance(left, right, "meters")
	require.NoError(t, err)
	require.InDelta(t, 111_195.0802335329, meters, 1e-6)
}

func TestGeoJSONDistanceReturnsZeroForIntersection(t *testing.T) {
	left := mustGeoJSON(t, `{"type":"LineString","coordinates":[[-105.36,40.02],[-105.30,40.02]]}`)
	right := mustGeoJSON(t, `{"type":"LineString","coordinates":[[-105.33,40.0],[-105.33,40.05]]}`)

	distance, err := GeoJSONDistance(left, right, "km")
	require.NoError(t, err)
	require.Equal(t, 0.0, distance)
}

func TestGeoJSONDistanceUsesNearestLineSegment(t *testing.T) {
	line := mustGeoJSON(t, `{"type":"LineString","coordinates":[[-1,0],[1,0]]}`)
	point := mustGeoJSON(t, `{"type":"Point","coordinates":[0,0.01]}`)

	distance, err := GeoJSONDistance(point, line, "meters")
	require.NoError(t, err)
	require.InDelta(t, 1111.950802335329, distance, 0.5)
}

func TestGeoJSONBounds(t *testing.T) {
	line := mustGeoJSON(t, `{"type":"LineString","coordinates":[[-105.36,40.02],[-105.30,40.05],[-105.31,40.0]]}`)

	bounds, err := GeoJSONBounds(line)
	require.NoError(t, err)
	require.Equal(t, 40.0, bounds.MinLat)
	require.Equal(t, -105.36, bounds.MinLon)
	require.Equal(t, 40.05, bounds.MaxLat)
	require.Equal(t, -105.30, bounds.MaxLon)
}

func mustGeoJSON(t *testing.T, raw string) GeoJSONGeometry {
	t.Helper()
	var geometry GeoJSONGeometry
	require.NoError(t, json.Unmarshal([]byte(raw), &geometry))
	require.NoError(t, geometry.Validate())
	return geometry
}
