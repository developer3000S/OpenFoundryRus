package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// AgentsHandlers exposes the agent-catalog CRUD surface mirroring
// libs/ai-kernel/src/handlers/agents.rs:
//   - GET   list_agents
//   - POST  create_agent
//   - PATCH update_agent
//   - POST  execute_agent  (501 until executor + purpose-checkpoint
//                           ports land — see ExecuteAgent)
type AgentsHandlers struct {
	Pool *pgxpool.Pool
}

const agentColumns = `id, name, description, status, system_prompt,
                      objective, tool_ids, planning_strategy,
                      max_iterations, memory, last_execution_at,
                      created_at, updated_at`

func scanAgent(s toolScanner) (models.AgentDefinition, error) {
	var a models.AgentDefinition
	var description, systemPrompt, objective, toolIDsRaw, memoryRaw []byte
	var lastExec *time.Time
	if err := s.Scan(
		&a.ID, &a.Name, &description, &a.Status, &systemPrompt,
		&objective, &toolIDsRaw, &a.PlanningStrategy, &a.MaxIterations,
		&memoryRaw, &lastExec, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return a, err
	}
	a.Description = string(description)
	a.SystemPrompt = string(systemPrompt)
	a.Objective = string(objective)
	if len(toolIDsRaw) > 0 {
		_ = json.Unmarshal(toolIDsRaw, &a.ToolIDs)
	}
	if a.ToolIDs == nil {
		a.ToolIDs = []uuid.UUID{}
	}
	if len(memoryRaw) > 0 {
		_ = json.Unmarshal(memoryRaw, &a.Memory)
	}
	a.LastExecutionAt = lastExec
	return a, nil
}

func (h *AgentsHandlers) loadAgent(ctx context.Context, id uuid.UUID) (*models.AgentDefinition, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+agentColumns+` FROM ai_agents WHERE id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAgents handles `GET /api/v1/agents`.
func (h *AgentsHandlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+agentColumns+` FROM ai_agents
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.AgentDefinition, 0)
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, models.ListAgentsResponse{Data: out})
}

// CreateAgent handles `POST /api/v1/agents`.
func (h *AgentsHandlers) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var body models.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "agent name is required")
		return
	}

	description := derefString(body.Description, "")
	status := derefString(body.Status, models.DefaultAgentStatus)
	systemPrompt := derefString(body.SystemPrompt, "")
	objective := derefString(body.Objective, "")
	planningStrategy := derefString(body.PlanningStrategy, models.DefaultAgentPlanningStrategy)
	maxIterations := models.DefaultAgentMaxIterations
	if body.MaxIterations != nil {
		maxIterations = *body.MaxIterations
	}
	toolIDs := body.ToolIDs
	if toolIDs == nil {
		toolIDs = []uuid.UUID{}
	}
	toolIDsJSON, _ := json.Marshal(toolIDs)
	memoryJSON, _ := json.Marshal(models.AgentMemorySnapshot{})

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ai_agents
              (id, name, description, status, system_prompt, objective,
               tool_ids, planning_strategy, max_iterations, memory,
               last_execution_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULL)
            RETURNING `+agentColumns,
		uuid.New(), strings.TrimSpace(body.Name), description, status,
		systemPrompt, objective, toolIDsJSON, planningStrategy,
		maxIterations, memoryJSON)
	a, err := scanAgent(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// UpdateAgent handles `PATCH /api/v1/agents/{id}`.
func (h *AgentsHandlers) UpdateAgent(w http.ResponseWriter, r *http.Request, agentID uuid.UUID) {
	current, err := h.loadAgent(r.Context(), agentID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var body models.UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := derefString(body.Name, current.Name)
	desc := derefString(body.Description, current.Description)
	status := derefString(body.Status, current.Status)
	systemPrompt := derefString(body.SystemPrompt, current.SystemPrompt)
	objective := derefString(body.Objective, current.Objective)
	planningStrategy := derefString(body.PlanningStrategy, current.PlanningStrategy)
	maxIterations := current.MaxIterations
	if body.MaxIterations != nil {
		maxIterations = *body.MaxIterations
	}
	toolIDs := current.ToolIDs
	if body.ToolIDs != nil {
		toolIDs = *body.ToolIDs
	}
	if toolIDs == nil {
		toolIDs = []uuid.UUID{}
	}
	memory := current.Memory
	if body.Memory != nil {
		memory = *body.Memory
	}
	toolIDsJSON, _ := json.Marshal(toolIDs)
	memoryJSON, _ := json.Marshal(memory)

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ai_agents SET
            name = $2, description = $3, status = $4,
            system_prompt = $5, objective = $6, tool_ids = $7,
            planning_strategy = $8, max_iterations = $9, memory = $10,
            updated_at = NOW()
          WHERE id = $1
          RETURNING `+agentColumns,
		agentID, name, desc, status, systemPrompt, objective,
		toolIDsJSON, planningStrategy, maxIterations, memoryJSON)
	a, err := scanAgent(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ExecuteAgent handles `POST /api/v1/agents/{id}/execute`. The Rust
// implementation chains agents/executor (1307 LOC, deferred) and the
// auth-middleware purpose-checkpoint hook. Until the executor port
// lands this surface validates input and returns 501 — consumers that
// only need List/Create/Update can wire those today; agent execution
// arrives in the executor slice.
func (h *AgentsHandlers) ExecuteAgent(w http.ResponseWriter, r *http.Request, agentID uuid.UUID) {
	var body models.ExecuteAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.UserMessage) == "" {
		writeError(w, http.StatusBadRequest, "agent execution requires a user message")
		return
	}

	current, err := h.loadAgent(r.Context(), agentID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeError(w, http.StatusNotImplemented, "agent execution surface lands with libs/ai-kernel-go/domain/agents/executor port")
}
