package handler

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	sparkpkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

func TestSparkFlinkDistributedRunnerSubmitsSparkThroughRuntimePort(t *testing.T) {
	fake := &fakeSparkClient{submittedName: "spark-app", status: &sparkpkg.SparkRunStatusReport{Status: sparkpkg.SparkRunSucceeded}}
	runner := NewSparkFlinkDistributedRunner(DistributedRuntimeConfig{
		SparkClientProvider: func() (sparkpkg.SparkClient, bool) { return fake, true },
		Namespace:           "cluster-ns",
		RunnerImage:         "runner:unit",
		PollInterval:        time.Nanosecond,
		Timeout:             time.Second,
	})

	result, err := runner.RunDistributedTransform(context.Background(), DistributedTransformRequest{
		Node: executor.NodeContext{
			BuildID: uuid.MustParse("11111111-2222-3333-4444-555555555555"),
			Node: executor.Node{
				ID:        "spark-output",
				DependsOn: []string{"ri.dataset.main.trails"},
				Outputs:   []executor.OutputTransaction{{DatasetRID: "ri.dataset.main.out"}},
			},
		},
		Payload:       json.RawMessage(`{"engine":"spark","sql":"SELECT * FROM trails","catalog":"lake","catalog_uri":"http://lake","s3_endpoint":"http://s3","resources":{"executor_instances":3}}`),
		TransformType: "output_dataset",
		Engine:        "spark",
	})

	require.NoError(t, err)
	require.NotEmpty(t, result.OutputContentHash)
	require.Equal(t, "distributed", result.Metadata["runtime"])
	require.Equal(t, "spark", result.Metadata["engine"])
	require.Equal(t, "spark-app", result.Metadata["spark_application"])
	require.Equal(t, "cluster-ns", fake.submitted.Namespace)
	require.Equal(t, "runner:unit", fake.submitted.PipelineRunnerImage)
	require.Equal(t, "ri.dataset.main.trails", fake.submitted.InputDatasetRID)
	require.Equal(t, "ri.dataset.main.out", fake.submitted.OutputDatasetRID)
	require.Equal(t, "SELECT * FROM trails", fake.submitted.InlineSQL)
	require.Equal(t, "lake", fake.submitted.Catalog)
	require.Equal(t, uint32(3), fake.submitted.Resources.ExecutorInstances)
}

func TestSparkFlinkDistributedRunnerSubmitsPySparkApplication(t *testing.T) {
	fake := &fakeSparkClient{submittedName: "pyspark-app", status: &sparkpkg.SparkRunStatusReport{Status: sparkpkg.SparkRunSucceeded}}
	runner := NewSparkFlinkDistributedRunner(DistributedRuntimeConfig{
		SparkClientProvider: func() (sparkpkg.SparkClient, bool) { return fake, true },
		Namespace:           "cluster-ns",
		RunnerImage:         "runner:unit",
		PollInterval:        time.Nanosecond,
		Timeout:             time.Second,
	})

	_, err := runner.RunDistributedTransform(context.Background(), DistributedTransformRequest{
		Node: executor.NodeContext{
			BuildID: uuid.MustParse("21111111-2222-3333-4444-555555555555"),
			Node: executor.Node{
				ID:        "py",
				DependsOn: []string{"ri.dataset.main.input"},
				Outputs:   []executor.OutputTransaction{{DatasetRID: "ri.dataset.main.output"}},
			},
		},
		Payload:       json.RawMessage(`{"engine":"pyspark"}`),
		TransformType: "pyspark",
	})

	require.NoError(t, err)
	require.Equal(t, sparkpkg.SparkApplicationPython, fake.submitted.ApplicationType)
}

func TestSparkFlinkDistributedRunnerFlinkIsExplicitlyAdapterGated(t *testing.T) {
	runner := NewSparkFlinkDistributedRunner(DistributedRuntimeConfig{})

	_, err := runner.RunDistributedTransform(context.Background(), DistributedTransformRequest{
		Node:          executor.NodeContext{BuildID: uuid.New(), Node: executor.Node{ID: "flink"}},
		TransformType: "flink",
		Engine:        "flink",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "flink_runtime_not_configured")
}

func TestDistributedPipelineClusterSmokeGated(t *testing.T) {
	if os.Getenv("OPENFOUNDRY_DISTRIBUTED_CLUSTER_SMOKE") != "1" {
		t.Skip("set OPENFOUNDRY_DISTRIBUTED_CLUSTER_SMOKE=1 with Kubernetes/Spark config to run this optional cluster smoke")
	}
	inputRID := os.Getenv("OPENFOUNDRY_DISTRIBUTED_INPUT_DATASET_RID")
	outputRID := os.Getenv("OPENFOUNDRY_DISTRIBUTED_OUTPUT_DATASET_RID")
	require.NotEmpty(t, inputRID, "OPENFOUNDRY_DISTRIBUTED_INPUT_DATASET_RID is required")
	require.NotEmpty(t, outputRID, "OPENFOUNDRY_DISTRIBUTED_OUTPUT_DATASET_RID is required")

	runner := NewSparkFlinkDistributedRunner(DistributedRuntimeConfig{
		PollInterval: time.Second,
		Timeout:      5 * time.Minute,
	})
	result, err := runner.RunDistributedTransform(context.Background(), DistributedTransformRequest{
		Node: executor.NodeContext{
			BuildID: uuid.New(),
			Node: executor.Node{
				ID:      "cluster-output",
				Outputs: []executor.OutputTransaction{{DatasetRID: outputRID}},
				Metadata: map[string]any{
					"input_dataset_ids": []string{inputRID},
				},
			},
		},
		Payload:       json.RawMessage(`{"engine":"spark","sql":"SELECT * FROM input_table"}`),
		TransformType: "output_dataset",
		Engine:        "spark",
	})
	require.NoError(t, err)
	require.Equal(t, "distributed", result.Metadata["runtime"])
}
