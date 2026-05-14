package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	runtimepkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/runtime"
)

func TestExecutePipelineLinearDAGViaHTTP(t *testing.T) {
	runner := &recordingNodeRunner{}
	committer := &recordingCommitter{}
	restore := SetExecutionPorts(ExecutionPorts{NodeRunner: runner, Committer: committer, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()

	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{
		"build_id":"11111111-1111-1111-1111-111111111111",
		"nodes":[
			{"id":"extract","outputs":[{"DatasetRID":"out.extract","TransactionRID":"txn.extract"}]},
			{"id":"transform","depends_on":["extract"],"outputs":[{"DatasetRID":"out.transform","TransactionRID":"txn.transform"}]}
		]
	}`))))

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	require.Equal(t, []string{"extract", "transform"}, runner.order)
	require.Equal(t, []string{"out.extract", "out.transform"}, committer.datasets)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildCompleted), payload.State)
	require.Equal(t, executor.NodeCompleted, payload.Nodes["transform"])
	require.NotContains(t, rr.Body.String(), "not_implemented")
}

func TestExecutePipelineFailureRollsBackAndAbortsDependent(t *testing.T) {
	tx := &recordingTransactions{}
	runner := &recordingNodeRunner{fail: map[string]error{"extract": errors.New("boom")}}
	restore := SetExecutionPorts(ExecutionPorts{NodeRunner: runner, Committer: &recordingCommitter{}, Transactions: tx, Parallelism: 1})
	defer restore()

	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"extract","outputs":[{"DatasetRID":"out.extract","TransactionRID":"txn.extract"}]},{"id":"load","depends_on":["extract"],"outputs":[{"DatasetRID":"out.load","TransactionRID":"txn.load"}]}]}`))))

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildFailed), payload.State)
	require.Equal(t, executor.NodeFailed, payload.Nodes["extract"])
	require.Equal(t, executor.NodeAborted, payload.Nodes["load"])
	require.ElementsMatch(t, []string{"out.extract", "out.load"}, tx.datasets)
}

func TestExecutePipelineCancellationAborts(t *testing.T) {
	tx := &recordingTransactions{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	restore := SetExecutionPorts(ExecutionPorts{NodeRunner: &recordingNodeRunner{}, Committer: &recordingCommitter{}, Transactions: tx})
	defer restore()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"n1","outputs":[{"DatasetRID":"out.cancel","TransactionRID":"txn.cancel"}]}]}`))).WithContext(ctx)
	rr := httptest.NewRecorder()
	ExecutePipeline(rr, req)

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildAborted), payload.State)
	require.Equal(t, executor.NodeAborted, payload.Nodes["n1"])
	require.Equal(t, []string{"out.cancel"}, tx.datasets)
}

func TestExecutePipelineMultiOutputPartialCommitFailure(t *testing.T) {
	tx := &recordingTransactions{}
	committer := &recordingCommitter{failDataset: "out.two"}
	restore := SetExecutionPorts(ExecutionPorts{NodeRunner: &recordingNodeRunner{}, Committer: committer, Transactions: tx})
	defer restore()

	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"multi","outputs":[{"DatasetRID":"out.one","TransactionRID":"txn.one"},{"DatasetRID":"out.two","TransactionRID":"txn.two"}]}]}`))))

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildFailed), payload.State)
	require.Equal(t, []string{"out.one", "out.two"}, committer.datasets)
	require.Equal(t, []string{"out.one", "out.two"}, tx.datasets)
}

func TestExecutePipelineEmitsPersistedAndLiveAuditLogs(t *testing.T) {
	buildID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	jobID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(buildID.String()+":log-node"))
	jobRID := "ri.foundry.main.job." + jobID.String()
	mem := livellogs.NewMemoryService()
	restoreLogs := SetJobLogService(&livellogs.Service{Store: mem, Subscriber: mem})
	defer restoreLogs()
	live, cancelLive, err := mem.Subscribe(context.Background(), jobRID)
	require.NoError(t, err)
	defer cancelLive()
	restore := SetExecutionPorts(ExecutionPorts{NodeRunner: &recordingNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}})
	defer restore()

	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{
		"build_id":"11111111-1111-1111-1111-111111111111",
		"nodes":[{"id":"log-node","outputs":[{"DatasetRID":"out.logs","TransactionRID":"txn.logs"}]}]
	}`))))
	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	select {
	case entry := <-live:
		require.Contains(t, entry.Message, "job state changed")
	case <-time.After(time.Second):
		t.Fatal("expected live job log entry")
	}

	logs, err := mem.History(context.Background(), jobRID, livellogs.Query{})
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	require.Contains(t, logs[len(logs)-1].Message, "job state changed")
	require.Equal(t, livellogs.LogInfo, logs[0].Level)
}

func TestExecutePipelinePythonSidecarContractSuccess(t *testing.T) {
	py := &recordingPython{result: &runtimepkg.TransformResult{
		RowsAffected: uint64Ptr(5),
		Output:       json.RawMessage(`{"status":"ok"}`),
	}}
	restore := SetExecutionPorts(ExecutionPorts{Python: py, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}})
	defer restore()

	body := `{"nodes":[{"id":"py","transform_type":"python","logic_payload":{"source":"print('ok')","config":{"mode":"unit"},"prepared_inputs":[{"dataset_id":"in.a"}],"timeout_seconds":9},"input_dataset_ids":["in.a"],"output_dataset_id":"out.py","outputs":[{"DatasetRID":"out.py","TransactionRID":"txn.py"}]}]}`
	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(body))))

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildCompleted), payload.State)
	require.Equal(t, executor.NodeCompleted, payload.Nodes["py"])
	require.Equal(t, "print('ok')", py.seen.Source)
	require.JSONEq(t, `{"mode":"unit"}`, string(py.seen.ConfigJSON))
	require.JSONEq(t, `[{"dataset_id":"in.a"}]`, string(py.seen.PreparedInputsJSON))
	require.Equal(t, []string{"in.a"}, py.seen.InputDatasetIDs)
	require.Equal(t, "out.py", py.seen.OutputDatasetID)
	require.Equal(t, uint32(9), py.seen.TimeoutSeconds)
}

func TestPythonTransformUsesUpstreamRowsAndStoresResultRows(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()
	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, json.RawMessage(`{
		"rows":[{"activity_id":"987","raw_distance_m":8046.72,"raw_gpx":"<gpx/>"}]
	}`), "dataset_input")
	require.NoError(t, err)

	rowsJSON := json.RawMessage(`[{"activity_id":"987","distance_miles":5,"gpx_points":2}]`)
	py := &recordingPython{result: &runtimepkg.TransformResult{
		RowsAffected:   uint64Ptr(1),
		Output:         json.RawMessage(`{"stdout":"parsed\n","stderr":"gpx ok\n","result":{"status":"ok"}}`),
		ResultRows:     []json.RawMessage{json.RawMessage(`{"activity_id":"987","distance_miles":5,"gpx_points":2}`)},
		ResultRowsJSON: rowsJSON,
		Stdout:         "parsed\n",
		Stderr:         "gpx ok\n",
	}}
	runner := runtimeNodeRunner{Python: py, Table: rt}

	result, err := runner.Run(ctx, executor.NodeContext{
		BuildID: buildID,
		Node: executor.Node{
			ID:        "py",
			DependsOn: []string{"input"},
			Metadata: map[string]any{
				"logic_kind":     "transform",
				"transform_type": "python",
				"logic_payload": json.RawMessage(`{
					"source":"result_rows = input_rows",
					"timeout_seconds":7,
					"packages":["json"],
					"allowed_packages":["json"],
					"output_schema":{"fields":[
						{"name":"activity_id","field_type":"STRING","nullable":false},
						{"name":"distance_miles","field_type":"DOUBLE","nullable":false},
						{"name":"gpx_points","field_type":"INTEGER","nullable":false}
					]}
				}`),
			},
		},
	})
	require.NoError(t, err)
	require.JSONEq(t, `[{"dataset_id":"input","node_id":"input","rows":[{"activity_id":"987","raw_distance_m":8046.72,"raw_gpx":"<gpx/>"}],"schema":[{"name":"activity_id","type":"STRING","nullable":false},{"name":"raw_distance_m","type":"DOUBLE","nullable":false},{"name":"raw_gpx","type":"STRING","nullable":false}],"row_count":1}]`, string(py.seen.PreparedInputsJSON))
	require.Equal(t, uint32(7), py.seen.TimeoutSeconds)
	require.Equal(t, "parsed\n", result.Metadata["stdout"])
	require.Equal(t, "gpx ok\n", result.Metadata["stderr"])
	require.Equal(t, []string{"activity_id", "distance_miles", "gpx_points"}, result.Metadata["columns"])
	stored := rt.snapshotRows("py")
	require.Len(t, stored, 1)
	require.JSONEq(t, `5`, string(stored[0]["distance_miles"]))
}

func TestPythonTransformRejectsPackageOutsideAllowlist(t *testing.T) {
	py := &recordingPython{result: &runtimepkg.TransformResult{Output: json.RawMessage(`{"status":"ok"}`)}}
	runner := runtimeNodeRunner{Python: py, Table: newLightweightTableRuntime()}

	_, err := runner.Run(context.Background(), executor.NodeContext{BuildID: uuid.New(), Node: executor.Node{
		ID: "py",
		Metadata: map[string]any{
			"logic_kind":     "transform",
			"transform_type": "python",
			"logic_payload":  json.RawMessage(`{"source":"import pandas\nresult_rows = []","allowed_packages":["json"]}`),
		},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "python_package_not_allowed:pandas")
	require.Empty(t, py.seen.Source)
}

func TestExecutePipelinePythonRuntimeError(t *testing.T) {
	restore := SetExecutionPorts(ExecutionPorts{Python: failingPython{err: errors.New("python exploded")}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}})
	defer restore()

	rr := httptest.NewRecorder()
	ExecutePipeline(rr, httptest.NewRequest(http.MethodPost, "/api/v1/execute", bytes.NewReader([]byte(`{"nodes":[{"id":"py","transform_type":"python","logic_payload":{"source":"raise Exception('x')"},"outputs":[{"DatasetRID":"out.py","TransactionRID":"txn.py"}]}]}`))))

	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload executePipelineResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, string(models.BuildFailed), payload.State)
	require.Contains(t, payload.Reasons["py"], "python exploded")
}

func TestLightweightTableRuntimeFiltersAndSelectsRows(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()

	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, json.RawMessage(`{
		"rows":[
			{"id":"mesa","distance":5,"name":"Mesa Trail"},
			{"id":"road","distance":2,"name":"Road"}
		]
	}`), "dataset_input")
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "filter", DependsOn: []string{"input"}}}, json.RawMessage(`{"predicate":"distance > 3"}`), "filter")
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "select", DependsOn: []string{"filter"}}}, json.RawMessage(`{"columns":["id","name"]}`), "select")
	require.NoError(t, err)

	rows := rt.snapshotRows("select")
	require.Len(t, rows, 1)
	require.JSONEq(t, `"mesa"`, string(rows[0]["id"]))
	require.JSONEq(t, `"Mesa Trail"`, string(rows[0]["name"]))
	require.NotContains(t, rows[0], "distance")
}

func TestTriggerPipelineRunUsesLightweightRuntimeAndCommitsOutput(t *testing.T) {
	pipelineID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	outputID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	nodes := []models.PipelineNode{
		{ID: "input", Label: "Input", TransformType: "dataset_input", Config: json.RawMessage(`{"rows":[{"id":"mesa","distance":5,"name":"Mesa Trail"},{"id":"road","distance":2,"name":"Road"}]}`)},
		{ID: "filter", Label: "Filter", TransformType: "filter", DependsOn: []string{"input"}, Config: json.RawMessage(`{"predicate":"distance > 3"}`)},
		{ID: "select", Label: "Select", TransformType: "select", DependsOn: []string{"filter"}, Config: json.RawMessage(`{"columns":["id","name"]}`)},
		{ID: "output", Label: "Output", TransformType: "output_dataset", DependsOn: []string{"select"}, Config: json.RawMessage(`{}`), OutputDatasetID: &outputID},
	}
	runRepo := newRecordingPipelineRunsWithNodes(pipelineID, nodes)
	runRepo.pipeline.PipelineType = models.PipelineTypeFaster
	committer := &recordingCommitter{}
	audit := &recordingAuditSink{}
	restore := SetExecutionPorts(ExecutionPorts{Runs: runRepo, Committer: committer, Transactions: &recordingTransactions{}, Audit: audit, Parallelism: 1})
	defer restore()

	rr := httptest.NewRecorder()
	TriggerPipelineRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/22222222-2222-2222-2222-222222222222/runs", bytes.NewReader([]byte(`{"skip_unchanged":false}`))))

	require.Equal(t, http.StatusCreated, rr.Result().StatusCode)
	require.Equal(t, 1, runRepo.opened)
	require.Equal(t, 1, runRepo.markedRunning)
	require.Equal(t, "succeeded", runRepo.finishedStatus)
	require.Equal(t, []string{outputID.String()}, committer.datasets)
	require.NotContains(t, rr.Body.String(), "runner_not_wired")
	require.NotContains(t, rr.Body.String(), "transform_runtime_not_wired")

	var results []models.PipelineNodeResult
	require.NoError(t, json.Unmarshal(runRepo.nodeResults, &results))
	require.Len(t, results, 4)
	require.Equal(t, "succeeded", results[3].Status)
	require.Equal(t, outputID.String(), results[3].OutputResources[0].RID)
	require.Equal(t, "committed", results[3].OutputResources[0].Status)
	require.Equal(t, "lightweight_table", results[3].Output["runtime"])
	require.Equal(t, "pipeline-expression", results[3].Output["engine"])
	require.NotNil(t, results[3].SchemaDelta)
	require.Contains(t, results[3].SchemaDelta.ColumnsAfter, "id")
	require.NotEmpty(t, results[3].Events)
	require.Contains(t, results[3].LogRID, "ri.foundry.main.job.")
	require.True(t, audit.containsReason("output committed"))
	require.True(t, audit.containsTransition(executor.NodeRunning, executor.NodeCompleted))
}

func TestOutputTransactionsForVirtualTableOutputKeepExternalMetadata(t *testing.T) {
	pipelineID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	runID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	nodes := []models.PipelineNode{
		{ID: "vt", Label: "Orders", TransformType: "virtual_table_input", Config: json.RawMessage(`{"source_kind":"virtual_table","virtual_table_rid":"ri.foundry.main.virtual-table.orders","source_rid":"ri.source.snowflake"}`)},
		{ID: "out", Label: "Orders out", TransformType: "output_virtual_table", DependsOn: []string{"vt"}, Config: json.RawMessage(`{"_output":{"kind":"virtual_table","virtual_table_rid":"ri.foundry.main.virtual-table.orders_out","source_rid":"ri.source.snowflake","provider":"SNOWFLAKE","table_type":"TABLE","external_reference":{"kind":"tabular","database":"FINANCE","schema":"PUBLIC","table":"ORDERS_OUT"},"storage":"external","orchestration":"openfoundry"}}`)},
	}

	outputs, rid := outputTransactionsForPipelineNode(pipelineID, runID, nodes[1], nodes)
	require.Equal(t, "ri.foundry.main.virtual-table.orders_out", rid)
	require.Len(t, outputs, 1)
	require.Equal(t, "virtual_table", outputs[0].OutputKind)
	require.Equal(t, "ri.source.snowflake", outputs[0].VirtualTableSourceRID)
	require.Equal(t, "SNOWFLAKE", outputs[0].VirtualTableProvider)
	require.Equal(t, "external", outputs[0].VirtualTableStorage)
	require.Equal(t, "openfoundry", outputs[0].VirtualTableOrchestration)
	require.False(t, canCommitDatasetOutput(outputs[0].DatasetRID))
}

func TestOutputTransactionsSkipLogicalViewOutputs(t *testing.T) {
	pipelineID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	runID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	node := models.PipelineNode{
		ID:            "logical_view",
		Label:         "Union view",
		TransformType: "output",
		Config:        json.RawMessage(`{"_output":{"kind":"logical_view","dataset_rid":"ri.foundry.main.view.global_sales"}}`),
	}

	outputs, rid := outputTransactionsForPipelineNode(pipelineID, runID, node, []models.PipelineNode{node})
	require.Empty(t, outputs)
	require.Empty(t, rid)
}

func TestTriggerPipelineRunDistributedPipelineUsesDistributedPort(t *testing.T) {
	pipelineID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	outputID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	nodes := []models.PipelineNode{
		{ID: "input", Label: "Input", TransformType: "dataset_input", Config: json.RawMessage(`{"dataset_rid":"ri.dataset.trails"}`)},
		{ID: "filter", Label: "Filter", TransformType: "filter", DependsOn: []string{"input"}, Config: json.RawMessage(`{"predicate":"distance > 10"}`)},
		{ID: "output", Label: "Output", TransformType: "output_dataset", DependsOn: []string{"filter"}, OutputDatasetID: &outputID},
	}
	runRepo := newRecordingPipelineRunsWithNodes(pipelineID, nodes)
	runRepo.pipeline.PipelineType = models.PipelineTypeDistributed
	runRepo.pipeline.DistributedConfig = json.RawMessage(`{"engine":"spark"}`)
	distributed := &recordingDistributedRunner{}
	committer := &recordingCommitter{}
	restore := SetExecutionPorts(ExecutionPorts{Runs: runRepo, Distributed: distributed, Committer: committer, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()

	rr := httptest.NewRecorder()
	TriggerPipelineRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/22222222-2222-2222-2222-222222222222/runs", bytes.NewReader([]byte(`{"skip_unchanged":false}`))))

	require.Equal(t, http.StatusCreated, rr.Result().StatusCode)
	require.Equal(t, "succeeded", runRepo.finishedStatus)
	require.Equal(t, []string{"dataset_input", "filter", "output_dataset"}, distributed.transformTypes())
	require.Equal(t, []string{"spark", "spark", "spark"}, distributed.engines())
	require.Equal(t, []string{outputID.String()}, committer.datasets)
	require.NotContains(t, rr.Body.String(), "runner_not_wired")
	require.NotContains(t, rr.Body.String(), "transform_runtime_not_wired")
}

func TestLightweightPipelineFixedFixtureSmoke(t *testing.T) {
	rows := runLightweightPipelineFixture(t, 128)
	require.Equal(t, 117, rows)
}

func BenchmarkLightweightPipelineFixedFixture(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if rows := runLightweightPipelineFixture(b, 1000); rows != 989 {
			b.Fatalf("unexpected row count: %d", rows)
		}
	}
}

func runLightweightPipelineFixture(tb testing.TB, rowCount int) int {
	tb.Helper()
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()
	rows := make([]map[string]any, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		rows = append(rows, map[string]any{
			"id":        fmt.Sprintf("trail-%04d", i),
			"distance":  i,
			"gain":      i % 250,
			"trailType": "trail",
		})
	}
	inputPayload, err := json.Marshal(map[string]any{"rows": rows})
	require.NoError(tb, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "source"}}, inputPayload, "dataset_input")
	require.NoError(tb, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "filter", DependsOn: []string{"source"}}}, json.RawMessage(`{"predicate":"distance > 10"}`), "filter")
	require.NoError(tb, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "select", DependsOn: []string{"filter"}}}, json.RawMessage(`{"columns":["id","distance","gain"]}`), "select")
	require.NoError(tb, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "output", DependsOn: []string{"select"}}}, json.RawMessage(`{}`), "output_dataset")
	require.NoError(tb, err)
	return len(rt.snapshotRows("output"))
}

func TestTriggerPipelineRunCreatesAndExecutesRun(t *testing.T) {
	pipelineID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	runRepo := newRecordingPipelineRuns(pipelineID)
	restore := SetExecutionPorts(ExecutionPorts{Runs: runRepo, NodeRunner: &recordingNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()

	rr := httptest.NewRecorder()
	TriggerPipelineRun(rr, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/22222222-2222-2222-2222-222222222222/runs", bytes.NewReader([]byte(`{"skip_unchanged":false}`))))

	require.Equal(t, http.StatusCreated, rr.Result().StatusCode)
	require.Equal(t, 1, runRepo.opened)
	require.Equal(t, 1, runRepo.markedRunning)
	require.Equal(t, "succeeded", runRepo.finishedStatus)
	var run models.PipelineRun
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&run))
	require.Equal(t, pipelineID, run.PipelineID)
	require.Equal(t, "succeeded", run.Status)
}

func TestApplyPipelineStalenessMarksMatchingPreviousRunIgnored(t *testing.T) {
	pipelineID := uuid.New()
	currentRunID := uuid.New()
	previousRunID := uuid.New()
	plan := executor.Plan{BuildID: currentRunID, Nodes: []executor.Node{
		{
			ID:      "extract",
			Outputs: []executor.OutputTransaction{{DatasetRID: "out.extract", TransactionRID: "txn.extract"}},
			Metadata: map[string]any{
				"logic_kind":        "transform",
				"transform_type":    "dataset_input",
				"logic_payload":     json.RawMessage(`{"dataset_rid":"in.orders"}`),
				"input_dataset_ids": []string{"in.orders"},
				"output_dataset_id": "out.extract",
			},
		},
		{
			ID:        "load",
			DependsOn: []string{"extract"},
			Outputs:   []executor.OutputTransaction{{DatasetRID: "out.load", TransactionRID: "txn.load"}},
			Metadata: map[string]any{
				"logic_kind":        "transform",
				"transform_type":    "output_dataset",
				"logic_payload":     json.RawMessage(`{"write_mode":"SNAPSHOT"}`),
				"input_dataset_ids": []string{"out.extract"},
				"output_dataset_id": "out.load",
			},
		},
	}}
	signatures := pipelineStalenessSignatures(plan.Nodes)
	rawResults, err := json.Marshal([]models.PipelineNodeResult{
		{NodeID: "extract", Status: "succeeded", Output: map[string]any{"staleness_signature": signatures["extract"]}},
		{NodeID: "load", Status: "succeeded", Output: map[string]any{"staleness_signature": signatures["load"]}},
	})
	require.NoError(t, err)
	history := pipelineRunHistoryStub{runs: []models.PipelineRun{{
		ID:          previousRunID,
		PipelineID:  pipelineID,
		Status:      "succeeded",
		NodeResults: rawResults,
		StartedAt:   time.Now().Add(-time.Minute),
	}}}

	applyPipelineStaleness(context.Background(), pipelineID, currentRunID, &plan, true, history)
	require.True(t, plan.Nodes[0].StaleSkipped)
	require.True(t, plan.Nodes[1].StaleSkipped)
	require.Equal(t, signatures["extract"], plan.Nodes[0].Metadata["staleness_signature"])

	outcome := executor.Outcome{
		FinalState: models.BuildCompleted,
		Ignored:    2,
		Nodes: map[string]executor.NodeState{
			"extract": executor.NodeCompleted,
			"load":    executor.NodeCompleted,
		},
		Attempts: map[string]int{},
		Results:  map[string]executor.NodeResult{},
	}
	status, errMsg := pipelineRunStatus(outcome)
	require.Equal(t, "ignored", status)
	require.Nil(t, errMsg)

	results := pipelineRunNodeResults(plan, outcome, nil)
	require.Len(t, results, 2)
	require.Equal(t, "ignored", results[1].Status)
	require.Equal(t, "fresh", results[1].Output["ignored_reason"])
	require.Equal(t, "unchanged", results[1].OutputResources[0].Status)
}

type recordingNodeRunner struct {
	mu    sync.Mutex
	order []string
	fail  map[string]error
}

func (r *recordingNodeRunner) Run(ctx context.Context, node executor.NodeContext) (executor.NodeResult, error) {
	if err := ctx.Err(); err != nil {
		return executor.NodeResult{}, err
	}
	r.mu.Lock()
	r.order = append(r.order, node.Node.ID)
	err := r.fail[node.Node.ID]
	r.mu.Unlock()
	if err != nil {
		return executor.NodeResult{}, err
	}
	return executor.NodeResult{OutputContentHash: "hash-" + node.Node.ID}, nil
}

type recordingCommitter struct {
	mu          sync.Mutex
	datasets    []string
	failDataset string
	results     []executor.NodeResult
}

func (c *recordingCommitter) Commit(_ context.Context, tx executor.OutputTransaction, result executor.NodeResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.datasets = append(c.datasets, tx.DatasetRID)
	c.results = append(c.results, result)
	if tx.DatasetRID == c.failDataset {
		return errors.New("commit failed")
	}
	return nil
}

type recordingTransactions struct {
	mu       sync.Mutex
	datasets []string
}

func (t *recordingTransactions) Abort(_ context.Context, tx executor.OutputTransaction) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.datasets = append(t.datasets, tx.DatasetRID)
	return nil
}

type recordingAuditSink struct {
	mu     sync.Mutex
	events []executor.AuditEvent
}

func (s *recordingAuditSink) Record(_ context.Context, event executor.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *recordingAuditSink) containsReason(reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.Reason == reason && event.JobID != uuid.Nil {
			return true
		}
	}
	return false
}

func (s *recordingAuditSink) containsTransition(from, to executor.NodeState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.From == from && event.To == to && event.JobID != uuid.Nil {
			return true
		}
	}
	return false
}

type recordingPython struct {
	result *runtimepkg.TransformResult
	err    error
	seen   runtimepkg.TransformRequest
}

func (r *recordingPython) ExecutePythonTransform(_ context.Context, req runtimepkg.TransformRequest) (*runtimepkg.TransformResult, error) {
	r.seen = req
	if r.err != nil {
		return nil, r.err
	}
	return r.result, nil
}

func uint64Ptr(v uint64) *uint64 { return &v }

type failingPython struct{ err error }

func (f failingPython) ExecutePythonTransform(context.Context, runtimepkg.TransformRequest) (*runtimepkg.TransformResult, error) {
	return nil, f.err
}

type recordingDistributedRunner struct {
	mu    sync.Mutex
	calls []DistributedTransformRequest
}

func (r *recordingDistributedRunner) RunDistributedTransform(_ context.Context, req DistributedTransformRequest) (executor.NodeResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, req)
	r.mu.Unlock()
	return executor.NodeResult{
		OutputContentHash: "distributed-" + req.Node.Node.ID,
		Metadata: map[string]any{
			"runtime":        "distributed",
			"engine":         req.Engine,
			"transform_type": req.TransformType,
			"rows_affected":  int64(1000),
			"columns":        []string{"id", "distance"},
		},
	}, nil
}

func (r *recordingDistributedRunner) transformTypes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		out = append(out, call.TransformType)
	}
	return out
}

func (r *recordingDistributedRunner) engines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		out = append(out, call.Engine)
	}
	return out
}

type recordingPipelineRuns struct {
	pipeline       *models.Pipeline
	opened         int
	markedRunning  int
	finishedStatus string
	nodeResults    json.RawMessage
}

type pipelineRunHistoryStub struct {
	runs []models.PipelineRun
}

func (s pipelineRunHistoryStub) ListPipelineRuns(context.Context, uuid.UUID, int64, int64) ([]models.PipelineRun, error) {
	return s.runs, nil
}

func newRecordingPipelineRuns(pipelineID uuid.UUID) *recordingPipelineRuns {
	nodes := []models.PipelineNode{{ID: "a", Label: "A", TransformType: "passthrough"}, {ID: "b", Label: "B", TransformType: "passthrough", DependsOn: []string{"a"}}}
	return newRecordingPipelineRunsWithNodes(pipelineID, nodes)
}

func newRecordingPipelineRunsWithNodes(pipelineID uuid.UUID, nodes []models.PipelineNode) *recordingPipelineRuns {
	owner := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	dag, _ := json.Marshal(nodes)
	return &recordingPipelineRuns{pipeline: &models.Pipeline{ID: pipelineID, Name: "p", OwnerID: owner, DAG: dag, PipelineType: models.PipelineTypeBatch, Lifecycle: models.PipelineLifecycleDraft, RetryPolicy: json.RawMessage(`{"max_attempts":1}`)}}
}

func (r *recordingPipelineRuns) LoadPipeline(_ context.Context, pipelineID uuid.UUID) (*models.Pipeline, error) {
	if r.pipeline.ID != pipelineID {
		return nil, nil
	}
	return r.pipeline, nil
}

func (r *recordingPipelineRuns) OpenPipelineRun(_ context.Context, pipeline *models.Pipeline, _ models.TriggerPipelineRequest, _ *uuid.UUID, contextJSON json.RawMessage) (*models.PipelineRun, error) {
	r.opened++
	if len(contextJSON) == 0 || !json.Valid(contextJSON) {
		return nil, errors.New("invalid context")
	}
	return &models.PipelineRun{ID: uuid.MustParse("44444444-4444-4444-4444-444444444444"), PipelineID: pipeline.ID, Status: "queued", TriggerType: "manual", AttemptNumber: 1, ExecutionContext: contextJSON, StartedAt: time.Now().UTC()}, nil
}

func (r *recordingPipelineRuns) MarkPipelineRunRunning(context.Context, uuid.UUID) error {
	r.markedRunning++
	return nil
}

func (r *recordingPipelineRuns) FinishPipelineRun(_ context.Context, _ uuid.UUID, status string, nodeResults json.RawMessage, errorMessage *string) error {
	r.finishedStatus = status
	r.nodeResults = append(json.RawMessage(nil), nodeResults...)
	if status == "succeeded" && errorMessage != nil && strings.TrimSpace(*errorMessage) != "" {
		return errors.New("succeeded run should not carry error")
	}
	return nil
}
