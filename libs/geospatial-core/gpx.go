package geospatialcore

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	metersToMiles = 0.000621371192237334
	metersToFeet  = 3.280839895013123
	earthRadiusM  = 6371008.8
)

var nonSlugCharacterPattern = regexp.MustCompile(`[^a-z0-9]+`)

type GPXParseOptions struct {
	TrailID    string
	TrailName  string
	SourceName string
}

type GPXTrackPoint struct {
	Lat             float64  `json:"lat"`
	Lon             float64  `json:"lon"`
	ElevationMeters *float64 `json:"elevation_meters,omitempty"`
}

type GPXTrail struct {
	TrailID             string          `json:"trail_id"`
	TrailName           string          `json:"trail_name"`
	PointCount          int             `json:"point_count"`
	DistanceMeters      float64         `json:"distance_meters"`
	DistanceMiles       float64         `json:"distance_miles"`
	ElevationGainMeters float64         `json:"elevation_gain_meters"`
	ElevationGainFeet   float64         `json:"elevation_gain_ft"`
	MinElevationMeters  *float64        `json:"min_elevation_meters,omitempty"`
	MaxElevationMeters  *float64        `json:"max_elevation_meters,omitempty"`
	MinElevationFeet    *float64        `json:"min_elevation_ft,omitempty"`
	MaxElevationFeet    *float64        `json:"max_elevation_ft,omitempty"`
	StartPoint          GeoPoint        `json:"start_point"`
	EndPoint            GeoPoint        `json:"end_point"`
	RouteBBox           BoundingBox     `json:"route_bbox"`
	RouteGeoJSON        GeoJSONGeometry `json:"route_geojson"`
	Points              []GPXTrackPoint `json:"points,omitempty"`
}

func (t GPXTrail) TrailheadOntologyString() (string, error) {
	return t.StartPoint.OntologyString()
}

func (t GPXTrail) RouteGeoJSONString() (string, error) {
	if err := t.RouteGeoJSON.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(t.RouteGeoJSON)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (t GPXTrail) RouteBBoxJSONString() (string, error) {
	values, err := t.RouteBBox.GeoJSONBBox()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ParseGPXTrail(data []byte, opts GPXParseOptions) (GPXTrail, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return GPXTrail{}, errors.New("GPX document cannot be empty")
	}
	var doc gpxDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return GPXTrail{}, fmt.Errorf("parse GPX XML: %w", err)
	}

	segments, discoveredNames, err := doc.trackSegments()
	if err != nil {
		return GPXTrail{}, err
	}
	points := flattenGPXSegments(segments)
	if len(points) < 2 {
		return GPXTrail{}, fmt.Errorf("GPX trail requires at least 2 track or route points, got %d", len(points))
	}

	geoPoints := make([]GeoPoint, 0, len(points))
	minLat, maxLat := points[0].Lat, points[0].Lat
	minLon, maxLon := points[0].Lon, points[0].Lon
	var minEle, maxEle *float64
	for i, point := range points {
		geoPoint, err := NewGeoPoint(point.Lat, point.Lon)
		if err != nil {
			return GPXTrail{}, fmt.Errorf("invalid GPX point %d: %w", i, err)
		}
		geoPoints = append(geoPoints, geoPoint)
		if point.Lat < minLat {
			minLat = point.Lat
		}
		if point.Lat > maxLat {
			maxLat = point.Lat
		}
		if point.Lon < minLon {
			minLon = point.Lon
		}
		if point.Lon > maxLon {
			maxLon = point.Lon
		}
		if point.ElevationMeters != nil {
			if minEle == nil || *point.ElevationMeters < *minEle {
				minEle = floatPtr(*point.ElevationMeters)
			}
			if maxEle == nil || *point.ElevationMeters > *maxEle {
				maxEle = floatPtr(*point.ElevationMeters)
			}
		}
	}

	distanceM, gainM := measureGPXSegments(segments)
	bbox, err := NewBoundingBox(minLat, minLon, maxLat, maxLon)
	if err != nil {
		return GPXTrail{}, err
	}
	line, err := NewGeoJSONLineString(geoPoints)
	if err != nil {
		return GPXTrail{}, err
	}
	line.BBox, err = bbox.GeoJSONBBox()
	if err != nil {
		return GPXTrail{}, err
	}

	trailName := firstNonEmptyString(append([]string{opts.TrailName}, discoveredNames...)...)
	if trailName == "" {
		trailName = sourceNameWithoutExtension(opts.SourceName)
	}
	if trailName == "" {
		trailName = "GPX Trail"
	}
	trailID := strings.TrimSpace(opts.TrailID)
	if trailID == "" {
		trailID = slugTrailName(trailName)
	}

	trail := GPXTrail{
		TrailID:             trailID,
		TrailName:           trailName,
		PointCount:          len(points),
		DistanceMeters:      distanceM,
		DistanceMiles:       distanceM * metersToMiles,
		ElevationGainMeters: gainM,
		ElevationGainFeet:   gainM * metersToFeet,
		MinElevationMeters:  minEle,
		MaxElevationMeters:  maxEle,
		StartPoint:          geoPoints[0],
		EndPoint:            geoPoints[len(geoPoints)-1],
		RouteBBox:           bbox,
		RouteGeoJSON:        line,
		Points:              append([]GPXTrackPoint(nil), points...),
	}
	if minEle != nil {
		trail.MinElevationFeet = floatPtr(*minEle * metersToFeet)
	}
	if maxEle != nil {
		trail.MaxElevationFeet = floatPtr(*maxEle * metersToFeet)
	}
	return trail, nil
}

type gpxDocument struct {
	Metadata gpxMetadata `xml:"metadata"`
	Tracks   []gpxTrack  `xml:"trk"`
	Routes   []gpxRoute  `xml:"rte"`
}

type gpxMetadata struct {
	Name string `xml:"name"`
}

type gpxTrack struct {
	Name     string            `xml:"name"`
	Segments []gpxTrackSegment `xml:"trkseg"`
}

type gpxTrackSegment struct {
	Points []gpxXMLPoint `xml:"trkpt"`
}

type gpxRoute struct {
	Name   string        `xml:"name"`
	Points []gpxXMLPoint `xml:"rtept"`
}

type gpxXMLPoint struct {
	Lat string `xml:"lat,attr"`
	Lon string `xml:"lon,attr"`
	Ele string `xml:"ele"`
}

func (d gpxDocument) trackSegments() ([][]GPXTrackPoint, []string, error) {
	segments := [][]GPXTrackPoint{}
	names := []string{strings.TrimSpace(d.Metadata.Name)}
	for _, track := range d.Tracks {
		names = append(names, strings.TrimSpace(track.Name))
		for _, segment := range track.Segments {
			points, err := parseGPXXMLPoints(segment.Points)
			if err != nil {
				return nil, nil, err
			}
			if len(points) > 0 {
				segments = append(segments, points)
			}
		}
	}
	for _, route := range d.Routes {
		names = append(names, strings.TrimSpace(route.Name))
		points, err := parseGPXXMLPoints(route.Points)
		if err != nil {
			return nil, nil, err
		}
		if len(points) > 0 {
			segments = append(segments, points)
		}
	}
	return segments, names, nil
}

func parseGPXXMLPoints(raw []gpxXMLPoint) ([]GPXTrackPoint, error) {
	points := make([]GPXTrackPoint, 0, len(raw))
	for i, point := range raw {
		lat, err := parseGPXFloat(point.Lat, "latitude")
		if err != nil {
			return nil, fmt.Errorf("invalid GPX point %d: %w", i, err)
		}
		lon, err := parseGPXFloat(point.Lon, "longitude")
		if err != nil {
			return nil, fmt.Errorf("invalid GPX point %d: %w", i, err)
		}
		parsed := GPXTrackPoint{Lat: lat, Lon: lon}
		if strings.TrimSpace(point.Ele) != "" {
			ele, err := parseGPXFloat(point.Ele, "elevation")
			if err != nil {
				return nil, fmt.Errorf("invalid GPX point %d: %w", i, err)
			}
			parsed.ElevationMeters = floatPtr(ele)
		}
		points = append(points, parsed)
	}
	return points, nil
}

func parseGPXFloat(value, label string) (float64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("%s is required", label)
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not numeric", label, value)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s must be finite", label)
	}
	return parsed, nil
}

func flattenGPXSegments(segments [][]GPXTrackPoint) []GPXTrackPoint {
	points := []GPXTrackPoint{}
	for _, segment := range segments {
		points = append(points, segment...)
	}
	return points
}

func measureGPXSegments(segments [][]GPXTrackPoint) (float64, float64) {
	var distanceM float64
	var gainM float64
	for _, segment := range segments {
		for i := 1; i < len(segment); i++ {
			prev, current := segment[i-1], segment[i]
			distanceM += haversineMeters(prev.Lat, prev.Lon, current.Lat, current.Lon)
			if prev.ElevationMeters != nil && current.ElevationMeters != nil {
				if delta := *current.ElevationMeters - *prev.ElevationMeters; delta > 0 {
					gainM += delta
				}
			}
		}
	}
	return distanceM, gainM
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad, lat2Rad := degreesToRadians(lat1), degreesToRadians(lat2)
	dLat := degreesToRadians(lat2 - lat1)
	dLon := degreesToRadians(lon2 - lon1)
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	a := sinLat*sinLat + math.Cos(lat1Rad)*math.Cos(lat2Rad)*sinLon*sinLon
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func sourceNameWithoutExtension(source string) string {
	base := strings.TrimSpace(filepath.Base(source))
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return strings.TrimSpace(base)
}

func slugTrailName(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = nonSlugCharacterPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "gpx-trail"
	}
	return value
}

func floatPtr(value float64) *float64 {
	return &value
}
