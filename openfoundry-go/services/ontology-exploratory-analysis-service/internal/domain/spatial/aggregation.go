// Aggregation primitives ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/domain/engine/aggregation.rs.
// Used by `tile_server::vector_tile` to produce the H3-style hex bins
// returned alongside vector-tile metadata.

package spatial

import (
	"math"
	"sort"
	"strconv"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// HexAggregate mirrors `domain::engine::aggregation::hex_aggregate`.
// The Rust version uses a HashMap (non-deterministic) and sorts only
// by count descending — ties fall back to insertion order. To keep Go
// output reproducible across runs we sort first by count desc, then
// by cell_id ascending; the count-desc primary key matches Rust 1:1.
func HexAggregate(layer models.LayerDefinition) []models.TileHexBin {
	type bin struct {
		count           int
		latSum, lonSum  float64
	}
	bins := make(map[string]*bin)
	for _, feature := range layer.Features {
		centroid := feature.Geometry.Centroid()
		key := strconv.FormatFloat(roundToTenth(centroid.Lat), 'g', -1, 64) +
			":" +
			strconv.FormatFloat(roundToTenth(centroid.Lon), 'g', -1, 64)
		entry, ok := bins[key]
		if !ok {
			entry = &bin{}
			bins[key] = entry
		}
		entry.count++
		entry.latSum += centroid.Lat
		entry.lonSum += centroid.Lon
	}

	maxCount := 1.0
	for _, b := range bins {
		if float64(b.count) > maxCount {
			maxCount = float64(b.count)
		}
	}

	result := make([]models.TileHexBin, 0, len(bins))
	for cellID, b := range bins {
		count := b.count
		result = append(result, models.TileHexBin{
			CellID: cellID,
			Centroid: models.Coordinate{
				Lat: b.latSum / float64(count),
				Lon: b.lonSum / float64(count),
			},
			Count:     count,
			Intensity: float64(count) / maxCount,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].CellID < result[j].CellID
	})

	return result
}

// roundToTenth replicates Rust `(x * 10.0).round() / 10.0`. Encoded
// with `%v` it emits the same minimal decimal representation as Rust's
// `Display` (e.g. `1.2`, not `1.2000000000000002`).
func roundToTenth(x float64) float64 {
	return math.Round(x*10.0) / 10.0
}
