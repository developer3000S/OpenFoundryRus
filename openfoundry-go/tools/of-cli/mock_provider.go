package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func serveMockProvider(ctx context.Context, host string, port int, stdout io.Writer) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { writeMockJSON(w, map[string]any{"status": "ok"}) })
	mux.HandleFunc("/v1/chat/completions", handleOpenAIChat)
	mux.HandleFunc("/v1/completions", handleOpenAIChat)
	mux.HandleFunc("/v1/embeddings", handleEmbeddings)
	mux.HandleFunc("/v1/messages", handleAnthropicMessages)
	server := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port), Handler: mux}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	fmt.Fprintf(stdout, "mock provider listening on http://%s:%d\n", host, port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	payload := readMockPayload(r)
	prompt := extractOpenAIPrompt(payload)
	writeMockJSON(w, map[string]any{"id": "chatcmpl_mock", "object": "chat.completion", "model": "openfoundry-mock", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": renderReply(prompt)}, "finish_reason": "stop"}}, "usage": mockUsage(prompt)})
}

func handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	payload := readMockPayload(r)
	prompt := extractAnthropicPrompt(payload)
	writeMockJSON(w, map[string]any{"id": "msg_mock", "type": "message", "role": "assistant", "model": "openfoundry-mock", "content": []any{map[string]any{"type": "text", "text": renderReply(prompt)}}, "stop_reason": "end_turn", "usage": mockUsage(prompt)})
}

func handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	payload := readMockPayload(r)
	input := flatten(payload["input"])
	writeMockJSON(w, map[string]any{"object": "list", "model": "openfoundry-mock-embedding", "data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": embed(input)}}, "usage": mockUsage(input)})
}

func readMockPayload(r *http.Request) map[string]any {
	defer r.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload == nil {
		payload = map[string]any{}
	}
	return payload
}
func writeMockJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
func renderReply(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return "OpenFoundry mock provider response."
	}
	return "OpenFoundry mock provider response: " + truncate(prompt, 180)
}
func extractOpenAIPrompt(payload map[string]any) string {
	if messages, ok := payload["messages"].([]any); ok {
		return flatten(messages)
	}
	if prompt, ok := payload["prompt"]; ok {
		return flatten(prompt)
	}
	return ""
}
func extractAnthropicPrompt(payload map[string]any) string {
	if messages, ok := payload["messages"].([]any); ok {
		return flatten(messages)
	}
	return flatten(payload["system"])
}
func flatten(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, flatten(item))
		}
		return strings.Join(parts, " ")
	case map[string]any:
		if s, ok := v["content"]; ok {
			return flatten(s)
		}
		if s, ok := v["text"]; ok {
			return flatten(s)
		}
		return ""
	default:
		return fmt.Sprint(v)
	}
}
func embed(content string) []float32 {
	out := make([]float32, 16)
	if content == "" {
		return out
	}
	for i, r := range content {
		out[i%len(out)] += float32((int(r)%97)+1) / 100.0
	}
	return out
}
func mockUsage(content string) map[string]any {
	tokens := len(strings.Fields(content))
	if tokens == 0 {
		tokens = 1
	}
	return map[string]any{"prompt_tokens": tokens, "completion_tokens": 8, "total_tokens": tokens + 8, "input_tokens": tokens, "output_tokens": 8}
}
