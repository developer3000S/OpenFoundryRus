// Package handlers exposes the HTTP surface of agent-runtime-service.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/copilot"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/llm"
	aimodels "github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/repo"
)

type Handlers struct {
	Repo                 *repo.Repo
	Runtime              llm.Runtime
	Provider             *aimodels.LlmProvider
	AllowFakeLLMProvider bool
	PurposeCheckpoint    *authmw.PurposeCheckpointClient
}

func (h *Handlers) completionRuntime() llm.Runtime {
	if h.Runtime != nil {
		return h.Runtime
	}
	return llm.HTTPRuntime{}
}

var errFakeLLMProviderDisabled = errors.New("configuration error: no LLM provider configured and ALLOW_FAKE_LLM_PROVIDER is not true")

func (h *Handlers) completionProvider() (*aimodels.LlmProvider, error) {
	if h.Provider != nil {
		return h.Provider, nil
	}
	if !h.AllowFakeLLMProvider {
		return nil, errFakeLLMProviderDisabled
	}
	provider := fakeAgentRuntimeProvider()
	return &provider, nil
}

func fakeAgentRuntimeProvider() aimodels.LlmProvider {
	return aimodels.LlmProvider{
		ID:              uuid.MustParse("00000000-0000-0000-0000-00000000a501"),
		Name:            "agent-runtime-fake",
		ProviderType:    "fake",
		ModelName:       "agent-runtime-default",
		EndpointURL:     "fake://agent-runtime",
		APIMode:         "fake",
		Enabled:         true,
		MaxOutputTokens: 1024,
		RouteRules:      aimodels.DefaultProviderRoutingRules(),
	}
}

func (h *Handlers) enforceChatPurposeCheckpoint(ctx context.Context, justification *string, requirePrivateNetwork bool, provider *aimodels.LlmProvider) error {
	if h.PurposeCheckpoint == nil || !requirePrivateNetwork {
		return nil
	}
	providerName := ""
	networkScope := ""
	if provider != nil {
		providerName = provider.Name
		networkScope = provider.RouteRules.NetworkScope
	}
	return h.PurposeCheckpoint.Enforce(ctx, authmw.PurposeCheckpointRequest{
		InteractionType:         "ai_chat_completion",
		ActorID:                 actorIDFromContext(ctx),
		PurposeJustification:    justification,
		RequestedPrivateNetwork: requirePrivateNetwork,
		Tags:                    []string{"ai", "chat", "private-network"},
		Evidence: mustJSONRaw(map[string]any{
			"service":       "agent-runtime-service",
			"provider_name": providerName,
			"network_scope": networkScope,
		}),
	})
}

func actorIDFromContext(ctx context.Context) *uuid.UUID {
	claims, ok := authmw.FromContext(ctx)
	if !ok || claims == nil {
		return nil
	}
	id := claims.Sub
	return &id
}

func mustJSONRaw(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func writePurposeCheckpointError(w http.ResponseWriter, err error) {
	var denied *authmw.PurposeCheckpointDeniedError
	if errors.As(err, &denied) {
		writeError(w, http.StatusForbidden, denied.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Repo.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var body models.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	agent, err := h.Repo.CreateAgent(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (h *Handlers) GetAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	agent, err := h.Repo.GetAgent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handlers) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	agent, err := h.Repo.UpdateAgent(r.Context(), id, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	rows, err := h.Repo.ListRuns(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) StartRun(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.StartRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := h.Repo.StartRun(r.Context(), agentID, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *Handlers) RecordStep(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "run_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "run_id must be a uuid")
		return
	}
	var body models.RecordStepRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	step, err := h.Repo.RecordStep(r.Context(), runID, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (h *Handlers) SubmitHumanApproval(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "run_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "run_id must be a uuid")
		return
	}
	var body models.HumanApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := json.Marshal(map[string]any{
		"decision":    body.Decision,
		"reviewer_id": body.ReviewerID,
		"note":        body.Note,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	step, err := h.Repo.RecordHumanApproval(r.Context(), runID, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

// CreateChatCompletion exposes an OpenAI-compatible chat completion route
// backed by libs/ai-kernel-go's provider runtime. Production wiring must
// inject a real LlmProvider unless ALLOW_FAKE_LLM_PROVIDER explicitly enables
// the local/test fake provider fallback.
func (h *Handlers) CreateChatCompletion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature           *float32 `json:"temperature"`
		MaxTokens             *int32   `json:"max_tokens"`
		PurposeJustification  *string  `json:"purpose_justification"`
		RequirePrivateNetwork bool     `json:"require_private_network"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	systemPrompt, userPrompt := "", ""
	for _, msg := range body.Messages {
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			if strings.TrimSpace(msg.Content) != "" {
				if systemPrompt != "" {
					systemPrompt += "\n\n"
				}
				systemPrompt += msg.Content
			}
		case "user":
			if strings.TrimSpace(msg.Content) != "" {
				if userPrompt != "" {
					userPrompt += "\n\n"
				}
				userPrompt += msg.Content
			}
		}
	}
	if strings.TrimSpace(userPrompt) == "" {
		writeError(w, http.StatusBadRequest, "chat completion requires a user message")
		return
	}
	provider, err := h.completionProvider()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	model := provider.ModelName
	if strings.TrimSpace(body.Model) != "" {
		model = body.Model
	}
	temperature := aimodels.DefaultTemperature
	if body.Temperature != nil {
		temperature = *body.Temperature
	}
	maxTokens := aimodels.DefaultMaxTokens
	if body.MaxTokens != nil {
		maxTokens = *body.MaxTokens
	}
	if err := h.enforceChatPurposeCheckpoint(r.Context(), body.PurposeJustification, body.RequirePrivateNetwork, provider); err != nil {
		writePurposeCheckpointError(w, err)
		return
	}
	completion, err := h.completionRuntime().CompleteText(r.Context(), llm.CompletionRequest{
		Provider: provider, SystemPrompt: systemPrompt, UserPrompt: userPrompt,
		Temperature: temperature, MaxTokens: maxTokens,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp := map[string]any{
		"id":      uuid.New(),
		"object":  "chat.completion",
		"model":   model,
		"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": completion.Text}, "finish_reason": "stop"}},
		"usage":   map[string]int32{"prompt_tokens": completion.PromptTokens, "completion_tokens": completion.CompletionTokens, "total_tokens": completion.TotalTokens},
	}
	writeJSON(w, http.StatusOK, resp)
}

// AskCopilot exposes the Rust-compatible copilot request/response shape
// backed by the ai-kernel copilot draft helper and injectable LLM runtime.
func (h *Handlers) AskCopilot(w http.ResponseWriter, r *http.Request) {
	var body aimodels.CopilotRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Question) == "" {
		writeError(w, http.StatusBadRequest, "copilot question is required")
		return
	}

	provider, err := h.completionProvider()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	citedKnowledge := []aimodels.KnowledgeSearchResult{}
	draft := copilot.Assist(
		body.Question,
		body.DatasetIDs,
		body.OntologyTypeIDs,
		citedKnowledge,
		body.IncludeSQL,
		body.IncludePipelinePlan,
	)
	userPrompt := copilot.BuildPrompt(
		body.Question,
		draft,
		body.DatasetIDs,
		body.OntologyTypeIDs,
		body.KnowledgeBaseIDs,
		citedKnowledge,
	)
	if body.PurposeJustification != nil && strings.TrimSpace(*body.PurposeJustification) != "" {
		userPrompt += "\nPurpose justification: " + strings.TrimSpace(*body.PurposeJustification)
	}

	maxTokens := provider.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = aimodels.DefaultMaxTokens
	}
	if maxTokens > 512 {
		maxTokens = 512
	}
	startedAt := time.Now()
	completion, err := h.completionRuntime().CompleteText(r.Context(), llm.CompletionRequest{
		Provider:     provider,
		SystemPrompt: copilot.SystemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  aimodels.DefaultTemperature,
		MaxTokens:    maxTokens,
	})
	latencyMs := int32(time.Since(startedAt).Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	promptTokens := completion.PromptTokens
	if promptTokens <= 0 {
		promptTokens = llm.EstimateTokens(copilot.SystemPrompt + " " + userPrompt)
	}
	completionTokens := completion.CompletionTokens
	if completionTokens <= 0 {
		completionTokens = llm.EstimateTokens(completion.Text)
	}
	totalTokens := completion.TotalTokens
	if totalTokens <= 0 {
		totalTokens = promptTokens + completionTokens
	}
	usage := aimodels.LlmUsageSummary{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		EstimatedCostUSD: estimateCompletionCost(provider, promptTokens, completionTokens),
		LatencyMs:        latencyMs,
		NetworkScope:     provider.RouteRules.NetworkScope,
		CacheHit:         false,
	}

	writeJSON(w, http.StatusOK, aimodels.CopilotResponse{
		Answer:              completion.Text,
		SuggestedSQL:        draft.SuggestedSQL,
		PipelineSuggestions: draft.PipelineSuggestions,
		OntologyHints:       draft.OntologyHints,
		CitedKnowledge:      citedKnowledge,
		ProviderName:        provider.Name,
		Cache: aimodels.SemanticCacheMetadata{
			CacheKey:        "",
			Hit:             false,
			SimilarityScore: 0,
		},
		Usage:     usage,
		CreatedAt: time.Now().UTC(),
	})
}

func estimateCompletionCost(provider *aimodels.LlmProvider, promptTokens, completionTokens int32) float32 {
	if provider == nil {
		return 0
	}
	return (float32(promptTokens)/1000)*provider.RouteRules.InputCostPer1KTokensUSD +
		(float32(completionTokens)/1000)*provider.RouteRules.OutputCostPer1KTokensUSD
}
