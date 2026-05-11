package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestDatasetServiceOutputCommitterPostsMaterializedDatasetAndMetadata(t *testing.T) {
	outputID := uuid.New()
	inputID := uuid.New()
	var got datasetOutputCommitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/datasets/ri.foundry.main.dataset."+outputID.String()+"/outputs:commit", r.URL.Path)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"transaction":{"status":"COMMITTED"}}`))
	}))
	defer srv.Close()

	metadataCommitter := &recordingCommitter{}
	committer := DatasetServiceOutputCommitter{BaseURL: srv.URL, BearerToken: "test-token", Client: srv.Client(), Metadata: metadataCommitter}
	result := executor.NodeResult{
		OutputContentHash: "sha256:rows",
		Metadata: map[string]any{
			"runtime": "lightweight_table",
			"columns": []string{"trail_id", "distance_miles"},
			"data_rows": []map[string]json.RawMessage{
				{"trail_id": json.RawMessage(`"mesa"`), "distance_miles": json.RawMessage(`4.8`)},
				{"trail_id": json.RawMessage(`"green"`), "distance_miles": json.RawMessage(`6.1`)},
			},
		},
	}
	err := committer.Commit(context.Background(), executor.OutputTransaction{
		DatasetRID:       outputID.String(),
		TransactionRID:   "pipeline-run:one:output",
		DatasetName:      "Trail output",
		Branch:           "main",
		WriteMode:        "SNAPSHOT",
		FileFormat:       "PARQUET",
		LogicalPath:      "part-00000.ndjson",
		OutputNodeID:     "output",
		SourceNodeID:     "select",
		PipelineRID:      "pipeline-one",
		InputDatasetRIDs: []string{inputID.String()},
		CreateIfMissing:  true,
	}, result)
	require.NoError(t, err)

	require.True(t, got.CreateIfMissing)
	require.Equal(t, "Trail output", got.DatasetName)
	require.Equal(t, "main", got.Branch)
	require.Equal(t, "SNAPSHOT", got.TransactionType)
	require.NotNil(t, got.Schema)
	require.Equal(t, "PARQUET", got.Schema.FileFormat)
	require.Equal(t, []datasetOutputField{
		{Name: "trail_id", Type: "STRING", Nullable: false},
		{Name: "distance_miles", Type: "DOUBLE", Nullable: false},
	}, got.Schema.Fields)
	require.Len(t, got.Files, 1)
	require.Equal(t, "part-00000.ndjson", got.Files[0].LogicalPath)
	require.Greater(t, got.Files[0].SizeBytes, int64(0))
	require.Equal(t, []string{"trail_id", "distance_miles"}, got.PreviewColumns)
	require.Len(t, got.PreviewRows, 2)
	require.Len(t, got.LineageLinks, 1)
	require.Equal(t, "ri.foundry.main.dataset."+inputID.String(), got.LineageLinks[0].TargetRID)
	require.Equal(t, []string{outputID.String()}, metadataCommitter.datasets)
	require.Equal(t, "sha256:rows", metadataCommitter.results[0].OutputContentHash)
}

func TestDatasetServiceOutputCommitterSkipsNonDatasetRIDs(t *testing.T) {
	metadataCommitter := &recordingCommitter{}
	committer := DatasetServiceOutputCommitter{BaseURL: "http://127.0.0.1:1", Metadata: metadataCommitter}
	err := committer.Commit(context.Background(), executor.OutputTransaction{DatasetRID: "out.alpha", TransactionRID: "txn"}, executor.NodeResult{OutputContentHash: "hash"})
	require.NoError(t, err)
	require.Equal(t, []string{"out.alpha"}, metadataCommitter.datasets)
}
