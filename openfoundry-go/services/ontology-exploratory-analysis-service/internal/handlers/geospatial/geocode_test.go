package geospatial

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func TestForwardGeocodeRejectsEmptyAddress(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	rec := postJSON(t, state.ForwardGeocode, models.GeocodeRequest{Address: "   "})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "address is required")
}

func TestForwardGeocodeRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`not json`)))
	rec := httptest.NewRecorder()
	state.ForwardGeocode(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}

func TestForwardGeocodeReturnsGazetteerHit(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	rec := postJSON(t, state.ForwardGeocode, models.GeocodeRequest{Address: "Madrid"})
	require.Equal(t, http.StatusOK, rec.Code)
	var got models.GeocodeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Madrid", got.Address)
	assert.Equal(t, "reference gazetteer", got.Source)
	assert.Equal(t, 0.96, got.Confidence)
}

func TestReverseGeocodeReturnsNearestKnown(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	rec := postJSON(t, state.ReverseGeocode, models.ReverseGeocodeRequest{
		Coordinate: models.Coordinate{Lat: 48.85, Lon: 2.35},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var got models.GeocodeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Paris", got.Address)
	assert.Equal(t, "reverse gazetteer", got.Source)
	assert.Equal(t, 0.91, got.Confidence)
}

func TestReverseGeocodeRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{`)))
	rec := httptest.NewRecorder()
	state.ReverseGeocode(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}
