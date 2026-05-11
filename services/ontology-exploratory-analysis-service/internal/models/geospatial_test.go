package models

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayerSourceKindParsing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want LayerSourceKind
		ok   bool
	}{
		{"dataset", LayerSourceKindDataset, true},
		{"vector_tile", LayerSourceKindVectorTile, true},
		{"reference", LayerSourceKindReference, true},
		{"DATASET", "", false},
		{"unknown", "", false},
	}
	for _, c := range cases {
		got, err := ParseLayerSourceKind(c.in)
		if c.ok {
			require.NoErrorf(t, err, "%q should parse", c.in)
			assert.Equal(t, c.want, got)
		} else {
			require.Errorf(t, err, "%q should fail", c.in)
			assert.Contains(t, err.Error(), "unsupported layer source kind")
		}
	}
}

func TestGeometryTypeParsing(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"point", "line_string", "polygon"} {
		g, err := ParseGeometryType(in)
		require.NoError(t, err)
		assert.Equal(t, in, g.String())
	}
	_, err := ParseGeometryType("MultiPoint")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported geometry type")
}

func TestGeometryRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		geom Geometry
	}{
		{
			name: "point",
			geom: Geometry{Type: GeometryTypePoint, Point: &Coordinate{Lat: 40.4168, Lon: -3.7038}},
		},
		{
			name: "line_string",
			geom: Geometry{
				Type:       GeometryTypeLineString,
				LineString: []Coordinate{{Lat: 0, Lon: 0}, {Lat: 1, Lon: 1}},
			},
		},
		{
			name: "polygon",
			geom: Geometry{
				Type:    GeometryTypePolygon,
				Polygon: []Coordinate{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 0, Lon: 0}},
			},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(c.geom)
			require.NoError(t, err)

			var head struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal(raw, &head))
			assert.Equal(t, c.name, head.Type, "tag is the snake_case variant name")

			var got Geometry
			require.NoError(t, json.Unmarshal(raw, &got))
			assert.Equal(t, c.geom.Type, got.Type)
			switch c.geom.Type {
			case GeometryTypePoint:
				require.NotNil(t, got.Point)
				assert.Equal(t, *c.geom.Point, *got.Point)
			case GeometryTypeLineString:
				assert.Equal(t, c.geom.LineString, got.LineString)
			case GeometryTypePolygon:
				assert.Equal(t, c.geom.Polygon, got.Polygon)
			}
		})
	}
}

func TestGeometryCentroidAndBounds(t *testing.T) {
	t.Parallel()
	point := Geometry{Type: GeometryTypePoint, Point: &Coordinate{Lat: 1, Lon: 2}}
	assert.Equal(t, Coordinate{Lat: 1, Lon: 2}, point.Centroid())
	assert.Equal(t, Bounds{MinLat: 1, MinLon: 2, MaxLat: 1, MaxLon: 2}, point.BoundsOf())

	line := Geometry{
		Type:       GeometryTypeLineString,
		LineString: []Coordinate{{Lat: 0, Lon: 0}, {Lat: 2, Lon: 4}},
	}
	assert.Equal(t, Coordinate{Lat: 1, Lon: 2}, line.Centroid())
	assert.Equal(t, Bounds{MinLat: 0, MinLon: 0, MaxLat: 2, MaxLon: 4}, line.BoundsOf())
}

func TestCoordinateDistanceKm(t *testing.T) {
	t.Parallel()
	// Same point ⇒ zero distance.
	a := Coordinate{Lat: 40, Lon: -3}
	assert.InDelta(t, 0.0, a.DistanceKm(a), 1e-9)

	// 1° latitude ≈ 111 km in this approximation.
	b := Coordinate{Lat: 41, Lon: -3}
	assert.InDelta(t, 111.0, a.DistanceKm(b), 1e-9)

	// At lat=40°, 1° longitude ≈ 111 * cos(40°) km.
	c := Coordinate{Lat: 40, Lon: -2}
	want := 111.0 * math.Abs(math.Cos(40*math.Pi/180.0))
	assert.InDelta(t, want, a.DistanceKm(c), 1e-9)

	// Floor on the cosine factor: above ~78° the value is clamped to 0.2.
	pole := Coordinate{Lat: 89, Lon: 0}
	pole2 := Coordinate{Lat: 89, Lon: 1}
	wantPole := 111.0 * 0.2
	assert.InDelta(t, wantPole, pole.DistanceKm(pole2), 1e-9)
}

func TestCoordinateAndBoundsValidation(t *testing.T) {
	t.Parallel()

	require.NoError(t, (Coordinate{Lat: 40.4168, Lon: -3.7038}).Validate())
	require.Error(t, (Coordinate{Lat: 91, Lon: 0}).Validate())
	require.NoError(t, (Bounds{MinLat: 40.3, MinLon: -3.8, MaxLat: 40.5, MaxLon: -3.6}).Validate())

	err := (Bounds{MinLat: 40.5, MinLon: -3.8, MaxLat: 40.3, MaxLon: -3.6}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_lat")
}

func TestBoundsContainsAndIntersects(t *testing.T) {
	t.Parallel()
	b := Bounds{MinLat: 0, MinLon: 0, MaxLat: 10, MaxLon: 10}
	assert.True(t, b.Contains(Coordinate{Lat: 5, Lon: 5}))
	assert.False(t, b.Contains(Coordinate{Lat: 11, Lon: 5}))
	assert.True(t, b.Contains(Coordinate{Lat: 0, Lon: 0}))
	assert.True(t, b.Contains(Coordinate{Lat: 10, Lon: 10}))

	overlap := Bounds{MinLat: 5, MinLon: 5, MaxLat: 15, MaxLon: 15}
	disjoint := Bounds{MinLat: 20, MinLon: 20, MaxLat: 30, MaxLon: 30}
	edge := Bounds{MinLat: 10, MinLon: 0, MaxLat: 20, MaxLon: 10}
	assert.True(t, b.Intersects(overlap))
	assert.False(t, b.Intersects(disjoint))
	assert.True(t, b.Intersects(edge), "boxes touching on an edge intersect (matches Rust)")
}

func TestLayerStyleDefaultMatchesRust(t *testing.T) {
	t.Parallel()
	got := NewDefaultLayerStyle()
	want := LayerStyle{
		Color:            "#D97706",
		Opacity:          0.78,
		Radius:           9.0,
		LineWidth:        2.0,
		HeatmapIntensity: 0.65,
		ClusterColor:     "#0F766E",
		ShowLabels:       true,
	}
	assert.Equal(t, want, got)
}

func TestLayerDefinitionWireShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	def := LayerDefinition{
		ID:            id,
		Name:          "fleet-zones",
		Description:   "operational zones",
		SourceKind:    LayerSourceKindDataset,
		SourceDataset: "ds_fleet",
		GeometryType:  GeometryTypePolygon,
		Style:         NewDefaultLayerStyle(),
		Features: []MapFeature{{
			ID:    "f1",
			Label: "zone-1",
			Geometry: Geometry{
				Type: GeometryTypePolygon,
				Polygon: []Coordinate{
					{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 0, Lon: 0},
				},
			},
		}},
		Tags:      []string{"alpha", "beta"},
		Indexed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	raw, err := json.Marshal(def)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, id.String(), got["id"])
	assert.Equal(t, "dataset", got["source_kind"])
	assert.Equal(t, "polygon", got["geometry_type"])
	assert.Equal(t, true, got["indexed"])
	assert.Contains(t, got, "style")
	assert.Contains(t, got, "features")
	assert.Contains(t, got, "tags")
}

func TestCreateLayerRequestOmittedDefaults(t *testing.T) {
	t.Parallel()
	// Minimum payload: optional `style`, `tags`, `indexed`, `description`
	// are all omitted. Decoder leaves them zero-valued / nil — the handler
	// substitutes the Rust defaults.
	body := []byte(`{
		"name": "x",
		"source_kind": "dataset",
		"source_dataset": "ds",
		"geometry_type": "point",
		"features": [
			{"id":"f","label":"l","geometry":{"type":"point","coordinates":{"lat":0,"lon":0}}}
		]
	}`)
	var req CreateLayerRequest
	require.NoError(t, json.Unmarshal(body, &req))
	assert.Equal(t, "", req.Description)
	assert.Nil(t, req.Style)
	assert.Nil(t, req.Tags)
	assert.Nil(t, req.Indexed)
	require.Len(t, req.Features, 1)
}

func TestLayerRowToDefinitionDecodesJSON(t *testing.T) {
	t.Parallel()
	style, err := json.Marshal(NewDefaultLayerStyle())
	require.NoError(t, err)
	feats, err := json.Marshal([]MapFeature{{
		ID: "f", Label: "l", Geometry: Geometry{Type: GeometryTypePoint, Point: &Coordinate{Lat: 1, Lon: 2}},
	}})
	require.NoError(t, err)
	tags, err := json.Marshal([]string{"a"})
	require.NoError(t, err)

	row := LayerRow{
		ID:            uuid.New(),
		Name:          "n",
		Description:   "d",
		SourceKind:    "vector_tile",
		SourceDataset: "ds",
		GeometryType:  "point",
		Style:         style,
		Features:      feats,
		Tags:          tags,
		Indexed:       false,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	def, err := row.ToDefinition()
	require.NoError(t, err)
	assert.Equal(t, LayerSourceKindVectorTile, def.SourceKind)
	assert.Equal(t, GeometryTypePoint, def.GeometryType)
	assert.Equal(t, NewDefaultLayerStyle(), def.Style)
	require.Len(t, def.Features, 1)
	assert.Equal(t, []string{"a"}, def.Tags)
}

func TestLayerRowToDefinitionRejectsUnknownEnum(t *testing.T) {
	t.Parallel()
	row := LayerRow{
		ID:           uuid.New(),
		SourceKind:   "weird",
		GeometryType: "point",
		Style:        []byte("{}"),
		Features:     []byte("[]"),
		Tags:         []byte("[]"),
	}
	_, err := row.ToDefinition()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported layer source kind")
}
