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

const logicFileColumns = `id, name, description, project_id, folder_id, owner_id,
                         current_draft_version_id, published_version_id,
                         execution_mode, permissions, archived_at, created_at, updated_at`

func scanLogicFile(s scanner) (models.LogicFile, error) {
	var lf models.LogicFile
	err := s.Scan(&lf.ID, &lf.Name, &lf.Description, &lf.ProjectID, &lf.FolderID,
		&lf.OwnerID, &lf.CurrentDraftVersionID, &lf.PublishedVersionID,
		&lf.ExecutionMode, &lf.Permissions, &lf.ArchivedAt, &lf.CreatedAt, &lf.UpdatedAt)
	return lf, err
}

func nullableUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}

func defaultLogicPermissions(ownerID uuid.UUID, raw *json.RawMessage) json.RawMessage {
	if raw != nil && len(*raw) > 0 {
		return *raw
	}
	b, _ := json.Marshal(map[string][]string{
		"owners":  {ownerID.String()},
		"editors": {},
		"viewers": {},
	})
	return b
}

func (r *Repo) CreateLogicFile(ctx context.Context, ownerID uuid.UUID, body models.CreateLogicFileRequest) (models.LogicFile, error) {
	executionMode := "user_scoped"
	if body.ExecutionMode != nil {
		executionMode = *body.ExecutionMode
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO logic_files
		        (id, name, description, project_id, folder_id, owner_id,
		         current_draft_version_id, execution_mode, permissions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+logicFileColumns,
		uuid.New(), body.Name, body.Description, body.ProjectID, body.FolderID,
		ownerID, uuid.New(), executionMode, defaultLogicPermissions(ownerID, body.Permissions))
	return scanLogicFile(row)
}

func (r *Repo) GetLogicFile(ctx context.Context, id uuid.UUID, actorID uuid.UUID, includeArchived bool, admin bool) (*models.LogicFile, error) {
	query := `SELECT ` + logicFileColumns + ` FROM logic_files
	          WHERE id = $1
	            AND ($2::bool OR archived_at IS NULL)
	            AND ($4::bool OR owner_id = $3 OR permissions->'owners' ? $3::text OR permissions->'editors' ? $3::text OR permissions->'viewers' ? $3::text)`
	lf, err := scanLogicFile(r.Pool.QueryRow(ctx, query, id, includeArchived, actorID, admin))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}

func (r *Repo) ListLogicFiles(ctx context.Context, projectID, folderID *uuid.UUID, actorID uuid.UUID, includeArchived bool, admin bool) ([]models.LogicFile, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+logicFileColumns+` FROM logic_files
		  WHERE ($1::uuid IS NULL OR project_id = $1)
		    AND ($2::uuid IS NULL OR folder_id = $2)
		    AND ($3::bool OR archived_at IS NULL)
		    AND ($5::bool OR owner_id = $4 OR permissions->'owners' ? $4::text OR permissions->'editors' ? $4::text OR permissions->'viewers' ? $4::text)
		  ORDER BY updated_at DESC, created_at DESC`,
		nullableUUID(projectID), nullableUUID(folderID), includeArchived, actorID, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.LogicFile, 0)
	for rows.Next() {
		lf, err := scanLogicFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lf)
	}
	return out, rows.Err()
}

func (r *Repo) UpdateLogicFileMetadata(ctx context.Context, id, actorID uuid.UUID, body models.UpdateLogicFileMetadataRequest, admin bool) (*models.LogicFile, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE logic_files
		    SET name = COALESCE($2, name),
		        description = COALESCE($3, description),
		        execution_mode = COALESCE($4, execution_mode),
		        permissions = COALESCE($5, permissions),
		        updated_at = now()
		  WHERE id = $1 AND archived_at IS NULL
		    AND ($7::bool OR owner_id = $6 OR permissions->'owners' ? $6::text OR permissions->'editors' ? $6::text)
		  RETURNING `+logicFileColumns,
		id, body.Name, body.Description, body.ExecutionMode, body.Permissions, actorID, admin)
	lf, err := scanLogicFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}

func (r *Repo) MoveLogicFile(ctx context.Context, id, actorID uuid.UUID, body models.MoveLogicFileRequest, admin bool) (*models.LogicFile, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE logic_files
		    SET project_id = $2, folder_id = $3, updated_at = now()
		  WHERE id = $1 AND archived_at IS NULL
		    AND ($5::bool OR owner_id = $4 OR permissions->'owners' ? $4::text OR permissions->'editors' ? $4::text)
		  RETURNING `+logicFileColumns,
		id, body.ProjectID, body.FolderID, actorID, admin)
	lf, err := scanLogicFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}

func (r *Repo) DuplicateLogicFile(ctx context.Context, id, actorID uuid.UUID, body models.DuplicateLogicFileRequest, admin bool) (*models.LogicFile, error) {
	newID := uuid.New()
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO logic_files
		        (id, name, description, project_id, folder_id, owner_id,
		         current_draft_version_id, execution_mode, permissions)
		 SELECT $1,
		        COALESCE($2, name || ' (copy)'),
		        COALESCE($3, description),
		        COALESCE($4, project_id),
		        COALESCE($5, folder_id),
		        $6,
		        $7,
		        execution_mode,
		        permissions
		   FROM logic_files
		  WHERE id = $8 AND archived_at IS NULL
		    AND ($10::bool OR owner_id = $9 OR permissions->'owners' ? $9::text OR permissions->'editors' ? $9::text)
		 RETURNING `+logicFileColumns,
		newID, body.Name, body.Description, nullableUUID(body.ProjectID), nullableUUID(body.FolderID), actorID, uuid.New(), id, actorID, admin)
	lf, err := scanLogicFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}

func (r *Repo) ArchiveLogicFile(ctx context.Context, id, actorID uuid.UUID, admin bool) (*models.LogicFile, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE logic_files
		    SET archived_at = COALESCE(archived_at, now()), updated_at = now()
		  WHERE id = $1
		    AND ($3::bool OR owner_id = $2 OR permissions->'owners' ? $2::text OR permissions->'editors' ? $2::text)
		  RETURNING `+logicFileColumns,
		id, actorID, admin)
	lf, err := scanLogicFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}

func (r *Repo) RestoreLogicFile(ctx context.Context, id, actorID uuid.UUID, admin bool) (*models.LogicFile, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE logic_files
		    SET archived_at = NULL, updated_at = now()
		  WHERE id = $1 AND archived_at IS NOT NULL
		    AND ($3::bool OR owner_id = $2 OR permissions->'owners' ? $2::text OR permissions->'editors' ? $2::text)
		  RETURNING `+logicFileColumns,
		id, actorID, admin)
	lf, err := scanLogicFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lf, nil
}
