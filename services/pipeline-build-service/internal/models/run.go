package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PipelineRun is the legacy per-pipeline run row. The `status` column
// is stored as TEXT and may carry either the canonical BuildState
// vocabulary or the pre-migration legacy values; ProjectBuildState
// implements the same fallback the Rust accessor uses.
type PipelineRun struct {
	ID                uuid.UUID       `json:"id"`
	PipelineID        uuid.UUID       `json:"pipeline_id"`
	Status            string          `json:"status"`
	TriggerType       string          `json:"trigger_type"`
	StartedBy         *uuid.UUID      `json:"started_by,omitempty"`
	AttemptNumber     int32           `json:"attempt_number"`
	StartedFromNodeID *string         `json:"started_from_node_id,omitempty"`
	RetryOfRunID      *uuid.UUID      `json:"retry_of_run_id,omitempty"`
	ExecutionContext  json.RawMessage `json:"execution_context"`
	NodeResults       json.RawMessage `json:"node_results,omitempty"`
	ErrorMessage      *string         `json:"error_message,omitempty"`
	StartedAt         time.Time       `json:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
}

// PipelineNodeResult is the persisted, UI-facing run explanation for one node.
// It is intentionally denormalized so a completed run page can explain what
// happened without replaying the DAG or querying output services.
type PipelineNodeResult struct {
	NodeID          string                      `json:"node_id"`
	Label           string                      `json:"label"`
	TransformType   string                      `json:"transform_type"`
	Status          string                      `json:"status"`
	RowsAffected    *int64                      `json:"rows_affected,omitempty"`
	Attempts        int                         `json:"attempts"`
	Output          map[string]any              `json:"output,omitempty"`
	Error           *string                     `json:"error,omitempty"`
	SchemaDelta     *PipelineRunSchemaDelta     `json:"schema_delta,omitempty"`
	OutputResources []PipelineRunOutputResource `json:"output_resources,omitempty"`
	Events          []PipelineRunEvent          `json:"events,omitempty"`
	LogRID          string                      `json:"log_rid,omitempty"`
}

type PipelineRunSchemaDelta struct {
	ColumnsBefore  []string `json:"columns_before"`
	ColumnsAfter   []string `json:"columns_after"`
	AddedColumns   []string `json:"added_columns,omitempty"`
	RemovedColumns []string `json:"removed_columns,omitempty"`
}

type PipelineRunOutputResource struct {
	Kind           string `json:"kind"`
	RID            string `json:"rid"`
	Name           string `json:"name,omitempty"`
	Branch         string `json:"branch,omitempty"`
	TransactionRID string `json:"transaction_rid,omitempty"`
	Status         string `json:"status"`
}

type PipelineRunEvent struct {
	At         time.Time `json:"at"`
	NodeID     string    `json:"node_id,omitempty"`
	EventType  string    `json:"event_type"`
	From       string    `json:"from,omitempty"`
	To         string    `json:"to,omitempty"`
	Attempt    int       `json:"attempt,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	DatasetRID string    `json:"dataset_rid,omitempty"`
}

// ProjectBuildState mirrors `PipelineRun::build_state` — it converts
// the legacy + canonical status strings into the typed BuildState,
// falling back to BuildRunning for unknown values so the queue UI
// keeps rendering during a migration.
func (r *PipelineRun) ProjectBuildState() BuildState {
	switch r.Status {
	case string(BuildResolution), string(BuildQueued), string(BuildRunning),
		string(BuildAborting), string(BuildFailed), string(BuildAborted),
		string(BuildCompleted):
		return BuildState(r.Status)
	case "pending", "queued":
		return BuildQueued
	case "running":
		return BuildRunning
	case "completed", "succeeded", "success":
		return BuildCompleted
	case "failed":
		return BuildFailed
	case "aborted", "cancelled", "canceled":
		return BuildAborted
	default:
		return BuildRunning
	}
}

// ListRunsQuery is the URL query for `GET /api/v1/pipelines/{id}/runs`.
type ListRunsQuery struct {
	Page    *int64 `json:"page,omitempty"`
	PerPage *int64 `json:"per_page,omitempty"`
}

// TriggerPipelineRequest is the JSON body for `POST /api/v1/pipelines/{id}/runs`.
type TriggerPipelineRequest struct {
	FromNodeID    *string         `json:"from_node_id,omitempty"`
	Context       json.RawMessage `json:"context,omitempty"`
	SkipUnchanged bool            `json:"skip_unchanged"`
}

// RetryPipelineRunRequest is the JSON body for the retry endpoint.
type RetryPipelineRunRequest struct {
	FromNodeID    *string `json:"from_node_id,omitempty"`
	SkipUnchanged bool    `json:"skip_unchanged"`
}
