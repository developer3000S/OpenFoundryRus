package geospatial

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// QueryFeatures and ClusterFeatures fail fast on payload validation
// before reaching the database — these tests cover the request-shape
// guards. End-to-end tests against a real layer live in the
// integration suite (requires Postgres).

func postJSON(t *testing.T, h http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestQueryFeaturesRejectsMissingBoundsForWithin(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	rec := postJSON(t, state.QueryFeatures, models.SpatialQueryRequest{
		Operation: models.SpatialOperationWithin,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "bounds are required")
}

func TestQueryFeaturesRejectsMissingPointForNearest(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	rec := postJSON(t, state.QueryFeatures, models.SpatialQueryRequest{
		Operation: models.SpatialOperationNearest,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "point is required")
}

func TestQueryFeaturesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`not json`)))
	rec := httptest.NewRecorder()
	state.QueryFeatures(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}

func TestClusterFeaturesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{`)))
	rec := httptest.NewRecorder()
	state.ClusterFeatures(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}

func TestRoutesIncludesNewSpatialEndpoints(t *testing.T) {
	t.Parallel()
	router := (&AppState{}).Routes()
	seen := map[string]bool{}
	walker := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen[method+" "+route] = true
		return nil
	}
	require.NoError(t, chi.Walk(router, walker))
	assert.True(t, seen["POST /query"], "POST /query missing: %v", seen)
	assert.True(t, seen["POST /clusters"], "POST /clusters missing: %v", seen)
	assert.True(t, seen["POST /cluster"], "POST /cluster missing: %v", seen)
}
