package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFeature_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	h := &FeaturesHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"   ","entity_name":"x"}`))
	w := httptest.NewRecorder()
	h.CreateFeature(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "feature name and entity name are required", body.Error)
}

func TestCreateFeature_RejectsEmptyEntity(t *testing.T) {
	t.Parallel()
	h := &FeaturesHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","entity_name":"  "}`))
	w := httptest.NewRecorder()
	h.CreateFeature(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateFeature_RejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &FeaturesHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreateFeature(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDerefStringFallback(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fallback", derefString(nil, "fallback"))
	v := "value"
	assert.Equal(t, "value", derefString(&v, "fallback"))
}

