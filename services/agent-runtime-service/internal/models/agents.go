// Package models holds the wire-format types for agent-runtime-service.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentDefinition mirrors the agent_definitions row.
type AgentDefinition struct {
	ID           uuid.UUID       `json:"id"`
	Slug         string          `json:"slug"`
	Name         string          `json:"name"`
	Description  *string         `json:"description"`
	SystemPrompt *string         `json:"system_prompt"`
	ProviderID   *uuid.UUID      `json:"provider_id"`
	Tools        json.RawMessage `json:"tools"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type CreateAgentRequest struct {
	Slug         string           `json:"slug"`
	Name         string           `json:"name"`
	Description  *string          `json:"description,omitempty"`
	SystemPrompt *string          `json:"system_prompt,omitempty"`
	ProviderID   *uuid.UUID       `json:"provider_id,omitempty"`
	Tools        *json.RawMessage `json:"tools,omitempty"`
}

type UpdateAgentRequest struct {
	Name         *string          `json:"name,omitempty"`
	Description  *string          `json:"description,omitempty"`
	SystemPrompt *string          `json:"system_prompt,omitempty"`
	Tools        *json.RawMessage `json:"tools,omitempty"`
	Status       *string          `json:"status,omitempty"`
}

// AgentRun mirrors agent_runs.
type AgentRun struct {
	ID             uuid.UUID       `json:"id"`
	AgentID        uuid.UUID       `json:"agent_id"`
	ConversationID *uuid.UUID      `json:"conversation_id"`
	Status         string          `json:"status"`
	Input          json.RawMessage `json:"input"`
	FinalOutput    json.RawMessage `json:"final_output"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type StartRunRequest struct {
	ConversationID *uuid.UUID      `json:"conversation_id,omitempty"`
	Input          json.RawMessage `json:"input"`
}

// AgentRunStep mirrors agent_run_steps.
type AgentRunStep struct {
	ID        uuid.UUID       `json:"id"`
	RunID     uuid.UUID       `json:"run_id"`
	StepIndex int32           `json:"step_index"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type RecordStepRequest struct {
	StepIndex int32           `json:"step_index"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
}

type HumanApprovalRequest struct {
	Decision   string     `json:"decision"`
	ReviewerID *uuid.UUID `json:"reviewer_id,omitempty"`
	Note       *string    `json:"note,omitempty"`
}

// LogicFile mirrors the AIP Logic file metadata row. Logic files are
// project/folder-managed resources; there is intentionally no
// personal-home-only placement field.
type LogicFile struct {
	ID                    uuid.UUID       `json:"id"`
	Name                  string          `json:"name"`
	Description           *string         `json:"description"`
	ProjectID             uuid.UUID       `json:"project_id"`
	FolderID              uuid.UUID       `json:"folder_id"`
	OwnerID               uuid.UUID       `json:"owner_id"`
	CurrentDraftVersionID uuid.UUID       `json:"current_draft_version_id"`
	PublishedVersionID    *uuid.UUID      `json:"published_version_id"`
	ExecutionMode         string          `json:"execution_mode"`
	Permissions           json.RawMessage `json:"permissions"`
	ArchivedAt            *time.Time      `json:"archived_at"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type CreateLogicFileRequest struct {
	Name          string           `json:"name"`
	Description   *string          `json:"description,omitempty"`
	ProjectID     uuid.UUID        `json:"project_id"`
	FolderID      uuid.UUID        `json:"folder_id"`
	ExecutionMode *string          `json:"execution_mode,omitempty"`
	Permissions   *json.RawMessage `json:"permissions,omitempty"`
}

type UpdateLogicFileMetadataRequest struct {
	Name          *string          `json:"name,omitempty"`
	Description   *string          `json:"description,omitempty"`
	ExecutionMode *string          `json:"execution_mode,omitempty"`
	Permissions   *json.RawMessage `json:"permissions,omitempty"`
}

type MoveLogicFileRequest struct {
	ProjectID uuid.UUID `json:"project_id"`
	FolderID  uuid.UUID `json:"folder_id"`
}

type DuplicateLogicFileRequest struct {
	Name        *string    `json:"name,omitempty"`
	ProjectID   *uuid.UUID `json:"project_id,omitempty"`
	FolderID    *uuid.UUID `json:"folder_id,omitempty"`
	Description *string    `json:"description,omitempty"`
}
