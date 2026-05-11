package geospatialcore

import (
	"fmt"
	"strings"
)

type DistanceUnit string

const (
	DistanceUnitMeters     DistanceUnit = "meters"
	DistanceUnitKilometers DistanceUnit = "km"
	DistanceUnitMiles      DistanceUnit = "miles"
)

func ParseDistanceUnit(value string) (DistanceUnit, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "m", "meter", "meters", "metre", "metres":
		return DistanceUnitMeters, nil
	case "km", "kilometer", "kilometers", "kilometre", "kilometres":
		return DistanceUnitKilometers, nil
	case "mi", "mile", "miles":
		return DistanceUnitMiles, nil
	default:
		return "", fmt.Errorf("unsupported distance unit %q: expected meters, km, or miles", value)
	}
}

func HaversineDistance(lat1, lon1, lat2, lon2 float64, unit string) (float64, error) {
	if err := ValidateLatitudeLongitude(lat1, lon1); err != nil {
		return 0, fmt.Errorf("invalid start coordinate: %w", err)
	}
	if err := ValidateLatitudeLongitude(lat2, lon2); err != nil {
		return 0, fmt.Errorf("invalid end coordinate: %w", err)
	}
	parsedUnit, err := ParseDistanceUnit(unit)
	if err != nil {
		return 0, err
	}
	meters := haversineMeters(lat1, lon1, lat2, lon2)
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

func HaversineDistanceBetweenPoints(start, end GeoPoint, unit string) (float64, error) {
	return HaversineDistance(start.Lat, start.Lon, end.Lat, end.Lon, unit)
}
