// Geospatial models — ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/models/
// (S8 / ADR-0030 — geospatial-intelligence-service absorbed). Held as a typed
// library namespace; the binary does not mount the geospatial routes yet
// (matches Rust `#[allow(dead_code)]`).
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// Coordinate is a (lat, lon) pair in WGS84 degrees.
type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// DistanceKm mirrors `Coordinate::distance_km` in feature.rs — a cheap
// equirectangular approximation, not great-circle. Values must match the
// Rust impl byte-for-byte so spatial-query results align across runtimes.
func (c Coordinate) DistanceKm(other Coordinate) float64 {
	latDelta := (c.Lat - other.Lat) * 111.0
	cos := math.Abs(math.Cos(c.Lat * math.Pi / 180.0))
	if cos < 0.2 {
		cos = 0.2
	}
	lonDelta := (c.Lon - other.Lon) * 111.0 * cos
	return math.Sqrt(latDelta*latDelta + lonDelta*lonDelta)
}

// Bounds is an axis-aligned bounding box in WGS84 degrees.
type Bounds struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

func (b Bounds) Contains(p Coordinate) bool {
	return p.Lat >= b.MinLat && p.Lat <= b.MaxLat &&
		p.Lon >= b.MinLon && p.Lon <= b.MaxLon
}

func (b Bounds) Intersects(other Bounds) bool {
	return b.MinLat <= other.MaxLat &&
		b.MaxLat >= other.MinLat &&
		b.MinLon <= other.MaxLon &&
		b.MaxLon >= other.MinLon
}

// GeometryType mirrors the Rust enum (snake_case wire form).
type GeometryType string

const (
	GeometryTypePoint      GeometryType = "point"
	GeometryTypeLineString GeometryType = "line_string"
	GeometryTypePolygon    GeometryType = "polygon"
)

func (g GeometryType) String() string { return string(g) }

func (g GeometryType) Valid() bool {
	switch g {
	case GeometryTypePoint, GeometryTypeLineString, GeometryTypePolygon:
		return true
	default:
		return false
	}
}

func ParseGeometryType(s string) (GeometryType, error) {
	g := GeometryType(s)
	if !g.Valid() {
		return "", fmt.Errorf("unsupported geometry type: %s", s)
	}
	return g, nil
}

func (g *GeometryType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseGeometryType(s)
	if err != nil {
		return err
	}
	*g = parsed
	return nil
}

// Geometry is the externally-tagged Rust enum
// `#[serde(tag = "type", content = "coordinates", rename_all = "snake_case")]`.
// Wire form: `{"type":"point","coordinates":{lat,lon}}` for points,
// or `{"type":"line_string","coordinates":[{lat,lon},...]}` for lines/polygons.
type Geometry struct {
	Type        GeometryType
	Point       *Coordinate
	LineString  []Coordinate
	Polygon     []Coordinate
}

func (g Geometry) MarshalJSON() ([]byte, error) {
	switch g.Type {
	case GeometryTypePoint:
		if g.Point == nil {
			return nil, errors.New("geometry: point variant requires Point coordinate")
		}
		return json.Marshal(struct {
			Type        GeometryType `json:"type"`
			Coordinates Coordinate   `json:"coordinates"`
		}{Type: GeometryTypePoint, Coordinates: *g.Point})
	case GeometryTypeLineString:
		return json.Marshal(struct {
			Type        GeometryType `json:"type"`
			Coordinates []Coordinate `json:"coordinates"`
		}{Type: GeometryTypeLineString, Coordinates: g.LineString})
	case GeometryTypePolygon:
		return json.Marshal(struct {
			Type        GeometryType `json:"type"`
			Coordinates []Coordinate `json:"coordinates"`
		}{Type: GeometryTypePolygon, Coordinates: g.Polygon})
	default:
		return nil, fmt.Errorf("geometry: unsupported variant %q", g.Type)
	}
}

func (g *Geometry) UnmarshalJSON(data []byte) error {
	var head struct {
		Type GeometryType `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	switch head.Type {
	case GeometryTypePoint:
		var body struct {
			Coordinates Coordinate `json:"coordinates"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return err
		}
		*g = Geometry{Type: GeometryTypePoint, Point: &body.Coordinates}
		return nil
	case GeometryTypeLineString:
		var body struct {
			Coordinates []Coordinate `json:"coordinates"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return err
		}
		*g = Geometry{Type: GeometryTypeLineString, LineString: body.Coordinates}
		return nil
	case GeometryTypePolygon:
		var body struct {
			Coordinates []Coordinate `json:"coordinates"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return err
		}
		*g = Geometry{Type: GeometryTypePolygon, Polygon: body.Coordinates}
		return nil
	default:
		return fmt.Errorf("geometry: unsupported variant %q", head.Type)
	}
}

// Centroid mirrors `Geometry::centroid` in Rust.
func (g Geometry) Centroid() Coordinate {
	switch g.Type {
	case GeometryTypePoint:
		if g.Point == nil {
			return Coordinate{}
		}
		return *g.Point
	case GeometryTypeLineString:
		return averageCoordinate(g.LineString)
	case GeometryTypePolygon:
		return averageCoordinate(g.Polygon)
	default:
		return Coordinate{}
	}
}

// BoundsOf mirrors `Geometry::bounds` in Rust.
func (g Geometry) BoundsOf() Bounds {
	switch g.Type {
	case GeometryTypePoint:
		if g.Point == nil {
			return Bounds{}
		}
		return Bounds{
			MinLat: g.Point.Lat, MaxLat: g.Point.Lat,
			MinLon: g.Point.Lon, MaxLon: g.Point.Lon,
		}
	case GeometryTypeLineString:
		return boundsOfPoints(g.LineString)
	case GeometryTypePolygon:
		return boundsOfPoints(g.Polygon)
	default:
		return Bounds{}
	}
}

func averageCoordinate(points []Coordinate) Coordinate {
	if len(points) == 0 {
		return Coordinate{}
	}
	var latSum, lonSum float64
	for _, p := range points {
		latSum += p.Lat
		lonSum += p.Lon
	}
	n := float64(len(points))
	return Coordinate{Lat: latSum / n, Lon: lonSum / n}
}

func boundsOfPoints(points []Coordinate) Bounds {
	minLat, minLon := math.MaxFloat64, math.MaxFloat64
	maxLat, maxLon := -math.MaxFloat64, -math.MaxFloat64
	for _, p := range points {
		if p.Lat < minLat {
			minLat = p.Lat
		}
		if p.Lat > maxLat {
			maxLat = p.Lat
		}
		if p.Lon < minLon {
			minLon = p.Lon
		}
		if p.Lon > maxLon {
			maxLon = p.Lon
		}
	}
	return Bounds{MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon}
}

// MapFeature is a single feature in a layer.
type MapFeature struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	Geometry   Geometry        `json:"geometry"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

// LayerStyle mirrors the Rust struct with its Default impl baked into
// `NewDefaultLayerStyle`. Not pointer-receiver — value semantics match
// Rust's `Clone + Default`.
type LayerStyle struct {
	Color            string  `json:"color"`
	Opacity          float64 `json:"opacity"`
	Radius           float64 `json:"radius"`
	LineWidth        float64 `json:"line_width"`
	HeatmapIntensity float64 `json:"heatmap_intensity"`
	ClusterColor     string  `json:"cluster_color"`
	ShowLabels       bool    `json:"show_labels"`
}

// NewDefaultLayerStyle is the Rust `LayerStyle::default()` — must be
// kept byte-exact with style.rs since the values leak to the wire when
// `style` is omitted from a CreateLayerRequest.
func NewDefaultLayerStyle() LayerStyle {
	return LayerStyle{
		Color:            "#D97706",
		Opacity:          0.78,
		Radius:           9.0,
		LineWidth:        2.0,
		HeatmapIntensity: 0.65,
		ClusterColor:     "#0F766E",
		ShowLabels:       true,
	}
}

// LayerSourceKind mirrors the Rust enum (snake_case wire form).
type LayerSourceKind string

const (
	LayerSourceKindDataset    LayerSourceKind = "dataset"
	LayerSourceKindVectorTile LayerSourceKind = "vector_tile"
	LayerSourceKindReference  LayerSourceKind = "reference"
)

func (k LayerSourceKind) String() string { return string(k) }

func (k LayerSourceKind) Valid() bool {
	switch k {
	case LayerSourceKindDataset, LayerSourceKindVectorTile, LayerSourceKindReference:
		return true
	default:
		return false
	}
}

func ParseLayerSourceKind(s string) (LayerSourceKind, error) {
	k := LayerSourceKind(s)
	if !k.Valid() {
		return "", fmt.Errorf("unsupported layer source kind: %s", s)
	}
	return k, nil
}

func (k *LayerSourceKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseLayerSourceKind(s)
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

// LayerDefinition is the canonical wire shape for a layer.
type LayerDefinition struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	SourceKind    LayerSourceKind `json:"source_kind"`
	SourceDataset string          `json:"source_dataset"`
	GeometryType  GeometryType    `json:"geometry_type"`
	Style         LayerStyle      `json:"style"`
	Features      []MapFeature    `json:"features"`
	Tags          []string        `json:"tags"`
	Indexed       bool            `json:"indexed"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CreateLayerRequest mirrors the Rust DTO. `Style`/`Tags`/`Indexed` are
// pointer/slice types so the handler can detect "field omitted" and
// substitute the Rust serde defaults (`LayerStyle::default()`, `vec![]`,
// `default_indexed` = true) — see `applyCreateRequestDefaults`.
type CreateLayerRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	SourceKind    LayerSourceKind `json:"source_kind"`
	SourceDataset string          `json:"source_dataset"`
	GeometryType  GeometryType    `json:"geometry_type"`
	Style         *LayerStyle     `json:"style,omitempty"`
	Features      []MapFeature    `json:"features"`
	Tags          []string        `json:"tags,omitempty"`
	Indexed       *bool           `json:"indexed,omitempty"`
}

// UpdateLayerRequest mirrors the Rust DTO; every field optional.
type UpdateLayerRequest struct {
	Name          *string          `json:"name,omitempty"`
	Description   *string          `json:"description,omitempty"`
	SourceKind    *LayerSourceKind `json:"source_kind,omitempty"`
	SourceDataset *string          `json:"source_dataset,omitempty"`
	GeometryType  *GeometryType    `json:"geometry_type,omitempty"`
	Style         *LayerStyle      `json:"style,omitempty"`
	Features      *[]MapFeature    `json:"features,omitempty"`
	Tags          *[]string        `json:"tags,omitempty"`
	Indexed       *bool            `json:"indexed,omitempty"`
}

// LayerRow is the raw row shape pulled from `geospatial_layers` —
// jsonb columns arrive as []byte and are decoded into typed fields by
// `(LayerRow).ToDefinition`.
type LayerRow struct {
	ID            uuid.UUID
	Name          string
	Description   string
	SourceKind    string
	SourceDataset string
	GeometryType  string
	Style         []byte
	Features      []byte
	Tags          []byte
	Indexed       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ToDefinition mirrors Rust's `TryFrom<LayerRow> for LayerDefinition`.
func (r LayerRow) ToDefinition() (LayerDefinition, error) {
	kind, err := ParseLayerSourceKind(r.SourceKind)
	if err != nil {
		return LayerDefinition{}, err
	}
	geom, err := ParseGeometryType(r.GeometryType)
	if err != nil {
		return LayerDefinition{}, err
	}

	var style LayerStyle
	if err := decodeJSONField(r.Style, "style", &style); err != nil {
		return LayerDefinition{}, err
	}
	var features []MapFeature
	if err := decodeJSONField(r.Features, "features", &features); err != nil {
		return LayerDefinition{}, err
	}
	var tags []string
	if err := decodeJSONField(r.Tags, "tags", &tags); err != nil {
		return LayerDefinition{}, err
	}

	return LayerDefinition{
		ID:            r.ID,
		Name:          r.Name,
		Description:   r.Description,
		SourceKind:    kind,
		SourceDataset: r.SourceDataset,
		GeometryType:  geom,
		Style:         style,
		Features:      features,
		Tags:          tags,
		Indexed:       r.Indexed,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}, nil
}

func decodeJSONField(raw []byte, field string, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("failed to decode %s: %w", field, err)
	}
	return nil
}

// ListResponse is the generic `{"items":[...]}` envelope used by every
// geospatial list endpoint (mirrors Rust `ListResponse<T>`).
type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// GeospatialOverview summarises the indexer state across all layers.
// Order of `SupportedOperations` is Rust-significant — see
// `domain/indexer.rs` `build_overview`.
type GeospatialOverview struct {
	LayerCount          int      `json:"layer_count"`
	IndexedLayers       int      `json:"indexed_layers"`
	TotalFeatures       int      `json:"total_features"`
	TileReadyLayers     int      `json:"tile_ready_layers"`
	SupportedOperations []string `json:"supported_operations"`
}

// SpatialOperation mirrors the Rust enum (snake_case wire form).
type SpatialOperation string

const (
	SpatialOperationWithin     SpatialOperation = "within"
	SpatialOperationIntersects SpatialOperation = "intersects"
	SpatialOperationNearest    SpatialOperation = "nearest"
	SpatialOperationBuffer     SpatialOperation = "buffer"
)

func (o SpatialOperation) String() string { return string(o) }

func (o SpatialOperation) Valid() bool {
	switch o {
	case SpatialOperationWithin, SpatialOperationIntersects,
		SpatialOperationNearest, SpatialOperationBuffer:
		return true
	default:
		return false
	}
}

func ParseSpatialOperation(s string) (SpatialOperation, error) {
	op := SpatialOperation(s)
	if !op.Valid() {
		return "", fmt.Errorf("unsupported spatial operation: %s", s)
	}
	return op, nil
}

func (o *SpatialOperation) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseSpatialOperation(s)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// SpatialQueryRequest mirrors the Rust DTO. `Bounds`, `Point`, `RadiusKm`
// and `Limit` are optional (pointer types) so the handler can detect
// "field omitted" vs the zero value, matching `#[serde(default)]`.
type SpatialQueryRequest struct {
	LayerID   uuid.UUID        `json:"layer_id"`
	Operation SpatialOperation `json:"operation"`
	Bounds    *Bounds          `json:"bounds,omitempty"`
	Point     *Coordinate      `json:"point,omitempty"`
	RadiusKm  *float64         `json:"radius_km,omitempty"`
	Limit     *int             `json:"limit,omitempty"`
}

// SpatialQuerySummary mirrors the Rust struct. `NearestDistanceKm` is
// only populated for the `nearest` operation; emit `null` otherwise.
type SpatialQuerySummary struct {
	MatchedCount      int      `json:"matched_count"`
	QueryTimeMs       int32    `json:"query_time_ms"`
	NearestDistanceKm *float64 `json:"nearest_distance_km"`
	Indexed           bool     `json:"indexed"`
}

// SpatialQueryResponse mirrors the Rust struct.
type SpatialQueryResponse struct {
	Operation       SpatialOperation    `json:"operation"`
	MatchedFeatures []MapFeature        `json:"matched_features"`
	Summary         SpatialQuerySummary `json:"summary"`
	BufferRing      []Coordinate        `json:"buffer_ring"`
}

// ClusterAlgorithm mirrors the Rust enum (snake_case wire form).
type ClusterAlgorithm string

const (
	ClusterAlgorithmDBSCAN ClusterAlgorithm = "dbscan"
	ClusterAlgorithmKMeans ClusterAlgorithm = "kmeans"
)

func (a ClusterAlgorithm) String() string { return string(a) }

func (a ClusterAlgorithm) Valid() bool {
	switch a {
	case ClusterAlgorithmDBSCAN, ClusterAlgorithmKMeans:
		return true
	default:
		return false
	}
}

func ParseClusterAlgorithm(s string) (ClusterAlgorithm, error) {
	a := ClusterAlgorithm(s)
	if !a.Valid() {
		return "", fmt.Errorf("unsupported cluster algorithm: %s", s)
	}
	return a, nil
}

func (a *ClusterAlgorithm) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseClusterAlgorithm(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// ClusterRequest mirrors the Rust DTO. Both `ClusterCount` and
// `RadiusKm` are pointer types — the engine substitutes algorithm
// defaults (kmeans → 3, dbscan → 8.0 km) when omitted.
type ClusterRequest struct {
	LayerID      uuid.UUID        `json:"layer_id"`
	Algorithm    ClusterAlgorithm `json:"algorithm"`
	ClusterCount *int             `json:"cluster_count,omitempty"`
	RadiusKm     *float64         `json:"radius_km,omitempty"`
}

// ClusterSummary mirrors the Rust struct.
type ClusterSummary struct {
	ClusterID   string     `json:"cluster_id"`
	Centroid    Coordinate `json:"centroid"`
	MemberCount int        `json:"member_count"`
	Density     float64    `json:"density"`
}

// ClusterResponse mirrors the Rust struct.
type ClusterResponse struct {
	Algorithm ClusterAlgorithm `json:"algorithm"`
	Clusters  []ClusterSummary `json:"clusters"`
	Outliers  int              `json:"outliers"`
}

// TileHexBin mirrors the Rust struct emitted by `hex_aggregate`. It
// is the per-cell row of a vector-tile aggregation.
type TileHexBin struct {
	CellID    string     `json:"cell_id"`
	Centroid  Coordinate `json:"centroid"`
	Count     int        `json:"count"`
	Intensity float64    `json:"intensity"`
}

// VectorTileResponse mirrors the Rust struct returned by
// `tile_server::vector_tile`. ZoomRange uses [2]uint8 — Rust emits
// `[u8; 2]` and clients rely on the array shape.
type VectorTileResponse struct {
	LayerID          uuid.UUID    `json:"layer_id"`
	LayerName        string       `json:"layer_name"`
	TileURLTemplate  string       `json:"tile_url_template"`
	Format           string       `json:"format"`
	ZoomRange        [2]uint8     `json:"zoom_range"`
	H3Bins           []TileHexBin `json:"h3_bins"`
	FeatureCount     int          `json:"feature_count"`
}

// GeocodeRequest mirrors the Rust DTO.
type GeocodeRequest struct {
	Address string `json:"address"`
}

// ReverseGeocodeRequest mirrors the Rust DTO.
type ReverseGeocodeRequest struct {
	Coordinate Coordinate `json:"coordinate"`
}

// GeocodeResponse mirrors the Rust struct emitted by both forward and
// reverse geocoding paths.
type GeocodeResponse struct {
	Address    string     `json:"address"`
	Coordinate Coordinate `json:"coordinate"`
	Confidence float64    `json:"confidence"`
	Source     string     `json:"source"`
}

// RouteMode mirrors the Rust enum (snake_case wire form).
type RouteMode string

const (
	RouteModeDrive RouteMode = "drive"
	RouteModeBike  RouteMode = "bike"
	RouteModeWalk  RouteMode = "walk"
)

func (m RouteMode) String() string { return string(m) }

func (m RouteMode) Valid() bool {
	switch m {
	case RouteModeDrive, RouteModeBike, RouteModeWalk:
		return true
	default:
		return false
	}
}

func ParseRouteMode(s string) (RouteMode, error) {
	m := RouteMode(s)
	if !m.Valid() {
		return "", fmt.Errorf("unsupported route mode: %s", s)
	}
	return m, nil
}

func (m *RouteMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseRouteMode(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// RouteRequest mirrors the Rust DTO. `MaxMinutes` is a pointer so the
// handler can detect "field omitted" matching `#[serde(default)]`.
type RouteRequest struct {
	Origin      Coordinate `json:"origin"`
	Destination Coordinate `json:"destination"`
	Mode        RouteMode  `json:"mode"`
	MaxMinutes  *uint32    `json:"max_minutes,omitempty"`
}

// IsochronePoint mirrors the Rust struct.
type IsochronePoint struct {
	Label      string     `json:"label"`
	Coordinate Coordinate `json:"coordinate"`
	EtaMinutes uint32     `json:"eta_minutes"`
}

// RouteResponse mirrors the Rust struct.
type RouteResponse struct {
	Mode       RouteMode        `json:"mode"`
	DistanceKm float64          `json:"distance_km"`
	DurationMin uint32          `json:"duration_min"`
	Polyline   []Coordinate     `json:"polyline"`
	Isochrone  []IsochronePoint `json:"isochrone"`
}
