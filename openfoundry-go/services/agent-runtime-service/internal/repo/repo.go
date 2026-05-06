package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/agent-runtime-service/internal/models"
)

type Repo struct {
	Pool *pgxpool.Pool
}

const agentColumns = `id, slug, name, description, system_prompt,
                      provider_id, tools, status, created_at, updated_at`

func scanAgent(s scanner) (models.AgentDefinition, error) {
	var a models.AgentDefinition
	err := s.Scan(&a.ID, &a.Slug, &a.Name, &a.Description, &a.SystemPrompt,
		&a.ProviderID, &a.Tools, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

type scanner interface{ Scan(...any) error }

func (r *Repo) ListAgents(ctx context.Context) ([]models.AgentDefinition, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+agentColumns+` FROM agent_definitions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AgentDefinition, 0)
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) CreateAgent(ctx context.Context, body models.CreateAgentRequest) (models.AgentDefinition, error) {
	tools := json.RawMessage(`[]`)
	if body.Tools != nil {
		tools = *body.Tools
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO agent_definitions
                (id, slug, name, description, system_prompt, provider_id, tools, status)
            VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')
            RETURNING `+agentColumns,
		uuid.New(), body.Slug, body.Name, body.Description, body.SystemPrompt,
		body.ProviderID, tools)
	return scanAgent(row)
}

func (r *Repo) GetAgent(ctx context.Context, id uuid.UUID) (*models.AgentDefinition, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+agentColumns+` FROM agent_definitions WHERE id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repo) UpdateAgent(ctx context.Context, id uuid.UUID, body models.UpdateAgentRequest) (*models.AgentDefinition, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE agent_definitions
            SET name = COALESCE($2, name),
                description = COALESCE($3, description),
                system_prompt = COALESCE($4, system_prompt),
                tools = COALESCE($5, tools),
                status = COALESCE($6, status),
                updated_at = NOW()
          WHERE id = $1
          RETURNING `+agentColumns,
		id, body.Name, body.Description, body.SystemPrompt, body.Tools, body.Status)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const runColumns = `id, agent_id, conversation_id, status, input, final_output, created_at, updated_at`

func scanRun(s scanner) (models.AgentRun, error) {
	var r models.AgentRun
	err := s.Scan(&r.ID, &r.AgentID, &r.ConversationID, &r.Status,
		&r.Input, &r.FinalOutput, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *Repo) ListRuns(ctx context.Context, agentID uuid.UUID) ([]models.AgentRun, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+runColumns+` FROM agent_runs WHERE agent_id = $1 ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AgentRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repo) StartRun(ctx context.Context, agentID uuid.UUID, body models.StartRunRequest) (models.AgentRun, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO agent_runs (id, agent_id, conversation_id, status, input)
           VALUES ($1, $2, $3, 'running', $4) RETURNING `+runColumns,
		uuid.New(), agentID, body.ConversationID, body.Input)
	return scanRun(row)
}

const stepColumns = `id, run_id, step_index, kind, payload, created_at`

func scanStep(s scanner) (models.AgentRunStep, error) {
	var st models.AgentRunStep
	err := s.Scan(&st.ID, &st.RunID, &st.StepIndex, &st.Kind, &st.Payload, &st.CreatedAt)
	return st, err
}

func (r *Repo) RecordStep(ctx context.Context, runID uuid.UUID, body models.RecordStepRequest) (models.AgentRunStep, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO agent_run_steps (id, run_id, step_index, kind, payload)
           VALUES ($1, $2, $3, $4, $5) RETURNING `+stepColumns,
		uuid.New(), runID, body.StepIndex, body.Kind, body.Payload)
	return scanStep(row)
}

func (r *Repo) RecordHumanApproval(ctx context.Context, runID uuid.UUID, payload []byte) (models.AgentRunStep, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO agent_run_steps (id, run_id, step_index, kind, payload)
           VALUES (
             $1,
             $2,
             COALESCE((SELECT MAX(step_index) + 1 FROM agent_run_steps WHERE run_id = $2), 0),
             'human_approval',
             $3
           )
           RETURNING `+stepColumns,
		uuid.New(), runID, payload)
	return scanStep(row)
}
