// Vector-tile handler ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/handlers/tiles.rs.
// Looks up a layer by id and returns the tile envelope produced by
// `spatial.VectorTile` (h3-style hex bins + zoom range + layer metadata).

package geospatial

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/domain/spatial"
)

// GetVectorTile mirrors `handlers::tiles::get_vector_tile`.
func (s *AppState) GetVectorTile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid layer id")
		return
	}
	layer, ok, err := LoadLayerRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "layer not found")
		return
	}
	writeJSON(w, http.StatusOK, spatial.VectorTile(layer))
}
