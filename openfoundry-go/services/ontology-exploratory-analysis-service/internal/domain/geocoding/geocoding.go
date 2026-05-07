// Package geocoding implements the in-process gazetteer ported 1:1
// from services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/domain/geocoding.rs.
// Forward and reverse lookups consult the same fixed lookup table; on
// miss `Forward` falls back to a deterministic byte-sum hash so client
// behavior remains stable across runs.
package geocoding

import (
	"strings"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// known is the gazetteer the Rust source declares as `KNOWN_LOCATIONS`.
// Order is significant — `Reverse` does a min-by-distance scan and a
// stable iteration order keeps tie-breaks reproducible (Rust uses
// `Iterator::min_by` whose tie-break preserves first occurrence).
var known = []struct {
	label      string
	coordinate models.Coordinate
}{
	{"madrid", models.Coordinate{Lat: 40.4168, Lon: -3.7038}},
	{"barcelona", models.Coordinate{Lat: 41.3874, Lon: 2.1686}},
	{"paris", models.Coordinate{Lat: 48.8566, Lon: 2.3522}},
	{"berlin", models.Coordinate{Lat: 52.52, Lon: 13.405}},
	{"lisbon", models.Coordinate{Lat: 38.7223, Lon: -9.1393}},
	{"london", models.Coordinate{Lat: 51.5072, Lon: -0.1276}},
	{"new york", models.Coordinate{Lat: 40.7128, Lon: -74.006}},
}

// Forward mirrors `geocoding::forward`. The address is lowercased and
// matched as a substring against the gazetteer; on miss it produces a
// deterministic fallback derived from the byte sum.
func Forward(address string) models.GeocodeResponse {
	normalized := strings.ToLower(strings.TrimSpace(address))
	for _, entry := range known {
		if strings.Contains(normalized, entry.label) {
			return models.GeocodeResponse{
				Address:    titleCase(entry.label),
				Coordinate: entry.coordinate,
				Confidence: 0.96,
				Source:     "reference gazetteer",
			}
		}
	}

	var hash uint64
	for _, b := range []byte(normalized) {
		hash += uint64(b)
	}
	return models.GeocodeResponse{
		Address: address,
		Coordinate: models.Coordinate{
			Lat: 35.0 + float64(hash%240)/10.0,
			Lon: -20.0 + float64(hash%400)/10.0,
		},
		Confidence: 0.68,
		Source:     "deterministic fallback",
	}
}

// Reverse mirrors `geocoding::reverse`. It scans the gazetteer for the
// nearest known location by `Coordinate.DistanceKm` and returns the
// canonical entry — confidence and source are constant per the Rust
// contract.
func Reverse(coordinate models.Coordinate) models.GeocodeResponse {
	// Rust's `expect("known locations cannot be empty")` panics if the
	// table is empty; replicating that invariant is a compile-time
	// guarantee here since `known` is a package-level non-empty slice.
	bestIdx := 0
	bestDist := coordinate.DistanceKm(known[0].coordinate)
	for i := 1; i < len(known); i++ {
		d := coordinate.DistanceKm(known[i].coordinate)
		if d < bestDist {
			bestIdx = i
			bestDist = d
		}
	}

	entry := known[bestIdx]
	return models.GeocodeResponse{
		Address:    titleCase(entry.label),
		Coordinate: entry.coordinate,
		Confidence: 0.91,
		Source:     "reverse gazetteer",
	}
}

// titleCase mirrors the Rust helper: split on whitespace, ASCII-
// uppercase the first byte of each token (`char::to_ascii_uppercase`),
// then rejoin with single spaces.
func titleCase(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		first := part[0]
		if first >= 'a' && first <= 'z' {
			first -= 'a' - 'A'
		}
		parts[i] = string(first) + part[1:]
	}
	return strings.Join(parts, " ")
}
