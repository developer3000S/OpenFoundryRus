package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	sparkpkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

type DistributedTransformRequest struct {
	Node          executor.NodeContext
	Payload       json.RawMessage
	TransformType string
	Engine        string
}

type DistributedTransformRunner interface {
	RunDistributedTransform(ctx context.Context, req DistributedTransformRequest) (executor.NodeResult, error)
}

type DistributedRuntimeConfig struct {
	SparkClientProvider func() (sparkpkg.SparkClient, bool)
	Namespace           string
	RunnerImage         string
	PollInterval        time.Duration
	Timeout             time.Duration
}

type SparkFlinkDistributedRunner struct {
	cfg DistributedRuntimeConfig
}

func NewSparkFlinkDistributedRunner(cfg DistributedRuntimeConfig) *SparkFlinkDistributedRunner {
	return &SparkFlinkDistributedRunner{cfg: cfg}
}

func (r *SparkFlinkDistributedRunner) RunDistributedTransform(ctx context.Context, req DistributedTransformRequest) (executor.NodeResult, error) {
	engine := normalizeDistributedEngine(req.Engine)
	if engine == "" {
		engine = distributedEngineForNode(req.TransformType, req.Payload, req.Node.Node.Metadata)
	}
	switch engine {
	case "spark", "pyspark":
		return r.runSpark(ctx, req, engine)
	case "flink":
		return executor.NodeResult{}, errors.New("flink_runtime_not_configured: inject a Flink DistributedTransformRunner or configure a Flink pipeline runtime adapter")
	default:
		return executor.NodeResult{}, fmt.Errorf("distributed_runtime_unsupported_engine:%s", engine)
	}
}

func (r *SparkFlinkDistributedRunner) runSpark(ctx context.Context, req DistributedTransformRequest, engine string) (executor.NodeResult, error) {
	client, ok := r.sparkClient()
	if !ok {
		return executor.NodeResult{}, errors.New("spark_client_not_wired: set KUBERNETES_API_URL or run in-cluster to dispatch SparkApplication CRs")
	}

	cfg := distributedSparkConfig{}
	if len(req.Payload) > 0 {
		_ = json.Unmarshal(req.Payload, &cfg)
	}

	pipelineID := strings.ReplaceAll(strings.TrimSpace(req.Node.BuildID.String()), "-", "")
	if len(pipelineID) > 8 {
		pipelineID = pipelineID[:8]
	}
	if pipelineID == "" {
		pipelineID = "pl"
	}
	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]

	outputDataset := ""
	if len(req.Node.Node.Outputs) > 0 {
		outputDataset = req.Node.Node.Outputs[0].DatasetRID
	}
	if outputDataset == "" {
		outputDataset = metadataString(req.Node.Node.Metadata, "output_dataset_id")
	}
	inputDataset := ""
	for _, id := range metadataStringSlice(req.Node.Node.Metadata, "input_dataset_ids") {
		if strings.TrimSpace(id) != "" {
			inputDataset = id
			break
		}
	}
	if inputDataset == "" && len(req.Node.Node.DependsOn) > 0 {
		inputDataset = req.Node.Node.DependsOn[0]
	}
	if inputDataset == "" {
		inputDataset = outputDataset
	}

	appType := sparkpkg.SparkApplicationScala
	if req.TransformType == "pyspark" || engine == "pyspark" || cfg.Application == "Python" {
		appType = sparkpkg.SparkApplicationPython
	} else if cfg.Application != "" {
		appType = sparkpkg.SparkApplicationType(cfg.Application)
	}

	namespace := firstNonEmpty(r.cfg.Namespace, os.Getenv("SPARK_NAMESPACE"), "openfoundry-spark")
	image := firstNonEmpty(cfg.RunnerImage, r.cfg.RunnerImage, os.Getenv("PIPELINE_RUNNER_IMAGE"), "localhost:5001/pipeline-runner:dev")

	input := sparkpkg.PipelineRunInput{
		PipelineID:          pipelineID,
		RunID:               runID,
		Namespace:           namespace,
		ApplicationType:     appType,
		PipelineRunnerImage: image,
		InputDatasetRID:     inputDataset,
		OutputDatasetRID:    outputDataset,
		Resources:           cfg.Resources,
		Catalog:             cfg.Catalog,
		CatalogURI:          cfg.CatalogURI,
		S3Endpoint:          cfg.S3Endpoint,
		InlineSQL:           firstNonEmpty(cfg.SQL, cfg.Statement),
		InlineFormat:        cfg.Format,
	}

	name, err := client.SubmitPipelineRun(ctx, input)
	if err != nil {
		return executor.NodeResult{}, fmt.Errorf("submit SparkApplication: %w", err)
	}

	timeout := r.cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	pollInterval := r.cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)

	for {
		report, err := client.GetPipelineRunStatus(ctx, namespace, name)
		if err != nil {
			return executor.NodeResult{}, fmt.Errorf("get SparkApplication status: %w", err)
		}
		if report != nil {
			switch report.Status {
			case sparkpkg.SparkRunSucceeded:
				meta := map[string]any{
					"runtime":           "distributed",
					"engine":            "spark",
					"spark_application": name,
					"namespace":         namespace,
					"output_dataset":    outputDataset,
					"transform_type":    req.TransformType,
				}
				hash := sha256.Sum256([]byte(name + ":" + outputDataset + ":" + req.TransformType))
				return executor.NodeResult{OutputContentHash: "sha256:" + hex.EncodeToString(hash[:]), Metadata: meta}, nil
			case sparkpkg.SparkRunFailed:
				msg := "spark application failed"
				if report.ErrorMessage != nil && *report.ErrorMessage != "" {
					msg = *report.ErrorMessage
				}
				return executor.NodeResult{}, fmt.Errorf("SparkApplication %s failed: %s", name, msg)
			}
		}
		if time.Now().After(deadline) {
			return executor.NodeResult{}, fmt.Errorf("SparkApplication %s timed out after %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return executor.NodeResult{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (r *SparkFlinkDistributedRunner) sparkClient() (sparkpkg.SparkClient, bool) {
	if r.cfg.SparkClientProvider != nil {
		return r.cfg.SparkClientProvider()
	}
	return currentSparkClient()
}

type distributedSparkConfig struct {
	Engine      string                          `json:"engine,omitempty"`
	SQL         string                          `json:"sql,omitempty"`
	Statement   string                          `json:"statement,omitempty"`
	Format      string                          `json:"format,omitempty"`
	Catalog     string                          `json:"catalog,omitempty"`
	CatalogURI  string                          `json:"catalog_uri,omitempty"`
	S3Endpoint  string                          `json:"s3_endpoint,omitempty"`
	Resources   sparkpkg.SparkResourceOverrides `json:"resources,omitempty"`
	RunnerImage string                          `json:"runner_image,omitempty"`
	Application string                          `json:"application_type,omitempty"`
}

func shouldUseDistributedRuntime(transformType string, pipelineType string, engine string, payload json.RawMessage, metadata map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(transformType)) {
	case "spark", "pyspark", "flink":
		return true
	}
	if models.NormalizePipelineType(pipelineType) == models.PipelineTypeDistributed {
		return true
	}
	preferred := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		metadataString(metadata, "preferred_runtime"),
		metadataString(metadata, "execution_mode"),
	)))
	if preferred == "distributed" || preferred == "spark" || preferred == "flink" {
		return true
	}
	return normalizeDistributedEngine(engine) != "" && configDeclaresDistributedEngine(payload)
}

func distributedEngineForNode(transformType string, payload json.RawMessage, metadata map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(transformType)) {
	case "spark":
		return "spark"
	case "pyspark":
		return "pyspark"
	case "flink":
		return "flink"
	}
	for _, candidate := range []string{
		metadataString(metadata, "distributed_engine"),
		metadataString(metadata, "compute_engine"),
		metadataString(metadata, "engine"),
	} {
		if engine := normalizeDistributedEngine(candidate); engine != "" {
			return engine
		}
	}
	if engine := normalizeDistributedEngine(distributedConfigEngine(payload)); engine != "" {
		return engine
	}
	return "spark"
}

func distributedEngineFromConfig(raw json.RawMessage) string {
	engine := normalizeDistributedEngine(distributedConfigEngine(raw))
	if engine == "" {
		return "spark"
	}
	return engine
}

func configDeclaresDistributedEngine(raw json.RawMessage) bool {
	return normalizeDistributedEngine(distributedConfigEngine(raw)) != ""
}

func distributedConfigEngine(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var cfg struct {
		Engine          string `json:"engine,omitempty"`
		ComputeEngine   string `json:"compute_engine,omitempty"`
		Distributed     string `json:"distributed,omitempty"`
		PreferredEngine string `json:"preferred_engine,omitempty"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	return firstNonEmpty(cfg.Engine, cfg.ComputeEngine, cfg.Distributed, cfg.PreferredEngine)
}

func normalizeDistributedEngine(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "spark", "scala":
		return "spark"
	case "pyspark", "python":
		return "pyspark"
	case "flink":
		return "flink"
	default:
		return ""
	}
}
