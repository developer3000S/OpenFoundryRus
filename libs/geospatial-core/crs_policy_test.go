package geospatialcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCRSPolicy(t *testing.T) {
	t.Parallel()

	policy := DefaultCRSPolicy()
	require.NoError(t, policy.Validate())
	assert.Equal(t, "EPSG:4326", policy.DefaultCRS.String())
	assert.Equal(t, CoordinateOrderLatLon, policy.InternalCoordinateOrder)
	assert.Equal(t, CoordinateOrderLonLat, policy.GeoJSONCoordinateOrder)
	assert.Equal(t, CoordinateOrderLatLon, policy.OntologyCoordinateOrder)
	assert.Equal(t, MapDisplayProjection, policy.MapDisplayProjection)
}

func TestNormalizeCRSDefaultsAndRejectsUnsupported(t *testing.T) {
	t.Parallel()

	got, err := NormalizeCRS(nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultCRSMetadata(), got)

	fromString, err := ParseCRSMetadata("wgs_84")
	require.NoError(t, err)
	assert.Equal(t, DefaultCRSMetadata(), fromString)

	_, err = ParseCRSMetadata("EPSG:3857")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only EPSG:4326")
}

func TestReprojectGeoPointNoop(t *testing.T) {
	t.Parallel()

	source := CRSMetadata{Authority: "epsg", Code: 4326}
	point := GeoPoint{Lat: 40.4168, Lon: -3.7038, CRS: &source}
	projected, err := ReprojectGeoPointNoop(point, nil)
	require.NoError(t, err)
	assert.Equal(t, 40.4168, projected.Lat)
	assert.Equal(t, -3.7038, projected.Lon)
	require.NotNil(t, projected.CRS)
	assert.Equal(t, "EPSG:4326", projected.CRS.String())

	unsupported := CRSMetadata{Authority: "EPSG", Code: 3857}
	_, err = ReprojectGeoPointNoop(GeoPoint{Lat: 40, Lon: -3, CRS: &unsupported}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source CRS")
}

func TestCoordinateOrderPairConversions(t *testing.T) {
	t.Parallel()

	latLon, err := GeoPointFromOrderedPair([]float64{40.4168, -3.7038}, CoordinateOrderLatLon, nil)
	require.NoError(t, err)
	assert.Equal(t, GeoPoint{Lat: 40.4168, Lon: -3.7038, CRS: ptr(DefaultCRSMetadata())}, latLon)

	lonLat, err := GeoPointFromOrderedPair([]float64{-3.7038, 40.4168}, CoordinateOrderLonLat, nil)
	require.NoError(t, err)
	assert.Equal(t, latLon, lonLat)

	geojsonPair, err := OrderedPairFromGeoPoint(latLon, CoordinateOrderLonLat)
	require.NoError(t, err)
	assert.Equal(t, []float64{-3.7038, 40.4168}, geojsonPair)

	_, err = GeoPointFromOrderedPair([]float64{95, -3}, CoordinateOrderLatLon, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latitude")
}

func TestFieldMetadataCoordinateOrderPolicy(t *testing.T) {
	t.Parallel()

	require.NoError(t, (FieldMetadata{
		LogicalType:     LogicalTypeGeoJSON,
		CoordinateOrder: CoordinateOrderLonLat,
	}).Validate())

	err := (FieldMetadata{
		LogicalType:     LogicalTypeGeoJSON,
		CoordinateOrder: CoordinateOrderLatLon,
	}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expects coordinate_order")

	require.NoError(t, (FieldMetadata{
		LogicalType:     LogicalTypeGeoPoint,
		CoordinateOrder: CoordinateOrderLatLon,
	}).Validate())
}

func ptr[T any](value T) *T {
	return &value
}
