// Package engine ports
// `services/pipeline-build-service/src/domain/engine/*` — pipeline DAG
// orchestration. The core package owns topological-sort, execution-stage
// planning, fingerprints, skip-unchanged semantics, and per-node dispatch.
// Runtime adapters live at the service edge: lightweight table transforms use
// the existing OpenFoundry expression stack, Python uses the sidecar executor,
// and distributed compute uses the Spark adapter when configured.
package engine

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

// ExecutionEnvironment mirrors `pub struct ExecutionEnvironment`.
//
// `state` is opaque here so the engine package stays independent of
// the pipeline-build AppState (which carries DB pools, dataset client,
// AI client). Concrete services pass their AppState; the engine only
// reads the `actor_id` for transform fan-out.
type ExecutionEnvironment struct {
	State   any
	ActorID uuid.UUID
}

// ExecutionRequest mirrors `pub struct ExecutionRequest`.
type ExecutionRequest struct {
	StartFromNode          *string
	MaxAttempts            uint32
	DistributedWorkerCount int
	SkipUnchanged          bool
	PriorNodeResults       map[string]NodeResult
}

// DefaultExecutionRequest mirrors the Rust `Default::default()`.
func DefaultExecutionRequest() ExecutionRequest {
	return ExecutionRequest{
		MaxAttempts:            1,
		DistributedWorkerCount: 1,
		SkipUnchanged:          true,
		PriorNodeResults:       map[string]NodeResult{},
	}
}

// NodeResult mirrors `pub struct NodeResult`. Values use pointers for
// the optional fields so JSON encoding emits `null` (matches the Rust
// `Option<T>` default behaviour); `Metadata` is a json.RawMessage
// because it's the encoded NodeExecutionMetadata.
type NodeResult struct {
	NodeID        string          `json:"node_id"`
	Label         string          `json:"label"`
	TransformType string          `json:"transform_type"`
	Status        string          `json:"status"`
	RowsAffected  *uint64         `json:"rows_affected"`
	Attempts      uint32          `json:"attempts"`
	Output        json.RawMessage `json:"output"`
	Error         *string         `json:"error"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	WorkerID      *string         `json:"worker_id,omitempty"`
	StageIndex    *int            `json:"stage_index,omitempty"`
}

// DatasetInputMetadata mirrors `pub struct DatasetInputMetadata`.
type DatasetInputMetadata struct {
	DatasetID uuid.UUID `json:"dataset_id"`
	Name      string    `json:"name"`
	Format    string    `json:"format"`
	Version   int32     `json:"version"`
	RowCount  int64     `json:"row_count"`
	SizeBytes int64     `json:"size_bytes"`
}

// NodeExecutionMetadata mirrors `pub struct NodeExecutionMetadata`.
type NodeExecutionMetadata struct {
	Fingerprint          string                 `json:"fingerprint"`
	Skipped              bool                   `json:"skipped"`
	InputDatasets        []DatasetInputMetadata `json:"input_datasets"`
	OutputDatasetID      *uuid.UUID             `json:"output_dataset_id"`
	OutputDatasetVersion *int32                 `json:"output_dataset_version"`
}

// LoadedDataset mirrors `pub struct LoadedDataset`. The byte payload
// is omitted in the Go skeleton — the runtime helpers that need it
// will populate the slot when their phase lands.
type LoadedDataset struct {
	Metadata    DatasetInputMetadata
	Bytes       []byte
	StoragePath string
}

// TransformResult is the shape every transform-kind helper returns.
type TransformResult struct {
	RowsAffected         *uint64
	Output               json.RawMessage
	OutputDatasetVersion *int32
}

// PipelineNode is a re-export so callers can write
// `engine.execute_pipeline(env, nodes, ...)` without depending on the
// models package. Re-exporting keeps the engine API surface flat.
type PipelineNode = models.PipelineNode
