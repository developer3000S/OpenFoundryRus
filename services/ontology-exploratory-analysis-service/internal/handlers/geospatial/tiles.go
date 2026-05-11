// Vector-tile handler ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/handlers/tiles.rs.
// Looks up a layer by id and returns the tile envelope produced by
// `spatial.VectorTile` (h3-style hex bins + zoom range + layer metadata).

package geospatial

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/domain/spatial"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
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

// GetViewportTileFeatures serves the JSON tile-loading mode consumed by
// Workshop Map. It filters a saved geospatial layer by viewport bounds, clamps
// pagination and returns simplified geometries when requested.
func (s *AppState) GetViewportTileFeatures(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid layer id")
		return
	}

	bounds, err := parseBoundsQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := bounds.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid bounds: %v", err))
		return
	}
	zoom, err := optionalFloatQuery(r, "zoom", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := optionalIntQuery(r, "limit", spatial.DefaultViewportTileLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := optionalIntQuery(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tolerance, err := optionalFloatQuery(r, "simplify_tolerance", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	writeJSON(w, http.StatusOK, spatial.ViewportTileFeatures(layer, spatial.ViewportTileOptions{
		Bounds:            bounds,
		Zoom:              zoom,
		Limit:             limit,
		Offset:            offset,
		SimplifyTolerance: tolerance,
	}))
}

func parseBoundsQuery(r *http.Request) (models.Bounds, error) {
	minLat, err := requiredFloatQuery(r, "min_lat")
	if err != nil {
		return models.Bounds{}, err
	}
	minLon, err := requiredFloatQuery(r, "min_lon")
	if err != nil {
		return models.Bounds{}, err
	}
	maxLat, err := requiredFloatQuery(r, "max_lat")
	if err != nil {
		return models.Bounds{}, err
	}
	maxLon, err := requiredFloatQuery(r, "max_lon")
	if err != nil {
		return models.Bounds{}, err
	}
	return models.Bounds{MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon}, nil
}

func requiredFloatQuery(r *http.Request, name string) (float64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	return value, nil
}

func optionalFloatQuery(r *http.Request, name string, fallback float64) (float64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	return value, nil
}

func optionalIntQuery(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}
