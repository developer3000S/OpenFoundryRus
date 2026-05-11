// Package config mirrors `services/pipeline-build-service/src/config.rs`.
package config

import (
	"os"
	"strconv"
)

type Config struct {
	Service struct {
		Name    string
		Version string
	}

	Host string
	Port uint16

	DatabaseURL string
	JWTSecret   string
	DataDir     string

	DatasetServiceURL               string
	DatasetServiceBearer            string
	OntologyDefinitionServiceURL    string
	OntologyDefinitionServiceBearer string
	ObjectDatabaseServiceURL        string
	ObjectDatabaseServiceBearer     string
	WorkflowServiceURL              string
	AIServiceURL                    string
	AIServiceBearer                 string

	StorageBackend   string
	StorageBucket    string
	S3Endpoint       string
	S3Region         string
	S3AccessKey      string
	S3SecretKey      string
	LocalStorageRoot string

	DistributedPipelineWorkers       int
	DistributedComputePollIntervalMS uint64
	DistributedComputeTimeoutSecs    uint64

	// FASE 3 / Tarea 3.4 — kube + Spark wiring.
	SparkNamespace      string
	PipelineRunnerImage string

	// ADR-0041 — Foundry Iceberg catalog client.
	FoundryIcebergCatalogURL    string
	FoundryIcebergCatalogBearer string

	// Python sidecar replaces the former Rust PyO3 embedded interpreter
	// for pipeline transform execution. Empty disables Python runtime
	// execution and leaves callers with an explicit runner_not_wired error.
	PythonSidecarBinary         string
	PythonSidecarTimeoutSeconds uint32
}

func FromEnv() (*Config, error) {
	c := &Config{}
	c.Service.Name = "pipeline-build-service"
	c.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	c.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	c.Port = parseUint16(os.Getenv("PORT"), 50081)
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	c.JWTSecret = os.Getenv("JWT_SECRET")
	c.DataDir = defaultStr(os.Getenv("DATA_DIR"), "/var/lib/openfoundry/pipeline-build")
	c.DatasetServiceURL = defaultStr(os.Getenv("DATASET_SERVICE_URL"), "http://localhost:50079")
	c.DatasetServiceBearer = os.Getenv("DATASET_SERVICE_BEARER")
	c.OntologyDefinitionServiceURL = defaultStr(os.Getenv("ONTOLOGY_DEFINITION_SERVICE_URL"), "http://localhost:50103")
	c.OntologyDefinitionServiceBearer = os.Getenv("ONTOLOGY_DEFINITION_SERVICE_BEARER")
	c.ObjectDatabaseServiceURL = defaultStr(os.Getenv("OBJECT_DATABASE_SERVICE_URL"), "http://localhost:50104")
	c.ObjectDatabaseServiceBearer = os.Getenv("OBJECT_DATABASE_SERVICE_BEARER")
	c.WorkflowServiceURL = defaultStr(os.Getenv("WORKFLOW_SERVICE_URL"), "http://localhost:50080")
	c.AIServiceURL = defaultStr(os.Getenv("AI_SERVICE_URL"), "http://localhost:50127")
	c.AIServiceBearer = os.Getenv("AI_SERVICE_BEARER")
	c.StorageBackend = defaultStr(os.Getenv("STORAGE_BACKEND"), "local")
	c.StorageBucket = os.Getenv("STORAGE_BUCKET")
	c.S3Endpoint = os.Getenv("S3_ENDPOINT")
	c.S3Region = os.Getenv("S3_REGION")
	c.S3AccessKey = os.Getenv("S3_ACCESS_KEY")
	c.S3SecretKey = os.Getenv("S3_SECRET_KEY")
	c.LocalStorageRoot = os.Getenv("LOCAL_STORAGE_ROOT")
	c.DistributedPipelineWorkers = int(parseUint16(os.Getenv("DISTRIBUTED_PIPELINE_WORKERS"), 4))
	c.DistributedComputePollIntervalMS = parseUint64(os.Getenv("DISTRIBUTED_COMPUTE_POLL_INTERVAL_MS"), 1000)
	c.DistributedComputeTimeoutSecs = parseUint64(os.Getenv("DISTRIBUTED_COMPUTE_TIMEOUT_SECS"), 1800)
	c.SparkNamespace = defaultStr(os.Getenv("SPARK_NAMESPACE"), "openfoundry-spark")
	c.PipelineRunnerImage = defaultStr(os.Getenv("PIPELINE_RUNNER_IMAGE"), "openfoundry/pipeline-runner:dev")
	c.FoundryIcebergCatalogURL = os.Getenv("FOUNDRY_ICEBERG_CATALOG_URL")
	c.FoundryIcebergCatalogBearer = os.Getenv("FOUNDRY_ICEBERG_CATALOG_BEARER")
	c.PythonSidecarBinary = os.Getenv("PYTHON_SIDECAR_BINARY")
	c.PythonSidecarTimeoutSeconds = parseUint32(os.Getenv("PYTHON_SIDECAR_TIMEOUT_SECONDS"), 60)
	return c, nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseUint16(v string, fallback uint16) uint16 {
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return fallback
	}
	return uint16(n)
}

func parseUint32(v string, fallback uint32) uint32 {
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(n)
}

func parseUint64(v string, fallback uint64) uint64 {
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
