package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/evaluation"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/llm"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// ChatHandlers exposes the chat / providers / conversations / guardrails
// surface mirroring libs/ai-kernel/src/handlers/chat.rs. This slice
// ports the read-only + provider-CRUD + guardrails-evaluation paths;
// the chat-completion / copilot / benchmark paths chain
// llm/runtime.complete_text + the cache/usage write paths and land in
// a follow-up slice once the full runtime port is in.
//
// The 501 stubs preserve input-validation contracts so consumers wiring
// this surface today can rely on shape-stable error semantics.
type ChatHandlers struct {
	Pool *pgxpool.Pool
}

// GetOverview handles `GET /api/v1/overview` — aggregate counts +
// derived metrics for the AI platform landing card.
func (h *ChatHandlers) GetOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	count := func(sql string) (int64, error) {
		var n int64
		err := h.Pool.QueryRow(ctx, sql).Scan(&n)
		return n, err
	}
	costSum := func() (float64, error) {
		var c float64
		err := h.Pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM ai_llm_usage_events`).Scan(&c)
		return c, err
	}

	type counterFn func() (int64, error)
	counters := []struct {
		name string
		fn   counterFn
	}{
		{"providers", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_providers`) }},
		{"private", func() (int64, error) {
			return count(`SELECT COUNT(*) FROM ai_providers WHERE COALESCE(route_rules->>'network_scope', 'public') IN ('private', 'hybrid', 'local')`)
		}},
		{"multimodal", func() (int64, error) {
			return count(`SELECT COUNT(*) FROM ai_providers WHERE COALESCE(route_rules->'supported_modalities', '[]'::jsonb) ? 'image'`)
		}},
		{"prompts", func() (int64, error) {
			return count(`SELECT COUNT(*) FROM ai_prompt_templates WHERE status = 'active'`)
		}},
		{"kbs", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_knowledge_bases`) }},
		{"docs", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_knowledge_documents`) }},
		{"chunks", func() (int64, error) {
			return count(`SELECT COALESCE(SUM(chunk_count), 0) FROM ai_knowledge_documents`)
		}},
		{"agents", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_agents`) }},
		{"conversations", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_conversations`) }},
		{"cacheEntries", func() (int64, error) { return count(`SELECT COUNT(*) FROM ai_semantic_cache`) }},
		{"cacheHits", func() (int64, error) {
			return count(`SELECT COALESCE(SUM(hit_count), 0) FROM ai_semantic_cache`)
		}},
		{"blocked", func() (int64, error) {
			return count(`SELECT COUNT(*) FROM ai_conversations WHERE last_guardrail_blocked = TRUE`)
		}},
		{"promptTokens", func() (int64, error) {
			return count(`SELECT COALESCE(SUM(prompt_tokens), 0) FROM ai_llm_usage_events`)
		}},
		{"completionTokens", func() (int64, error) {
			return count(`SELECT COALESCE(SUM(completion_tokens), 0) FROM ai_llm_usage_events`)
		}},
		{"benchmarkRuns", func() (int64, error) {
			return count(`SELECT COUNT(DISTINCT benchmark_group_id) FROM ai_llm_usage_events WHERE benchmark_group_id IS NOT NULL`)
		}},
	}

	values := make(map[string]int64, len(counters))
	for _, c := range counters {
		v, err := c.fn()
		if err != nil {
			dbError(w, err)
			return
		}
		values[c.name] = v
	}
	cost, err := costSum()
	if err != nil {
		dbError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.AiPlatformOverview{
		ProviderCount:           values["providers"],
		PrivateProviderCount:    values["private"],
		MultimodalProviderCount: values["multimodal"],
		PromptCount:             values["prompts"],
		KnowledgeBaseCount:      values["kbs"],
		IndexedDocumentCount:    values["docs"],
		IndexedChunkCount:       values["chunks"],
		AgentCount:              values["agents"],
		ConversationCount:       values["conversations"],
		CacheEntryCount:         values["cacheEntries"],
		CacheHitRate:            evaluation.CacheHitRate(values["cacheEntries"], values["cacheHits"]),
		BlockedGuardrailEvents:  values["blocked"],
		LlmPromptTokens:         values["promptTokens"],
		LlmCompletionTokens:     values["completionTokens"],
		EstimatedLlmCostUSD:     cost,
		BenchmarkRunCount:       values["benchmarkRuns"],
	})
}

// ListProviders handles `GET /api/v1/providers`.
func (h *ChatHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+providerColumns+` FROM ai_providers
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.LlmProvider, 0)
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, models.ListProvidersResponse{Data: out})
}

// CreateProvider handles `POST /api/v1/providers`.
func (h *ChatHandlers) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var body models.CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "provider name is required")
		return
	}

	providerType := derefString(body.ProviderType, models.DefaultProviderType)
	modelName := derefString(body.ModelName, models.DefaultModelName)
	endpointURL := derefString(body.EndpointURL, models.DefaultEndpointURL)
	apiMode := derefString(body.APIMode, models.DefaultAPIMode)
	costTier := derefString(body.CostTier, models.DefaultCostTier)

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	loadBalanceWeight := models.DefaultLoadBalanceWeight
	if body.LoadBalanceWeight != nil {
		loadBalanceWeight = *body.LoadBalanceWeight
	}
	maxOutputTokens := models.DefaultMaxOutputTokens
	if body.MaxOutputTokens != nil {
		maxOutputTokens = *body.MaxOutputTokens
	}
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	routeRules := models.DefaultProviderRoutingRules()
	if body.RouteRules != nil {
		routeRules = *body.RouteRules
	}
	healthState := models.ProviderHealthState{Status: "offline"}
	if enabled {
		healthState.Status = "healthy"
	}

	tagsJSON, _ := json.Marshal(tags)
	routeJSON, _ := json.Marshal(routeRules)
	healthJSON, _ := json.Marshal(healthState)

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ai_providers
              (id, name, provider_type, model_name, endpoint_url,
               api_mode, credential_reference, enabled,
               load_balance_weight, max_output_tokens, cost_tier,
               tags, route_rules, health_state)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
            RETURNING `+providerColumns,
		uuid.New(), strings.TrimSpace(body.Name), providerType, modelName,
		endpointURL, apiMode, body.CredentialReference, enabled,
		loadBalanceWeight, maxOutputTokens, costTier,
		tagsJSON, routeJSON, healthJSON)
	p, err := scanProvider(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// UpdateProvider handles `PATCH /api/v1/providers/{id}`.
func (h *ChatHandlers) UpdateProvider(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	current, err := h.loadProvider(r.Context(), providerID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	var body models.UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	healthState := current.HealthState
	if body.HealthState != nil {
		healthState = *body.HealthState
	}
	if body.Enabled != nil {
		if !*body.Enabled {
			healthState.Status = "offline"
		} else if healthState.Status == "offline" {
			healthState.Status = "healthy"
		}
	}

	name := derefString(body.Name, current.Name)
	providerType := derefString(body.ProviderType, current.ProviderType)
	modelName := derefString(body.ModelName, current.ModelName)
	endpointURL := derefString(body.EndpointURL, current.EndpointURL)
	apiMode := derefString(body.APIMode, current.APIMode)
	costTier := derefString(body.CostTier, current.CostTier)
	credRef := current.CredentialReference
	if body.CredentialReference != nil {
		credRef = body.CredentialReference
	}
	enabled := current.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	loadBalanceWeight := current.LoadBalanceWeight
	if body.LoadBalanceWeight != nil {
		loadBalanceWeight = *body.LoadBalanceWeight
	}
	maxOutputTokens := current.MaxOutputTokens
	if body.MaxOutputTokens != nil {
		maxOutputTokens = *body.MaxOutputTokens
	}
	tags := current.Tags
	if body.Tags != nil {
		tags = *body.Tags
	}
	if tags == nil {
		tags = []string{}
	}
	routeRules := current.RouteRules
	if body.RouteRules != nil {
		routeRules = *body.RouteRules
	}

	tagsJSON, _ := json.Marshal(tags)
	routeJSON, _ := json.Marshal(routeRules)
	healthJSON, _ := json.Marshal(healthState)

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ai_providers SET
            name = $2, provider_type = $3, model_name = $4,
            endpoint_url = $5, api_mode = $6, credential_reference = $7,
            enabled = $8, load_balance_weight = $9,
            max_output_tokens = $10, cost_tier = $11, tags = $12,
            route_rules = $13, health_state = $14, updated_at = NOW()
          WHERE id = $1
          RETURNING `+providerColumns,
		providerID, name, providerType, modelName, endpointURL,
		apiMode, credRef, enabled, loadBalanceWeight, maxOutputTokens,
		costTier, tagsJSON, routeJSON, healthJSON)
	p, err := scanProvider(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// loadProvider matches the KnowledgeHandlers helper but lives here so
// the chat surface can be wired without a KnowledgeHandlers instance.
func (h *ChatHandlers) loadProvider(ctx context.Context, id uuid.UUID) (*models.LlmProvider, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+providerColumns+` FROM ai_providers WHERE id = $1`, id)
	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

const conversationColumns = `id, title, messages, provider_id,
                             last_cache_hit, last_guardrail_blocked,
                             created_at, last_activity_at`

func scanConversation(s toolScanner) (models.Conversation, error) {
	var c models.Conversation
	var messagesRaw []byte
	var providerID *uuid.UUID
	if err := s.Scan(
		&c.ID, &c.Title, &messagesRaw, &providerID,
		&c.LastCacheHit, &c.LastGuardrailBlocked,
		&c.CreatedAt, &c.LastActivityAt,
	); err != nil {
		return c, err
	}
	c.ProviderID = providerID
	if len(messagesRaw) > 0 {
		_ = json.Unmarshal(messagesRaw, &c.Messages)
	}
	if c.Messages == nil {
		c.Messages = []models.ChatMessage{}
	}
	return c, nil
}

// ListConversations handles `GET /api/v1/conversations`.
func (h *ChatHandlers) ListConversations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+conversationColumns+` FROM ai_conversations
          ORDER BY last_activity_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.ConversationSummary, 0)
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, conversationSummary(c))
	}
	writeJSON(w, http.StatusOK, models.ListConversationsResponse{Data: out})
}

// GetConversation handles `GET /api/v1/conversations/{id}`.
func (h *ChatHandlers) GetConversation(w http.ResponseWriter, r *http.Request, conversationID uuid.UUID) {
	row := h.Pool.QueryRow(r.Context(),
		`SELECT `+conversationColumns+` FROM ai_conversations WHERE id = $1`, conversationID)
	c, err := scanConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// EvaluateGuardrails handles `POST /api/v1/guardrails/evaluate`.
// Pure-logic path — runs domain/llm.EvaluateText then derives the
// recommendations list mirroring the Rust BTreeSet ordering (sorted).
func (h *ChatHandlers) EvaluateGuardrails(w http.ResponseWriter, r *http.Request) {
	var body models.EvaluateGuardrailsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "guardrail evaluation requires content")
		return
	}

	verdict := llm.EvaluateText(body.Content)

	recSet := map[string]struct{}{}
	if verdict.Blocked {
		recSet["Remove prompt-injection or toxic content before retrying."] = struct{}{}
	}
	for _, flag := range verdict.Flags {
		if strings.HasPrefix(flag.Kind, "pii_") {
			recSet["Redact PII before routing prompts to external LLM providers."] = struct{}{}
		}
	}
	if len(recSet) == 0 {
		recSet["No blocking issues detected; response is safe to continue."] = struct{}{}
	}
	recommendations := make([]string, 0, len(recSet))
	for k := range recSet {
		recommendations = append(recommendations, k)
	}
	sort.Strings(recommendations)

	writeJSON(w, http.StatusOK, models.EvaluateGuardrailsResponse{
		Verdict:         verdict,
		RiskScore:       evaluation.RiskScore(&verdict),
		Recommendations: recommendations,
	})
}

// conversationSummary mirrors fn conversation_summary in chat.rs.
func conversationSummary(c models.Conversation) models.ConversationSummary {
	preview := "No messages yet"
	if n := len(c.Messages); n > 0 {
		preview = summarizeTitle(c.Messages[n-1].Content)
	}
	return models.ConversationSummary{
		ID:                 c.ID,
		Title:              c.Title,
		LastMessagePreview: preview,
		ProviderID:         c.ProviderID,
		MessageCount:       int32(len(c.Messages)),
		LastCacheHit:       c.LastCacheHit,
		LastActivityAt:     c.LastActivityAt,
	}
}

// summarizeTitle mirrors fn summarize_title — first 60 runes, "..."
// suffix if longer, "New conversation" if empty.
func summarizeTitle(content string) string {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) > 60 {
		return string(runes[:60]) + "..."
	}
	if len(runes) == 0 {
		return "New conversation"
	}
	return string(runes)
}

