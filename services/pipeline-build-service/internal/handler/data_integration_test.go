//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestDryRunResolveEndpointUsesDataIntegrationPipelineRID(t *testing.T) {
	jobSpecs := newHandlerJobSpecRepo()
	versioning := newHandlerDatasetRepo()
	versioning.branches["out.users"] = []models.BranchSnapshot{{Name: "master"}}
	jobSpecs.specs["out.users"] = models.JobSpec{RID: "spec.users", PipelineRID: "ri.pipeline.1", BranchName: "master", OutputDatasetRIDs: []string{"out.users"}}
	restore := SetBuildLifecyclePorts(BuildLifecyclePorts{JobSpecs: jobSpecs, Versioning: versioning, Locks: newHandlerLockRepo(), Builds: &recordingBuildRepo{}})
	defer restore()
	req := requestWithURLParam(http.MethodPost, "/api/v1/data-integration/pipelines/ri.pipeline.1/dry-run-resolve", bytes.NewReader([]byte(`{"build_branch":"master","output_dataset_rids":["out.users"]}`)), "pipeline_rid", "ri.pipeline.1")
	rr := httptest.NewRecorder()
	DryRunResolve(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var payload dryRunResolveResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Len(t, payload.Jobs, 1)
	require.Empty(t, payload.Errors)
}

func TestRunOutcomeSucceededWhenBuildStarted(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: successNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()
	rr := httptest.NewRecorder()
	TriggerPipelineRun(rr, requestWithURLParam(http.MethodPost, "/api/v1/data-integration/pipelines/"+repo.pipeline.ID.String()+"/runs", bytes.NewReader([]byte(`{"skip_unchanged":true}`)), "id", repo.pipeline.ID.String()))
	require.Equal(t, http.StatusCreated, rr.Code)
	require.Len(t, repo.runs, 1)
	for _, run := range repo.runs {
		require.Equal(t, "completed", run.Status)
	}
}

func TestRunOutcomeFailedWhenBuildService5xx(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: failingNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()
	rr := httptest.NewRecorder()
	TriggerPipelineRun(rr, requestWithURLParam(http.MethodPost, "/api/v1/data-integration/pipelines/"+repo.pipeline.ID.String()+"/runs", bytes.NewReader([]byte(`{}`)), "id", repo.pipeline.ID.String()))
	require.Equal(t, http.StatusCreated, rr.Code)
	for _, run := range repo.runs {
		require.Equal(t, "failed", run.Status)
		require.NotNil(t, run.ErrorMessage)
	}
}

func TestRunOutcomeIgnoredWhenOutputsFresh(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	runID := uuid.New()
	repo.runs[runID] = models.PipelineRun{ID: runID, PipelineID: repo.pipeline.ID, Status: "completed", TriggerType: "scheduled", StartedAt: time.Now().UTC()}
	repo.summary["completed"] = 1
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo})
	defer restore()
	rr := httptest.NewRecorder()
	DataIntegrationQueueSummary(rr, httptest.NewRequest(http.MethodGet, "/api/v1/data-integration/builds/_summary", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var payload map[string]map[string]float64
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, float64(1), payload["last_24h"]["completed"])
}

func TestCoalesceReRunWhenPreviousActive(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	activeID := uuid.New()
	repo.runs[activeID] = models.PipelineRun{ID: activeID, PipelineID: repo.pipeline.ID, Status: "running", TriggerType: "manual", AttemptNumber: 1, StartedAt: time.Now().UTC()}
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo})
	defer restore()
	rr := httptest.NewRecorder()
	AbortDataIntegrationBuild(rr, requestWithURLParam(http.MethodPost, "/api/v1/data-integration/builds/"+activeID.String()+"/abort", nil, "run_id", activeID.String()))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "aborted", repo.runs[activeID].Status)
}

func TestEventTriggerObservedPersistsUntilRun(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	repo.due = []models.Pipeline{repo.pipeline}
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: successNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()
	rr := httptest.NewRecorder()
	RunDueScheduledPipelines(rr, httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/pipelines/_scheduler/run-due", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var payload map[string]float64
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, float64(1), payload["triggered_runs"])
	require.True(t, repo.nextRunUpdated)
}

func TestRetryPipelineRunUsesDataIntegrationRepository(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	previous, err := repo.OpenPipelineRun(context.Background(), &repo.pipeline, models.TriggerPipelineRequest{}, nil, json.RawMessage(`{"trigger":{"type":"manual"}}`))
	require.NoError(t, err)
	previous.Status = "failed"
	previous.NodeResults = json.RawMessage(`{"n1":"FAILED"}`)
	repo.runs[previous.ID] = *previous
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: successNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/pipelines/"+repo.pipeline.ID.String()+"/runs/"+previous.ID.String()+"/retry", strings.NewReader(`{"skip_unchanged":true}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", repo.pipeline.ID.String())
	rctx.URLParams.Add("run_id", previous.ID.String())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	RetryPipelineRun(rr, r)

	require.Equal(t, http.StatusCreated, rr.Code)
	var run models.PipelineRun
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&run))
	require.Equal(t, "retry", run.TriggerType)
	require.Equal(t, previous.ID, *run.RetryOfRunID)
	require.Equal(t, int32(2), run.AttemptNumber)
}

func TestRunDueScheduledPipelinesRecomputesNextRunAtFromCron(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	// Override the schedule_config to carry a real cron expression so the
	// scheduler must reach for ComputeNextRunAt to advance the watermark.
	repo.pipeline.ScheduleConfig = json.RawMessage(`{"enabled":true,"cron":"0 9 * * *"}`)
	repo.due = []models.Pipeline{repo.pipeline}
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: successNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()
	rr := httptest.NewRecorder()
	RunDueScheduledPipelines(rr, httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/pipelines/_scheduler/run-due", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, repo.nextRunUpdated, "scheduler must persist the recomputed next_run_at")
	require.NotNil(t, repo.nextRunAt, "valid cron must yield a non-nil next_run_at (regression: legacy code always wrote nil)")
	require.True(t, repo.nextRunAt.After(time.Now().Add(-time.Minute)), "next_run_at must be in the future, got %v", repo.nextRunAt)
}

func TestRunDueScheduledPipelinesLeavesNextRunAtNilWhenCronInvalid(t *testing.T) {
	repo := newDataIntegrationRepo(t)
	repo.pipeline.ScheduleConfig = json.RawMessage(`{"enabled":true,"cron":"not a cron"}`)
	repo.due = []models.Pipeline{repo.pipeline}
	restore := SetExecutionPorts(ExecutionPorts{Runs: repo, NodeRunner: successNodeRunner{}, Committer: &recordingCommitter{}, Transactions: &recordingTransactions{}, Parallelism: 1})
	defer restore()
	rr := httptest.NewRecorder()
	RunDueScheduledPipelines(rr, httptest.NewRequest(http.MethodPost, "/api/v1/data-integration/pipelines/_scheduler/run-due", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, repo.nextRunUpdated)
	require.Nil(t, repo.nextRunAt, "invalid cron must clear next_run_at to keep the pipeline out of the run-due loop")
}

type dataIntegrationRepo struct {
	pipeline       models.Pipeline
	runs           map[uuid.UUID]models.PipelineRun
	due            []models.Pipeline
	summary        map[string]int64
	nextRunUpdated bool
	nextRunAt      *time.Time
}

func newDataIntegrationRepo(t *testing.T) *dataIntegrationRepo {
	t.Helper()
	pipelineID := uuid.New()
	ownerID := uuid.New()
	dag := json.RawMessage(`[{"id":"n1","label":"n1","transform_type":"noop","config":{},"depends_on":[]}]`)
	return &dataIntegrationRepo{pipeline: models.Pipeline{ID: pipelineID, Name: "p", OwnerID: ownerID, DAG: dag, Status: "active", RetryPolicy: json.RawMessage(`{"max_attempts":1,"retry_on_failure":false,"allow_partial_reexecution":true}`), ScheduleConfig: json.RawMessage(`{"enabled":true}`), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, runs: map[uuid.UUID]models.PipelineRun{}, summary: map[string]int64{}}
}

func (r *dataIntegrationRepo) LoadPipeline(context.Context, uuid.UUID) (*models.Pipeline, error) {
	return &r.pipeline, nil
}
func (r *dataIntegrationRepo) OpenPipelineRun(ctx context.Context, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, contextJSON json.RawMessage) (*models.PipelineRun, error) {
	return r.OpenPipelineRunWithOptions(ctx, pipeline, req, startedBy, "manual", req.FromNodeID, nil, 1, contextJSON)
}
func (r *dataIntegrationRepo) OpenPipelineRunWithOptions(_ context.Context, pipeline *models.Pipeline, _ models.TriggerPipelineRequest, startedBy *uuid.UUID, triggerType string, fromNodeID *string, retryOfRunID *uuid.UUID, attemptNumber int32, contextJSON json.RawMessage) (*models.PipelineRun, error) {
	id := uuid.New()
	run := models.PipelineRun{ID: id, PipelineID: pipeline.ID, Status: "running", TriggerType: triggerType, StartedBy: startedBy, AttemptNumber: attemptNumber, StartedFromNodeID: fromNodeID, RetryOfRunID: retryOfRunID, ExecutionContext: contextJSON, StartedAt: time.Now().UTC()}
	r.runs[id] = run
	return &run, nil
}
func (r *dataIntegrationRepo) FinishPipelineRun(_ context.Context, runID uuid.UUID, status string, nodeResults json.RawMessage, errorMessage *string) error {
	run := r.runs[runID]
	run.Status = status
	run.NodeResults = nodeResults
	run.ErrorMessage = errorMessage
	now := time.Now().UTC()
	run.FinishedAt = &now
	r.runs[runID] = run
	return nil
}
func (r *dataIntegrationRepo) ListPipelineRuns(context.Context, uuid.UUID, int64, int64) ([]models.PipelineRun, error) {
	return r.allRuns(), nil
}
func (r *dataIntegrationRepo) GetPipelineRun(_ context.Context, _, runID uuid.UUID) (*models.PipelineRun, error) {
	run, ok := r.runs[runID]
	if !ok {
		return nil, nil
	}
	return &run, nil
}
func (r *dataIntegrationRepo) ListBuildQueue(context.Context, BuildQueueQuery) ([]models.PipelineRun, error) {
	return r.allRuns(), nil
}
func (r *dataIntegrationRepo) AbortPipelineRun(_ context.Context, runID uuid.UUID) (*models.PipelineRun, bool, error) {
	run, ok := r.runs[runID]
	if !ok {
		return nil, false, nil
	}
	if run.Status != "running" {
		return nil, true, nil
	}
	run.Status = "aborted"
	r.runs[runID] = run
	return &run, true, nil
}
func (r *dataIntegrationRepo) QueueSummary(context.Context) (map[string]int64, error) {
	return r.summary, nil
}
func (r *dataIntegrationRepo) ListDuePipelines(context.Context) ([]models.Pipeline, error) {
	return r.due, nil
}
func (r *dataIntegrationRepo) UpdatePipelineNextRun(_ context.Context, _ uuid.UUID, nextRunAt *time.Time) error {
	r.nextRunUpdated = true
	r.nextRunAt = nextRunAt
	return nil
}
func (r *dataIntegrationRepo) allRuns() []models.PipelineRun {
	out := make([]models.PipelineRun, 0, len(r.runs))
	for _, run := range r.runs {
		out = append(out, run)
	}
	return out
}

type successNodeRunner struct{}

func (successNodeRunner) Run(context.Context, executor.NodeContext) (executor.NodeResult, error) {
	return executor.NodeResult{}, nil
}

type failingNodeRunner struct{}

func (failingNodeRunner) Run(context.Context, executor.NodeContext) (executor.NodeResult, error) {
	return executor.NodeResult{}, errBuildService5xx{}
}

type errBuildService5xx struct{}

func (errBuildService5xx) Error() string { return "build service 5xx" }
