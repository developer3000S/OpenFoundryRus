package postgres

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/resolver"
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func newMockRepo(t *testing.T) (pgxmock.PgxPoolIface, *Repository) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	return mock, NewRepository(mock)
}

func TestRepositoryOpenListGetBuild(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectExec("INSERT INTO builds").
		WithArgs(buildID, "ri.pipeline.1", "master", pgxmock.AnyArg(), []string{"out.users"}, "MANUAL", false, "user-1", "DEPENDENT_ONLY").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	require.NoError(t, repo.OpenBuild(ctx, resolver.ResolveBuildArgs{PipelineRID: "ri.pipeline.1", BuildBranch: "master", OutputDatasetRIDs: []string{"out.users"}, RequestedBy: "user-1"}, buildID))

	buildRows := pgxmock.NewRows([]string{"id", "rid", "pipeline_rid", "build_branch", "job_spec_fallback", "target_dataset_rids", "state", "trigger_kind", "force_build", "abort_policy", "queued_at", "started_at", "finished_at", "error_message", "requested_by", "created_at"}).
		AddRow(buildID, "ri.foundry.main.build."+buildID.String(), "ri.pipeline.1", "master", []string{}, []string{"out.users"}, string(models.BuildResolution), "MANUAL", false, string(models.AbortDependentOnly), nil, nil, nil, nil, "user-1", now)
	mock.ExpectQuery("SELECT id, rid, pipeline_rid").WithArgs("ri.pipeline.1", "", "", pgxmock.AnyArg(), pgxmock.AnyArg(), int64(25)).WillReturnRows(buildRows)
	limit := int64(25)
	items, err := repo.ListBuilds(ctx, models.ListBuildsQuery{PipelineRID: "ri.pipeline.1", Limit: &limit})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, buildID, items[0].ID)

	getRows := pgxmock.NewRows([]string{"id", "rid", "pipeline_rid", "build_branch", "job_spec_fallback", "target_dataset_rids", "state", "trigger_kind", "force_build", "abort_policy", "queued_at", "started_at", "finished_at", "error_message", "requested_by", "created_at"}).
		AddRow(buildID, "ri.foundry.main.build."+buildID.String(), "ri.pipeline.1", "master", []string{}, []string{"out.users"}, string(models.BuildResolution), "MANUAL", false, string(models.AbortDependentOnly), nil, nil, nil, nil, "user-1", now)
	mock.ExpectQuery("FROM builds WHERE").WithArgs(buildID.String()).WillReturnRows(getRows)
	jobRows := pgxmock.NewRows(jobSelectColumnsForTest())
	mock.ExpectQuery("FROM jobs WHERE build_id").WithArgs(buildID).WillReturnRows(jobRows)
	env, err := repo.GetBuild(ctx, buildID.String())
	require.NoError(t, err)
	require.NotNil(t, env)
	require.Equal(t, buildID, env.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPersistResolvedBuildStoresBuildTargetsAndJobSpecSnapshot(t *testing.T) {
	mock, _ := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	jobID := uuid.New()
	specRID := "ri.foundry.main.job_spec.shared"
	resolved := &models.ResolvedBuild{
		BuildID:    buildID,
		State:      models.BuildResolution,
		ForceBuild: true,
		JobSpecs: []models.JobSpec{{
			RID:               specRID,
			LogicKind:         "TRANSFORM",
			ContentHash:       "hash-shared",
			Inputs:            []models.InputSpec{{DatasetRID: "in.orders"}},
			OutputDatasetRIDs: []string{"out.beta", "out.alpha"},
		}},
		OpenedTransactions: []models.OpenedTransaction{
			{DatasetRID: "out.alpha", TransactionRID: "txn-alpha"},
			{DatasetRID: "out.beta", TransactionRID: "txn-beta"},
		},
		Jobs: []models.ResolvedJob{{
			ID:                    jobID,
			JobSpecRID:            specRID,
			OutputTransactionRIDs: []string{"txn-alpha", "txn-beta"},
		}},
	}

	mock.ExpectExec("UPDATE builds").
		WithArgs(buildID, string(models.BuildResolution), []string{"out.alpha", "out.beta"}).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO jobs").
		WithArgs(jobID, buildID, specRID, "TRANSFORM", "hash-shared", []string{"in.orders"}, []string{"out.alpha", "out.beta"}, pgxmock.AnyArg(), "hash-shared", false, []string{"txn-alpha", "txn-beta"}).
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	mock.ExpectExec("INSERT INTO job_outputs").
		WithArgs(jobID, "out.alpha", "txn-alpha").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	mock.ExpectExec("INSERT INTO job_outputs").
		WithArgs(jobID, "out.beta", "txn-beta").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

	require.NoError(t, persistResolvedBuild(ctx, mock, resolved))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPersistResolvedBuildMarksFreshJobsSkipped(t *testing.T) {
	mock, _ := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	jobID := uuid.New()
	specRID := "ri.foundry.main.job_spec.fresh"
	head := "ri.foundry.main.transaction.input-head"
	resolved := &models.ResolvedBuild{
		BuildID: buildID,
		State:   models.BuildResolution,
		JobSpecs: []models.JobSpec{{
			RID:               specRID,
			LogicKind:         "TRANSFORM",
			ContentHash:       "hash-fresh",
			Inputs:            []models.InputSpec{{DatasetRID: "in.orders"}},
			OutputDatasetRIDs: []string{"out.orders"},
		}},
		InputViews: []models.ResolvedInputView{{
			DatasetRID:         "in.orders",
			Branch:             "master",
			HeadTransactionRID: &head,
			Schema:             json.RawMessage(`{"fields":[{"name":"id","type":"STRING"}]}`),
		}},
		OpenedTransactions: []models.OpenedTransaction{
			{DatasetRID: "out.orders", TransactionRID: "txn-orders"},
		},
		Jobs: []models.ResolvedJob{{
			ID:                    jobID,
			JobSpecRID:            specRID,
			OutputTransactionRIDs: []string{"txn-orders"},
		}},
	}

	mock.ExpectExec("UPDATE builds").
		WithArgs(buildID, string(models.BuildResolution), []string{"out.orders"}).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(specRID, "hash-fresh", pgxmock.AnyArg(), []string{"out.orders"}).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO jobs").
		WithArgs(jobID, buildID, specRID, "TRANSFORM", "hash-fresh", []string{"in.orders"}, []string{"out.orders"}, pgxmock.AnyArg(), "hash-fresh", true, []string{"txn-orders"}).
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	mock.ExpectExec("INSERT INTO job_outputs").
		WithArgs(jobID, "out.orders", "txn-orders").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

	require.NoError(t, persistResolvedBuild(ctx, mock, resolved))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryGetBuildEnrichesExecutionHistory(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	extractID := uuid.New()
	loadID := uuid.New()
	started := time.Unix(100, 0).UTC()
	finished := time.Unix(130, 0).UTC()
	buildRID := "ri.foundry.main.build." + buildID.String()

	buildRows := pgxmock.NewRows([]string{"id", "rid", "pipeline_rid", "build_branch", "job_spec_fallback", "target_dataset_rids", "state", "trigger_kind", "force_build", "abort_policy", "queued_at", "started_at", "finished_at", "error_message", "requested_by", "created_at"}).
		AddRow(buildID, buildRID, "pipe", "master", []string{}, []string{"out.extract", "out.load"}, string(models.BuildCompleted), "MANUAL", false, string(models.AbortDependentOnly), nil, &started, &finished, nil, "user-1", started.Add(-time.Second))
	mock.ExpectQuery("FROM builds WHERE").WithArgs(buildRID).WillReturnRows(buildRows)
	mock.ExpectQuery("FROM jobs WHERE build_id").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows(jobSelectColumnsForTest()).
			AddRow(extractID, "ri.foundry.main.job."+extractID.String(), buildID, "spec.extract", "TRANSFORM", "hash-extract", []string{"in.raw"}, []string{"out.extract"}, "input-a", "logic-a", string(models.JobCompleted), []string{"txn-extract"}, started, &started, &finished, int32(1), false, "lightweight_table", "pipeline-expression", int64(42), int64(1), []byte(`{"runtime":"lightweight_table","rows_affected":42}`), nil, ptrString("hash-output"), started).
			AddRow(loadID, "ri.foundry.main.job."+loadID.String(), buildID, "spec.load", "TRANSFORM", "hash-load", []string{"out.extract"}, []string{"out.load"}, "input-b", "logic-b", string(models.JobCompleted), []string{"txn-load"}, finished, &finished, &finished, int32(0), true, "", "", nil, nil, []byte(`{"ignored_reason":"fresh"}`), nil, nil, finished))
	mock.ExpectQuery("FROM job_dependencies jd JOIN jobs dep").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows([]string{"job_id", "depends_on_spec"}).
			AddRow(loadID, "spec.extract"))
	mock.ExpectQuery("FROM job_outputs jo").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows([]string{"job_id", "output_dataset_rid", "transaction_rid", "committed", "aborted"}).
			AddRow(extractID, "out.extract", "txn-extract", true, false).
			AddRow(loadID, "out.load", "txn-load", false, true))

	env, err := repo.GetBuild(ctx, buildRID)
	require.NoError(t, err)
	require.NotNil(t, env)
	require.Equal(t, "succeeded", env.ExecutionStatus)
	require.NotNil(t, env.DurationMillis)
	require.Equal(t, int64(30000), *env.DurationMillis)
	require.Equal(t, map[string]int{"succeeded": 1, "ignored": 1}, env.StatusCounts)
	require.Len(t, env.JobDAG, 1)
	require.Equal(t, "spec.load", env.JobDAG[0].JobSpecRID)
	require.Equal(t, "spec.extract", env.JobDAG[0].DependsOnJobSpecRID)
	require.Equal(t, "lightweight_table", env.Jobs[0].Runtime)
	require.Equal(t, "pipeline-expression", env.Jobs[0].WorkerID)
	require.Equal(t, int64(42), *env.Jobs[0].RowCount)
	require.Equal(t, int64(1), *env.Jobs[0].FileCount)
	require.Equal(t, "committed", env.Jobs[0].OutputTransactions[0].Status)
	require.Equal(t, "ignored", env.Jobs[1].ExecutionStatus)
	require.Equal(t, "aborted", env.Jobs[1].OutputTransactions[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCommitPersistsExecutionMetrics(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	result := executor.NodeResult{
		OutputContentHash: "sha256:abc",
		Metadata: map[string]any{
			"runtime":       "python",
			"engine":        "python_sidecar",
			"rows_affected": int64(7),
			"data_rows":     []map[string]any{{"id": "hidden"}},
		},
	}

	mock.ExpectExec("UPDATE job_outputs SET committed=TRUE").
		WithArgs("out.python", "txn-python").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("UPDATE jobs").
		WithArgs("out.python", "txn-python", "sha256:abc", "python", "python_sidecar", ptrInt64(7), (*int64)(nil), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	require.NoError(t, repo.Commit(ctx, executor.OutputTransaction{DatasetRID: "out.python", TransactionRID: "txn-python"}, result))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryLookupJobSpecFallback(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	mock.ExpectQuery("FROM pipeline_job_specs").WithArgs("pipe", "out", "feature").WillReturnRows(pgxmock.NewRows([]string{"rid", "pipeline_rid", "branch_name", "inputs", "output_dataset_rid", "job_spec_json", "content_hash"}))
	body := []byte(`{"logic_kind":"TRANSFORM","logic_payload":{"sql":"select 1"},"output_dataset_rids":["out"]}`)
	inputs := []byte(`[{"dataset_rid":"in","fallback_chain":["master"]}]`)
	rows := pgxmock.NewRows([]string{"rid", "pipeline_rid", "branch_name", "inputs", "output_dataset_rid", "job_spec_json", "content_hash"}).AddRow("spec-1", "pipe", "master", inputs, "out", body, "hash")
	mock.ExpectQuery("FROM pipeline_job_specs").WithArgs("pipe", "out", "master").WillReturnRows(rows)
	spec, err := repo.Lookup(ctx, "pipe", "out", "feature", []string{"master"})
	require.NoError(t, err)
	require.NotNil(t, spec)
	require.Equal(t, "TRANSFORM", spec.LogicKind)
	require.Equal(t, []string{"out"}, spec.OutputDatasetRIDs)
	require.Len(t, spec.Inputs, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryPipelineRunsAbortAndLogs(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	pipelineID := uuid.New()
	runID := uuid.New()
	jobID := uuid.New()
	now := time.Now().UTC()

	pipelineRows := pgxmock.NewRows([]string{
		"id", "name", "description", "owner_id", "dag", "status",
		"pipeline_type", "lifecycle",
		"schedule_config", "retry_policy", "next_run_at",
		"external_config", "incremental_config", "streaming_config", "distributed_config", "compute_profile_id", "project_id",
		"draft_dag", "published_dag", "branch_name",
		"draft_updated_at", "published_at", "active_version_id",
		"proposal_state", "proposal_title", "proposal_description",
		"created_at", "updated_at",
	}).AddRow(
		pipelineID, "p", "", uuid.New(), []byte(`[]`), "active",
		"BATCH", "DRAFT",
		[]byte(`{}`), []byte(`{"max_attempts":1}`), nil,
		nil, nil, nil, nil, nil, nil,
		[]byte(`[]`), []byte(`null`), "main",
		&now, nil, nil,
		"none", nil, nil,
		now, now,
	)
	mock.ExpectQuery("FROM pipelines WHERE id").WithArgs(pipelineID).WillReturnRows(pipelineRows)
	p, err := repo.LoadPipeline(ctx, pipelineID)
	require.NoError(t, err)
	require.NotNil(t, p)

	runRows := pgxmock.NewRows([]string{"id", "pipeline_id", "status", "trigger_type", "started_by", "attempt_number", "started_from_node_id", "retry_of_run_id", "execution_context", "node_results", "error_message", "started_at", "finished_at"}).
		AddRow(runID, pipelineID, "queued", "manual", nil, int32(1), nil, nil, []byte(`{}`), nil, nil, now, nil)
	mock.ExpectQuery("INSERT INTO pipeline_runs").WithArgs(pgxmock.AnyArg(), pipelineID, pgxmock.AnyArg(), "manual", int32(1), pgxmock.AnyArg(), pgxmock.AnyArg(), json.RawMessage(`{}`)).WillReturnRows(runRows)
	run, err := repo.OpenPipelineRun(ctx, p, models.TriggerPipelineRequest{Context: json.RawMessage(`{}`)}, nil, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, runID, run.ID)
	require.Equal(t, "queued", run.Status)
	mock.ExpectExec("UPDATE pipeline_runs SET status='running'").WithArgs(runID).WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	require.NoError(t, repo.MarkPipelineRunRunning(ctx, runID))
	mock.ExpectExec("UPDATE pipeline_runs SET status").WithArgs(runID, "succeeded", json.RawMessage(`[]`), pgxmock.AnyArg()).WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	require.NoError(t, repo.FinishPipelineRun(ctx, runID, "succeeded", json.RawMessage(`[]`), nil))

	mock.ExpectExec("UPDATE builds SET state='BUILD_ABORTING'").WithArgs(runID, "user abort").WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	require.NoError(t, repo.MarkBuildAborting(ctx, runID, "user abort"))
	mock.ExpectExec("UPDATE jobs SET state").WithArgs(jobID, string(models.JobAborted), "abort", string(models.JobWaiting)).WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO job_state_transitions").WithArgs(jobID, string(models.JobWaiting), string(models.JobAborted), "abort").WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	require.NoError(t, repo.TransitionJob(ctx, jobID, models.JobWaiting, models.JobAborted, "abort"))

	logRows := pgxmock.NewRows([]string{"sequence", "job_rid", "ts", "level", "message", "params"}).AddRow(int64(7), jobID.String(), now, string(livellogs.LogInfo), "hello", []byte(`{"x":1}`))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT sequence, $1::text, ts, level, message, params FROM job_logs")).WithArgs(jobID.String(), jobID, int64(0), pgxmock.AnyArg(), pgxmock.AnyArg(), []string{}, int64(10)).WillReturnRows(logRows)
	limit := int64(10)
	logs, err := repo.History(ctx, jobID.String(), livellogs.Query{Limit: limit})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, int64(7), logs[0].Sequence)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryLoadPlanPopulatesNodeMetadata(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	jobID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT id, build_branch, abort_policy, force_build FROM builds WHERE id").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "build_branch", "abort_policy", "force_build"}).
			AddRow(buildID, "master", string(models.AbortDependentOnly), true))

	mock.ExpectQuery(regexp.QuoteMeta(jobSelectSQL + " WHERE build_id")).
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows(jobSelectColumnsForTest()).
			AddRow(jobID, "ri.foundry.main.job."+jobID.String(), buildID, "ri.foundry.main.job_spec.alpha", "TRANSFORM", "hash-alpha", []string{"in.alpha"}, []string{"out.alpha"}, "input-sig", "logic-sig", string(models.JobWaiting), []string{}, now, nil, nil, int32(0), false, "", "", nil, nil, []byte(`{}`), nil, nil, now))

	mock.ExpectQuery("FROM job_dependencies jd JOIN jobs dep").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows([]string{"job_id", "depends_on_spec"}))

	mock.ExpectQuery("FROM jobs j\nJOIN job_specs js ON js.rid").
		WithArgs(buildID).
		WillReturnRows(pgxmock.NewRows([]string{"rid", "logic_kind", "logic_payload", "inputs", "output_dataset_rids"}).
			AddRow("ri.foundry.main.job_spec.alpha", "TRANSFORM", []byte(`{"transform_type":"python","source":"select 1"}`), []byte(`[{"dataset_rid":"in.alpha"}]`), []string{"out.alpha"}))

	mock.ExpectQuery("FROM job_outputs WHERE job_id").
		WithArgs(jobID).
		WillReturnRows(pgxmock.NewRows([]string{"output_dataset_rid", "transaction_rid"}).
			AddRow("out.alpha", "ri.foundry.main.transaction.tx-1"))

	plan, err := repo.LoadPlan(ctx, buildID)
	require.NoError(t, err)
	require.Equal(t, buildID, plan.BuildID)
	require.Equal(t, executor.AbortDependentOnly, plan.AbortPolicy)
	require.Len(t, plan.Nodes, 1)
	node := plan.Nodes[0]
	require.Equal(t, "ri.foundry.main.job_spec.alpha", node.ID)
	require.Equal(t, jobID, node.JobID)
	require.Equal(t, "TRANSFORM", node.Metadata["logic_kind"])
	require.Equal(t, "python", node.Metadata["transform_type"])
	require.Equal(t, "out.alpha", node.Metadata["output_dataset_id"])
	require.Equal(t, []string{"in.alpha"}, node.Metadata["input_dataset_ids"])
	require.Equal(t, true, node.Metadata["force_build"])
	require.Equal(t, "input-sig", node.Metadata["input_signature"])
	require.Equal(t, "logic-sig", node.Metadata["canonical_logic_hash"])
	require.NotEmpty(t, node.Metadata["staleness_signature"])
	require.Equal(t, json.RawMessage(`{"transform_type":"python","source":"select 1"}`), node.Metadata["logic_payload"])
	require.Len(t, node.Outputs, 1)
	require.Equal(t, "out.alpha", node.Outputs[0].DatasetRID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryAuditSinkPersistsLifecycleTransitions(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	buildID := uuid.New()
	jobID := uuid.New()

	mock.ExpectExec("UPDATE builds").
		WithArgs(buildID).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("UPDATE jobs SET state").
		WithArgs(jobID, string(models.JobRunPending), string(models.JobWaiting), 0).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO job_state_transitions").
		WithArgs(jobID, string(models.JobWaiting), string(models.JobRunPending), "dispatching").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{BuildID: buildID, JobID: jobID, NodeID: "n", From: executor.NodeWaiting, To: executor.NodeRunPending, Reason: "dispatching"}))

	mock.ExpectExec("UPDATE jobs SET state=\\$2, state_changed_at=NOW\\(\\), finished_at=COALESCE\\(finished_at, NOW\\(\\)\\), failure_reason=\\$3").
		WithArgs(jobID, string(models.JobFailed), "boom", string(models.JobRunning), 2).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO job_state_transitions").
		WithArgs(jobID, string(models.JobRunning), string(models.JobFailed), "boom").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{BuildID: buildID, JobID: jobID, NodeID: "n", From: executor.NodeRunning, To: executor.NodeFailed, Attempt: 2, Reason: "boom"}))

	mock.ExpectExec("UPDATE jobs SET state=\\$2, state_changed_at=NOW\\(\\), started_at=COALESCE\\(started_at, NOW\\(\\)\\), finished_at=COALESCE\\(finished_at, NOW\\(\\)\\), stale_skipped=TRUE").
		WithArgs(jobID, string(models.JobCompleted), string(models.JobWaiting), 0).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectExec("INSERT INTO job_state_transitions").
		WithArgs(jobID, string(models.JobWaiting), string(models.JobCompleted), "ignored because fresh").
		WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{BuildID: buildID, JobID: jobID, NodeID: "n", From: executor.NodeWaiting, To: executor.NodeCompleted, Reason: "ignored because fresh"}))

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryAuditSinkSkipsNonTransitionEvents(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	jobID := uuid.New()

	require.NoError(t, repo.Record(ctx, executor.AuditEvent{NodeID: "n", From: executor.NodeWaiting, To: executor.NodeRunPending}))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{JobID: jobID, NodeID: "n", DatasetRID: "out.x", Reason: "output committed"}))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{JobID: jobID, NodeID: "n", From: executor.NodeRunning, To: executor.NodeRunning, Reason: "noop"}))

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryAuditSinkIdempotentWhenStateAlreadyAdvanced(t *testing.T) {
	mock, repo := newMockRepo(t)
	ctx := context.Background()
	jobID := uuid.New()

	mock.ExpectExec("UPDATE jobs SET state").
		WithArgs(jobID, string(models.JobCompleted), string(models.JobRunning), 1).
		WillReturnResult(pgconn.NewCommandTag("UPDATE 0"))
	require.NoError(t, repo.Record(ctx, executor.AuditEvent{JobID: jobID, NodeID: "n", From: executor.NodeRunning, To: executor.NodeCompleted, Attempt: 1, Reason: "all outputs committed"}))

	require.NoError(t, mock.ExpectationsWereMet())
}

func jobSelectColumnsForTest() []string {
	return []string{
		"id", "rid", "build_id", "job_spec_rid", "logic_kind", "job_spec_content_hash",
		"input_dataset_rids", "output_dataset_rids", "input_signature", "canonical_logic_hash",
		"state", "output_transaction_rids", "state_changed_at", "started_at", "finished_at",
		"attempt", "stale_skipped", "runtime", "worker_id", "row_count", "file_count",
		"output_metadata", "failure_reason", "output_content_hash", "created_at",
	}
}

func ptrString(value string) *string { return &value }

func ptrInt64(value int64) *int64 { return &value }
