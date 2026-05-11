package geospatialcore

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	MapDisplayProjection = "Web Mercator"
)

type CoordinateOrder string

const (
	CoordinateOrderLatLon CoordinateOrder = "lat_lon"
	CoordinateOrderLonLat CoordinateOrder = "lon_lat"
)

type CRSPolicy struct {
	DefaultCRS              CRSMetadata     `json:"default_crs"`
	MapDisplayProjection    string          `json:"map_display_projection"`
	InternalCoordinateOrder CoordinateOrder `json:"internal_coordinate_order"`
	GeoJSONCoordinateOrder  CoordinateOrder `json:"geojson_coordinate_order"`
	OntologyCoordinateOrder CoordinateOrder `json:"ontology_coordinate_order"`
	SupportedCRS            []CRSMetadata   `json:"supported_crs"`
	ProjectionLimitations   []string        `json:"projection_limitations"`
}

func DefaultCRSPolicy() CRSPolicy {
	return CRSPolicy{
		DefaultCRS:              DefaultCRSMetadata(),
		MapDisplayProjection:    MapDisplayProjection,
		InternalCoordinateOrder: CoordinateOrderLatLon,
		GeoJSONCoordinateOrder:  CoordinateOrderLonLat,
		OntologyCoordinateOrder: CoordinateOrderLatLon,
		SupportedCRS:            []CRSMetadata{DefaultCRSMetadata()},
		ProjectionLimitations: []string{
			"Input data must be supplied in WGS 84 / EPSG:4326.",
			"Map rendering may project WGS 84 coordinates to Web Mercator for display only.",
			"Reprojection from non-WGS84 CRS is rejected until a dedicated projection adapter exists.",
		},
	}
}

func (p CRSPolicy) Validate() error {
	if err := p.DefaultCRS.Validate(); err != nil {
		return fmt.Errorf("default CRS is invalid: %w", err)
	}
	if _, err := ParseCoordinateOrder(string(p.InternalCoordinateOrder)); err != nil {
		return fmt.Errorf("internal coordinate order is invalid: %w", err)
	}
	if _, err := ParseCoordinateOrder(string(p.GeoJSONCoordinateOrder)); err != nil {
		return fmt.Errorf("GeoJSON coordinate order is invalid: %w", err)
	}
	if _, err := ParseCoordinateOrder(string(p.OntologyCoordinateOrder)); err != nil {
		return fmt.Errorf("Ontology coordinate order is invalid: %w", err)
	}
	if len(p.SupportedCRS) == 0 {
		return fmt.Errorf("CRS policy requires at least one supported CRS")
	}
	for _, crs := range p.SupportedCRS {
		if err := crs.Validate(); err != nil {
			return fmt.Errorf("supported CRS is invalid: %w", err)
		}
	}
	return nil
}

func ParseCRSMetadata(value string) (CRSMetadata, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DefaultCRSMetadata(), nil
	}
	upper := strings.ToUpper(strings.ReplaceAll(trimmed, "_", " "))
	switch upper {
	case "WGS84", "WGS 84", "EPSG:4326", "EPSG 4326", "4326":
		return DefaultCRSMetadata(), nil
	}
	if strings.HasPrefix(upper, "EPSG:") {
		code, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(upper, "EPSG:")))
		if err != nil {
			return CRSMetadata{}, fmt.Errorf("invalid EPSG code in %q", value)
		}
		crs := CRSMetadata{Authority: DefaultCRSAuthority, Code: code}
		if err := crs.Validate(); err != nil {
			return CRSMetadata{}, err
		}
		return crs.Normalize(), nil
	}
	return CRSMetadata{}, fmt.Errorf("unsupported CRS %q: only EPSG:4326 is currently supported", value)
}

func NormalizeCRS(crs *CRSMetadata) (CRSMetadata, error) {
	if crs == nil {
		return DefaultCRSMetadata(), nil
	}
	normalized := crs.Normalize()
	if err := normalized.Validate(); err != nil {
		return CRSMetadata{}, err
	}
	return normalized, nil
}

func ReprojectGeoPointNoop(point GeoPoint, target *CRSMetadata) (GeoPoint, error) {
	if err := ValidateLatitudeLongitude(point.Lat, point.Lon); err != nil {
		return GeoPoint{}, err
	}
	source, err := NormalizeCRS(point.CRS)
	if err != nil {
		return GeoPoint{}, fmt.Errorf("source CRS is invalid: %w", err)
	}
	destination, err := NormalizeCRS(target)
	if err != nil {
		return GeoPoint{}, fmt.Errorf("target CRS is invalid: %w", err)
	}
	if source.String() != destination.String() {
		return GeoPoint{}, fmt.Errorf("cannot reproject %s to %s: projection adapter is not configured", source.String(), destination.String())
	}
	out := point
	out.CRS = &destination
	return out, nil
}

func ParseCoordinateOrder(value string) (CoordinateOrder, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, "/", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "", "lat_lon", "latlong", "lat_lng", "latitude_longitude":
		return CoordinateOrderLatLon, nil
	case "lon_lat", "longlat", "lng_lat", "longitude_latitude", "geojson":
		return CoordinateOrderLonLat, nil
	default:
		return "", fmt.Errorf("unsupported coordinate order %q", value)
	}
}

func CoordinateOrderForLogicalType(logicalType GeospatialLogicalType) CoordinateOrder {
	normalized, err := ParseGeospatialLogicalType(string(logicalType))
	if err != nil {
		return CoordinateOrderLatLon
	}
	switch normalized {
	case LogicalTypeGeoJSON, LogicalTypeBoundingBox:
		return CoordinateOrderLonLat
	default:
		return CoordinateOrderLatLon
	}
}

func GeoPointFromOrderedPair(values []float64, order CoordinateOrder, crs *CRSMetadata) (GeoPoint, error) {
	if len(values) != 2 {
		return GeoPoint{}, fmt.Errorf("coordinate pair requires exactly 2 values, got %d", len(values))
	}
	parsedOrder, err := ParseCoordinateOrder(string(order))
	if err != nil {
		return GeoPoint{}, err
	}
	normalizedCRS, err := NormalizeCRS(crs)
	if err != nil {
		return GeoPoint{}, err
	}
	point := GeoPoint{CRS: &normalizedCRS}
	switch parsedOrder {
	case CoordinateOrderLatLon:
		point.Lat = values[0]
		point.Lon = values[1]
	case CoordinateOrderLonLat:
		point.Lat = values[1]
		point.Lon = values[0]
	default:
		return GeoPoint{}, fmt.Errorf("unsupported coordinate order %q", order)
	}
	if err := point.Validate(); err != nil {
		return GeoPoint{}, err
	}
	return point, nil
}

func OrderedPairFromGeoPoint(point GeoPoint, order CoordinateOrder) ([]float64, error) {
	projected, err := ReprojectGeoPointNoop(point, nil)
	if err != nil {
		return nil, err
	}
	parsedOrder, err := ParseCoordinateOrder(string(order))
	if err != nil {
		return nil, err
	}
	switch parsedOrder {
	case CoordinateOrderLatLon:
		return []float64{projected.Lat, projected.Lon}, nil
	case CoordinateOrderLonLat:
		return []float64{projected.Lon, projected.Lat}, nil
	default:
		return nil, fmt.Errorf("unsupported coordinate order %q", order)
	}
}
