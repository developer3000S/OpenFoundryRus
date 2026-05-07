// Tile-server primitive ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/domain/tile_server.rs.
// Wraps `HexAggregate` + the layer's static metadata into the vector-
// tile envelope returned by the `/tiles/{id}` handler.

package spatial

import (
	"fmt"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

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
