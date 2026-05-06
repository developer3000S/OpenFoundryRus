package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/copilot"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/evaluation"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/llm"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/rag"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// chat_runtime hosts the helpers that back chat completion / copilot
// ask / provider benchmark — the three endpoints that chain
// llm/runtime.CompleteText. All helpers are 1:1 with their Rust
// counterparts in libs/ai-kernel/src/handlers/chat.rs.

// loadProviderRows mirrors fn load_provider_rows. Returns every
// configured provider ordered by updated_at desc, created_at desc.
func loadProviderRows(ctx context.Context, pool *pgxpool.Pool) ([]models.LlmProvider, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+providerColumns+` FROM ai_providers
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.LlmProvider, 0)
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// previewText mirrors fn preview_text — first `limit` runes of the
// trimmed content; appends "..." if truncated.
func previewText(content string, limit int) string {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return string(runes)
}

// attachmentContext mirrors fn attachment_context — formats
// attachments as "- <label>: …" lines for inclusion in the
// prompt-used echo.
func attachmentContext(attachments []models.ChatAttachment) string {
	if len(attachments) == 0 {
		return "none"
	}
	lines := make([]string, 0, len(attachments))
	for _, a := range attachments {
		label := "attachment"
		if a.Name != nil && strings.TrimSpace(*a.Name) != "" {
			label = *a.Name
		}
		switch a.Kind {
		case "image_url":
			url := "missing-url"
			if a.URL != nil {
				url = *a.URL
			}
			lines = append(lines, fmt.Sprintf("- %s: image url %s", label, url))
		case "image_base64":
			mime := "unknown"
			if a.MimeType != nil {
				mime = *a.MimeType
			}
			lines = append(lines, fmt.Sprintf("- %s: embedded %s image", label, mime))
		default:
			text := "text attachment"
			if a.Text != nil {
				text = *a.Text
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", label, text))
		}
	}
	return strings.Join(lines, "\n")
}

// requiredModalities mirrors fn required_modalities — always includes
// "text"; appends "image" when any attachment kind starts with
// "image".
func requiredModalities(attachments []models.ChatAttachment) []string {
	out := []string{"text"}
	for _, a := range attachments {
		if strings.HasPrefix(a.Kind, "image") {
			out = append(out, "image")
			break
		}
	}
	return out
}

// modalityLabel mirrors fn modality_label.
func modalityLabel(required []string) string {
	for _, m := range required {
		if strings.EqualFold(m, "image") {
			return "image+text"
		}
	}
	return "text"
}

// privacyReason mirrors fn privacy_reason — returns the explicit
// "private network explicitly requested" when the body flag is set,
// or the PII-detected fallback when guardrail flagged a pii_* kind.
func privacyReason(verdict models.GuardrailVerdict, requirePrivateNetwork bool) *string {
	if requirePrivateNetwork {
		s := "private network explicitly requested"
		return &s
	}
	for _, f := range verdict.Flags {
		if strings.HasPrefix(f.Kind, "pii_") {
			s := "PII detected in prompt, preferring private-network providers"
			return &s
		}
	}
	return nil
}

// routingMetadata mirrors fn routing_metadata.
func routingMetadata(
	provider models.LlmProvider,
	requestedPrivateNetwork bool,
	privacyReason *string,
	candidates []models.LlmProvider,
	required []string,
) models.ChatRoutingMetadata {
	ids := make([]uuid.UUID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.ID)
	}
	return models.ChatRoutingMetadata{
		RequestedPrivateNetwork: requestedPrivateNetwork,
		UsedPrivateNetwork:      llm.ProviderUsesPrivateNetwork(provider),
		PrivacyReason:           privacyReason,
		CandidateProviderIDs:    ids,
		RequiredModalities:      append([]string{}, required...),
	}
}

// usageSummary mirrors fn usage_summary.
func usageSummary(provider models.LlmProvider, promptTokens, completionTokens, latencyMs int32, cacheHit bool) models.LlmUsageSummary {
	pt := promptTokens
	if pt < 0 {
		pt = 0
	}
	ct := completionTokens
	if ct < 0 {
		ct = 0
	}
	return models.LlmUsageSummary{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      pt + ct,
		EstimatedCostUSD: evaluation.EstimatedCostUSD(&provider, promptTokens, completionTokens, cacheHit),
		LatencyMs:        latencyMs,
		NetworkScope:     provider.RouteRules.NetworkScope,
		CacheHit:         cacheHit,
	}
}

// findCachedResponse mirrors fn find_cached_response — scans the
// most recent 64 ai_semantic_cache rows for the given kind, picks the
// best (cosine_similarity ≥ 0.92) match. On hit increments hit_count
// + last_hit_at and returns (raw response payload, metadata, original
// provider_id). Returns nil payload on miss.
func findCachedResponse(ctx context.Context, pool *pgxpool.Pool, kind, prompt string) (json.RawMessage, *models.SemanticCacheMetadata, *uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, cache_key, fingerprint, response, provider_id
         FROM ai_semantic_cache
         WHERE kind = $1
         ORDER BY last_hit_at DESC
         LIMIT 64`, kind)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	exactKey := llm.CacheKey(kind, prompt)
	queryFingerprint := llm.Fingerprint(prompt)

	type match struct {
		id          uuid.UUID
		cacheKey    string
		response    json.RawMessage
		providerID  *uuid.UUID
		score       float32
	}
	var best *match

	for rows.Next() {
		var (
			id            uuid.UUID
			cacheKey      string
			fingerprintB  []byte
			responseB     []byte
			providerID    *uuid.UUID
		)
		if err := rows.Scan(&id, &cacheKey, &fingerprintB, &responseB, &providerID); err != nil {
			return nil, nil, nil, err
		}
		var rowFingerprint []float32
		if len(fingerprintB) > 0 {
			_ = json.Unmarshal(fingerprintB, &rowFingerprint)
		}

		score := float32(0)
		if cacheKey == exactKey {
			score = 1.0
		} else {
			score = llm.CosineSimilarity(queryFingerprint, rowFingerprint)
		}
		if score < 0.92 {
			continue
		}
		if best == nil || score > best.score {
			best = &match{
				id:         id,
				cacheKey:   cacheKey,
				response:   responseB,
				providerID: providerID,
				score:      score,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if best == nil {
		return nil, nil, nil, nil
	}

	if _, err := pool.Exec(ctx,
		`UPDATE ai_semantic_cache SET hit_count = hit_count + 1, last_hit_at = NOW() WHERE id = $1`,
		best.id); err != nil {
		return nil, nil, nil, err
	}
	meta := &models.SemanticCacheMetadata{
		CacheKey:        best.cacheKey,
		Hit:             true,
		SimilarityScore: best.score,
	}
	return best.response, meta, best.providerID, nil
}

// upsertCachedResponse mirrors fn upsert_cached_response — INSERT
// ON CONFLICT updates the row (kind, cache_key) with the fresh
// fingerprint + response + provider_id. Returns the metadata footer
// (hit=false, similarity=0.0) for inclusion in the live reply.
func upsertCachedResponse(ctx context.Context, pool *pgxpool.Pool, kind, prompt string, providerID *uuid.UUID, payload any) (models.SemanticCacheMetadata, error) {
	cacheKey := llm.CacheKey(kind, prompt)
	normalizedPrompt := llm.NormalizeText(prompt)
	fingerprint := llm.Fingerprint(prompt)
	fingerprintJSON, _ := json.Marshal(fingerprint)
	responseJSON, _ := json.Marshal(payload)

	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO ai_semantic_cache (
            id, kind, cache_key, normalized_prompt, fingerprint,
            response, provider_id, hit_count, last_hit_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, 0, NOW())
         ON CONFLICT (kind, cache_key) DO UPDATE SET
            normalized_prompt = EXCLUDED.normalized_prompt,
            fingerprint = EXCLUDED.fingerprint,
            response = EXCLUDED.response,
            provider_id = EXCLUDED.provider_id,
            last_hit_at = NOW()`,
		id, kind, cacheKey, normalizedPrompt, fingerprintJSON,
		responseJSON, providerID,
	)
	if err != nil {
		return models.SemanticCacheMetadata{}, err
	}
	return models.SemanticCacheMetadata{
		CacheKey:        cacheKey,
		Hit:             false,
		SimilarityScore: 0,
	}, nil
}

// loadDocumentsForBases mirrors fn load_documents_for_bases — for
// each KB id, fetch the documents (latest first) and aggregate.
func loadDocumentsForBases(ctx context.Context, pool *pgxpool.Pool, knowledgeBaseIDs []uuid.UUID) ([]models.KnowledgeDocument, error) {
	out := make([]models.KnowledgeDocument, 0)
	for _, kbID := range knowledgeBaseIDs {
		rows, err := pool.Query(ctx,
			`SELECT `+knowledgeDocumentColumns+` FROM ai_knowledge_documents
             WHERE knowledge_base_id = $1
             ORDER BY updated_at DESC`, kbID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			d, err := scanKnowledgeDocument(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// cachedCopilotPayload is the cacheable subset of the copilot reply
// (mirrors Rust struct CachedCopilotPayload).
type cachedCopilotPayload struct {
	Answer              string                          `json:"answer"`
	SuggestedSQL        *string                         `json:"suggested_sql"`
	PipelineSuggestions []string                        `json:"pipeline_suggestions"`
	OntologyHints       []string                        `json:"ontology_hints"`
	CitedKnowledge      []models.KnowledgeSearchResult  `json:"cited_knowledge"`
}

// AskCopilot handles `POST /api/v1/copilot/ask`. Mirrors fn
// ask_copilot verbatim: validates → loads providers → cache lookup
// (skips cached row when private-network policy doesn't accept the
// cached provider) → routes provider → loads KB docs + RAG retrieval
// → copilot.Assist deterministic draft → llm.CompleteText (skipped
// when guardrail blocked) → upsert cache → record usage → return.
func (h *ChatHandlers) AskCopilot(w http.ResponseWriter, r *http.Request) {
	var body models.CopilotRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Question) == "" {
		writeError(w, http.StatusBadRequest, "copilot question is required")
		return
	}
	ctx := r.Context()

	providers, err := loadProviderRows(ctx, h.Pool)
	if err != nil {
		dbError(w, err)
		return
	}
	if len(providers) == 0 {
		writeError(w, http.StatusNotFound, "no AI providers configured")
		return
	}

	promptUsed := fmt.Sprintf("question=%s datasets=%v ontology=%v knowledge_bases=%v",
		body.Question, body.DatasetIDs, body.OntologyTypeIDs, body.KnowledgeBaseIDs)
	guardrail := llm.EvaluateText(body.Question)
	privacy := privacyReason(guardrail, false)
	required := []string{"text"}

	// Cache fast-path.
	cachedRaw, cachedMeta, cachedProviderID, err := findCachedResponse(ctx, h.Pool, "copilot", promptUsed)
	if err != nil {
		dbError(w, err)
		return
	}
	if cachedMeta != nil && len(cachedRaw) > 0 {
		var cachedPayload cachedCopilotPayload
		if err := json.Unmarshal(cachedRaw, &cachedPayload); err == nil {
			// Pick the originating provider; fall back to providers[0].
			var cachedProvider models.LlmProvider
			cachedProvider = providers[0]
			if cachedProviderID != nil {
				for _, p := range providers {
					if p.ID == *cachedProviderID {
						cachedProvider = p
						break
					}
				}
			}
			useCached := true
			if privacy != nil {
				useCached = llm.ProviderUsesPrivateNetwork(cachedProvider)
			}
			if useCached {
				usage := usageSummary(cachedProvider,
					llm.EstimateTokens(promptUsed),
					llm.EstimateTokens(cachedPayload.Answer),
					0, true)
				_ = recordUsageEvent(ctx, h.Pool, cachedProvider.ID, nil,
					"copilot", "copilot", "text", usage, nil,
					map[string]any{
						"cache_key":            cachedMeta.CacheKey,
						"cache_hit":            true,
						"knowledge_base_count": len(body.KnowledgeBaseIDs),
					})
				writeJSON(w, http.StatusOK, models.CopilotResponse{
					Answer:              cachedPayload.Answer,
					SuggestedSQL:        cachedPayload.SuggestedSQL,
					PipelineSuggestions: cachedPayload.PipelineSuggestions,
					OntologyHints:       cachedPayload.OntologyHints,
					CitedKnowledge:      cachedPayload.CitedKnowledge,
					ProviderName:        cachedProvider.Name,
					Cache:               *cachedMeta,
					Usage:               usage,
					CreatedAt:           time.Now().UTC(),
				})
				return
			}
		}
	}

	routed := llm.RouteProviders(providers, body.PreferredProviderID, "copilot", required, false, privacy != nil)
	provider := llm.SelectProvider(routed, true)
	if provider == nil {
		writeError(w, http.StatusNotFound, "no AI provider available")
		return
	}

	documents, err := loadDocumentsForBases(ctx, h.Pool, body.KnowledgeBaseIDs)
	if err != nil {
		dbError(w, err)
		return
	}
	citedKnowledge := rag.Search(body.Question, documents, 6, 0.55)

	draft := copilot.Assist(body.Question, body.DatasetIDs, body.OntologyTypeIDs,
		citedKnowledge, body.IncludeSQL, body.IncludePipelinePlan)

	var providerAnswer string
	var promptTokens, completionTokens, totalTokens int32
	var latencyMs int32

	if !guardrail.Blocked {
		startedAt := time.Now()

		// Build the LLM user prompt mirroring the Rust formatter.
		suggestedSQL := ""
		if draft.SuggestedSQL != nil {
			suggestedSQL = *draft.SuggestedSQL
		}
		knowledgeContext := make([]string, 0, len(citedKnowledge))
		for _, hit := range citedKnowledge {
			knowledgeContext = append(knowledgeContext, fmt.Sprintf("- %s: %s", hit.DocumentTitle, hit.Excerpt))
		}
		userPrompt := fmt.Sprintf(
			"Question: %s\nDraft answer: %s\nSuggested SQL: %q\nPipeline suggestions: %v\nOntology hints: %v\nKnowledge context:\n%s",
			body.Question, draft.Answer, suggestedSQL,
			draft.PipelineSuggestions, draft.OntologyHints,
			strings.Join(knowledgeContext, "\n"))

		maxOut := provider.MaxOutputTokens
		if maxOut > 512 {
			maxOut = 512
		}
		completion, completionErr := llm.CompleteText(ctx, nil, provider,
			"You are OpenFoundry Copilot. Ground answers in retrieval context and suggested platform actions.",
			userPrompt, nil, 0.2, maxOut)
		latencyMs = int32(time.Since(startedAt).Milliseconds())
		if latencyMs < 0 {
			latencyMs = 0
		}
		if completionErr != nil {
			writeError(w, http.StatusInternalServerError, completionErr.Error())
			return
		}
		providerAnswer = completion.Text
		promptTokens = completion.PromptTokens
		if promptTokens <= 0 {
			promptTokens = llm.EstimateTokens(promptUsed)
		}
		completionTokens = completion.CompletionTokens
		if completionTokens <= 0 {
			completionTokens = llm.EstimateTokens(providerAnswer)
		}
		totalTokens = completion.TotalTokens
		if totalTokens <= 0 {
			totalTokens = promptTokens + completionTokens
		}
	} else {
		providerAnswer = "Guardrails blocked this copilot request. Remove unsafe instructions and retry."
		promptTokens = llm.EstimateTokens(promptUsed)
	}

	var usage models.LlmUsageSummary
	if guardrail.Blocked {
		usage = models.LlmUsageSummary{
			PromptTokens:     promptTokens,
			CompletionTokens: 0,
			TotalTokens:      promptTokens,
			EstimatedCostUSD: 0,
			LatencyMs:        0,
			NetworkScope:     provider.RouteRules.NetworkScope,
			CacheHit:         false,
		}
	} else {
		usage = usageSummary(*provider, promptTokens, completionTokens, latencyMs, false)
		usage.TotalTokens = totalTokens
	}

	payload := cachedCopilotPayload{
		Answer:         providerAnswer,
		CitedKnowledge: citedKnowledge,
	}
	if !guardrail.Blocked {
		payload.SuggestedSQL = draft.SuggestedSQL
		payload.PipelineSuggestions = draft.PipelineSuggestions
		payload.OntologyHints = draft.OntologyHints
	}

	cache, err := upsertCachedResponse(ctx, h.Pool, "copilot", promptUsed, &provider.ID, payload)
	if err != nil {
		dbError(w, err)
		return
	}
	if err := recordUsageEvent(ctx, h.Pool, provider.ID, nil,
		"copilot", "copilot", "text", usage, nil,
		map[string]any{
			"cache_key":           cache.CacheKey,
			"cache_hit":           false,
			"knowledge_hit_count": len(citedKnowledge),
		}); err != nil {
		dbError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.CopilotResponse{
		Answer:              payload.Answer,
		SuggestedSQL:        payload.SuggestedSQL,
		PipelineSuggestions: payload.PipelineSuggestions,
		OntologyHints:       payload.OntologyHints,
		CitedKnowledge:      payload.CitedKnowledge,
		ProviderName:        provider.Name,
		Cache:               cache,
		Usage:               usage,
		CreatedAt:           time.Now().UTC(),
	})
}

// persistConversation mirrors fn persist_conversation. Either appends
// the (user, assistant) message pair onto an existing conversation
// (if conversation_id resolves to a row) or inserts a fresh row with
// the two messages as the seed. Returns the conversation_id used.
func persistConversation(
	ctx context.Context,
	pool *pgxpool.Pool,
	conversationID *uuid.UUID,
	userMessage string,
	userAttachments []models.ChatAttachment,
	reply string,
	providerID uuid.UUID,
	citations []models.KnowledgeSearchResult,
	guardrail models.GuardrailVerdict,
	cacheHit bool,
) (uuid.UUID, error) {
	now := time.Now().UTC()
	g := guardrail
	userEntry := models.ChatMessage{
		Role:             "user",
		Content:          userMessage,
		Attachments:      userAttachments,
		GuardrailVerdict: &g,
		CreatedAt:        now,
	}
	assistantEntry := models.ChatMessage{
		Role:       "assistant",
		Content:    reply,
		ProviderID: &providerID,
		Citations:  citations,
		CreatedAt:  now,
	}

	if conversationID != nil {
		row := pool.QueryRow(ctx,
			`SELECT `+conversationColumns+` FROM ai_conversations WHERE id = $1`, *conversationID)
		current, err := scanConversation(row)
		if err == nil {
			messages := append(current.Messages, userEntry, assistantEntry)
			messagesJSON, _ := json.Marshal(messages)
			if _, err := pool.Exec(ctx,
				`UPDATE ai_conversations SET messages = $2, provider_id = $3,
                    last_cache_hit = $4, last_guardrail_blocked = $5,
                    last_activity_at = NOW() WHERE id = $1`,
				*conversationID, messagesJSON, providerID, cacheHit, guardrail.Blocked); err != nil {
				return uuid.Nil, err
			}
			return *conversationID, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, err
		}
		// fall through to insert with a fresh id
	}

	newID, err := uuid.NewV7()
	if err != nil {
		newID = uuid.New()
	}
	title := summarizeTitle(userMessage)
	messages := []models.ChatMessage{userEntry, assistantEntry}
	messagesJSON, _ := json.Marshal(messages)
	if _, err := pool.Exec(ctx,
		`INSERT INTO ai_conversations (id, title, messages, provider_id,
            last_cache_hit, last_guardrail_blocked)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		newID, title, messagesJSON, providerID, cacheHit, guardrail.Blocked); err != nil {
		return uuid.Nil, err
	}
	return newID, nil
}

// cachedChatPayload is the cacheable subset of the chat-completion
// reply (mirrors Rust struct CachedChatPayload).
type cachedChatPayload struct {
	Reply            string                          `json:"reply"`
	Citations        []models.KnowledgeSearchResult  `json:"citations"`
	CompletionTokens int32                           `json:"completion_tokens"`
}

// CreateChatCompletion handles `POST /api/v1/chat/completions`.
// Mirrors fn create_chat_completion verbatim:
//
//  1. validate user_message + load providers.
//  2. resolve base prompt (template + variables, or system_prompt
//     fallback, or the canned "OpenFoundry copilot" line).
//  3. evaluate guardrail + derive privacy/required_modalities.
//  4. (Rust additionally calls enforce_purpose_checkpoint here when
//     require_private_network or PII is detected — that integration
//     is deferred until the auth-middleware-go purpose-checkpoint
//     surface lands; route-level gating + guardrail.blocked covers
//     the policy outcomes for now.)
//  5. retrieve KB hits when knowledge_base_id is set.
//  6. build prompt_used signature + route via gateway.
//  7. cache fast-path: cosine ≥ 0.92, skipping cached row when
//     private-network policy doesn't accept the cached provider.
//  8. live path: llm.CompleteText (skipped when blocked), build
//     usage, upsert cache, persistConversation, recordUsageEvent.
func (h *ChatHandlers) CreateChatCompletion(w http.ResponseWriter, r *http.Request) {
	var body models.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.UserMessage) == "" {
		writeError(w, http.StatusBadRequest, "chat completion requires a user message")
		return
	}
	ctx := r.Context()

	providers, err := loadProviderRows(ctx, h.Pool)
	if err != nil {
		dbError(w, err)
		return
	}
	if len(providers) == 0 {
		writeError(w, http.StatusNotFound, "no AI providers configured")
		return
	}

	// 2. Base prompt
	basePrompt := ""
	if body.PromptTemplateID != nil {
		ph := &PromptsHandlers{Pool: h.Pool}
		template, err := ph.loadPromptRow(ctx, *body.PromptTemplateID)
		if err != nil {
			dbError(w, err)
			return
		}
		if template == nil {
			writeError(w, http.StatusNotFound, "prompt template not found")
			return
		}
		rendered, _ := llm.InterpolateTemplate(template.CurrentVersion.Content, body.PromptVariables, false)
		if body.SystemPrompt != nil {
			basePrompt = *body.SystemPrompt + "\n\n" + rendered
		} else {
			basePrompt = rendered
		}
	} else if body.SystemPrompt != nil {
		basePrompt = *body.SystemPrompt
	} else {
		basePrompt = "You are the OpenFoundry platform copilot. Provide grounded operational guidance."
	}

	guardrail := llm.EvaluateText(body.UserMessage)
	required := requiredModalities(body.Attachments)
	privacy := privacyReason(guardrail, body.RequirePrivateNetwork)

	// 5. Knowledge retrieval
	knowledgeHits := []models.KnowledgeSearchResult{}
	if body.KnowledgeBaseID != nil {
		docs, err := loadDocumentsForBases(ctx, h.Pool, []uuid.UUID{*body.KnowledgeBaseID})
		if err != nil {
			dbError(w, err)
			return
		}
		knowledgeHits = rag.Search(body.UserMessage, docs, 4, 0.55)
	}

	// 6. Prompt-used signature + routing
	knowledgeContext := make([]string, 0, len(knowledgeHits))
	for _, hit := range knowledgeHits {
		knowledgeContext = append(knowledgeContext, fmt.Sprintf("- %s: %s", hit.DocumentTitle, hit.Excerpt))
	}
	promptUsed := fmt.Sprintf(
		"%s\n\nUser request: %s\n\nAttachments:\n%s\n\nRetrieved context count: %d\n\nRetrieved context:\n%s",
		basePrompt, guardrail.RedactedText, attachmentContext(body.Attachments),
		len(knowledgeHits), strings.Join(knowledgeContext, "\n"))

	routed := llm.RouteProviders(providers, body.PreferredProviderID, "chat", required, body.RequirePrivateNetwork, privacy != nil)
	if body.RequirePrivateNetwork && len(routed) == 0 {
		writeError(w, http.StatusBadRequest, "no private-network AI provider is configured for this request")
		return
	}
	provider := llm.SelectProvider(routed, body.FallbackEnabled)
	if provider == nil {
		writeError(w, http.StatusNotFound, "no AI provider available")
		return
	}
	routing := routingMetadata(*provider, body.RequirePrivateNetwork, privacy, routed, required)
	modality := modalityLabel(required)

	// 7. Cache fast-path
	cachedRaw, cachedMeta, cachedProviderID, err := findCachedResponse(ctx, h.Pool, "chat", promptUsed)
	if err != nil {
		dbError(w, err)
		return
	}
	if cachedMeta != nil && len(cachedRaw) > 0 {
		var cachedPayload cachedChatPayload
		if err := json.Unmarshal(cachedRaw, &cachedPayload); err == nil {
			cachedProvider := *provider
			if cachedProviderID != nil {
				for _, p := range providers {
					if p.ID == *cachedProviderID {
						cachedProvider = p
						break
					}
				}
			}
			useCached := true
			if body.RequirePrivateNetwork || privacy != nil {
				useCached = llm.ProviderUsesPrivateNetwork(cachedProvider)
			}
			if useCached {
				conversationID, err := persistConversation(ctx, h.Pool,
					body.ConversationID, body.UserMessage, body.Attachments,
					cachedPayload.Reply, cachedProvider.ID,
					cachedPayload.Citations, guardrail, true)
				if err != nil {
					dbError(w, err)
					return
				}
				usage := usageSummary(cachedProvider,
					llm.EstimateTokens(promptUsed),
					cachedPayload.CompletionTokens, 0, true)
				_ = recordUsageEvent(ctx, h.Pool, cachedProvider.ID, &conversationID,
					"chat", "chat", modality, usage, nil,
					map[string]any{
						"cache_key":           cachedMeta.CacheKey,
						"cache_hit":           true,
						"required_modalities": required,
					})

				writeJSON(w, http.StatusOK, models.ChatCompletionResponse{
					ConversationID:   conversationID,
					ProviderID:       cachedProvider.ID,
					ProviderName:     cachedProvider.Name,
					Reply:            cachedPayload.Reply,
					Citations:        cachedPayload.Citations,
					Guardrail:        guardrail,
					Cache:            *cachedMeta,
					PromptUsed:       promptUsed,
					CompletionTokens: cachedPayload.CompletionTokens,
					Usage:            usage,
					Routing:          routingMetadata(cachedProvider, body.RequirePrivateNetwork, privacy, routed, required),
					CreatedAt:        time.Now().UTC(),
				})
				return
			}
		}
	}

	// 8. Live path
	var reply string
	var promptTokens, completionTokens, totalTokens int32
	var latencyMs int32

	if !guardrail.Blocked {
		startedAt := time.Now()
		completion, completionErr := llm.CompleteText(ctx, nil, provider,
			basePrompt, promptUsed, body.Attachments,
			body.Temperature, body.MaxTokens)
		latencyMs = int32(time.Since(startedAt).Milliseconds())
		if latencyMs < 0 {
			latencyMs = 0
		}
		if completionErr != nil {
			writeError(w, http.StatusInternalServerError, completionErr.Error())
			return
		}
		reply = completion.Text
		promptTokens = completion.PromptTokens
		if promptTokens <= 0 {
			promptTokens = llm.EstimateTokens(promptUsed)
		}
		completionTokens = completion.CompletionTokens
		if completionTokens <= 0 {
			est := llm.EstimateTokens(reply)
			if est < body.MaxTokens {
				completionTokens = est
			} else {
				completionTokens = body.MaxTokens
			}
		}
		totalTokens = completion.TotalTokens
		if totalTokens <= 0 {
			totalTokens = promptTokens + completionTokens
		}
	} else {
		reply = "Guardrails blocked this request. Remove prompt-injection or toxic content and retry."
		promptTokens = llm.EstimateTokens(promptUsed)
	}

	var usage models.LlmUsageSummary
	if guardrail.Blocked {
		usage = models.LlmUsageSummary{
			PromptTokens:     promptTokens,
			CompletionTokens: 0,
			TotalTokens:      promptTokens,
			EstimatedCostUSD: 0,
			LatencyMs:        0,
			NetworkScope:     provider.RouteRules.NetworkScope,
			CacheHit:         false,
		}
	} else {
		usage = usageSummary(*provider, promptTokens, completionTokens, latencyMs, false)
		usage.TotalTokens = totalTokens
	}

	payload := cachedChatPayload{
		Reply:            reply,
		Citations:        knowledgeHits,
		CompletionTokens: usage.CompletionTokens,
	}

	cache, err := upsertCachedResponse(ctx, h.Pool, "chat", promptUsed, &provider.ID, payload)
	if err != nil {
		dbError(w, err)
		return
	}
	conversationID, err := persistConversation(ctx, h.Pool,
		body.ConversationID, body.UserMessage, body.Attachments,
		reply, provider.ID, knowledgeHits, guardrail, false)
	if err != nil {
		dbError(w, err)
		return
	}
	if err := recordUsageEvent(ctx, h.Pool, provider.ID, &conversationID,
		"chat", "chat", modality, usage, nil,
		map[string]any{
			"cache_key":           cache.CacheKey,
			"cache_hit":           false,
			"knowledge_hit_count": len(knowledgeHits),
			"required_modalities": required,
		}); err != nil {
		dbError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ChatCompletionResponse{
		ConversationID:   conversationID,
		ProviderID:       provider.ID,
		ProviderName:     provider.Name,
		Reply:            reply,
		Citations:        knowledgeHits,
		Guardrail:        guardrail,
		Cache:            cache,
		PromptUsed:       promptUsed,
		CompletionTokens: usage.CompletionTokens,
		Usage:            usage,
		Routing:          routing,
		CreatedAt:        time.Now().UTC(),
	})
}

// recordUsageEvent mirrors fn record_usage_event — best-effort insert
// into ai_llm_usage_events. Non-fatal at the call site (chat /
// benchmark return their replies even if the insert fails).
func recordUsageEvent(
	ctx context.Context,
	pool *pgxpool.Pool,
	providerID uuid.UUID,
	conversationID *uuid.UUID,
	requestKind, useCase, modality string,
	usage models.LlmUsageSummary,
	benchmarkGroupID *uuid.UUID,
	metadata any,
) error {
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = pool.Exec(ctx,
		`INSERT INTO ai_llm_usage_events (
            id, provider_id, conversation_id, request_kind, use_case,
            network_scope, modality, cache_hit, prompt_tokens,
            completion_tokens, total_tokens, estimated_cost_usd,
            latency_ms, benchmark_group_id, metadata
         ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		id, providerID, conversationID, requestKind, useCase,
		usage.NetworkScope, modality, usage.CacheHit, usage.PromptTokens,
		usage.CompletionTokens, usage.TotalTokens, usage.EstimatedCostUSD,
		usage.LatencyMs, benchmarkGroupID, metadataJSON,
	)
	return err
}

// BenchmarkProviders handles `POST /api/v1/providers/benchmark`.
// Mirrors fn benchmark_providers verbatim:
//   - validates prompt + guardrail (block sanitises 400 if blocked)
//   - loads providers, optionally filtered by body.provider_ids
//   - routes via gateway with privacy + modality filters
//   - calls llm.CompleteText for each routed provider, capturing
//     latency, tokens, error
//   - records ai_llm_usage_events per success
//   - scores quality/safety/latency/cost/overall, sorts desc,
//     picks the head as recommended_provider_id
func (h *ChatHandlers) BenchmarkProviders(w http.ResponseWriter, r *http.Request) {
	var body models.ProviderBenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "benchmark prompt is required")
		return
	}

	promptVerdict := llm.EvaluateText(body.Prompt)
	if promptVerdict.Blocked {
		writeError(w, http.StatusBadRequest, "benchmark prompt is blocked by guardrails; sanitize it before benchmarking")
		return
	}

	ctx := r.Context()
	providers, err := loadProviderRows(ctx, h.Pool)
	if err != nil {
		dbError(w, err)
		return
	}
	if len(providers) == 0 {
		writeError(w, http.StatusNotFound, "no AI providers configured")
		return
	}

	candidates := providers
	if len(body.ProviderIDs) > 0 {
		filterSet := map[uuid.UUID]struct{}{}
		for _, id := range body.ProviderIDs {
			filterSet[id] = struct{}{}
		}
		filtered := make([]models.LlmProvider, 0, len(filterSet))
		for _, p := range providers {
			if _, ok := filterSet[p.ID]; ok {
				filtered = append(filtered, p)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		writeError(w, http.StatusNotFound, "no benchmark providers matched the requested ids")
		return
	}

	required := requiredModalities(body.Attachments)
	privacy := privacyReason(promptVerdict, body.RequirePrivateNetwork)
	routed := llm.RouteProviders(candidates, nil, body.UseCase, required, body.RequirePrivateNetwork, privacy != nil)
	if body.RequirePrivateNetwork && len(routed) == 0 {
		writeError(w, http.StatusBadRequest, "no private-network AI provider is configured for this benchmark")
		return
	}
	if len(routed) == 0 {
		writeError(w, http.StatusNotFound, "no eligible providers support this benchmark")
		return
	}

	benchmarkGroupID, err := uuid.NewV7()
	if err != nil {
		benchmarkGroupID = uuid.New()
	}

	systemPrompt := "You are an enterprise AI benchmark harness. Answer the user prompt clearly and concretely."
	if body.SystemPrompt != nil && strings.TrimSpace(*body.SystemPrompt) != "" {
		systemPrompt = *body.SystemPrompt
	}
	promptUsed := fmt.Sprintf("%s\n\nUser request: %s\n\nAttachments:\n%s",
		systemPrompt, promptVerdict.RedactedText, attachmentContext(body.Attachments))

	results := make([]models.ProviderBenchmarkResult, 0, len(routed))
	for _, provider := range routed {
		startedAt := time.Now()
		completion, completionErr := llm.CompleteText(ctx, nil, &provider,
			systemPrompt, body.Prompt, body.Attachments,
			0.2, body.MaxTokens)
		latencyMs := int32(time.Since(startedAt).Milliseconds())
		if latencyMs < 0 {
			latencyMs = 0
		}

		if completionErr != nil {
			errStr := completionErr.Error()
			results = append(results, models.ProviderBenchmarkResult{
				ProviderID:       provider.ID,
				ProviderName:     provider.Name,
				NetworkScope:     provider.RouteRules.NetworkScope,
				ReplyPreview:     "",
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
				EstimatedCostUSD: 0,
				LatencyMs:        latencyMs,
				CacheHit:         false,
				Guardrail:        models.DefaultGuardrailVerdict(),
				Score:            models.ProviderBenchmarkScore{},
				Error:            &errStr,
			})
			continue
		}

		promptTokens := completion.PromptTokens
		if est := llm.EstimateTokens(promptUsed); est > promptTokens {
			promptTokens = est
		}
		completionTokens := completion.CompletionTokens
		if est := llm.EstimateTokens(completion.Text); est > completionTokens {
			completionTokens = est
		}
		usage := usageSummary(provider, promptTokens, completionTokens, latencyMs, false)
		if completion.TotalTokens > usage.TotalTokens {
			usage.TotalTokens = completion.TotalTokens
		}

		replyVerdict := llm.EvaluateText(completion.Text)

		// Best-effort usage-event insert; ignore error.
		_ = recordUsageEvent(ctx, h.Pool, provider.ID, nil, "benchmark",
			body.UseCase, modalityLabel(required), usage, &benchmarkGroupID,
			map[string]any{
				"rubric_keywords": body.RubricKeywords,
				"provider_name":   provider.Name,
			})

		results = append(results, models.ProviderBenchmarkResult{
			ProviderID:       provider.ID,
			ProviderName:     provider.Name,
			NetworkScope:     usage.NetworkScope,
			ReplyPreview:     previewText(completion.Text, 280),
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			EstimatedCostUSD: usage.EstimatedCostUSD,
			LatencyMs:        usage.LatencyMs,
			CacheHit:         false,
			Guardrail:        replyVerdict,
			Score:            models.ProviderBenchmarkScore{},
		})
	}

	// Score successful results.
	successful := make([]int, 0, len(results))
	for i, r := range results {
		if r.Error == nil {
			successful = append(successful, i)
		}
	}

	minLatency, maxLatency := float32(0), float32(0)
	minCost, maxCost := float32(0), float32(0)
	if len(successful) > 0 {
		first := successful[0]
		minLatency = float32(results[first].LatencyMs)
		maxLatency = minLatency
		minCost = results[first].EstimatedCostUSD
		maxCost = minCost
		for _, idx := range successful[1:] {
			lat := float32(results[idx].LatencyMs)
			if lat < minLatency {
				minLatency = lat
			}
			if lat > maxLatency {
				maxLatency = lat
			}
			cost := results[idx].EstimatedCostUSD
			if cost < minCost {
				minCost = cost
			}
			if cost > maxCost {
				maxCost = cost
			}
		}
	}

	for _, idx := range successful {
		r := &results[idx]
		quality := evaluation.QualityScore(r.ReplyPreview, body.RubricKeywords)
		safety := evaluation.SafetyScore(&r.Guardrail)
		latency := evaluation.NormalizedScore(float32(r.LatencyMs), minLatency, maxLatency, true)
		cost := evaluation.NormalizedScore(r.EstimatedCostUSD, minCost, maxCost, true)
		r.Score = models.ProviderBenchmarkScore{
			Quality: quality,
			Latency: latency,
			Cost:    cost,
			Safety:  safety,
			Overall: evaluation.OverallBenchmarkScore(quality, safety, latency, cost),
		}
	}

	// Sort overall desc.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score.Overall > results[j].Score.Overall
	})

	var recommended *uuid.UUID
	for _, r := range results {
		if r.Error == nil {
			id := r.ProviderID
			recommended = &id
			break
		}
	}

	writeJSON(w, http.StatusOK, models.ProviderBenchmarkResponse{
		BenchmarkGroupID:        benchmarkGroupID,
		UseCase:                 body.UseCase,
		PromptExcerpt:           summarizeTitle(body.Prompt),
		RequiredModalities:      required,
		RequestedPrivateNetwork: body.RequirePrivateNetwork,
		RecommendedProviderID:   recommended,
		Results:                 results,
		CreatedAt:               time.Now().UTC(),
	})
}
