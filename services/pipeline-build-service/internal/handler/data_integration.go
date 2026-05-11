package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/schedule"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

type BuildQueueQuery struct {
	Status      string
	TriggerType string
	PipelineID  *uuid.UUID
	Page        int64
	PerPage     int64
}

func ListPipelineRuns(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	page, perPage := pageParams(r, 20, 100)
	runs, err := repo.ListPipelineRuns(r.Context(), pipelineID, page, perPage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": runs})
}

func GetPipelineRun(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	runID, err := pipelineRunIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := repo.GetPipelineRun(r.Context(), pipelineID, runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func RetryPipelineRun(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	runID, err := pipelineRunIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	var body models.RetryPipelineRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	pipeline, err := repo.LoadPipeline(r.Context(), pipelineID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pipeline == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	previous, err := repo.GetPipelineRun(r.Context(), pipelineID, runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if previous == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	policy := pipeline.ParsedRetryPolicy()
	if body.FromNodeID != nil && !policy.AllowPartialReexecution {
		writeJSON(w, http.StatusBadRequest, "partial re-execution is not enabled for this pipeline")
		return
	}
	fromNodeID := body.FromNodeID
	if fromNodeID == nil && policy.AllowPartialReexecution {
		fromNodeID = firstFailedNode(previous.NodeResults)
	}
	req := models.TriggerPipelineRequest{FromNodeID: fromNodeID, SkipUnchanged: body.SkipUnchanged}
	run, err := startPipelineRun(r, pipeline, req, previous.StartedBy, "retry", fromNodeID, &previous.ID, previous.AttemptNumber+1, previous.ExecutionContext, repo)
	if err != nil {
		var validationFailure pipelineStrictValidationFailure
		if errors.As(err, &validationFailure) {
			writePipelineSchemaValidationFailure(w, validationFailure.Report)
			return
		}
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func CancelPipelineRun(w http.ResponseWriter, r *http.Request) { AbortDataIntegrationBuild(w, r) }

func ListDataIntegrationBuilds(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	page, perPage := pageParams(r, 50, 200)
	query := BuildQueueQuery{Status: r.URL.Query().Get("status"), TriggerType: r.URL.Query().Get("trigger_type"), Page: page, PerPage: perPage}
	if raw := r.URL.Query().Get("pipeline_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, "invalid pipeline_id")
			return
		}
		query.PipelineID = &id
	}
	runs, err := repo.ListBuildQueue(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": runs, "page": page, "per_page": perPage})
}

func DataIntegrationQueueSummary(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	summary, err := repo.QueueSummary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"last_24h": summary})
}

func AbortDataIntegrationBuild(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	runID, err := pipelineRunIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	run, exists, err := repo.AbortPipelineRun(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if run != nil {
		writeJSON(w, http.StatusOK, run)
		return
	}
	if exists {
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func RunDueScheduledPipelines(w http.ResponseWriter, r *http.Request) {
	repo, ok := currentDataIntegrationRunRepository(w)
	if !ok {
		return
	}
	pipelines, err := repo.ListDuePipelines(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	triggered := 0
	for i := range pipelines {
		pipeline := &pipelines[i]
		contextJSON, _ := json.Marshal(map[string]any{"trigger": map[string]any{"type": "scheduled", "scheduled_at": time.Now().UTC()}})
		req := models.TriggerPipelineRequest{SkipUnchanged: true}
		if _, err := startPipelineRun(r, pipeline, req, nil, "scheduled", nil, nil, 1, contextJSON, repo); err == nil {
			triggered++
			cfg := pipeline.Schedule()
			next := schedule.ComputeNextRunAt(pipeline.Status, cfg.Enabled, cfg.Cron, time.Now().UTC())
			_ = repo.UpdatePipelineNextRun(r.Context(), pipeline.ID, next)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"triggered_runs": triggered})
}

func currentDataIntegrationRunRepository(w http.ResponseWriter) (DataIntegrationRunRepository, bool) {
	ports, ok := currentExecutionPorts()
	if !ok || ports.Runs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "execution_ports_not_configured", "detail": "data-integration routes require DATABASE_URL-backed run repository wiring"})
		return nil, false
	}
	repo, ok := ports.Runs.(DataIntegrationRunRepository)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "data_integration_repository_not_configured"})
		return nil, false
	}
	return repo, true
}

func startPipelineRun(r *http.Request, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, triggerType string, fromNodeID *string, retryOfRunID *uuid.UUID, attemptNumber int32, contextJSON json.RawMessage, repo DataIntegrationRunRepository) (*models.PipelineRun, error) {
	ports, _ := currentExecutionPorts()
	validationFailure, err := validationFailureForRuntimePipeline(pipeline)
	if err != nil {
		return nil, err
	}
	if validationFailure != nil {
		return nil, *validationFailure
	}
	run, err := repo.OpenPipelineRunWithOptions(r.Context(), pipeline, req, startedBy, triggerType, fromNodeID, retryOfRunID, attemptNumber, contextJSON)
	if err != nil {
		return nil, err
	}
	plan, err := planFromPipeline(pipeline, run.ID, req, ports)
	if err != nil {
		finishRunBestEffort(r.Context(), repo, run.ID, "failed", nil, err.Error())
		return nil, err
	}
	if err := repo.MarkPipelineRunRunning(r.Context(), run.ID); err != nil {
		finishRunBestEffort(r.Context(), repo, run.ID, "failed", nil, err.Error())
		return nil, err
	}
	runner := ports.NodeRunner
	if runner == nil {
		runner = newRuntimeNodeRunner(ports)
	}
	audit := newCapturingAuditSink(ports.Audit)
	execCtx, cancel := context.WithCancel(r.Context())
	unregister := registerExecutionCancel(plan.BuildID, cancel)
	defer unregister()
	outcome, err := executor.Execute(execCtx, plan, runner, ports.Transactions, ports.Committer, audit)
	if err != nil {
		finishRunBestEffort(r.Context(), repo, run.ID, "failed", nil, err.Error())
		return nil, err
	}
	status, errMsg := pipelineRunStatus(outcome)
	nodeResults, _ := json.Marshal(pipelineRunNodeResults(plan, outcome, audit.Events()))
	if err := repo.FinishPipelineRun(r.Context(), run.ID, status, nodeResults, errMsg); err != nil {
		return nil, err
	}
	fresh, err := repo.GetPipelineRun(r.Context(), pipeline.ID, run.ID)
	if err == nil && fresh != nil {
		return fresh, nil
	}
	return run, nil
}

func pipelineRunIDFromRequest(r *http.Request) (uuid.UUID, error) {
	for _, key := range []string{"run_id", "id"} {
		if raw := strings.TrimSpace(chiParam(r, key)); raw != "" {
			return uuid.Parse(raw)
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "runs" && i+1 < len(parts) {
			return uuid.Parse(parts[i+1])
		}
		if part == "builds" && i+1 < len(parts) {
			return uuid.Parse(parts[i+1])
		}
	}
	return uuid.Nil, errors.New("pipeline run id is required")
}

func chiParam(r *http.Request, key string) string {
	return strings.TrimSpace(chi.URLParam(r, key))
}

func pageParams(r *http.Request, defaultPerPage, maxPerPage int64) (int64, int64) {
	page := int64(1)
	perPage := defaultPerPage
	if raw := r.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			page = parsed
		}
	}
	if raw := r.URL.Query().Get("per_page"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			perPage = parsed
		}
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 1
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

func firstFailedNode(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	var rows []struct {
		NodeID string `json:"node_id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(raw, &rows) == nil {
		for _, row := range rows {
			if strings.EqualFold(row.Status, "failed") && row.NodeID != "" {
				return &row.NodeID
			}
		}
	}
	var legacy map[string]string
	if json.Unmarshal(raw, &legacy) == nil {
		for id, state := range legacy {
			if strings.EqualFold(state, "failed") || strings.EqualFold(state, "BUILD_FAILED") {
				v := id
				return &v
			}
		}
	}
	var states map[string]struct {
		State string `json:"state"`
	}
	if json.Unmarshal(raw, &states) == nil {
		for id, state := range states {
			if strings.EqualFold(state.State, "failed") || strings.EqualFold(state.State, "BUILD_FAILED") {
				v := id
				return &v
			}
		}
	}
	return nil
}
