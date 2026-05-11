package geospatialcore

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGPXTrailTrackFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/mesa_loop.gpx")
	require.NoError(t, err)

	trail, err := ParseGPXTrail(raw, GPXParseOptions{SourceName: "mesa_loop.gpx"})
	require.NoError(t, err)

	require.Equal(t, "mesa-loop", trail.TrailID)
	require.Equal(t, "Mesa Loop", trail.TrailName)
	require.Equal(t, 4, trail.PointCount)
	require.InDelta(t, 35, trail.ElevationGainMeters, 0.0001)
	require.InDelta(t, 114.829, trail.ElevationGainFeet, 0.001)
	require.NotNil(t, trail.MinElevationMeters)
	require.Equal(t, 1700.0, *trail.MinElevationMeters)
	require.NotNil(t, trail.MaxElevationMeters)
	require.Equal(t, 1730.0, *trail.MaxElevationMeters)
	require.Equal(t, 40.0, trail.StartPoint.Lat)
	require.Equal(t, -105.3, trail.StartPoint.Lon)
	require.Equal(t, 40.003, trail.EndPoint.Lat)
	require.Equal(t, -105.295, trail.EndPoint.Lon)
	require.Greater(t, trail.DistanceMeters, 500.0)
	require.Less(t, trail.DistanceMeters, 700.0)

	trailhead, err := trail.TrailheadOntologyString()
	require.NoError(t, err)
	require.Equal(t, "40,-105.3", trailhead)

	route, err := trail.RouteGeoJSONString()
	require.NoError(t, err)
	require.Contains(t, route, `"type":"LineString"`)
	require.Contains(t, route, `[-105.3,40]`)

	bbox, err := trail.RouteBBoxJSONString()
	require.NoError(t, err)
	require.JSONEq(t, `[-105.3,40,-105.295,40.003]`, bbox)
}

func TestParseGPXTrailRouteFixtureUsesSourceNameFallback(t *testing.T) {
	raw, err := os.ReadFile("testdata/ridge_route.gpx")
	require.NoError(t, err)

	trail, err := ParseGPXTrail(raw, GPXParseOptions{SourceName: "ridge_route.gpx"})
	require.NoError(t, err)

	require.Equal(t, "ridge-route", trail.TrailID)
	require.Equal(t, "ridge_route", trail.TrailName)
	require.Equal(t, 3, trail.PointCount)
	require.InDelta(t, 18, trail.ElevationGainMeters, 0.0001)
	require.Equal(t, 39.99, trail.StartPoint.Lat)
	require.Equal(t, -105.28, trail.StartPoint.Lon)
}

func TestParseGPXTrailRejectsInvalidCoordinates(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><gpx><trk><trkseg><trkpt lat="91" lon="-105"/><trkpt lat="40" lon="-105"/></trkseg></trk></gpx>`)
	_, err := ParseGPXTrail(raw, GPXParseOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "latitude")
}

func TestGPXTrailGeoJSONRoundTrips(t *testing.T) {
	raw, err := os.ReadFile("testdata/mesa_loop.gpx")
	require.NoError(t, err)
	trail, err := ParseGPXTrail(raw, GPXParseOptions{})
	require.NoError(t, err)

	route, err := trail.RouteGeoJSONString()
	require.NoError(t, err)
	var geometry GeoJSONGeometry
	require.NoError(t, json.Unmarshal([]byte(route), &geometry))
	require.NoError(t, geometry.Validate())
	require.Equal(t, GeoJSONGeometryTypeLineString, geometry.Type)
	require.Equal(t, []float64{-105.3, 40, -105.295, 40.003}, geometry.BBox)
}
