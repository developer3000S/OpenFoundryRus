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

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

func TestCreateProvider_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"   "}`))
	w := httptest.NewRecorder()
	h.CreateProvider(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "provider name is required", body.Error)
}

func TestEvaluateGuardrails_PureLogicPath(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"content":"Send the report to alice@example.com please"}`))
	w := httptest.NewRecorder()
	h.EvaluateGuardrails(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.EvaluateGuardrailsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotEmpty(t, resp.Verdict.Flags, "email content should produce at least one PII flag")
	piiFlagSeen := false
	for _, f := range resp.Verdict.Flags {
		if strings.HasPrefix(f.Kind, "pii_") {
			piiFlagSeen = true
		}
	}
	assert.True(t, piiFlagSeen, "expected pii_* flag for email content")
	assert.Contains(t, resp.Recommendations, "Redact PII before routing prompts to external LLM providers.")
}

func TestEvaluateGuardrails_RejectsEmptyContent(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"content":"   "}`))
	w := httptest.NewRecorder()
	h.EvaluateGuardrails(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "guardrail evaluation requires content", body.Error)
}

func TestEvaluateGuardrails_NoIssuesPath(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"content":"hello world how are you"}`))
	w := httptest.NewRecorder()
	h.EvaluateGuardrails(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.EvaluateGuardrailsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp.Recommendations, "No blocking issues detected; response is safe to continue.")
}

func TestCreateChatCompletion_RejectsEmptyMessage(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"user_message":"   "}`))
	w := httptest.NewRecorder()
	h.CreateChatCompletion(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAskCopilot_RejectsEmptyQuestion(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"question":"   "}`))
	w := httptest.NewRecorder()
	h.AskCopilot(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBenchmarkProviders_RejectsEmptyPrompt(t *testing.T) {
	t.Parallel()
	h := &ChatHandlers{Pool: nil}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"   ","use_case":"chat","max_tokens":1024}`))
	w := httptest.NewRecorder()
	h.BenchmarkProviders(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSummarizeTitleFallbacks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "New conversation", summarizeTitle(""))
	assert.Equal(t, "New conversation", summarizeTitle("   "))
	short := strings.Repeat("a", 60)
	assert.Equal(t, short, summarizeTitle(short))
	long := strings.Repeat("a", 61)
	assert.Equal(t, strings.Repeat("a", 60)+"...", summarizeTitle(long))
}

func TestConversationSummaryUsesLastMessage(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	c := models.Conversation{
		ID: id,
		Messages: []models.ChatMessage{
			{Content: "first"},
			{Content: "second"},
		},
	}
	got := conversationSummary(c)
	assert.Equal(t, "second", got.LastMessagePreview)
	assert.Equal(t, int32(2), got.MessageCount)

	empty := models.Conversation{ID: id}
	got2 := conversationSummary(empty)
	assert.Equal(t, "No messages yet", got2.LastMessagePreview)
	assert.Equal(t, int32(0), got2.MessageCount)
}
