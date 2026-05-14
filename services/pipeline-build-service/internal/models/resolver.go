package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ViewFilter is an opaque resolve-time input-view selector. The Rust
// service materialises these selectors immediately before creating job rows;
// the Go resolver carries them through without executing DAG runtime logic.
type ViewFilter json.RawMessage

// InputSpec mirrors the resolver-facing subset of Rust build_resolution::InputSpec.
type InputSpec struct {
	DatasetRID    string       `json:"dataset_rid"`
	FallbackChain []string     `json:"fallback_chain,omitempty"`
	ViewFilter    []ViewFilter `json:"view_filter,omitempty"`
	RequireFresh  bool         `json:"require_fresh,omitempty"`
}

// JobSpec is the declarative recipe consumed by build resolution. Pipeline
// authoring remains the source of truth; resolver repositories load this shape.
type JobSpec struct {
	RID               string          `json:"rid"`
	PipelineRID       string          `json:"pipeline_rid"`
	BranchName        string          `json:"branch_name"`
	Inputs            []InputSpec     `json:"inputs"`
	OutputDatasetRIDs []string        `json:"output_dataset_rids"`
	LogicKind         string          `json:"logic_kind"`
	LogicPayload      json.RawMessage `json:"logic_payload,omitempty"`
	ContentHash       string          `json:"content_hash"`
}

// BranchSnapshot is a dataset branch as seen by dataset-versioning-service.
type BranchSnapshot struct {
	Name               string  `json:"name"`
	HeadTransactionRID *string `json:"head_transaction_rid,omitempty"`
}

// OpenedTransaction records the transaction opened for a resolved output.
type OpenedTransaction struct {
	DatasetRID     string `json:"dataset_rid"`
	TransactionRID string `json:"transaction_rid"`
}

// ResolvedInputView is the schema bundle selected for an external input.
type ResolvedInputView struct {
	DatasetRID          string          `json:"dataset_rid"`
	Branch              string          `json:"branch"`
	HeadTransactionRID  *string         `json:"head_transaction_rid,omitempty"`
	Schema              json.RawMessage `json:"schema"`
}

// ResolvedJob is the resolver's DAG-executor-neutral job plan entry.
type ResolvedJob struct {
	ID                    uuid.UUID `json:"id"`
	JobSpecRID            string    `json:"job_spec_rid"`
	OutputTransactionRIDs []string  `json:"output_transaction_rids"`
	DependsOnJobSpecRIDs  []string  `json:"depends_on_job_spec_rids,omitempty"`
}

// ResolvedBuild is the full resolve_build outcome handed to callers/tests.
type ResolvedBuild struct {
	BuildID            uuid.UUID           `json:"build_id"`
	State              BuildState          `json:"state"`
	ForceBuild         bool                `json:"force_build,omitempty"`
	JobSpecs           []JobSpec           `json:"job_specs"`
	InputViews         []ResolvedInputView `json:"input_views"`
	OpenedTransactions []OpenedTransaction `json:"opened_transactions"`
	Jobs               []ResolvedJob       `json:"jobs"`
	FanOutStages       [][]string          `json:"fan_out_stages"`
	QueuedReason       *string             `json:"queued_reason,omitempty"`
	ResolvedAt         time.Time           `json:"resolved_at"`
}
