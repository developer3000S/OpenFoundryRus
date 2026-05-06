// Package handlers exposes the HTTP surface of agent-runtime-service.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/repo"
)

type Handlers struct {
	Repo *repo.Repo
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

// CreateChatCompletion is the OpenAI-style stub the Rust binary
// returns. The real LLM dispatch lands alongside libs/ai-kernel-go/handlers/chat.
func (h *Handlers) CreateChatCompletion(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&body)
	model := json.RawMessage(`"agent-runtime-default"`)
	if body != nil {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(body, &m); err == nil {
			if v, ok := m["model"]; ok {
				model = v
			}
		}
	}
	resp := map[string]any{
		"id":     uuid.New(),
		"object": "chat.completion",
		"model":  model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": "agent-runtime stub: chat completion not yet implemented",
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
	writeJSON(w, http.StatusOK, resp)
}

// AskCopilot is the copilot-style stub the Rust binary returns.
func (h *Handlers) AskCopilot(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&body)
	resp := map[string]any{
		"id":      uuid.New(),
		"answer":  "agent-runtime stub: copilot answer not yet implemented",
		"context": body,
	}
	writeJSON(w, http.StatusOK, resp)
}
