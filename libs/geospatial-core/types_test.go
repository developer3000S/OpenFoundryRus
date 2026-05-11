package geospatialcore

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeoPointValidationAndOntologyString(t *testing.T) {
	t.Parallel()

	point := GeoPoint{Lat: 40.4168, Lon: -3.7038}
	require.NoError(t, point.Validate())

	ontologyString, err := point.OntologyString()
	require.NoError(t, err)
	assert.Equal(t, "40.4168,-3.7038", ontologyString)

	parsed, err := ParseOntologyGeoPointString(ontologyString)
	require.NoError(t, err)
	assert.Equal(t, point, parsed)

	wkt, err := ParseOntologyGeoPointString("POINT(-3.7038 40.4168)")
	require.NoError(t, err)
	assert.Equal(t, point, wkt)
}

func TestGeoPointRejectsInvalidCoordinates(t *testing.T) {
	t.Parallel()

	cases := []GeoPoint{
		{Lat: 91, Lon: 0},
		{Lat: -91, Lon: 0},
		{Lat: 0, Lon: 181},
		{Lat: 0, Lon: -181},
		{Lat: math.NaN(), Lon: 0},
		{Lat: 0, Lon: math.Inf(1)},
	}
	for _, point := range cases {
		require.Error(t, point.Validate())
	}
}

func TestGeoPointJSONAliasesAndCRSValidation(t *testing.T) {
	t.Parallel()

	var point GeoPoint
	require.NoError(t, json.Unmarshal([]byte(`{
		"latitude": 40.4168,
		"longitude": -3.7038,
		"crs": {"authority": "epsg", "code": 4326}
	}`), &point))
	require.NotNil(t, point.CRS)
	assert.Equal(t, CRSMetadata{Authority: "epsg", Code: 4326}, *point.CRS)
	require.NoError(t, point.Validate())

	var unsupported GeoPoint
	err := json.Unmarshal([]byte(`{"lat": 40, "lon": -3, "crs": {"authority": "EPSG", "code": 3857}}`), &unsupported)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only EPSG:4326")
}

func TestGeoJSONPointValidationAndCoordinates(t *testing.T) {
	t.Parallel()

	geom, err := ParseGeoJSONGeometry([]byte(`{
		"type": "Point",
		"coordinates": [-3.7038, 40.4168],
		"bbox": [-3.8, 40.3, -3.6, 40.5]
	}`))
	require.NoError(t, err)

	points, err := geom.PointCoordinates()
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, GeoPoint{Lat: 40.4168, Lon: -3.7038}, points[0])

	_, err = ParseGeoJSONGeometry([]byte(`{"type":"Point","coordinates":[200,95]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latitude")
}

func TestGeoJSONLineStringAndPolygonValidation(t *testing.T) {
	t.Parallel()

	line, err := NewGeoJSONLineString([]GeoPoint{
		{Lat: 40.4, Lon: -3.7},
		{Lat: 40.5, Lon: -3.8},
	})
	require.NoError(t, err)
	require.NoError(t, line.Validate())

	_, err = ParseGeoJSONGeometry([]byte(`{"type":"LineString","coordinates":[[-3.7,40.4]]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2")

	polygon := GeoJSONGeometry{
		Type:        GeoJSONGeometryTypePolygon,
		Coordinates: json.RawMessage(`[[[-3.7,40.4],[-3.8,40.4],[-3.8,40.5],[-3.7,40.4]]]`),
	}
	require.NoError(t, polygon.Validate())

	openPolygon := GeoJSONGeometry{
		Type:        GeoJSONGeometryTypePolygon,
		Coordinates: json.RawMessage(`[[[-3.7,40.4],[-3.8,40.4],[-3.8,40.5],[-3.7,40.6]]]`),
	}
	err = openPolygon.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be closed")
}

func TestBoundingBoxValidationAndGeoJSONOrder(t *testing.T) {
	t.Parallel()

	box, err := NewBoundingBox(40.3, -3.8, 40.5, -3.6)
	require.NoError(t, err)

	geojsonBox, err := box.GeoJSONBBox()
	require.NoError(t, err)
	assert.Equal(t, []float64{-3.8, 40.3, -3.6, 40.5}, geojsonBox)

	roundTrip, err := BoundingBoxFromGeoJSONBBox(geojsonBox)
	require.NoError(t, err)
	assert.Equal(t, box, roundTrip)

	_, err = NewBoundingBox(40.5, -3.8, 40.3, -3.6)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_lat")

	_, err = BoundingBoxFromGeoJSONBBox([]float64{-3.8, 95, -3.6, 96})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latitude")
}

func TestH3IndexValidation(t *testing.T) {
	t.Parallel()

	require.NoError(t, H3Index("8928308280fffff").Validate())
	require.NoError(t, H3Index("8a2a1072b59ffff").Validate())

	for _, value := range []H3Index{"", "not-h3", "0000000000000000", "8928308280fffffff"} {
		require.Error(t, value.Validate())
	}
}

func TestGeospatialLogicalTypeOntologyMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input        string
		wantType     GeospatialLogicalType
		wantOntology string
		wantStorage  string
	}{
		{"GeoPoint", LogicalTypeGeoPoint, "geo_point", "object"},
		{"geometry", LogicalTypeGeometry, "json", "json"},
		{"geo_json", LogicalTypeGeoJSON, "json", "json"},
		{"bbox", LogicalTypeBoundingBox, "json", "json"},
		{"H3", LogicalTypeH3Index, "string", "string"},
		{"crs", LogicalTypeCRSMetadata, "string", "string"},
	}

	for _, tc := range cases {
		got, err := ParseGeospatialLogicalType(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.wantType, got)
		assert.Equal(t, tc.wantOntology, got.OntologyPropertyType())
		assert.Equal(t, tc.wantStorage, got.StorageType())

		metadata := FieldMetadata{LogicalType: got, OntologyPropertyType: tc.wantOntology}
		require.NoError(t, metadata.Validate())
	}

	bad := FieldMetadata{LogicalType: LogicalTypeH3Index, OntologyPropertyType: "geo_point"}
	err := bad.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maps to ontology property type")
}
