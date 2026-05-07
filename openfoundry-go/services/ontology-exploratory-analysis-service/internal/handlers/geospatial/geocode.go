// Geocoding handlers ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/handlers/geocode.rs.
// Forward and reverse lookups delegate to the in-process gazetteer in
// internal/domain/geocoding (no DB / no external geocoder).

package geospatial

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/domain/geocoding"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// ForwardGeocode mirrors `handlers::geocode::forward_geocode`.
func (s *AppState) ForwardGeocode(w http.ResponseWriter, r *http.Request) {
	var req models.GeocodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	writeJSON(w, http.StatusOK, geocoding.Forward(req.Address))
}

// ReverseGeocode mirrors `handlers::geocode::reverse_geocode`.
func (s *AppState) ReverseGeocode(w http.ResponseWriter, r *http.Request) {
	var req models.ReverseGeocodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writeJSON(w, http.StatusOK, geocoding.Reverse(req.Coordinate))
}
