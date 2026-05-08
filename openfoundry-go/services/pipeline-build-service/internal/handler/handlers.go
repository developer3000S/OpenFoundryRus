// Package handler hosts the HTTP handlers for pipeline-build-service.
//
// Status: every URL the Rust crate exposes is mounted with the same
// path / verb. Handlers either execute repository/runtime-backed work or
// return explicit machine-readable configuration errors when a production
// adapter has not been wired; see the README for the full port-status breakdown.
//
// What IS ported 1:1 in this binary:
//
//   - Models (build, job, pipeline, run) — `internal/models`
//   - Job lifecycle state machine — `internal/domain/joblifecycle`
//   - Marking propagation SQL — `internal/domain/markings`
//   - Build resolver — `internal/domain/resolver` plus HTTP wiring
//   - DAG executor — `internal/domain/executor` plus HTTP wiring
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	sparkpkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

const defaultSSEInitialDelay = 10 * time.Second
const defaultSSEHeartbeatInterval = time.Second

type jobLogStreamConfig struct {
	InitialDelay      time.Duration
	HeartbeatInterval time.Duration
}

var jobLogService atomic.Value               // stores *livellogs.Service
var streamConfig atomic.Value                // stores jobLogStreamConfig
var sparkClientValue atomic.Value            // stores *sparkClientSlot
var sparkSubmissionRepository atomic.Value   // stores *sparkSubmissionSlot
var buildQueryRepository atomic.Value        // stores *buildQuerySlot
var pipelineAuthoringRepository atomic.Value // stores *pipelineAuthoringSlot

type sparkClientSlot struct {
	client sparkpkg.SparkClient
}

type SparkSubmissionRepository interface {
	SaveSparkSubmission(ctx context.Context, submission SparkSubmission) error
	GetSparkSubmission(ctx context.Context, pipelineRunID uuid.UUID) (*SparkSubmission, error)
	UpdateSparkSubmissionStatus(ctx context.Context, pipelineRunID uuid.UUID, status sparkpkg.SparkRunStatus, errorMessage *string) error
	ListSparkSubmissions(ctx context.Context, limit int64) ([]SparkSubmission, error)
}

type SparkSubmission struct {
	PipelineRunID  uuid.UUID               `json:"pipeline_run_id"`
	Namespace      string                  `json:"namespace"`
	SparkAppName   string                  `json:"spark_app_name"`
	Status         sparkpkg.SparkRunStatus `json:"status"`
	ErrorMessage   *string                 `json:"error_message,omitempty"`
	SubmittedAt    *time.Time              `json:"submitted_at,omitempty"`
	LastObservedAt *time.Time              `json:"last_observed_at,omitempty"`
}

type sparkSubmissionSlot struct {
	repo SparkSubmissionRepository
}

type BuildQueryRepository interface {
	ListBuilds(ctx context.Context, query models.ListBuildsQuery) ([]models.BuildEnvelope, error)
	GetBuild(ctx context.Context, idOrRID string) (*models.BuildEnvelope, error)
	ListJobsForBuildID(ctx context.Context, idOrRID string) ([]models.Job, error)
	GetJob(ctx context.Context, idOrRID string) (*models.Job, error)
}

type BuildV1Repository interface {
	BuildQueryRepository
	ListDatasetBuilds(ctx context.Context, datasetRID string, limit int64) ([]models.Build, error)
	GetJobOutputs(ctx context.Context, jobRID string) (*JobOutputsResponse, error)
	GetJobInputResolutions(ctx context.Context, jobRID string) (json.RawMessage, error)
	PublishJobSpec(ctx context.Context, kind string, req CreateJobSpecRequest, createdBy string) (PublishedJobSpec, error)
}

type LogAppendStore interface {
	AppendLogByRID(ctx context.Context, jobRID string, level livellogs.LogLevel, message string, params json.RawMessage) (livellogs.LogEntry, error)
}

type buildQuerySlot struct {
	repo BuildQueryRepository
}

type PipelineAuthoringRepository interface {
	ListPipelines(ctx context.Context, query models.ListPipelinesQuery) (models.ListPipelinesResponse, error)
	CreatePipeline(ctx context.Context, req models.CreatePipelineRequest, ownerID uuid.UUID) (*models.Pipeline, error)
	GetPipeline(ctx context.Context, id uuid.UUID) (*models.Pipeline, error)
	UpdatePipeline(ctx context.Context, id uuid.UUID, req models.UpdatePipelineRequest) (*models.Pipeline, error)
	DeletePipeline(ctx context.Context, id uuid.UUID) (bool, error)
}

type pipelineAuthoringSlot struct {
	repo PipelineAuthoringRepository
}

func init() {
	streamConfig.Store(jobLogStreamConfig{InitialDelay: defaultSSEInitialDelay, HeartbeatInterval: defaultSSEHeartbeatInterval})
}

// SetJobLogService injects the live-log ports used by StreamJobLogs. It returns
// a restore function so tests can isolate global handler state.
func SetJobLogService(service *livellogs.Service) func() {
	previous, _ := jobLogService.Load().(*livellogs.Service)
	jobLogService.Store(service)
	return func() { jobLogService.Store(previous) }
}

// SetJobLogStreamConfig adjusts SSE timing. Production defaults mirror Rust's
// 10-second live-log delay; tests can set zero delay for immediate history.
func SetJobLogStreamConfig(initialDelay, heartbeatInterval time.Duration) func() {
	previous := streamConfig.Load().(jobLogStreamConfig)
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultSSEHeartbeatInterval
	}
	streamConfig.Store(jobLogStreamConfig{InitialDelay: initialDelay, HeartbeatInterval: heartbeatInterval})
	return func() { streamConfig.Store(previous) }
}

// SetSparkClient injects the Kubernetes/Spark client used by SubmitSparkRun and
// GetSparkRun. It returns a restore function for tests.
func SetSparkClient(client sparkpkg.SparkClient) func() {
	previous, _ := sparkClientValue.Load().(*sparkClientSlot)
	if previous == nil {
		previous = &sparkClientSlot{}
	}
	sparkClientValue.Store(&sparkClientSlot{client: client})
	return func() { sparkClientValue.Store(previous) }
}

// SetSparkSubmissionRepository injects the persistence adapter for Rust-compatible
// /api/v1/pipeline/builds SparkApplication submission routes.
func SetSparkSubmissionRepository(repo SparkSubmissionRepository) func() {
	previous, _ := sparkSubmissionRepository.Load().(*sparkSubmissionSlot)
	sparkSubmissionRepository.Store(&sparkSubmissionSlot{repo: repo})
	return func() { sparkSubmissionRepository.Store(previous) }
}

func currentSparkSubmissionRepository() (SparkSubmissionRepository, bool) {
	slot, _ := sparkSubmissionRepository.Load().(*sparkSubmissionSlot)
	if slot == nil || slot.repo == nil {
		return nil, false
	}
	return slot.repo, true
}

// SetBuildQueryRepository injects read-side build/job repositories for list/get
// handlers. Without it, those handlers return an explicit 503 instead of a
// silent empty success.
func SetBuildQueryRepository(repo BuildQueryRepository) func() {
	previous, _ := buildQueryRepository.Load().(*buildQuerySlot)
	buildQueryRepository.Store(&buildQuerySlot{repo: repo})
	return func() { buildQueryRepository.Store(previous) }
}

func currentBuildQueryRepository() (BuildQueryRepository, bool) {
	slot, _ := buildQueryRepository.Load().(*buildQuerySlot)
	if slot == nil || slot.repo == nil {
		return nil, false
	}
	return slot.repo, true
}

// SetPipelineAuthoringRepository injects the productive repository for legacy
// /api/v1/pipelines CRUD aliases. Without it the handlers return explicit 503s.
func SetPipelineAuthoringRepository(repo PipelineAuthoringRepository) func() {
	previous, _ := pipelineAuthoringRepository.Load().(*pipelineAuthoringSlot)
	if previous == nil {
		previous = &pipelineAuthoringSlot{}
	}
	pipelineAuthoringRepository.Store(&pipelineAuthoringSlot{repo: repo})
	return func() { pipelineAuthoringRepository.Store(previous) }
}

func currentPipelineAuthoringRepository() (PipelineAuthoringRepository, bool) {
	slot, _ := pipelineAuthoringRepository.Load().(*pipelineAuthoringSlot)
	if slot == nil || slot.repo == nil {
		return nil, false
	}
	return slot.repo, true
}

func requirePipelineAuthoringRepository(w http.ResponseWriter, detail string) (PipelineAuthoringRepository, bool) {
	repo, ok := currentPipelineAuthoringRepository()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipeline_authoring_repository_not_configured", "detail": detail})
		return nil, false
	}
	return repo, true
}

func requireBuildQueryRepository(w http.ResponseWriter, detail string) (BuildQueryRepository, bool) {
	repo, ok := currentBuildQueryRepository()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "build_query_repository_not_configured", "detail": detail})
		return nil, false
	}
	return repo, true
}

func requireBuildV1Repository(w http.ResponseWriter, detail string) (BuildV1Repository, bool) {
	repo, ok := requireBuildQueryRepository(w, detail)
	if !ok {
		return nil, false
	}
	v1, ok := repo.(BuildV1Repository)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "build_v1_repository_not_configured", "detail": detail})
		return nil, false
	}
	return v1, true
}

func requireJobLogStore(w http.ResponseWriter, detail string) (*livellogs.Service, bool) {
	service, _ := jobLogService.Load().(*livellogs.Service)
	if service == nil || service.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live_logs_not_configured", "detail": detail})
		return nil, false
	}
	return service, true
}

func requireLogAppender(w http.ResponseWriter, detail string) (LogAppendStore, *livellogs.Service, bool) {
	service, ok := requireJobLogStore(w, detail)
	if !ok {
		return nil, nil, false
	}
	appender, ok := service.Store.(LogAppendStore)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "log_append_not_configured", "detail": detail})
		return nil, nil, false
	}
	return appender, service, true
}

func requireJobLogSubscriber(w http.ResponseWriter, detail string) (*livellogs.Service, bool) {
	service, _ := jobLogService.Load().(*livellogs.Service)
	if service == nil || service.Subscriber == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live_logs_not_configured", "detail": detail})
		return nil, false
	}
	return service, true
}

func writeLogStoreUnavailable(w http.ResponseWriter, errorName string, err error) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errorName, "detail": err.Error()})
}

func writeLogSubscriberUnavailable(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live_logs_not_configured", "detail": detail})
}

// Builds (v1).
func ListBuilds(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "ListBuilds requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			limit = parsed
		}
	}
	items, err := repo.ListBuilds(r.Context(), models.ListBuildsQuery{Branch: r.URL.Query().Get("branch"), Status: r.URL.Query().Get("status"), PipelineRID: r.URL.Query().Get("pipeline_rid"), Limit: &limit})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_builds_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}
func GetBuild(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "GetBuild requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	env, err := repo.GetBuild(r.Context(), buildIDParam(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_build_failed", "detail": err.Error()})
		return
	}
	if env == nil {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// Jobs and job logs.
func ListJobs(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "ListJobs requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	items, err := repo.ListJobsForBuildID(r.Context(), buildIDParam(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_jobs_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}
func GetJob(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "GetJob requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	job, err := repo.GetJob(r.Context(), jobRIDFromRequest(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_job_failed", "detail": err.Error()})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func ListJobLogs(w http.ResponseWriter, r *http.Request) {
	service, ok := requireJobLogStore(w, "ListJobLogs requires DATABASE_URL-backed log store wiring")
	if !ok {
		return
	}
	query, err := parseLogsQuery(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_logs_query", "detail": err.Error()})
		return
	}
	query.Follow = false
	items, err := service.Store.History(r.Context(), jobRIDFromRequest(r), query)
	if err != nil {
		writeLogStoreUnavailable(w, "log_store_unavailable", err)
		return
	}
	rows := make([]livellogs.RowDTO, 0, len(items))
	for _, item := range items {
		rows = append(rows, item.RowDTO())
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "total": len(rows)})
}

// StreamJobLogs streams Rust-compatible Server-Sent Events for job logs.
func StreamJobLogs(w http.ResponseWriter, r *http.Request) {
	service, ok := requireJobLogStore(w, "live logs not configured")
	if !ok {
		return
	}
	jobRID := jobRIDFromRequest(r)
	if jobRID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_job_id"})
		return
	}
	query, err := parseLogsQuery(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_logs_query", "detail": err.Error()})
		return
	}

	history, err := service.Store.History(r.Context(), jobRID, query)
	if err != nil {
		writeLogStoreUnavailable(w, "log_store_unavailable", err)
		return
	}

	var live <-chan livellogs.LogEntry
	var unsubscribe func()
	if query.Follow {
		if service.Subscriber == nil {
			writeLogSubscriberUnavailable(w, "log subscriber not configured")
			return
		}
		live, unsubscribe, err = service.Subscriber.Subscribe(r.Context(), jobRID)
		if err != nil {
			writeLogStoreUnavailable(w, "log_subscriber_unavailable", err)
			return
		}
		defer unsubscribe()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming_not_supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	cfg := streamConfig.Load().(jobLogStreamConfig)
	writeHeartbeat(w, flusher, cfg.InitialDelay, "Live logs are streamed in real-time. Time range filters do not apply.")
	if !waitInitialDelay(r.Context(), w, flusher, cfg) {
		return
	}
	for _, entry := range history {
		if !writeLogEvent(w, flusher, entry) {
			return
		}
	}
	if !query.Follow {
		return
	}

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = w.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
		case entry, ok := <-live:
			if !ok {
				return
			}
			if entry.JobRID != "" && entry.JobRID != jobRID {
				continue
			}
			if !queryAllowsLive(query, entry) {
				continue
			}
			if !writeLogEvent(w, flusher, entry) {
				return
			}
		}
	}
}

// Dry-run + execute (resolution preview / immediate trigger).
func DryRunValidate(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": "request body is required"})
		return
	}
	var body dryRunResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	errorsOut := []dryRunError{}
	if strings.TrimSpace(body.PipelineRID) == "" {
		errorsOut = append(errorsOut, dryRunError{Kind: "validation", Message: "pipeline_rid is required"})
	}
	if strings.TrimSpace(body.BuildBranch) == "" {
		errorsOut = append(errorsOut, dryRunError{Kind: "validation", Message: "build_branch is required"})
	}
	if len(body.OutputDatasetRIDs) == 0 && len(body.InlineSpecs) == 0 {
		errorsOut = append(errorsOut, dryRunError{Kind: "validation", Message: "output_dataset_rids or inline_specs is required"})
	}
	status := http.StatusOK
	if len(errorsOut) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{"valid": len(errorsOut) == 0, "errors": errorsOut})
}

// Pipeline CRUD (legacy surface that still owns the cron schedule rows).
func ListPipelines(w http.ResponseWriter, r *http.Request) {
	repo, ok := requirePipelineAuthoringRepository(w, "ListPipelines requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	query := models.ListPipelinesQuery{}
	if raw := r.URL.Query().Get("page"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_page"})
			return
		}
		query.Page = &parsed
	}
	if raw := r.URL.Query().Get("per_page"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_per_page"})
			return
		}
		query.PerPage = &parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("search")); raw != "" {
		query.Search = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		query.Status = &raw
	}
	out, err := repo.ListPipelines(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_pipelines_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func CreatePipeline(w http.ResponseWriter, r *http.Request) {
	repo, ok := requirePipelineAuthoringRepository(w, "CreatePipeline requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	var req models.CreatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	ownerID := uuid.Nil
	if claims, ok := authmw.FromContext(r.Context()); ok {
		ownerID = claims.Sub
	}
	pipeline, err := repo.CreatePipeline(r.Context(), req, ownerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "create_pipeline_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, pipeline)
}
func GetPipeline(w http.ResponseWriter, r *http.Request) {
	repo, ok := requirePipelineAuthoringRepository(w, "GetPipeline requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	id, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	pipeline, err := repo.GetPipeline(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_pipeline_failed", "detail": err.Error()})
		return
	}
	if pipeline == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, pipeline)
}
func UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	repo, ok := requirePipelineAuthoringRepository(w, "UpdatePipeline requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	id, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	var req models.UpdatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	pipeline, err := repo.UpdatePipeline(r.Context(), id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "update_pipeline_failed", "detail": err.Error()})
		return
	}
	if pipeline == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, pipeline)
}
func DeletePipeline(w http.ResponseWriter, r *http.Request) {
	repo, ok := requirePipelineAuthoringRepository(w, "DeletePipeline requires DATABASE_URL-backed pipeline authoring repository wiring")
	if !ok {
		return
	}
	id, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	deleted, err := repo.DeletePipeline(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete_pipeline_failed", "detail": err.Error()})
		return
	}
	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Pipeline runs (legacy run table; coexists with the new builds table).
// Implemented in data_integration.go.

// SparkApplication-backed runs (FASE 3 / Tarea 3.4). When kube_client
// is unavailable the Rust crate returns 503; we mirror that shape.
func ListSparkRuns(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireSparkSubmissionRepository(w, "ListSparkRuns requires DATABASE_URL-backed pipeline_run_submissions wiring")
	if !ok {
		return
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := repo.ListSparkSubmissions(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_spark_runs_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func requireSparkSubmissionRepository(w http.ResponseWriter, detail string) (SparkSubmissionRepository, bool) {
	repo, ok := currentSparkSubmissionRepository()
	if !ok {
		writeSparkSubmissionRepositoryUnavailable(w, detail)
		return nil, false
	}
	return repo, true
}

func writeSparkSubmissionRepositoryUnavailable(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "spark_submission_repository_not_configured", "detail": detail})
}

type submitSparkRunRequest struct {
	PipelineRunID       *uuid.UUID                      `json:"pipeline_run_id,omitempty"`
	PipelineID          string                          `json:"pipeline_id"`
	RunID               string                          `json:"run_id,omitempty"`
	InputDatasetRID     string                          `json:"input_dataset_rid"`
	OutputDatasetRID    string                          `json:"output_dataset_rid"`
	ApplicationType     *sparkpkg.SparkApplicationType  `json:"application_type,omitempty"`
	PipelineRunnerImage string                          `json:"pipeline_runner_image,omitempty"`
	Namespace           string                          `json:"namespace,omitempty"`
	Resources           sparkpkg.SparkResourceOverrides `json:"resources,omitempty"`
}

func SubmitSparkRun(w http.ResponseWriter, r *http.Request) {
	client, ok := currentSparkClient()
	if !ok {
		writeKubeUnavailable(w)
		return
	}
	var body submitSparkRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	runID := uuid.New()
	if body.PipelineRunID != nil {
		runID = *body.PipelineRunID
	}
	shortRunID := body.RunID
	if shortRunID == "" {
		shortRunID = strings.ReplaceAll(runID.String(), "-", "")[:12]
	}
	appType := sparkpkg.SparkApplicationScala
	if body.ApplicationType != nil {
		appType = *body.ApplicationType
	}
	image := body.PipelineRunnerImage
	if image == "" {
		image = "openfoundry/pipeline-runner:dev"
	}
	namespace := body.Namespace
	if namespace == "" {
		namespace = "openfoundry-spark"
	}
	input := sparkpkg.PipelineRunInput{PipelineID: body.PipelineID, RunID: shortRunID, Namespace: namespace, ApplicationType: appType, PipelineRunnerImage: image, InputDatasetRID: body.InputDatasetRID, OutputDatasetRID: body.OutputDatasetRID, Resources: body.Resources}
	name, err := client.SubmitPipelineRun(r.Context(), input)
	if err != nil {
		writeSparkError(w, err)
		return
	}
	if repo, ok := currentSparkSubmissionRepository(); ok {
		if err := repo.SaveSparkSubmission(r.Context(), SparkSubmission{PipelineRunID: runID, Namespace: namespace, SparkAppName: name, Status: sparkpkg.SparkRunSubmitted}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "spark_submission_persist_failed", "detail": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"pipeline_run_id": runID, "namespace": namespace, "spark_app_name": name, "status": string(sparkpkg.SparkRunSubmitted)})
}

func SubmitPipelineBuildRun(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSparkSubmissionRepository(w, "Rust-compatible /api/v1/pipeline/builds/run requires DATABASE_URL-backed pipeline_run_submissions persistence"); !ok {
		return
	}
	SubmitSparkRun(w, r)
}

func GetPipelineBuildRunStatus(w http.ResponseWriter, r *http.Request) {
	client, ok := currentSparkClient()
	if !ok {
		writeKubeUnavailable(w)
		return
	}
	repo, ok := requireSparkSubmissionRepository(w, "status lookup requires DATABASE_URL-backed pipeline_run_submissions persistence")
	if !ok {
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "run_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_run_id", "detail": err.Error()})
		return
	}
	submission, err := repo.GetSparkSubmission(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "spark_submission_lookup_failed", "detail": err.Error()})
		return
	}
	if submission == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_pipeline_run_id"})
		return
	}
	report, err := client.GetPipelineRunStatus(r.Context(), submission.Namespace, submission.SparkAppName)
	if err != nil {
		writeSparkError(w, err)
		return
	}
	if report != nil {
		if err := repo.UpdateSparkSubmissionStatus(r.Context(), runID, report.Status, report.ErrorMessage); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "spark_submission_status_update_failed", "detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pipeline_run_id": runID, "namespace": submission.Namespace, "spark_app_name": submission.SparkAppName, "status": string(report.Status), "error_message": report.ErrorMessage})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipeline_run_id": runID, "namespace": submission.Namespace, "spark_app_name": submission.SparkAppName, "status": string(submission.Status), "error_message": submission.ErrorMessage, "note": "SparkApplication CR no longer present in cluster"})
}

func GetSparkRun(w http.ResponseWriter, r *http.Request) {
	client, ok := currentSparkClient()
	if !ok {
		writeKubeUnavailable(w)
		return
	}
	name := chi.URLParam(r, "id")
	if name == "" {
		name = strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/"):], "/")
	}
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "openfoundry-spark"
	}
	report, err := client.GetPipelineRunStatus(r.Context(), namespace, name)
	if err != nil {
		writeSparkError(w, err)
		return
	}
	if report == nil {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"namespace": namespace, "spark_app_name": name, "status": string(report.Status), "error_message": report.ErrorMessage})
}

func GetSpecForRun(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, nil)
}

func parseLogsQuery(r *http.Request) (livellogs.Query, error) {
	values := r.URL.Query()
	query := livellogs.Query{Follow: true, Limit: 5000}
	if raw := values.Get("from_sequence"); raw != "" {
		seq, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return query, fmt.Errorf("from_sequence: %w", err)
		}
		query.FromSequence = seq
	} else if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		seq, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			query.FromSequence = seq + 1
		}
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return query, fmt.Errorf("limit: %w", err)
		}
		if limit < 1 {
			limit = 1
		}
		if limit > 5000 {
			limit = 5000
		}
		query.Limit = limit
	}
	if raw := values.Get("follow"); raw != "" {
		follow, err := strconv.ParseBool(raw)
		if err != nil {
			return query, fmt.Errorf("follow: %w", err)
		}
		query.Follow = follow
	}
	if raw := values.Get("levels"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			level, ok := livellogs.ParseLogLevel(item)
			if ok {
				query.Levels = append(query.Levels, level)
			}
		}
	}
	if raw := values.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return query, fmt.Errorf("since: %w", err)
		}
		query.Since = &t
	}
	if raw := values.Get("until"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return query, fmt.Errorf("until: %w", err)
		}
		query.Until = &t
	}
	return query, nil
}

func waitInitialDelay(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, cfg jobLogStreamConfig) bool {
	if cfg.InitialDelay <= 0 {
		return true
	}
	remainingTicks := int(cfg.InitialDelay / cfg.HeartbeatInterval)
	if cfg.InitialDelay%cfg.HeartbeatInterval != 0 {
		remainingTicks++
	}
	for remainingTicks > 0 {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(cfg.HeartbeatInterval):
			remainingTicks--
			writeHeartbeat(w, flusher, time.Duration(remainingTicks)*cfg.HeartbeatInterval, "")
		}
	}
	return true
}

func writeHeartbeat(w http.ResponseWriter, flusher http.Flusher, remaining time.Duration, message string) {
	payload := map[string]any{"phase": "initializing", "delay_remaining_seconds": int64(remaining.Seconds())}
	if message != "" {
		payload["message"] = message
	}
	_ = writeSSE(w, "heartbeat", "", payload)
	flusher.Flush()
}

func writeLogEvent(w http.ResponseWriter, flusher http.Flusher, entry livellogs.LogEntry) bool {
	id := ""
	if entry.Sequence > 0 {
		id = strconv.FormatInt(entry.Sequence, 10)
	}
	if err := writeSSE(w, "log", id, entry.RowDTO()); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func writeSSE(w http.ResponseWriter, eventName, id string, payload any) error {
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err = w.Write([]byte("\n"))
	return err
}

func jobRIDFromRequest(r *http.Request) string {
	if rid := chi.URLParam(r, "id"); rid != "" {
		return rid
	}
	if rid := chi.URLParam(r, "rid"); rid != "" {
		return rid
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "jobs" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func queryAllowsLive(query livellogs.Query, entry livellogs.LogEntry) bool {
	if entry.Sequence < query.FromSequence {
		return false
	}
	if len(query.Levels) == 0 {
		return true
	}
	for _, level := range query.Levels {
		if entry.Level == level {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func currentSparkClient() (sparkpkg.SparkClient, bool) {
	if slot, ok := sparkClientValue.Load().(*sparkClientSlot); ok && slot != nil && slot.client != nil {
		if _, disabled := slot.client.(noSparkClient); disabled {
			return nil, false
		}
		return slot.client, true
	}
	client, err := sparkpkg.NewKubernetesClientFromEnv()
	if err != nil {
		return nil, false
	}
	return client, true
}

func writeKubeUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error":  "kube_client_unavailable",
		"detail": "SparkApplication endpoints require KUBERNETES_API_URL or an in-cluster/kubeconfig Kubernetes client",
	})
}

func writeSparkError(w http.ResponseWriter, err error) {
	var invalid *sparkpkg.InvalidInputError
	var render *sparkpkg.RenderError
	var kube *sparkpkg.KubeError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_spark_spec", "detail": err.Error()})
	case errors.As(err, &render):
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "spark_render_failed", "detail": err.Error()})
	case errors.As(err, &kube):
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "kubernetes_api_error", "detail": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "spark_error", "detail": err.Error()})
	}
}

type noSparkClient struct{}

func (noSparkClient) SubmitPipelineRun(context.Context, sparkpkg.PipelineRunInput) (string, error) {
	return "", &sparkpkg.UnavailableError{}
}
func (noSparkClient) GetPipelineRunStatus(context.Context, string, string) (*sparkpkg.SparkRunStatusReport, error) {
	return nil, &sparkpkg.UnavailableError{}
}

// V1 Builds API wrappers. These mount the Rust `/v1` route group while
// reusing the existing resolver/executor/log ports.
func CreateBuildV1(w http.ResponseWriter, r *http.Request) { CreateBuild(w, r) }

func ListBuildsV1(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "ListBuildsV1 requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	var since *time.Time
	if raw := r.URL.Query().Get("since"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			since = &parsed
		}
	}
	items, err := repo.ListBuilds(r.Context(), models.ListBuildsQuery{Branch: r.URL.Query().Get("branch"), Status: r.URL.Query().Get("status"), PipelineRID: r.URL.Query().Get("pipeline_rid"), Cursor: r.URL.Query().Get("cursor"), Since: since, Limit: &limit})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_builds_failed", "detail": err.Error()})
		return
	}
	var next *string
	if len(items) > 0 {
		cursor := items[len(items)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
		next = &cursor
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "next_cursor": next, "limit": limit})
}

func GetBuildV1(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireBuildQueryRepository(w, "GetBuildV1 requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	env, err := repo.GetBuild(r.Context(), chi.URLParam(r, "rid"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_build_failed", "detail": err.Error()})
		return
	}
	if env == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	etag := buildETag(env)
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, env)
}

func AbortBuildV1(w http.ResponseWriter, r *http.Request) { AbortBuild(w, r) }

func ListDatasetBuildsV1(w http.ResponseWriter, r *http.Request) {
	v1, ok := requireBuildV1Repository(w, "ListDatasetBuilds requires DATABASE_URL-backed repository wiring")
	if !ok {
		return
	}
	rows, err := v1.ListDatasetBuilds(r.Context(), chi.URLParam(r, "rid"), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_dataset_builds_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "dataset_rid": chi.URLParam(r, "rid")})
}

type JobOutputRow struct {
	OutputDatasetRID string `json:"output_dataset_rid"`
	TransactionRID   string `json:"transaction_rid"`
	Committed        bool   `json:"committed"`
	Aborted          bool   `json:"aborted"`
}

type JobOutputsResponse struct {
	RID          string         `json:"rid"`
	State        string         `json:"state"`
	StaleSkipped bool           `json:"stale_skipped"`
	Outputs      []JobOutputRow `json:"outputs"`
}

func GetJobOutputsV1(w http.ResponseWriter, r *http.Request) {
	v1, ok := currentV1Repository(w)
	if !ok {
		return
	}
	resp, err := v1.GetJobOutputs(r.Context(), chi.URLParam(r, "rid"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_job_outputs_failed", "detail": err.Error()})
		return
	}
	if resp == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func GetJobInputResolutionsV1(w http.ResponseWriter, r *http.Request) {
	v1, ok := currentV1Repository(w)
	if !ok {
		return
	}
	raw, err := v1.GetJobInputResolutions(r.Context(), chi.URLParam(r, "rid"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_job_input_resolutions_failed", "detail": err.Error()})
		return
	}
	if raw == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rid": chi.URLParam(r, "rid"), "input_view_resolutions": json.RawMessage(raw)})
}

type CreateJobSpecRequest struct {
	PipelineRID       string             `json:"pipeline_rid"`
	BranchName        string             `json:"branch_name"`
	Inputs            []models.InputSpec `json:"inputs,omitempty"`
	OutputDatasetRIDs []string           `json:"output_dataset_rids,omitempty"`
	LogicPayload      json.RawMessage    `json:"logic_payload,omitempty"`
	ContentHash       *string            `json:"content_hash,omitempty"`
}

type PublishedJobSpec struct {
	RID         string `json:"rid"`
	LogicKind   string `json:"logic_kind"`
	ContentHash string `json:"content_hash"`
}

func CreateJobSpecV1(w http.ResponseWriter, r *http.Request) {
	v1, ok := currentV1Repository(w)
	if !ok {
		return
	}
	var body CreateJobSpecRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	kind := strings.ToUpper(chi.URLParam(r, "kind"))
	published, err := v1.PublishJobSpec(r.Context(), kind, body, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, published)
}

func ListJobLogsV1(w http.ResponseWriter, r *http.Request) { ListJobLogs(w, r) }

func EmitJobLogV1(w http.ResponseWriter, r *http.Request) {
	appender, service, ok := requireLogAppender(w, "EmitJobLog requires DATABASE_URL-backed log store wiring")
	if !ok {
		return
	}
	var body struct {
		Level   livellogs.LogLevel `json:"level"`
		Message string             `json:"message"`
		Params  json.RawMessage    `json:"params,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	entry, err := appender.AppendLogByRID(r.Context(), chi.URLParam(r, "rid"), body.Level, body.Message, body.Params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emit_log_failed", "detail": err.Error()})
		return
	}
	if mem, ok := service.Subscriber.(*livellogs.MemoryService); ok {
		mem.Emit(chi.URLParam(r, "rid"), body.Level, body.Message, body.Params)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rid": chi.URLParam(r, "rid"), "sequence": entry.Sequence})
}

func StreamJobLogsV1(w http.ResponseWriter, r *http.Request) { StreamJobLogs(w, r) }

func WSJobLogsV1(w http.ResponseWriter, r *http.Request) {
	service, ok := requireJobLogSubscriber(w, "websocket logs require a subscriber")
	if !ok {
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	ch, cancel, err := service.Subscriber.Subscribe(r.Context(), chi.URLParam(r, "rid"))
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if err := wsWriteJSON(r.Context(), conn, entry.RowDTO()); err != nil {
				return
			}
		}
	}
}

func currentV1Repository(w http.ResponseWriter) (BuildV1Repository, bool) {
	return requireBuildV1Repository(w, "v1 build/job routes require DATABASE_URL-backed repository wiring")
}

func buildETag(env *models.BuildEnvelope) string {
	raw, _ := json.Marshal(env)
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func wsWriteJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, raw)
}
