package geospatial

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestGetVectorTileRejectsInvalidUUID(t *testing.T) {
	t.Parallel()
	state := &AppState{}
	r := chi.NewRouter()
	r.Get("/tiles/{id}", state.GetVectorTile)

	req := httptest.NewRequest(http.MethodGet, "/tiles/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid layer id")
}

func TestRoutesIncludesTilesAndGeocode(t *testing.T) {
	t.Parallel()
	router := (&AppState{}).Routes()
	seen := map[string]bool{}
	walker := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen[method+" "+route] = true
		return nil
	}
	if err := chi.Walk(router, walker); err != nil {
		t.Fatalf("walk: %v", err)
	}
	assert.True(t, seen["GET /tiles/{id}"], "GET /tiles/{id} missing: %v", seen)
	assert.True(t, seen["POST /geocode"], "POST /geocode missing: %v", seen)
	assert.True(t, seen["POST /geocode/reverse"], "POST /geocode/reverse missing: %v", seen)
}
