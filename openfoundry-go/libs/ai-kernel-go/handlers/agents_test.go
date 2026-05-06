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

func TestCreateAgent_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	h := &AgentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"   "}`))
	w := httptest.NewRecorder()
	h.CreateAgent(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "agent name is required", body.Error)
}

func TestCreateAgent_RejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &AgentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreateAgent(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExecuteAgent_RejectsEmptyMessage(t *testing.T) {
	t.Parallel()
	h := &AgentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"user_message":"   "}`))
	w := httptest.NewRecorder()
	h.ExecuteAgent(w, req, uuid.New())
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "agent execution requires a user message", body.Error)
}

func TestExecuteAgent_RejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &AgentsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.ExecuteAgent(w, req, uuid.New())
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
