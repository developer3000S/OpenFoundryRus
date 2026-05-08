package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/iceberg"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	runtimepkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/runtime"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

func TestConfigGatesReturnExplicitErrors(t *testing.T) {
	t.Run("DATABASE_URL build repository", func(t *testing.T) {
		restore := SetBuildQueryRepository(nil)
		defer restore()

		rr := httptest.NewRecorder()
		ListBuilds(rr, httptest.NewRequest(http.MethodGet, "/api/v1/builds", nil))

		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
		var payload map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "build_query_repository_not_configured", payload["error"])
		require.Contains(t, payload["detail"], "DATABASE_URL")
	})

	t.Run("PYTHON_SIDECAR_BINARY", func(t *testing.T) {
		restore := SetExecutionPorts(ExecutionPorts{Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}})
		defer restore()

		rr := httptest.NewRecorder()
		ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"py","transform_type":"python","logic_payload":{"source":"print('hi')"},"outputs":[{"DatasetRID":"out.py","TransactionRID":"txn.py"}]}]}`))))

		require.Equal(t, http.StatusOK, rr.Code)
		var payload executePipelineResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, string(models.BuildFailed), payload.State)
		require.Contains(t, payload.Reasons["py"], "python_sidecar_not_configured")
		require.Contains(t, payload.Reasons["py"], "PYTHON_SIDECAR_BINARY")
	})

	t.Run("KUBERNETES_API_URL", func(t *testing.T) {
		restore := SetSparkClient(noSparkClient{})
		defer restore()

		rr := httptest.NewRecorder()
		SubmitSparkRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/spark-runs", bytes.NewReader([]byte(`{"pipeline_id":"p","input_dataset_rid":"in","output_dataset_rid":"out"}`))))

		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
		var payload map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "kube_client_unavailable", payload["error"])
		require.Contains(t, payload["detail"], "KUBERNETES_API_URL")
	})

	t.Run("FOUNDRY_ICEBERG_CATALOG_URL", func(t *testing.T) {
		restore := SetExecutionPorts(ExecutionPorts{
			NodeRunner:   &recordingNodeRunner{},
			Committer:    ConfigGatedOutputCommitter{Metadata: &recordingCommitter{}},
			Transactions: ConfigGatedTransactionManager{Metadata: &recordingTransactions{}},
		})
		defer restore()

		rr := httptest.NewRecorder()
		ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"ice","outputs":[{"DatasetRID":"`+iceberg.DatasetRIDPrefix+`orders","TransactionRID":"txn.ice"}]}]}`))))

		require.Equal(t, http.StatusOK, rr.Code)
		var payload executePipelineResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, string(models.BuildFailed), payload.State)
		require.Contains(t, payload.Reasons["ice"], "foundry_iceberg_catalog_not_configured")
		require.Contains(t, payload.Reasons["ice"], "FOUNDRY_ICEBERG_CATALOG_URL")
	})
}

func TestConfigGatesHappyPathsWithFakes(t *testing.T) {
	t.Run("DATABASE_URL fake build repository", func(t *testing.T) {
		repo := &fakeBuildQueryRepo{builds: []models.BuildEnvelope{{Build: models.Build{ID: uuid.New(), RID: "ri.build.1", State: string(models.BuildCompleted)}}}}
		restore := SetBuildQueryRepository(repo)
		defer restore()

		rr := httptest.NewRecorder()
		ListBuilds(rr, httptest.NewRequest(http.MethodGet, "/api/v1/builds?limit=1", nil))

		require.Equal(t, http.StatusOK, rr.Code)
		var payload struct {
			Data  []models.BuildEnvelope `json:"data"`
			Total int                    `json:"total"`
		}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, 1, payload.Total)
		require.Equal(t, int64(1), *repo.lastQuery.Limit)
	})

	t.Run("PYTHON_SIDECAR_BINARY fake sidecar", func(t *testing.T) {
		py := &recordingPython{result: &runtimepkg.TransformResult{RowsAffected: uint64Ptr(1), Output: json.RawMessage(`{"status":"ok"}`)}}
		restore := SetExecutionPorts(ExecutionPorts{Python: py, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}})
		defer restore()

		rr := httptest.NewRecorder()
		ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"py","transform_type":"python","logic_payload":{"source":"print('ok')"},"outputs":[{"DatasetRID":"out.py","TransactionRID":"txn.py"}]}]}`))))

		require.Equal(t, http.StatusOK, rr.Code)
		var payload executePipelineResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, string(models.BuildCompleted), payload.State)
		require.Equal(t, "print('ok')", py.seen.Source)
	})

	t.Run("KUBERNETES_API_URL fake Spark client", func(t *testing.T) {
		fake := &fakeSparkClient{submittedName: "spark-app"}
		restore := SetSparkClient(fake)
		defer restore()

		rr := httptest.NewRecorder()
		SubmitSparkRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/spark-runs", bytes.NewReader([]byte(`{"pipeline_id":"p","run_id":"r","input_dataset_rid":"in","output_dataset_rid":"out","pipeline_runner_image":"img"}`))))

		require.Equal(t, http.StatusAccepted, rr.Code)
		require.Equal(t, "p", fake.submitted.PipelineID)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, "spark-app", payload["spark_app_name"])
	})

	t.Run("FOUNDRY_ICEBERG_CATALOG_URL fake Iceberg client", func(t *testing.T) {
		ice := &fakeIcebergTxClient{}
		restore := SetExecutionPorts(ExecutionPorts{
			NodeRunner:   &recordingNodeRunner{},
			Committer:    ConfigGatedOutputCommitter{Metadata: &recordingCommitter{}, Iceberg: ice},
			Transactions: ConfigGatedTransactionManager{Metadata: &recordingTransactions{}, Iceberg: ice},
		})
		defer restore()

		rr := httptest.NewRecorder()
		ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"ice","outputs":[{"DatasetRID":"`+iceberg.DatasetRIDPrefix+`orders","TransactionRID":"txn.ice"}]}]}`))))

		require.Equal(t, http.StatusOK, rr.Code)
		var payload executePipelineResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
		require.Equal(t, string(models.BuildCompleted), payload.State)
		require.Equal(t, []string{iceberg.DatasetRIDPrefix + "orders"}, ice.committed)
	})
}

type fakeBuildQueryRepo struct {
	builds    []models.BuildEnvelope
	lastQuery models.ListBuildsQuery
}

func (f *fakeBuildQueryRepo) ListBuilds(_ context.Context, query models.ListBuildsQuery) ([]models.BuildEnvelope, error) {
	f.lastQuery = query
	return f.builds, nil
}
func (f *fakeBuildQueryRepo) GetBuild(context.Context, string) (*models.BuildEnvelope, error) {
	return nil, nil
}
func (f *fakeBuildQueryRepo) ListJobsForBuildID(context.Context, string) ([]models.Job, error) {
	return nil, nil
}
func (f *fakeBuildQueryRepo) GetJob(context.Context, string) (*models.Job, error) { return nil, nil }

type fakeIcebergTxClient struct{ committed []string }

func (f *fakeIcebergTxClient) Commit(_ context.Context, tx executor.OutputTransaction) error {
	f.committed = append(f.committed, tx.DatasetRID)
	return nil
}
func (f *fakeIcebergTxClient) Abort(context.Context, executor.OutputTransaction) error { return nil }

var _ spark.SparkClient = (*fakeSparkClient)(nil)
