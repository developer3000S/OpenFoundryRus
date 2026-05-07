// Routing primitives ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/domain/engine/routing.rs.
// Returns a synthetic route + isochrone derived from the great-circle
// midpoint — no real OSRM/Valhalla integration. Mirrors the Rust impl
// byte-for-byte so client wire shapes align.

package spatial

import (
	"fmt"
	"math"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// Route mirrors `domain::engine::routing::route`.
func Route(request models.RouteRequest) models.RouteResponse {
	distanceKm := request.Origin.DistanceKm(request.Destination)
	speedKmh := 58.0
	switch request.Mode {
	case models.RouteModeBike:
		speedKmh = 18.0
	case models.RouteModeWalk:
		speedKmh = 5.0
	case models.RouteModeDrive:
		speedKmh = 58.0
	}
	// Rust: `((distance_km / speed_kmh) * 60.0).ceil().max(1.0) as u32`.
	// `f64::max(1.0)` clamps to ≥1 minute before the u32 cast.
	durationMin := uint32(math.Max(math.Ceil((distanceKm/speedKmh)*60.0), 1.0))
	midpoint := models.Coordinate{
		Lat: (request.Origin.Lat+request.Destination.Lat)/2.0 + 0.04,
		Lon: (request.Origin.Lon+request.Destination.Lon)/2.0 - 0.03,
	}
	// Rust: `request.max_minutes.unwrap_or(duration_min.max(20))`. The
	// fallback uses `u32::max` (max of duration_min and 20).
	var maxMinutes uint32
	if request.MaxMinutes != nil {
		maxMinutes = *request.MaxMinutes
	} else {
		maxMinutes = durationMin
		if maxMinutes < 20 {
			maxMinutes = 20
		}
	}
	// Rust: `(max_minutes / 3).max(5)` — integer division, then max.
	step := maxMinutes / 3
	if step < 5 {
		step = 5
	}

	return models.RouteResponse{
		Mode:        request.Mode,
		DistanceKm:  distanceKm,
		DurationMin: durationMin,
		Polyline:    []models.Coordinate{request.Origin, midpoint, request.Destination},
		Isochrone: []models.IsochronePoint{
			{
				Label:      "10 min",
				Coordinate: routeOffset(request.Origin, 0.03, 0.01),
				EtaMinutes: step,
			},
			{
				Label:      "20 min",
				Coordinate: routeOffset(request.Origin, -0.02, 0.04),
				EtaMinutes: step * 2,
			},
			{
				Label:      fmt.Sprintf("%d min", step*3),
				Coordinate: routeOffset(request.Origin, 0.01, -0.05),
				EtaMinutes: step * 3,
			},
		},
	}
}

func routeOffset(origin models.Coordinate, latDelta, lonDelta float64) models.Coordinate {
	return models.Coordinate{
		Lat: origin.Lat + latDelta,
		Lon: origin.Lon + lonDelta,
	}
}
