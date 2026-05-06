package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateExperiment_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"   "}`))
	w := httptest.NewRecorder()
	h.CreateExperiment(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "experiment name is required", body.Error)
}

func TestCreateExperiment_RejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreateExperiment(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListRuns_Stub501(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ListRuns(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestCreateRun_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"   ","experiment_id":"`+uuid.New().String()+`"}`))
	w := httptest.NewRecorder()
	h.CreateRun(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "run name is required", body.Error)
}

func TestCompareRuns_RejectsEmptyRunIDs(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"run_ids":[]}`))
	w := httptest.NewRecorder()
	h.CompareRuns(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "at least one run id is required", body.Error)
}

func TestCompareRuns_RejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &ExperimentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CompareRuns(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
