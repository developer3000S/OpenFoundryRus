package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Pipeline is the legacy `pipelines` table row.
type Pipeline struct {
	ID                  uuid.UUID       `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	OwnerID             uuid.UUID       `json:"owner_id"`
	DAG                 json.RawMessage `json:"dag"`
	Status              string          `json:"status"`
	PipelineType        string          `json:"pipeline_type"`
	Lifecycle           string          `json:"lifecycle,omitempty"`
	ScheduleConfig      json.RawMessage `json:"schedule_config"`
	RetryPolicy         json.RawMessage `json:"retry_policy"`
	NextRunAt           *time.Time      `json:"next_run_at,omitempty"`
	ExternalConfig      json.RawMessage `json:"external_config,omitempty"`
	IncrementalConfig   json.RawMessage `json:"incremental_config,omitempty"`
	StreamingConfig     json.RawMessage `json:"streaming_config,omitempty"`
	DistributedConfig   json.RawMessage `json:"distributed_config,omitempty"`
	ComputeProfileID    *string         `json:"compute_profile_id,omitempty"`
	ProjectID           *uuid.UUID      `json:"project_id,omitempty"`
	DraftDAG            json.RawMessage `json:"draft_dag,omitempty"`
	PublishedDAG        json.RawMessage `json:"published_dag,omitempty"`
	BranchName          string          `json:"branch_name"`
	DraftUpdatedAt      *time.Time      `json:"draft_updated_at,omitempty"`
	PublishedAt         *time.Time      `json:"published_at,omitempty"`
	ActiveVersionID     *uuid.UUID      `json:"active_version_id,omitempty"`
	ProposalState       string          `json:"proposal_state,omitempty"`
	ProposalTitle       *string         `json:"proposal_title,omitempty"`
	ProposalDescription *string         `json:"proposal_description,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// ParsedIR decodes the persisted DAG JSON into the stable Pipeline IR. It
// accepts the legacy `[]PipelineNode` DAG shape so older rows remain readable.
func (p *Pipeline) ParsedIR() (PipelineIR, error) {
	return ParsePipelineIR(p.DAG)
}

// ParsedNodes decodes the DAG JSON into the typed PipelineNode slice.
func (p *Pipeline) ParsedNodes() ([]PipelineNode, error) {
	ir, err := p.ParsedIR()
	if err != nil {
		return nil, err
	}
	return ir.LegacyNodes(), nil
}

func (p *Pipeline) RuntimeIR() (PipelineIR, error) {
	return ParsePipelineIR(p.RuntimeDAG())
}

func (p *Pipeline) RuntimeNodes() ([]PipelineNode, error) {
	ir, err := p.RuntimeIR()
	if err != nil {
		return nil, err
	}
	return ir.LegacyNodes(), nil
}

func (p *Pipeline) RuntimeDAG() json.RawMessage {
	if rawJSONPresent(p.PublishedDAG) {
		return p.PublishedDAG
	}
	if rawJSONPresent(p.DAG) {
		return p.DAG
	}
	return p.DraftDAG
}

func rawJSONPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !strings.EqualFold(string(trimmed), "null")
}

// Schedule decodes the schedule_config JSON, falling back to the
// zero-value when decoding fails (matches the Rust `unwrap_or_default`).
func (p *Pipeline) Schedule() PipelineScheduleConfig {
	var s PipelineScheduleConfig
	_ = json.Unmarshal(p.ScheduleConfig, &s)
	return s
}

// ParsedRetryPolicy mirrors the Rust accessor + Default impl.
func (p *Pipeline) ParsedRetryPolicy() PipelineRetryPolicy {
	out := DefaultPipelineRetryPolicy()
	if len(p.RetryPolicy) > 0 {
		_ = json.Unmarshal(p.RetryPolicy, &out)
	}
	return out
}

// PipelineScheduleConfig is the parsed schedule_config JSON.
type PipelineScheduleConfig struct {
	Enabled bool    `json:"enabled"`
	Cron    *string `json:"cron,omitempty"`
}

// PipelineRetryPolicy is the parsed retry_policy JSON.
type PipelineRetryPolicy struct {
	MaxAttempts             uint32 `json:"max_attempts"`
	RetryOnFailure          bool   `json:"retry_on_failure"`
	AllowPartialReexecution bool   `json:"allow_partial_reexecution"`
}

const (
	PipelineTypeBatch       = "BATCH"
	PipelineTypeFaster      = "FASTER"
	PipelineTypeIncremental = "INCREMENTAL"
	PipelineTypeStreaming   = "STREAMING"
	PipelineTypeExternal    = "EXTERNAL"
	PipelineTypeDistributed = "DISTRIBUTED"

	PipelineLifecycleDraft     = "DRAFT"
	PipelineLifecycleValidated = "VALIDATED"
	PipelineLifecycleDeployed  = "DEPLOYED"
	PipelineLifecycleArchived  = "ARCHIVED"
)

// NormalizePipelineType accepts Foundry-compatible names and OpenFoundry
// aliases while preserving the database vocabulary.
func NormalizePipelineType(raw string) string {
	value := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), "-", "_"))
	switch value {
	case "", "STANDARD":
		return PipelineTypeBatch
	case "LIGHTWEIGHT", "LIGHTWEIGHT_BATCH", "LOCAL", "LOCAL_TABLE", "FAST":
		return PipelineTypeFaster
	case "SPARK", "PYSPARK", "FLINK", "DISTRIBUTED_COMPUTE", "DISTRIBUTED_ENGINE":
		return PipelineTypeDistributed
	default:
		return value
	}
}

func ValidatePipelineType(raw string) error {
	switch NormalizePipelineType(raw) {
	case PipelineTypeBatch, PipelineTypeFaster, PipelineTypeIncremental, PipelineTypeStreaming, PipelineTypeExternal, PipelineTypeDistributed:
		return nil
	default:
		return fmt.Errorf("unsupported pipeline_type %q", raw)
	}
}

func IsLightweightPipelineType(raw string) bool {
	return NormalizePipelineType(raw) == PipelineTypeFaster
}

func NormalizePipelineLifecycle(raw string) string {
	value := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), "-", "_"))
	if value == "" {
		return PipelineLifecycleDraft
	}
	return value
}

func ValidatePipelineLifecycle(raw string) error {
	switch NormalizePipelineLifecycle(raw) {
	case PipelineLifecycleDraft, PipelineLifecycleValidated, PipelineLifecycleDeployed, PipelineLifecycleArchived:
		return nil
	default:
		return fmt.Errorf("unsupported lifecycle %q", raw)
	}
}

// DefaultPipelineRetryPolicy mirrors the Rust `Default::default()`.
func DefaultPipelineRetryPolicy() PipelineRetryPolicy {
	return PipelineRetryPolicy{
		MaxAttempts:             1,
		RetryOnFailure:          false,
		AllowPartialReexecution: true,
	}
}

// PipelineColumnMapping is one entry under `node.config.column_mappings`.
type PipelineColumnMapping struct {
	SourceDatasetID *uuid.UUID `json:"source_dataset_id,omitempty"`
	SourceColumn    string     `json:"source_column"`
	TargetColumn    string     `json:"target_column"`
}

// PipelineNode is one node of the pipeline DAG.
type PipelineNode struct {
	ID              string          `json:"id"`
	Label           string          `json:"label"`
	TransformType   string          `json:"transform_type"`
	Config          json.RawMessage `json:"config,omitempty"`
	DependsOn       []string        `json:"depends_on,omitempty"`
	InputDatasetIDs []uuid.UUID     `json:"input_dataset_ids,omitempty"`
	OutputDatasetID *uuid.UUID      `json:"output_dataset_id,omitempty"`
}

// ColumnMappings extracts the column-mapping list embedded in the
// node's config blob.
func (n *PipelineNode) ColumnMappings() []PipelineColumnMapping {
	if len(n.Config) == 0 {
		return nil
	}
	var holder struct {
		Mappings []PipelineColumnMapping `json:"column_mappings"`
	}
	if err := json.Unmarshal(n.Config, &holder); err != nil {
		return nil
	}
	return holder.Mappings
}

// CreatePipelineRequest is the JSON body for `POST /api/v1/pipelines`.
type CreatePipelineRequest struct {
	Name           string                  `json:"name"`
	Description    *string                 `json:"description,omitempty"`
	Status         *string                 `json:"status,omitempty"`
	PipelineType   *string                 `json:"pipeline_type,omitempty"`
	Lifecycle      *string                 `json:"lifecycle,omitempty"`
	BranchName     *string                 `json:"branch_name,omitempty"`
	DAG            json.RawMessage         `json:"dag,omitempty"`
	IR             *PipelineIR             `json:"ir,omitempty"`
	Nodes          []PipelineNode          `json:"nodes,omitempty"`
	ScheduleConfig *PipelineScheduleConfig `json:"schedule_config,omitempty"`
	RetryPolicy    *PipelineRetryPolicy    `json:"retry_policy,omitempty"`
	External       json.RawMessage         `json:"external,omitempty"`
	Incremental    json.RawMessage         `json:"incremental,omitempty"`
	Streaming      json.RawMessage         `json:"streaming,omitempty"`
	Distributed    json.RawMessage         `json:"distributed,omitempty"`
	ComputeProfile *string                 `json:"compute_profile_id,omitempty"`
	ProjectID      *uuid.UUID              `json:"project_id,omitempty"`
}

// UpdatePipelineRequest is the JSON body for `PATCH /api/v1/pipelines/{id}`.
type UpdatePipelineRequest struct {
	Name           *string                 `json:"name,omitempty"`
	Description    *string                 `json:"description,omitempty"`
	Status         *string                 `json:"status,omitempty"`
	PipelineType   *string                 `json:"pipeline_type,omitempty"`
	Lifecycle      *string                 `json:"lifecycle,omitempty"`
	BranchName     *string                 `json:"branch_name,omitempty"`
	DAG            json.RawMessage         `json:"dag,omitempty"`
	IR             *PipelineIR             `json:"ir,omitempty"`
	Nodes          *[]PipelineNode         `json:"nodes,omitempty"`
	ScheduleConfig *PipelineScheduleConfig `json:"schedule_config,omitempty"`
	RetryPolicy    *PipelineRetryPolicy    `json:"retry_policy,omitempty"`
	External       json.RawMessage         `json:"external,omitempty"`
	Incremental    json.RawMessage         `json:"incremental,omitempty"`
	Streaming      json.RawMessage         `json:"streaming,omitempty"`
	Distributed    json.RawMessage         `json:"distributed,omitempty"`
	ComputeProfile *string                 `json:"compute_profile_id,omitempty"`
	ProjectID      *uuid.UUID              `json:"project_id,omitempty"`
}

func (r CreatePipelineRequest) CanonicalDAG() (json.RawMessage, error) {
	if r.IR != nil {
		if report := r.IR.Validate(); !report.Valid() {
			return nil, report
		}
		ir := r.IR.Normalize()
		return json.Marshal(ir)
	}
	if len(r.DAG) > 0 {
		return CanonicalPipelineDAG(r.DAG)
	}
	return CanonicalPipelineDAGFromNodes(r.Nodes)
}

func (r UpdatePipelineRequest) HasGraphUpdate() bool {
	return r.IR != nil || len(r.DAG) > 0 || r.Nodes != nil
}

func (r UpdatePipelineRequest) CanonicalDAG() (json.RawMessage, error) {
	if r.IR != nil {
		if report := r.IR.Validate(); !report.Valid() {
			return nil, report
		}
		ir := r.IR.Normalize()
		return json.Marshal(ir)
	}
	if len(r.DAG) > 0 {
		return CanonicalPipelineDAG(r.DAG)
	}
	if r.Nodes != nil {
		return CanonicalPipelineDAGFromNodes(*r.Nodes)
	}
	return nil, ErrNoPipelineGraph
}

// ListPipelinesQuery is the URL query for `GET /api/v1/pipelines`.
type ListPipelinesQuery struct {
	Page    *int64  `json:"page,omitempty"`
	PerPage *int64  `json:"per_page,omitempty"`
	Search  *string `json:"search,omitempty"`
	Status  *string `json:"status,omitempty"`
}

// ListPipelinesResponse is the envelope for the list endpoint.
type ListPipelinesResponse struct {
	Data    []Pipeline `json:"data"`
	Total   int64      `json:"total"`
	Page    int64      `json:"page"`
	PerPage int64      `json:"per_page"`
}

type PipelineVersion struct {
	ID                    uuid.UUID       `json:"id"`
	PipelineID            uuid.UUID       `json:"pipeline_id"`
	VersionNumber         int64           `json:"version_number"`
	BranchName            string          `json:"branch_name"`
	VersionKind           string          `json:"version_kind"`
	DAG                   json.RawMessage `json:"dag"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	ScheduleConfig        json.RawMessage `json:"schedule_config"`
	RetryPolicy           json.RawMessage `json:"retry_policy"`
	CreatedBy             *uuid.UUID      `json:"created_by,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	Message               string          `json:"message,omitempty"`
	RestoredFromVersionID *uuid.UUID      `json:"restored_from_version_id,omitempty"`
}

type ListPipelineVersionsResponse struct {
	Data []PipelineVersion `json:"data"`
}

type PublishPipelineRequest struct {
	Message             string  `json:"message,omitempty"`
	BranchName          *string `json:"branch_name,omitempty"`
	ProposalTitle       *string `json:"proposal_title,omitempty"`
	ProposalDescription *string `json:"proposal_description,omitempty"`
}

type PipelinePublishResponse struct {
	Pipeline Pipeline        `json:"pipeline"`
	Version  PipelineVersion `json:"version"`
}

type CreatePipelineProposalRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	BranchName  *string `json:"branch_name,omitempty"`
}

type RestorePipelineVersionRequest struct {
	AsDraft bool   `json:"as_draft"`
	Message string `json:"message,omitempty"`
}
