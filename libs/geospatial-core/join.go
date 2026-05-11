package geospatialcore

import (
	"encoding/json"
	"fmt"
	"math"
)

const geometryEpsilon = 1e-10

func GeoJSONIntersects(left, right GeoJSONGeometry) (bool, error) {
	leftBounds, err := GeoJSONBounds(left)
	if err != nil {
		return false, err
	}
	rightBounds, err := GeoJSONBounds(right)
	if err != nil {
		return false, err
	}
	if !boundsIntersect(leftBounds, rightBounds) {
		return false, nil
	}
	leftShape, err := geometryShape(left)
	if err != nil {
		return false, err
	}
	rightShape, err := geometryShape(right)
	if err != nil {
		return false, err
	}
	return shapesIntersect(leftShape, rightShape), nil
}

func GeoJSONDistance(left, right GeoJSONGeometry, unit string) (float64, error) {
	intersects, err := GeoJSONIntersects(left, right)
	if err != nil {
		return 0, err
	}
	if intersects {
		return 0, nil
	}
	leftShape, err := geometryShape(left)
	if err != nil {
		return 0, err
	}
	rightShape, err := geometryShape(right)
	if err != nil {
		return 0, err
	}
	minDistanceMeters := minGeometryDistanceMeters(leftShape, rightShape)
	if math.IsInf(minDistanceMeters, 1) {
		return 0, fmt.Errorf("geometry distance requires at least one coordinate on each side")
	}
	return convertMetersToUnit(minDistanceMeters, unit)
}

func GeoJSONBounds(geometry GeoJSONGeometry) (BoundingBox, error) {
	points, err := geometry.PointCoordinates()
	if err != nil {
		return BoundingBox{}, err
	}
	if len(points) == 0 {
		return BoundingBox{}, fmt.Errorf("geometry bounds require at least one coordinate")
	}
	minLat, minLon := points[0].Lat, points[0].Lon
	maxLat, maxLon := points[0].Lat, points[0].Lon
	for _, point := range points[1:] {
		minLat = math.Min(minLat, point.Lat)
		minLon = math.Min(minLon, point.Lon)
		maxLat = math.Max(maxLat, point.Lat)
		maxLon = math.Max(maxLon, point.Lon)
	}
	return NewBoundingBox(minLat, minLon, maxLat, maxLon)
}

type planarPoint struct {
	x float64
	y float64
}

type geoShape struct {
	typ    GeoJSONGeometryType
	points []planarPoint
}

func geometryShape(geometry GeoJSONGeometry) (geoShape, error) {
	points, err := geometry.PointCoordinates()
	if err != nil {
		return geoShape{}, err
	}
	out := geoShape{typ: geometry.Type, points: make([]planarPoint, 0, len(points))}
	for _, point := range points {
		out.points = append(out.points, planarPoint{x: point.Lon, y: point.Lat})
	}
	return out, nil
}

func shapesIntersect(left, right geoShape) bool {
	if left.typ == GeoJSONGeometryTypePoint {
		return pointIntersectsShape(left.points[0], right)
	}
	if right.typ == GeoJSONGeometryTypePoint {
		return pointIntersectsShape(right.points[0], left)
	}
	for _, ls := range segments(left.points, left.typ == GeoJSONGeometryTypePolygon) {
		for _, rs := range segments(right.points, right.typ == GeoJSONGeometryTypePolygon) {
			if segmentsIntersect(ls[0], ls[1], rs[0], rs[1]) {
				return true
			}
		}
	}
	if left.typ == GeoJSONGeometryTypePolygon && len(right.points) > 0 && pointInRing(right.points[0], left.points) {
		return true
	}
	if right.typ == GeoJSONGeometryTypePolygon && len(left.points) > 0 && pointInRing(left.points[0], right.points) {
		return true
	}
	return false
}

func minGeometryDistanceMeters(left, right geoShape) float64 {
	minDistance := math.Inf(1)
	for _, lp := range left.points {
		for _, rp := range right.points {
			minDistance = math.Min(minDistance, haversineMeters(lp.y, lp.x, rp.y, rp.x))
		}
	}
	leftSegments := shapeSegments(left)
	rightSegments := shapeSegments(right)
	for _, point := range left.points {
		for _, segment := range rightSegments {
			minDistance = math.Min(minDistance, pointToSegmentDistanceMeters(point, segment[0], segment[1]))
		}
	}
	for _, point := range right.points {
		for _, segment := range leftSegments {
			minDistance = math.Min(minDistance, pointToSegmentDistanceMeters(point, segment[0], segment[1]))
		}
	}
	return minDistance
}

func pointIntersectsShape(point planarPoint, shape geoShape) bool {
	switch shape.typ {
	case GeoJSONGeometryTypePoint:
		return samePlanarPoint(point, shape.points[0])
	case GeoJSONGeometryTypeLineString:
		for _, segment := range segments(shape.points, false) {
			if pointOnSegment(point, segment[0], segment[1]) {
				return true
			}
		}
		return false
	case GeoJSONGeometryTypePolygon:
		return pointInRing(point, shape.points)
	default:
		return false
	}
}

func shapeSegments(shape geoShape) [][2]planarPoint {
	return segments(shape.points, shape.typ == GeoJSONGeometryTypePolygon)
}

func segments(points []planarPoint, closed bool) [][2]planarPoint {
	if len(points) < 2 {
		return nil
	}
	out := make([][2]planarPoint, 0, len(points))
	for i := 1; i < len(points); i++ {
		out = append(out, [2]planarPoint{points[i-1], points[i]})
	}
	if closed && !samePlanarPoint(points[0], points[len(points)-1]) {
		out = append(out, [2]planarPoint{points[len(points)-1], points[0]})
	}
	return out
}

func pointToSegmentDistanceMeters(point, a, b planarPoint) float64 {
	refLat := degreesToRadians((point.y + a.y + b.y) / 3)
	px, py := projectedMeters(point, refLat)
	ax, ay := projectedMeters(a, refLat)
	bx, by := projectedMeters(b, refLat)
	dx := bx - ax
	dy := by - ay
	if math.Abs(dx) <= geometryEpsilon && math.Abs(dy) <= geometryEpsilon {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	closestX := ax + t*dx
	closestY := ay + t*dy
	return math.Hypot(px-closestX, py-closestY)
}

func projectedMeters(point planarPoint, refLatRad float64) (float64, float64) {
	return earthRadiusM * degreesToRadians(point.x) * math.Cos(refLatRad),
		earthRadiusM * degreesToRadians(point.y)
}

func convertMetersToUnit(meters float64, unit string) (float64, error) {
	parsedUnit, err := ParseDistanceUnit(unit)
	if err != nil {
		return 0, err
	}
	switch parsedUnit {
	case DistanceUnitMeters:
		return meters, nil
	case DistanceUnitKilometers:
		return meters / 1000, nil
	case DistanceUnitMiles:
		return meters * metersToMiles, nil
	default:
		return 0, fmt.Errorf("unsupported distance unit %q", unit)
	}
}

func segmentsIntersect(a, b, c, d planarPoint) bool {
	o1 := orientation(a, b, c)
	o2 := orientation(a, b, d)
	o3 := orientation(c, d, a)
	o4 := orientation(c, d, b)
	if o1 != o2 && o3 != o4 {
		return true
	}
	return (o1 == 0 && pointOnSegment(c, a, b)) ||
		(o2 == 0 && pointOnSegment(d, a, b)) ||
		(o3 == 0 && pointOnSegment(a, c, d)) ||
		(o4 == 0 && pointOnSegment(b, c, d))
}

func orientation(a, b, c planarPoint) int {
	value := (b.y-a.y)*(c.x-b.x) - (b.x-a.x)*(c.y-b.y)
	if math.Abs(value) <= geometryEpsilon {
		return 0
	}
	if value > 0 {
		return 1
	}
	return 2
}

func pointOnSegment(point, a, b planarPoint) bool {
	cross := (point.y-a.y)*(b.x-a.x) - (point.x-a.x)*(b.y-a.y)
	if math.Abs(cross) > geometryEpsilon {
		return false
	}
	return point.x <= math.Max(a.x, b.x)+geometryEpsilon &&
		point.x+geometryEpsilon >= math.Min(a.x, b.x) &&
		point.y <= math.Max(a.y, b.y)+geometryEpsilon &&
		point.y+geometryEpsilon >= math.Min(a.y, b.y)
}

func pointInRing(point planarPoint, ring []planarPoint) bool {
	if len(ring) < 3 {
		return false
	}
	inside := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a := ring[i]
		b := ring[j]
		if pointOnSegment(point, a, b) {
			return true
		}
		intersects := (a.y > point.y) != (b.y > point.y) &&
			point.x < (b.x-a.x)*(point.y-a.y)/((b.y-a.y)+math.SmallestNonzeroFloat64)+a.x
		if intersects {
			inside = !inside
		}
	}
	return inside
}

func samePlanarPoint(left, right planarPoint) bool {
	return math.Abs(left.x-right.x) <= geometryEpsilon && math.Abs(left.y-right.y) <= geometryEpsilon
}

func boundsIntersect(left, right BoundingBox) bool {
	return left.MinLat <= right.MaxLat &&
		left.MaxLat >= right.MinLat &&
		left.MinLon <= right.MaxLon &&
		left.MaxLon >= right.MinLon
}

func GeoJSONGeometryJSONString(geometry GeoJSONGeometry) string {
	raw, _ := json.Marshal(geometry)
	return string(raw)
}
