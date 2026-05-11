package geospatialcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	DefaultCRSAuthority = "EPSG"
	DefaultCRSCode      = 4326
	DefaultCRSName      = "WGS 84"
	DefaultCRSAxisOrder = "lat_lon"
)

var h3IndexPattern = regexp.MustCompile(`^8[0-9a-fA-F]{14,15}$`)

// CRSMetadata is the explicit coordinate-reference-system contract for
// OpenFoundry geospatial logical types. The productive runtime currently
// supports WGS84 / EPSG:4326 only; other CRS values are preserved as metadata
// candidates but rejected by Validate until reprojection exists.
type CRSMetadata struct {
	Authority string `json:"authority,omitempty"`
	Code      int    `json:"code,omitempty"`
	Name      string `json:"name,omitempty"`
	AxisOrder string `json:"axis_order,omitempty"`
}

func DefaultCRSMetadata() CRSMetadata {
	return CRSMetadata{
		Authority: DefaultCRSAuthority,
		Code:      DefaultCRSCode,
		Name:      DefaultCRSName,
		AxisOrder: DefaultCRSAxisOrder,
	}
}

func (c CRSMetadata) Normalize() CRSMetadata {
	normalized := c
	normalized.Authority = strings.ToUpper(strings.TrimSpace(normalized.Authority))
	normalized.Name = strings.TrimSpace(normalized.Name)
	normalized.AxisOrder = strings.ToLower(strings.TrimSpace(normalized.AxisOrder))
	if normalized.Authority == "" {
		normalized.Authority = DefaultCRSAuthority
	}
	if normalized.Code == 0 {
		normalized.Code = DefaultCRSCode
	}
	if normalized.Name == "" && normalized.Authority == DefaultCRSAuthority && normalized.Code == DefaultCRSCode {
		normalized.Name = DefaultCRSName
	}
	if normalized.AxisOrder == "" {
		normalized.AxisOrder = DefaultCRSAxisOrder
	}
	return normalized
}

func (c CRSMetadata) Validate() error {
	normalized := c.Normalize()
	if normalized.Authority != DefaultCRSAuthority || normalized.Code != DefaultCRSCode {
		return fmt.Errorf("unsupported CRS %s:%d: only EPSG:4326 is currently supported", normalized.Authority, normalized.Code)
	}
	switch normalized.AxisOrder {
	case "lat_lon", "lon_lat":
		return nil
	default:
		return fmt.Errorf("unsupported CRS axis_order %q", normalized.AxisOrder)
	}
}

func (c CRSMetadata) String() string {
	normalized := c.Normalize()
	return fmt.Sprintf("%s:%d", normalized.Authority, normalized.Code)
}

// GeoPoint stores an ontology-compatible point in latitude/longitude degrees.
type GeoPoint struct {
	Lat float64      `json:"lat"`
	Lon float64      `json:"lon"`
	CRS *CRSMetadata `json:"crs,omitempty"`
}

func NewGeoPoint(lat, lon float64) (GeoPoint, error) {
	p := GeoPoint{Lat: lat, Lon: lon}
	if err := p.Validate(); err != nil {
		return GeoPoint{}, err
	}
	return p, nil
}

func (p GeoPoint) Validate() error {
	if err := ValidateLatitudeLongitude(p.Lat, p.Lon); err != nil {
		return err
	}
	if p.CRS != nil {
		return p.CRS.Validate()
	}
	return nil
}

// OntologyString returns the string-backed Ontology GeoPoint bridge format
// used by Pipeline Builder expressions: "lat,lon".
func (p GeoPoint) OntologyString() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	return formatFloat(p.Lat) + "," + formatFloat(p.Lon), nil
}

func ParseOntologyGeoPointString(value string) (GeoPoint, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return GeoPoint{}, errors.New("ontology GeoPoint string cannot be empty")
	}
	if strings.HasPrefix(trimmed, "{") {
		var p GeoPoint
		if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
			return GeoPoint{}, err
		}
		if err := p.Validate(); err != nil {
			return GeoPoint{}, err
		}
		return p, nil
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), "POINT(") && strings.HasSuffix(trimmed, ")") {
		body := strings.TrimSuffix(trimmed[len("POINT("):], ")")
		parts := strings.Fields(body)
		if len(parts) != 2 {
			return GeoPoint{}, fmt.Errorf("invalid POINT WKT %q", value)
		}
		lon, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return GeoPoint{}, fmt.Errorf("invalid POINT longitude %q", parts[0])
		}
		lat, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return GeoPoint{}, fmt.Errorf("invalid POINT latitude %q", parts[1])
		}
		return NewGeoPoint(lat, lon)
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(parts) != 2 {
		return GeoPoint{}, fmt.Errorf("invalid ontology GeoPoint string %q", value)
	}
	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return GeoPoint{}, fmt.Errorf("invalid latitude %q", parts[0])
	}
	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return GeoPoint{}, fmt.Errorf("invalid longitude %q", parts[1])
	}
	return NewGeoPoint(lat, lon)
}

func (p *GeoPoint) UnmarshalJSON(data []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if obj == nil {
		return errors.New("geo point must be a JSON object")
	}
	latRaw, ok := pickRaw(obj, "lat", "latitude")
	if !ok {
		return errors.New("geo point requires lat or latitude")
	}
	lonRaw, ok := pickRaw(obj, "lon", "longitude")
	if !ok {
		return errors.New("geo point requires lon or longitude")
	}
	var lat, lon float64
	if err := json.Unmarshal(latRaw, &lat); err != nil {
		return fmt.Errorf("geo point latitude must be numeric: %w", err)
	}
	if err := json.Unmarshal(lonRaw, &lon); err != nil {
		return fmt.Errorf("geo point longitude must be numeric: %w", err)
	}
	var crs *CRSMetadata
	if crsRaw, ok := obj["crs"]; ok && len(bytes.TrimSpace(crsRaw)) > 0 && !bytes.Equal(bytes.TrimSpace(crsRaw), []byte("null")) {
		var parsed CRSMetadata
		if err := json.Unmarshal(crsRaw, &parsed); err != nil {
			return fmt.Errorf("geo point CRS is invalid: %w", err)
		}
		crs = &parsed
	}
	parsed := GeoPoint{Lat: lat, Lon: lon, CRS: crs}
	if err := parsed.Validate(); err != nil {
		return err
	}
	*p = parsed
	return nil
}

func ValidateLatitudeLongitude(lat, lon float64) error {
	if !finite(lat) || !finite(lon) {
		return errors.New("latitude and longitude must be finite numbers")
	}
	if lat < -90 || lat > 90 {
		return fmt.Errorf("latitude %.12g out of range [-90, 90]", lat)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("longitude %.12g out of range [-180, 180]", lon)
	}
	return nil
}

type BoundingBox struct {
	MinLat float64      `json:"min_lat"`
	MinLon float64      `json:"min_lon"`
	MaxLat float64      `json:"max_lat"`
	MaxLon float64      `json:"max_lon"`
	CRS    *CRSMetadata `json:"crs,omitempty"`
}

func NewBoundingBox(minLat, minLon, maxLat, maxLon float64) (BoundingBox, error) {
	b := BoundingBox{MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon}
	if err := b.Validate(); err != nil {
		return BoundingBox{}, err
	}
	return b, nil
}

func (b BoundingBox) Validate() error {
	if err := ValidateLatitudeLongitude(b.MinLat, b.MinLon); err != nil {
		return fmt.Errorf("invalid min corner: %w", err)
	}
	if err := ValidateLatitudeLongitude(b.MaxLat, b.MaxLon); err != nil {
		return fmt.Errorf("invalid max corner: %w", err)
	}
	if b.MinLat > b.MaxLat {
		return fmt.Errorf("min_lat %.12g cannot be greater than max_lat %.12g", b.MinLat, b.MaxLat)
	}
	if b.MinLon > b.MaxLon {
		return fmt.Errorf("min_lon %.12g cannot be greater than max_lon %.12g", b.MinLon, b.MaxLon)
	}
	if b.CRS != nil {
		return b.CRS.Validate()
	}
	return nil
}

// GeoJSONBBox returns the standard [minLon, minLat, maxLon, maxLat] order.
func (b BoundingBox) GeoJSONBBox() ([]float64, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return []float64{b.MinLon, b.MinLat, b.MaxLon, b.MaxLat}, nil
}

func BoundingBoxFromGeoJSONBBox(values []float64) (BoundingBox, error) {
	if len(values) != 4 {
		return BoundingBox{}, fmt.Errorf("GeoJSON bbox requires 4 values, got %d", len(values))
	}
	return NewBoundingBox(values[1], values[0], values[3], values[2])
}

type GeoJSONGeometryType string

const (
	GeoJSONGeometryTypePoint      GeoJSONGeometryType = "Point"
	GeoJSONGeometryTypeLineString GeoJSONGeometryType = "LineString"
	GeoJSONGeometryTypePolygon    GeoJSONGeometryType = "Polygon"
)

func ParseGeoJSONGeometryType(value string) (GeoJSONGeometryType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "point":
		return GeoJSONGeometryTypePoint, nil
	case "line_string", "linestring":
		return GeoJSONGeometryTypeLineString, nil
	case "polygon":
		return GeoJSONGeometryTypePolygon, nil
	default:
		return "", fmt.Errorf("unsupported GeoJSON geometry type %q", value)
	}
}

func (t GeoJSONGeometryType) Valid() bool {
	_, err := ParseGeoJSONGeometryType(string(t))
	return err == nil
}

func (t *GeoJSONGeometryType) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseGeoJSONGeometryType(value)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

type GeoJSONGeometry struct {
	Type        GeoJSONGeometryType `json:"type"`
	Coordinates json.RawMessage     `json:"coordinates"`
	CRS         *CRSMetadata        `json:"crs,omitempty"`
	BBox        []float64           `json:"bbox,omitempty"`
}

func NewGeoJSONPoint(point GeoPoint) (GeoJSONGeometry, error) {
	if err := point.Validate(); err != nil {
		return GeoJSONGeometry{}, err
	}
	raw, err := json.Marshal([]float64{point.Lon, point.Lat})
	if err != nil {
		return GeoJSONGeometry{}, err
	}
	return GeoJSONGeometry{Type: GeoJSONGeometryTypePoint, Coordinates: raw, CRS: point.CRS}, nil
}

func NewGeoJSONLineString(points []GeoPoint) (GeoJSONGeometry, error) {
	if len(points) < 2 {
		return GeoJSONGeometry{}, fmt.Errorf("GeoJSON LineString requires at least 2 positions")
	}
	coords := make([][]float64, 0, len(points))
	for _, point := range points {
		if err := point.Validate(); err != nil {
			return GeoJSONGeometry{}, err
		}
		coords = append(coords, []float64{point.Lon, point.Lat})
	}
	raw, err := json.Marshal(coords)
	if err != nil {
		return GeoJSONGeometry{}, err
	}
	return GeoJSONGeometry{Type: GeoJSONGeometryTypeLineString, Coordinates: raw}, nil
}

func ParseGeoJSONGeometry(data []byte) (GeoJSONGeometry, error) {
	var geom GeoJSONGeometry
	if err := json.Unmarshal(data, &geom); err != nil {
		return GeoJSONGeometry{}, err
	}
	if err := geom.Validate(); err != nil {
		return GeoJSONGeometry{}, err
	}
	return geom, nil
}

func (g GeoJSONGeometry) Validate() error {
	parsedType, err := ParseGeoJSONGeometryType(string(g.Type))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(g.Coordinates)) == 0 {
		return errors.New("GeoJSON geometry requires coordinates")
	}
	if g.CRS != nil {
		if err := g.CRS.Validate(); err != nil {
			return err
		}
	}
	if len(g.BBox) > 0 {
		if _, err := BoundingBoxFromGeoJSONBBox(g.BBox); err != nil {
			return err
		}
	}
	switch parsedType {
	case GeoJSONGeometryTypePoint:
		var pos []float64
		if err := json.Unmarshal(g.Coordinates, &pos); err != nil {
			return fmt.Errorf("invalid GeoJSON Point coordinates: %w", err)
		}
		_, err := geoPointFromPosition(pos)
		return err
	case GeoJSONGeometryTypeLineString:
		var line [][]float64
		if err := json.Unmarshal(g.Coordinates, &line); err != nil {
			return fmt.Errorf("invalid GeoJSON LineString coordinates: %w", err)
		}
		if len(line) < 2 {
			return fmt.Errorf("GeoJSON LineString requires at least 2 positions")
		}
		for i, pos := range line {
			if _, err := geoPointFromPosition(pos); err != nil {
				return fmt.Errorf("invalid LineString position %d: %w", i, err)
			}
		}
		return nil
	case GeoJSONGeometryTypePolygon:
		var polygon [][][]float64
		if err := json.Unmarshal(g.Coordinates, &polygon); err != nil {
			return fmt.Errorf("invalid GeoJSON Polygon coordinates: %w", err)
		}
		if len(polygon) == 0 {
			return fmt.Errorf("GeoJSON Polygon requires at least 1 ring")
		}
		for ringIndex, ring := range polygon {
			if len(ring) < 4 {
				return fmt.Errorf("GeoJSON Polygon ring %d requires at least 4 positions", ringIndex)
			}
			for pointIndex, pos := range ring {
				if _, err := geoPointFromPosition(pos); err != nil {
					return fmt.Errorf("invalid Polygon ring %d position %d: %w", ringIndex, pointIndex, err)
				}
			}
			if !samePosition(ring[0], ring[len(ring)-1]) {
				return fmt.Errorf("GeoJSON Polygon ring %d must be closed", ringIndex)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported GeoJSON geometry type %q", g.Type)
	}
}

func (g GeoJSONGeometry) PointCoordinates() ([]GeoPoint, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	switch g.Type {
	case GeoJSONGeometryTypePoint:
		var pos []float64
		if err := json.Unmarshal(g.Coordinates, &pos); err != nil {
			return nil, err
		}
		p, err := geoPointFromPosition(pos)
		if err != nil {
			return nil, err
		}
		return []GeoPoint{p}, nil
	case GeoJSONGeometryTypeLineString:
		var line [][]float64
		if err := json.Unmarshal(g.Coordinates, &line); err != nil {
			return nil, err
		}
		points := make([]GeoPoint, 0, len(line))
		for _, pos := range line {
			p, err := geoPointFromPosition(pos)
			if err != nil {
				return nil, err
			}
			points = append(points, p)
		}
		return points, nil
	case GeoJSONGeometryTypePolygon:
		var polygon [][][]float64
		if err := json.Unmarshal(g.Coordinates, &polygon); err != nil {
			return nil, err
		}
		points := make([]GeoPoint, 0, len(polygon[0]))
		for _, pos := range polygon[0] {
			p, err := geoPointFromPosition(pos)
			if err != nil {
				return nil, err
			}
			points = append(points, p)
		}
		return points, nil
	default:
		return nil, fmt.Errorf("unsupported GeoJSON geometry type %q", g.Type)
	}
}

type H3Index string

func (h H3Index) Validate() error {
	value := strings.TrimSpace(string(h))
	if value == "" {
		return errors.New("H3 index cannot be empty")
	}
	if !h3IndexPattern.MatchString(value) {
		return fmt.Errorf("invalid H3 index %q: expected a 15-16 character H3 cell index", value)
	}
	parsed, err := strconv.ParseUint(value, 16, 64)
	if err != nil {
		return fmt.Errorf("invalid H3 index %q: %w", value, err)
	}
	if parsed == 0 {
		return fmt.Errorf("invalid H3 index %q: zero is not a cell index", value)
	}
	return nil
}

type GeospatialLogicalType string

const (
	LogicalTypeGeoPoint    GeospatialLogicalType = "geo_point"
	LogicalTypeGeometry    GeospatialLogicalType = "geometry"
	LogicalTypeGeoJSON     GeospatialLogicalType = "geojson"
	LogicalTypeBoundingBox GeospatialLogicalType = "bounding_box"
	LogicalTypeH3Index     GeospatialLogicalType = "h3_index"
	LogicalTypeCRSMetadata GeospatialLogicalType = "crs_metadata"
)

func ParseGeospatialLogicalType(value string) (GeospatialLogicalType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "geopoint", "geo_point", "ontology_geo_point":
		return LogicalTypeGeoPoint, nil
	case "geometry", "geoshape", "geo_shape", "geobuf":
		return LogicalTypeGeometry, nil
	case "geojson", "geo_json":
		return LogicalTypeGeoJSON, nil
	case "bbox", "bounding_box", "lat_lon_bounding_box":
		return LogicalTypeBoundingBox, nil
	case "h3", "h3index", "h3_index":
		return LogicalTypeH3Index, nil
	case "crs", "crs_metadata":
		return LogicalTypeCRSMetadata, nil
	default:
		return "", fmt.Errorf("unsupported geospatial logical type %q", value)
	}
}

func (t GeospatialLogicalType) Validate() error {
	_, err := ParseGeospatialLogicalType(string(t))
	return err
}

func (t GeospatialLogicalType) OntologyPropertyType() string {
	normalized, err := ParseGeospatialLogicalType(string(t))
	if err != nil {
		return ""
	}
	switch normalized {
	case LogicalTypeGeoPoint:
		return "geo_point"
	case LogicalTypeGeometry, LogicalTypeGeoJSON, LogicalTypeBoundingBox:
		return "json"
	case LogicalTypeH3Index, LogicalTypeCRSMetadata:
		return "string"
	default:
		return ""
	}
}

func (t GeospatialLogicalType) StorageType() string {
	normalized, err := ParseGeospatialLogicalType(string(t))
	if err != nil {
		return ""
	}
	switch normalized {
	case LogicalTypeGeoPoint:
		return "object"
	case LogicalTypeGeometry, LogicalTypeGeoJSON, LogicalTypeBoundingBox:
		return "json"
	case LogicalTypeH3Index, LogicalTypeCRSMetadata:
		return "string"
	default:
		return ""
	}
}

type FieldMetadata struct {
	LogicalType          GeospatialLogicalType `json:"logical_type"`
	CRS                  *CRSMetadata          `json:"crs,omitempty"`
	CoordinateOrder      CoordinateOrder       `json:"coordinate_order,omitempty"`
	OntologyPropertyType string                `json:"ontology_property_type,omitempty"`
}

func (m FieldMetadata) Validate() error {
	normalized, err := ParseGeospatialLogicalType(string(m.LogicalType))
	if err != nil {
		return err
	}
	if m.CRS != nil {
		if err := m.CRS.Validate(); err != nil {
			return err
		}
	}
	if m.CoordinateOrder != "" {
		order, err := ParseCoordinateOrder(string(m.CoordinateOrder))
		if err != nil {
			return err
		}
		expected := CoordinateOrderForLogicalType(normalized)
		if order != expected {
			return fmt.Errorf("logical type %q expects coordinate_order %q, got %q", normalized, expected, order)
		}
	}
	if m.OntologyPropertyType != "" && m.OntologyPropertyType != normalized.OntologyPropertyType() {
		return fmt.Errorf("logical type %q maps to ontology property type %q, got %q", normalized, normalized.OntologyPropertyType(), m.OntologyPropertyType)
	}
	return nil
}

func geoPointFromPosition(pos []float64) (GeoPoint, error) {
	if len(pos) < 2 {
		return GeoPoint{}, fmt.Errorf("GeoJSON position requires [lon, lat], got %d values", len(pos))
	}
	return NewGeoPoint(pos[1], pos[0])
}

func samePosition(a, b []float64) bool {
	if len(a) < 2 || len(b) < 2 {
		return false
	}
	return a[0] == b[0] && a[1] == b[1]
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func pickRaw(obj map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, key := range keys {
		if raw, ok := obj[key]; ok {
			return raw, true
		}
	}
	return nil, false
}
