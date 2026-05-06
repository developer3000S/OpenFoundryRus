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

func TestCreatePromptRejectsEmptyNameOrContent(t *testing.T) {
	t.Parallel()
	h := &PromptsHandlers{Pool: nil}
	cases := []string{
		`{"name":"","content":"hello"}`,
		`{"name":"x","content":""}`,
		`{"name":"   ","content":"   "}`,
	}
	for _, payload := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		w := httptest.NewRecorder()
		h.CreatePrompt(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "payload %q should 400", payload)
		var body ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "prompt name and content are required", body.Error)
	}
}

func TestCreatePromptRejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := &PromptsHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreatePrompt(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestItoa32(t *testing.T) {
	t.Parallel()
	cases := map[int32]string{
		0: "0", 1: "1", 9: "9", 10: "10", 99: "99",
		100: "100", 12345: "12345",
		-1: "-1", -42: "-42",
	}
	for n, want := range cases {
		assert.Equal(t, want, itoa32(n))
	}
}
