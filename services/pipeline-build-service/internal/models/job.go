package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// JobState mirrors Foundry "Builds.md § Job states" verbatim.
type JobState string

const (
	JobWaiting      JobState = "WAITING"
	JobRunPending   JobState = "RUN_PENDING"
	JobRunning      JobState = "RUNNING"
	JobAbortPending JobState = "ABORT_PENDING"
	JobAborted      JobState = "ABORTED"
	JobFailed       JobState = "FAILED"
	JobCompleted    JobState = "COMPLETED"
)

// AllJobStates lists every valid JobState — matches the Rust ALL slice.
var AllJobStates = []JobState{
	JobWaiting, JobRunPending, JobRunning, JobAbortPending,
	JobAborted, JobFailed, JobCompleted,
}

// IsTerminal mirrors the Rust `JobState::is_terminal`.
func (s JobState) IsTerminal() bool {
	return s == JobAborted || s == JobFailed || s == JobCompleted
}

// UnknownJobState is returned by ParseJobState on unknown input.
type UnknownJobState struct{ Value string }

func (e *UnknownJobState) Error() string { return "unknown job state: " + e.Value }

// ParseJobState converts a wire string to a typed JobState.
func ParseJobState(s string) (JobState, error) {
	for _, candidate := range AllJobStates {
		if string(candidate) == s {
			return candidate, nil
		}
	}
	return "", &UnknownJobState{Value: s}
}

// Job is the row shape for the `jobs` table.
type Job struct {
	ID                    uuid.UUID              `json:"id"`
	RID                   string                 `json:"rid"`
	BuildID               uuid.UUID              `json:"build_id"`
	JobSpecRID            string                 `json:"job_spec_rid"`
	ExecutionStatus       string                 `json:"execution_status,omitempty"`
	LogicKind             string                 `json:"logic_kind,omitempty"`
	JobSpecContentHash    string                 `json:"job_spec_content_hash,omitempty"`
	InputDatasetRIDs      []string               `json:"input_dataset_rids,omitempty"`
	OutputDatasetRIDs     []string               `json:"output_dataset_rids,omitempty"`
	DependsOnJobSpecRIDs  []string               `json:"depends_on_job_spec_rids,omitempty"`
	InputSignature        string                 `json:"input_signature,omitempty"`
	CanonicalLogicHash    string                 `json:"canonical_logic_hash,omitempty"`
	State                 string                 `json:"state"`
	OutputTransactionRIDs []string               `json:"output_transaction_rids"`
	StateChangedAt        time.Time              `json:"state_changed_at"`
	StartedAt             *time.Time             `json:"started_at,omitempty"`
	FinishedAt            *time.Time             `json:"finished_at,omitempty"`
	DurationMillis        *int64                 `json:"duration_ms,omitempty"`
	Attempt               int32                  `json:"attempt"`
	StaleSkipped          bool                   `json:"stale_skipped"`
	Runtime               string                 `json:"runtime,omitempty"`
	WorkerID              string                 `json:"worker_id,omitempty"`
	RowCount              *int64                 `json:"row_count,omitempty"`
	FileCount             *int64                 `json:"file_count,omitempty"`
	OutputMetadata        json.RawMessage        `json:"output_metadata,omitempty"`
	OutputTransactions    []JobOutputTransaction `json:"output_transactions,omitempty"`
	FailureReason         *string                `json:"failure_reason,omitempty"`
	OutputContentHash     *string                `json:"output_content_hash,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
}

// JobState projects the string column to a typed value.
func (j *Job) JobState() (JobState, error) { return ParseJobState(j.State) }

type JobOutputTransaction struct {
	OutputDatasetRID string `json:"output_dataset_rid"`
	TransactionRID   string `json:"transaction_rid"`
	Committed        bool   `json:"committed"`
	Aborted          bool   `json:"aborted"`
	Status           string `json:"status"`
}

// NormalizeJobExecutionStatus maps Foundry's internal job lifecycle states to
// the UI/search status vocabulary used by build and schedule history.
func NormalizeJobExecutionStatus(state string, staleSkipped bool, reason *string) string {
	switch JobState(state) {
	case JobCompleted:
		if staleSkipped {
			return "ignored"
		}
		return "succeeded"
	case JobFailed:
		return "failed"
	case JobAborted, JobAbortPending:
		if reason != nil && strings.Contains(strings.ToLower(*reason), "dependency") {
			return "skipped"
		}
		return "cancelled"
	case JobRunning:
		return "running"
	default:
		return "queued"
	}
}
