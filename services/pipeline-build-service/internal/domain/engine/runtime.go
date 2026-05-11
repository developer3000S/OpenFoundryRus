// Package engine — node fingerprint + metadata helpers + transform
// runtime dispatch helpers.
//
// The lightweight table path is intentionally dependency-free: it uses
// OpenFoundry's existing JSON row and expression stack for local Pipeline
// Builder transforms. Python and Spark are dispatched by the HTTP handler
// runtime, while this legacy engine keeps explicit unavailable errors for
// runtime families that still need host adapters:
//
//   - LLM → ai-service HTTP phase
//   - WASM → wasmtime-go phase
//   - External/Remote → connector-management-service HTTP phase
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

// SparkRuntime is the narrow port the engine needs from the host AppState to
// dispatch SparkApplication submissions. Implemented by the production
// pipeline-build-service AppState (which embeds spark.SparkClient + the
// per-cluster defaults); satisfied in tests by a thin in-memory fake.
type SparkRuntime interface {
	SparkClient() spark.SparkClient
	SparkNamespace() string
	SparkRunnerImage() string
	SparkPollInterval() time.Duration
	SparkPollTimeout() time.Duration
}

// distributedComputeNodeConfig is the JSON shape stored under
// PipelineNode.Config when transform_type ∈ {spark, pyspark}. The
// pipeline-build-service handlers serialise the user's pipeline DAG into this
// shape; the engine reads it back here.
type distributedComputeNodeConfig struct {
	SQL         string                       `json:"sql,omitempty"`
	Format      string                       `json:"format,omitempty"`
	Catalog     string                       `json:"catalog,omitempty"`
	CatalogURI  string                       `json:"catalog_uri,omitempty"`
	S3Endpoint  string                       `json:"s3_endpoint,omitempty"`
	Resources   spark.SparkResourceOverrides `json:"resources,omitempty"`
	RunnerImage string                       `json:"runner_image,omitempty"`
	Application spark.SparkApplicationType   `json:"application_type,omitempty"`
}

// nodeFingerprint mirrors `pub fn node_fingerprint`. Hashes the node
// definition + dependency fingerprints + sorted input metadata so
// `skip_unchanged` can collapse re-runs whose effective inputs are
// unchanged.
func nodeFingerprint(
	node *PipelineNode,
	inputs []LoadedDataset,
	dependencyFingerprints map[string]string,
) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(node.ID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(node.Label))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(node.TransformType))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(node.Config)
	_, _ = h.Write([]byte{0})

	type inputKey struct {
		datasetID   string
		version     int32
		sizeBytes   int64
		storagePath string
	}
	keys := make([]inputKey, 0, len(inputs))
	for _, in := range inputs {
		keys = append(keys, inputKey{
			datasetID:   in.Metadata.DatasetID.String(),
			version:     in.Metadata.Version,
			sizeBytes:   in.Metadata.SizeBytes,
			storagePath: in.StoragePath,
		})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].datasetID < keys[j].datasetID })
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s|%d|%d|%s|", k.datasetID, k.version, k.sizeBytes, k.storagePath)
	}
	_, _ = h.Write([]byte{0})

	type depKey struct{ name, fp string }
	deps := make([]depKey, 0, len(node.DependsOn))
	for _, name := range node.DependsOn {
		deps = append(deps, depKey{name: name, fp: dependencyFingerprints[name]})
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].name < deps[j].name })
	for _, d := range deps {
		_, _ = fmt.Fprintf(h, "%s=%s|", d.name, d.fp)
	}

	return fmt.Sprintf("%016x", h.Sum64())
}

// buildMetadata mirrors `pub fn build_metadata`. Encodes the
// `NodeExecutionMetadata` struct into the canonical JSON blob the
// runner stores on the NodeResult.
func buildMetadata(
	fingerprint string,
	skipped bool,
	inputs []LoadedDataset,
	outputDatasetID *uuid.UUID,
	outputDatasetVersion *int32,
) json.RawMessage {
	datasets := make([]DatasetInputMetadata, 0, len(inputs))
	for _, in := range inputs {
		datasets = append(datasets, in.Metadata)
	}
	out, _ := json.Marshal(NodeExecutionMetadata{
		Fingerprint:          fingerprint,
		Skipped:              skipped,
		InputDatasets:        datasets,
		OutputDatasetID:      outputDatasetID,
		OutputDatasetVersion: outputDatasetVersion,
	})
	return out
}

// fingerprintFromMetadata mirrors `pub fn fingerprint_from_metadata`.
func fingerprintFromMetadata(metadata json.RawMessage) (string, bool) {
	if len(metadata) == 0 {
		return "", false
	}
	var m NodeExecutionMetadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		return "", false
	}
	return m.Fingerprint, true
}

// outputDatasetVersionFromMetadata mirrors
// `pub fn output_dataset_version_from_metadata`.
func outputDatasetVersionFromMetadata(metadata json.RawMessage) *int32 {
	if len(metadata) == 0 {
		return nil
	}
	var m NodeExecutionMetadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil
	}
	return m.OutputDatasetVersion
}

// ── Transform runtime dispatch ─────────────────────────────────────

// loadNodeInputs mirrors `runtime::load_node_inputs`. The Phase A
// version returns an empty list — the dataset-versioning client +
// storage-fetch wiring belong to a follow-up. This is enough for the
// orchestration layer to drive forward when no node depends on
// physical inputs (passthrough nodes, tests).
func loadNodeInputs(_ context.Context, _ any, _ uuid.UUID, _ *PipelineNode) ([]LoadedDataset, error) {
	return []LoadedDataset{}, nil
}

// transformRuntimeUnavailable is the canonical failure for transform families
// that need host adapters outside this legacy engine package.
func transformRuntimeUnavailable(kind string) error {
	return fmt.Errorf("transform_runtime_unavailable:%s", kind)
}

func executeSQLTransform(_ context.Context, _ any, node *PipelineNode, _ []LoadedDataset) (TransformResult, error) {
	rows := uint64(countInlineRows(node.Config))
	output := legacyRuntimeOutput("lightweight_table", node.TransformType, rows)
	return TransformResult{RowsAffected: &rows, Output: output}, nil
}

func executePythonTransform(_ context.Context, _ any, _ *PipelineNode, _ []LoadedDataset) (TransformResult, error) {
	return TransformResult{}, fmt.Errorf("python_sidecar_not_configured: use handler runtimeNodeRunner with a Python TransformExecutor")
}

func executeLLMTransform(_ context.Context, _ any, _ *PipelineNode, _ []LoadedDataset) (TransformResult, error) {
	return TransformResult{}, transformRuntimeUnavailable("llm")
}

func executeWASMTransform(_ *PipelineNode) (*uint64, json.RawMessage, error) {
	return nil, nil, transformRuntimeUnavailable("wasm")
}

func executePassthroughTransform(_ context.Context, _ any, node *PipelineNode, inputs []LoadedDataset) (*uint64, json.RawMessage, *int32, error) {
	rows := uint64(len(inputs))
	if inline := countInlineRows(node.Config); inline > 0 {
		rows = uint64(inline)
	}
	output := legacyRuntimeOutput("lightweight_table", node.TransformType, rows)
	return &rows, output, nil, nil
}

// executeDistributedComputeTransform submits a SparkApplication CR via the
// host AppState's SparkClient and watches the CR until it terminates. The host
// AppState is checked for the SparkRuntime interface — if the interface is not
// satisfied (e.g. because pipeline-build-service was booted without the k8s
// client wiring) we surface the canonical Phase A failure so callers can
// produce a clear configuration error rather than crashing.
func executeDistributedComputeTransform(ctx context.Context, state any, node *PipelineNode, inputs []LoadedDataset) (TransformResult, error) {
	runtime, ok := state.(SparkRuntime)
	if !ok || runtime == nil || runtime.SparkClient() == nil {
		return TransformResult{}, transformRuntimeUnavailable("distributed")
	}

	cfg, err := parseDistributedComputeConfig(node)
	if err != nil {
		return TransformResult{}, err
	}

	pipelineID := node.ID
	runID := uuid.NewString()

	inputDataset := ""
	if len(inputs) > 0 {
		inputDataset = inputs[0].Metadata.DatasetID.String()
	}
	outputDataset := ""
	if node.OutputDatasetID != nil {
		outputDataset = node.OutputDatasetID.String()
	}
	if inputDataset == "" {
		inputDataset = outputDataset
	}

	image := strings.TrimSpace(cfg.RunnerImage)
	if image == "" {
		image = runtime.SparkRunnerImage()
	}

	input := spark.PipelineRunInput{
		PipelineID:          pipelineID,
		RunID:               runID,
		Namespace:           runtime.SparkNamespace(),
		ApplicationType:     cfg.Application,
		PipelineRunnerImage: image,
		InputDatasetRID:     inputDataset,
		OutputDatasetRID:    outputDataset,
		Resources:           cfg.Resources,
		Catalog:             cfg.Catalog,
		CatalogURI:          cfg.CatalogURI,
		S3Endpoint:          cfg.S3Endpoint,
		InlineSQL:           cfg.SQL,
		InlineFormat:        cfg.Format,
	}

	client := runtime.SparkClient()
	name, err := client.SubmitPipelineRun(ctx, input)
	if err != nil {
		return TransformResult{}, fmt.Errorf("submit SparkApplication: %w", err)
	}

	pollInterval := runtime.SparkPollInterval()
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	pollTimeout := runtime.SparkPollTimeout()
	if pollTimeout <= 0 {
		pollTimeout = 30 * time.Minute
	}
	deadline := time.Now().Add(pollTimeout)

	for {
		report, err := client.GetPipelineRunStatus(ctx, runtime.SparkNamespace(), name)
		if err != nil {
			return TransformResult{}, fmt.Errorf("get SparkApplication status: %w", err)
		}
		if report != nil {
			switch report.Status {
			case spark.SparkRunSucceeded:
				output, _ := json.Marshal(map[string]any{
					"spark_application": name,
					"namespace":         runtime.SparkNamespace(),
					"run_id":            runID,
					"output_dataset":    outputDataset,
				})
				return TransformResult{Output: output}, nil
			case spark.SparkRunFailed:
				msg := "spark application failed"
				if report.ErrorMessage != nil && *report.ErrorMessage != "" {
					msg = *report.ErrorMessage
				}
				return TransformResult{}, fmt.Errorf("SparkApplication %s failed: %s", name, msg)
			}
		}

		if time.Now().After(deadline) {
			return TransformResult{}, fmt.Errorf("SparkApplication %s timed out after %s", name, pollTimeout)
		}

		select {
		case <-ctx.Done():
			return TransformResult{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func parseDistributedComputeConfig(node *PipelineNode) (distributedComputeNodeConfig, error) {
	cfg := distributedComputeNodeConfig{}
	if len(node.Config) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return cfg, fmt.Errorf("parse distributed compute config: %w", err)
	}
	return cfg, nil
}

func executeRemoteComputeTransform(_ context.Context, _ any, _ *PipelineNode, _ []LoadedDataset, _ string) (TransformResult, error) {
	return TransformResult{}, transformRuntimeUnavailable("remote")
}

func legacyRuntimeOutput(runtime, transformType string, rows uint64) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"runtime":        runtime,
		"transform_type": transformType,
		"rows_affected":  rows,
	})
	return out
}

func countInlineRows(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return 0
	}
	for _, key := range []string{"rows", "seed_rows", "records", "data"} {
		if rows, ok := countRowsField(cfg[key]); ok {
			return rows
		}
	}
	return 0
}

func countRowsField(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return 0, false
	}
	return len(rows), true
}
