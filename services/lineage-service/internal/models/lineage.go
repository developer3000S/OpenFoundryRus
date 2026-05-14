// Package models holds the wire-format types for lineage-service.
//
// Field order, JSON tags and types mirror the Rust source under
// `services/lineage-service/src/domain/lineage/mod.rs` exactly so an
// existing Foundry-parity UI hitting either runtime sees the same
// payloads.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ColumnLineageEdge mirrors the FromRow ColumnLineageEdge in Rust.
type ColumnLineageEdge struct {
	ID              uuid.UUID  `json:"id"`
	SourceDatasetID uuid.UUID  `json:"source_dataset_id"`
	SourceColumn    string     `json:"source_column"`
	TargetDatasetID uuid.UUID  `json:"target_dataset_id"`
	TargetColumn    string     `json:"target_column"`
	PipelineID      *uuid.UUID `json:"pipeline_id"`
	NodeID          *string    `json:"node_id"`
	CreatedAt       time.Time  `json:"created_at"`
}

// LineageGraph is the {nodes, edges} graph payload.
type LineageGraph struct {
	Nodes []LineageNode      `json:"nodes"`
	Edges []LineageGraphEdge `json:"edges"`
}

// LineageNode mirrors fusion_base::lineage::LineageNode.
type LineageNode struct {
	ID       uuid.UUID       `json:"id"`
	Kind     string          `json:"kind"`
	Label    string          `json:"label"`
	Marking  string          `json:"marking"`
	Metadata json.RawMessage `json:"metadata"`
}

// LineageGraphEdge mirrors LineageGraphEdge.
type LineageGraphEdge struct {
	ID               uuid.UUID       `json:"id"`
	Source           uuid.UUID       `json:"source"`
	SourceKind       string          `json:"source_kind"`
	Target           uuid.UUID       `json:"target"`
	TargetKind       string          `json:"target_kind"`
	RelationKind     string          `json:"relation_kind"`
	PipelineID       *uuid.UUID      `json:"pipeline_id"`
	WorkflowID       *uuid.UUID      `json:"workflow_id"`
	NodeID           *string         `json:"node_id"`
	StepID           *string         `json:"step_id"`
	EffectiveMarking string          `json:"effective_marking"`
	Metadata         json.RawMessage `json:"metadata"`
}

// LineagePathHop mirrors LineagePathHop.
type LineagePathHop struct {
	SourceID         uuid.UUID `json:"source_id"`
	SourceKind       string    `json:"source_kind"`
	TargetID         uuid.UUID `json:"target_id"`
	TargetKind       string    `json:"target_kind"`
	RelationKind     string    `json:"relation_kind"`
	EffectiveMarking string    `json:"effective_marking"`
}

// LineageImpactItem mirrors LineageImpactItem.
type LineageImpactItem struct {
	ID                      uuid.UUID        `json:"id"`
	Kind                    string           `json:"kind"`
	Label                   string           `json:"label"`
	Distance                int              `json:"distance"`
	Marking                 string           `json:"marking"`
	EffectiveMarking        string           `json:"effective_marking"`
	RequiresAcknowledgement bool             `json:"requires_acknowledgement"`
	Metadata                json.RawMessage  `json:"metadata"`
	Path                    []LineagePathHop `json:"path"`
}

// LineageBuildCandidate mirrors LineageBuildCandidate.
type LineageBuildCandidate struct {
	ID                      uuid.UUID       `json:"id"`
	Kind                    string          `json:"kind"`
	Label                   string          `json:"label"`
	Status                  *string         `json:"status"`
	Distance                int             `json:"distance"`
	Triggerable             bool            `json:"triggerable"`
	Marking                 string          `json:"marking"`
	EffectiveMarking        string          `json:"effective_marking"`
	RequiresAcknowledgement bool            `json:"requires_acknowledgement"`
	BlockedReason           *string         `json:"blocked_reason"`
	Metadata                json.RawMessage `json:"metadata"`
}

// LineageImpactAnalysis mirrors LineageImpactAnalysis.
type LineageImpactAnalysis struct {
	Root              LineageNode             `json:"root"`
	PropagatedMarking string                  `json:"propagated_marking"`
	Upstream          []LineageImpactItem     `json:"upstream"`
	Downstream        []LineageImpactItem     `json:"downstream"`
	BuildCandidates   []LineageBuildCandidate `json:"build_candidates"`
}

// LineageBuildRequest mirrors the POST body for trigger_dataset_lineage_builds.
type LineageBuildRequest struct {
	IncludeWorkflows            bool            `json:"include_workflows,omitempty"`
	DryRun                      bool            `json:"dry_run,omitempty"`
	AcknowledgeSensitiveLineage bool            `json:"acknowledge_sensitive_lineage,omitempty"`
	MaxDepth                    *int            `json:"max_depth,omitempty"`
	Context                     json.RawMessage `json:"context,omitempty"`
}

// LineageBuildTriggerResult mirrors LineageBuildTriggerResult.
type LineageBuildTriggerResult struct {
	ID      uuid.UUID  `json:"id"`
	Kind    string     `json:"kind"`
	Label   string     `json:"label"`
	RunID   *uuid.UUID `json:"run_id"`
	Status  string     `json:"status"`
	Message *string    `json:"message"`
}

// LineageBuildResult mirrors LineageBuildResult.
type LineageBuildResult struct {
	Root                         LineageNode                 `json:"root"`
	DryRun                       bool                        `json:"dry_run"`
	AcknowledgedSensitiveLineage bool                        `json:"acknowledged_sensitive_lineage"`
	PropagatedMarking            string                      `json:"propagated_marking"`
	Candidates                   []LineageBuildCandidate     `json:"candidates"`
	Triggered                    []LineageBuildTriggerResult `json:"triggered"`
	Skipped                      []LineageBuildTriggerResult `json:"skipped"`
}

// WorkflowLineageSyncRequest mirrors the workflow sync POST body.
type WorkflowLineageSyncRequest struct {
	Workflow  WorkflowLineageNodeInput       `json:"workflow"`
	Relations []WorkflowLineageRelationInput `json:"relations,omitempty"`
}

// WorkflowLineageNodeInput mirrors WorkflowLineageNodeInput.
type WorkflowLineageNodeInput struct {
	ID          uuid.UUID       `json:"id"`
	Label       string          `json:"label"`
	Status      string          `json:"status"`
	TriggerType string          `json:"trigger_type"`
	Marking     *string         `json:"marking,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// WorkflowLineageRelationInput mirrors WorkflowLineageRelationInput.
type WorkflowLineageRelationInput struct {
	SourceID     uuid.UUID       `json:"source_id"`
	SourceKind   string          `json:"source_kind"`
	TargetID     uuid.UUID       `json:"target_id"`
	TargetKind   string          `json:"target_kind"`
	RelationKind string          `json:"relation_kind"`
	StepID       *string         `json:"step_id,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	Marking      *string         `json:"marking,omitempty"`
}

// InternalWorkflowLineageRunRequest mirrors the body POSTed to the
// workflow service /internal/.../runs/lineage endpoint.
type InternalWorkflowLineageRunRequest struct {
	Context json.RawMessage `json:"context,omitempty"`
}

// LineageNodeRecord is the persisted overlay row from `lineage_nodes`.
//
// Exposed at package scope (vs Rust's private struct) because the
// store + repo packages both manipulate it.
type LineageNodeRecord struct {
	EntityID   uuid.UUID       `json:"entity_id"`
	EntityKind string          `json:"entity_kind"`
	Label      string          `json:"label"`
	Marking    string          `json:"marking"`
	Metadata   json.RawMessage `json:"metadata"`
}

// LineageRelationRecord is the runtime-store relation row consumed by
// the graph builder / BFS / impact analysis. Mirrors the private Rust
// `LineageRelationRecord`.
type LineageRelationRecord struct {
	ID               uuid.UUID       `json:"id"`
	SourceID         uuid.UUID       `json:"source_id"`
	SourceKind       string          `json:"source_kind"`
	TargetID         uuid.UUID       `json:"target_id"`
	TargetKind       string          `json:"target_kind"`
	RelationKind     string          `json:"relation_kind"`
	PipelineID       *uuid.UUID      `json:"pipeline_id"`
	WorkflowID       *uuid.UUID      `json:"workflow_id"`
	NodeID           *string         `json:"node_id"`
	StepID           *string         `json:"step_id"`
	EffectiveMarking string          `json:"effective_marking"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedAt        time.Time       `json:"created_at"`
}

// DatasetMetadata mirrors the wire-format response of the
// dataset-service /internal/datasets/{id}/metadata endpoint.
type DatasetMetadata struct {
	ID                 uuid.UUID `json:"id"`
	RID                string    `json:"rid"`
	Name               string    `json:"name"`
	DisplayName        string    `json:"display_name"`
	Format             string    `json:"format"`
	Marking            string    `json:"marking"`
	Tags               []string  `json:"tags"`
	CurrentVersion     int32     `json:"current_version"`
	ActiveBranch       string    `json:"active_branch"`
	OwnerID            uuid.UUID `json:"owner_id"`
	ParentFolderRID    string    `json:"parent_folder_rid"`
	FolderPath         string    `json:"folder_path"`
	ProjectID          string    `json:"project_id"`
	ProjectRID         string    `json:"project_rid"`
	Path               string    `json:"path"`
	ResourceVisibility string    `json:"resource_visibility"`
	Links              struct {
		Self    string `json:"self"`
		Preview string `json:"preview"`
		Lineage string `json:"lineage"`
	} `json:"links"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NodeKind enumerates the valid entity_kind / source_kind / target_kind
// values. We expose the parser here because the runtime store, graph
// builder and snapshot loader all need to translate between strings
// and the typed enum.
type NodeKind string

const (
	KindDataset  NodeKind = "dataset"
	KindPipeline NodeKind = "pipeline"
	KindWorkflow NodeKind = "workflow"
)

// ParseNodeKind returns (kind, true) for valid kinds, or (_, false).
func ParseNodeKind(s string) (NodeKind, bool) {
	switch s {
	case "dataset":
		return KindDataset, true
	case "pipeline":
		return KindPipeline, true
	case "workflow":
		return KindWorkflow, true
	default:
		return "", false
	}
}

// String returns the canonical lowercase form.
func (k NodeKind) String() string { return string(k) }

// NodeKey is the (id, kind) tuple used as a map key throughout the
// graph + BFS + impact code paths.
type NodeKey struct {
	ID   uuid.UUID
	Kind NodeKind
}

// Relation kind constants — kept identical to the Rust ones so callers
// across both runtimes can still compare on equality.
const (
	RelationKindDerives       = "derives"
	RelationKindColumnDerives = "column_derives"
	RelationKindConsumes      = "consumes"
	RelationKindProduces      = "produces"
	MetadataSourceColumnKey   = "source_column"
	MetadataTargetColumnKey   = "target_column"
)
