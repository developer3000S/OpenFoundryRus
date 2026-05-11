// Tile-server primitive ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/domain/tile_server.rs.
// Wraps `HexAggregate` + the layer's static metadata into the vector-
// tile envelope returned by the `/tiles/{id}` handler.

package spatial

import (
	"fmt"
	"math"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

const (
	DefaultViewportTileLimit = 500
	MaxViewportTileLimit     = 5000
)

// ViewportTileOptions controls the JSON feature-page tile mode. Bounds are
// required by the handler; other fields are clamped here so tests can exercise
// the domain primitive directly without duplicating HTTP parsing rules.
type ViewportTileOptions struct {
	Bounds            models.Bounds
	Zoom              float64
	Limit             int
	Offset            int
	SimplifyTolerance float64
}

// VectorTile mirrors `tile_server::vector_tile`.
func VectorTile(layer models.LayerDefinition) models.VectorTileResponse {
	return models.VectorTileResponse{
		LayerID:         layer.ID,
		LayerName:       layer.Name,
		TileURLTemplate: fmt.Sprintf("/api/v1/geospatial/tiles/%s?z={z}&x={x}&y={y}", layer.ID),
		Format:          "mvt",
		ZoomRange:       [2]uint8{4, 14},
		H3Bins:          HexAggregate(layer),
		FeatureCount:    len(layer.Features),
	}
}

// ViewportTileFeatures returns a bounded, paginated and optionally simplified
// feature page for large Workshop/Map layers.
func ViewportTileFeatures(layer models.LayerDefinition, options ViewportTileOptions) models.ViewportTileFeaturePage {
	limit := clampInt(options.Limit, 1, MaxViewportTileLimit, DefaultViewportTileLimit)
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	tolerance := options.SimplifyTolerance
	if tolerance < 0 {
		tolerance = 0
	}

	matching := make([]models.MapFeature, 0, minInt(len(layer.Features), limit))
	totalMatching := 0
	for _, feature := range layer.Features {
		if !feature.Geometry.BoundsOf().Intersects(options.Bounds) {
			continue
		}
		totalMatching++
		if totalMatching <= offset {
			continue
		}
		if len(matching) >= limit {
			continue
		}
		matching = append(matching, models.MapFeature{
			ID:         feature.ID,
			Label:      feature.Label,
			Geometry:   SimplifyGeometry(feature.Geometry, tolerance),
			Properties: feature.Properties,
		})
	}

	var nextOffset *int
	if offset+len(matching) < totalMatching {
		next := offset + len(matching)
		nextOffset = &next
	}

	return models.ViewportTileFeaturePage{
		LayerID:            layer.ID,
		LayerName:          layer.Name,
		Bounds:             options.Bounds,
		Zoom:               options.Zoom,
		SimplifyTolerance:  tolerance,
		Limit:              limit,
		Offset:             offset,
		NextOffset:         nextOffset,
		TotalMatchingCount: totalMatching,
		ReturnedCount:      len(matching),
		Features:           matching,
	}
}

func SimplifyGeometry(geometry models.Geometry, tolerance float64) models.Geometry {
	if tolerance <= 0 {
		return geometry
	}
	switch geometry.Type {
	case models.GeometryTypeLineString:
		points := simplifyCoordinates(geometry.LineString, tolerance, 2)
		return models.Geometry{Type: models.GeometryTypeLineString, LineString: points}
	case models.GeometryTypePolygon:
		closed := isClosedRing(geometry.Polygon)
		points := geometry.Polygon
		if closed && len(points) > 1 {
			points = points[:len(points)-1]
		}
		points = simplifyCoordinates(points, tolerance, 3)
		if len(points) < 3 {
			return geometry
		}
		if closed {
			points = append(points, points[0])
		}
		return models.Geometry{Type: models.GeometryTypePolygon, Polygon: points}
	default:
		return geometry
	}
}

func simplifyCoordinates(points []models.Coordinate, tolerance float64, minPoints int) []models.Coordinate {
	if len(points) <= minPoints {
		return cloneCoordinates(points)
	}
	simplified := douglasPeucker(points, tolerance)
	if len(simplified) < minPoints {
		return cloneCoordinates(points)
	}
	return simplified
}

func douglasPeucker(points []models.Coordinate, tolerance float64) []models.Coordinate {
	if len(points) <= 2 {
		return cloneCoordinates(points)
	}
	maxDistance := 0.0
	index := 0
	start := points[0]
	end := points[len(points)-1]
	for i := 1; i < len(points)-1; i++ {
		d := perpendicularDistance(points[i], start, end)
		if d > maxDistance {
			maxDistance = d
			index = i
		}
	}
	if maxDistance <= tolerance {
		return []models.Coordinate{start, end}
	}
	left := douglasPeucker(points[:index+1], tolerance)
	right := douglasPeucker(points[index:], tolerance)
	return append(left[:len(left)-1], right...)
}

func perpendicularDistance(point, start, end models.Coordinate) float64 {
	x := point.Lon
	y := point.Lat
	x1 := start.Lon
	y1 := start.Lat
	x2 := end.Lon
	y2 := end.Lat
	dx := x2 - x1
	dy := y2 - y1
	if dx == 0 && dy == 0 {
		return math.Hypot(x-x1, y-y1)
	}
	return math.Abs(dy*x-dx*y+x2*y1-y2*x1) / math.Hypot(dx, dy)
}

func isClosedRing(points []models.Coordinate) bool {
	if len(points) < 2 {
		return false
	}
	first := points[0]
	last := points[len(points)-1]
	return first.Lat == last.Lat && first.Lon == last.Lon
}

func cloneCoordinates(points []models.Coordinate) []models.Coordinate {
	if len(points) == 0 {
		return nil
	}
	out := make([]models.Coordinate, len(points))
	copy(out, points)
	return out
}

func clampInt(value, min, max, fallback int) int {
	if value == 0 {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
