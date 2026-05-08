package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/runners"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/iceberg"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	runtimepkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/runtime"
)

// BuildPlanRepository adapts persisted build/job state into executor.Plan.
type BuildPlanRepository interface {
	LoadPlan(ctx context.Context, buildID uuid.UUID) (executor.Plan, error)
}

// PipelineRunRepository is the minimal legacy runs adapter used by
// TriggerPipelineRun. Implementations persist the Rust-compatible pipeline_runs
// lifecycle; tests use fakes.
type PipelineRunRepository interface {
	LoadPipeline(ctx context.Context, pipelineID uuid.UUID) (*models.Pipeline, error)
	OpenPipelineRun(ctx context.Context, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, contextJSON json.RawMessage) (*models.PipelineRun, error)
	FinishPipelineRun(ctx context.Context, runID uuid.UUID, status string, nodeResults json.RawMessage, errorMessage *string) error
}

type DataIntegrationRunRepository interface {
	PipelineRunRepository
	ListPipelineRuns(ctx context.Context, pipelineID uuid.UUID, page, perPage int64) ([]models.PipelineRun, error)
	GetPipelineRun(ctx context.Context, pipelineID, runID uuid.UUID) (*models.PipelineRun, error)
	OpenPipelineRunWithOptions(ctx context.Context, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, triggerType string, fromNodeID *string, retryOfRunID *uuid.UUID, attemptNumber int32, contextJSON json.RawMessage) (*models.PipelineRun, error)
	ListBuildQueue(ctx context.Context, query BuildQueueQuery) ([]models.PipelineRun, error)
	AbortPipelineRun(ctx context.Context, runID uuid.UUID) (*models.PipelineRun, bool, error)
	QueueSummary(ctx context.Context) (map[string]int64, error)
	ListDuePipelines(ctx context.Context) ([]models.Pipeline, error)
	UpdatePipelineNextRun(ctx context.Context, pipelineID uuid.UUID, nextRunAt *time.Time) error
}

type ExecutionPorts struct {
	Plans        BuildPlanRepository
	Runs         PipelineRunRepository
	NodeRunner   executor.NodeRunner
	JobRunner    runners.JobRunner
	Python       runtimepkg.TransformExecutor
	Transactions executor.TransactionManager
	Committer    executor.OutputCommitter
	Audit        executor.AuditSink
	Parallelism  int
}

// ConfigGatedOutputCommitter routes Foundry Iceberg outputs to the ADR-0041
// catalog adapter and fails with stable configuration errors when the catalog
// URL is unset or when the URL is present but the transaction-store adapter is
// not wired. Non-Iceberg outputs continue to use the metadata committer.
type ConfigGatedOutputCommitter struct {
	Metadata          executor.OutputCommitter
	Iceberg           executor.OutputCommitter
	CatalogConfigured bool
}

func (c ConfigGatedOutputCommitter) Commit(ctx context.Context, tx executor.OutputTransaction) error {
	if iceberg.Handles(tx.DatasetRID) {
		if c.Iceberg == nil {
			if c.CatalogConfigured {
				return errors.New("foundry_iceberg_catalog_adapter_not_configured: FOUNDRY_ICEBERG_CATALOG_URL is set but the Iceberg transaction store is not wired")
			}
			return errors.New("foundry_iceberg_catalog_not_configured: set FOUNDRY_ICEBERG_CATALOG_URL to commit Iceberg outputs")
		}
		return c.Iceberg.Commit(ctx, tx)
	}
	if c.Metadata == nil {
		return nil
	}
	return c.Metadata.Commit(ctx, tx)
}

// ConfigGatedTransactionManager mirrors ConfigGatedOutputCommitter for aborts
// so failed/cancelled Iceberg nodes roll back through the catalog adapter when
// it is wired and otherwise surface the same stable config error.
type ConfigGatedTransactionManager struct {
	Metadata          executor.TransactionManager
	Iceberg           executor.TransactionManager
	CatalogConfigured bool
}

func (m ConfigGatedTransactionManager) Abort(ctx context.Context, tx executor.OutputTransaction) error {
	if iceberg.Handles(tx.DatasetRID) {
		if m.Iceberg == nil {
			if m.CatalogConfigured {
				return errors.New("foundry_iceberg_catalog_adapter_not_configured: FOUNDRY_ICEBERG_CATALOG_URL is set but the Iceberg transaction store is not wired")
			}
			return errors.New("foundry_iceberg_catalog_not_configured: set FOUNDRY_ICEBERG_CATALOG_URL to abort Iceberg outputs")
		}
		return m.Iceberg.Abort(ctx, tx)
	}
	if m.Metadata == nil {
		return nil
	}
	return m.Metadata.Abort(ctx, tx)
}

type executionSlot struct{ ports ExecutionPorts }

var executionPorts atomic.Value // stores *executionSlot
var executionCancels sync.Map   // stores map[uuid.UUID]context.CancelFunc

// SetExecutionPorts injects executor dependencies for ExecutePipeline and
// TriggerPipelineRun. It returns a restore function for tests.
func SetExecutionPorts(ports ExecutionPorts) func() {
	previous, _ := executionPorts.Load().(*executionSlot)
	executionPorts.Store(&executionSlot{ports: ports})
	return func() { executionPorts.Store(previous) }
}

func currentExecutionPorts() (ExecutionPorts, bool) {
	slot, _ := executionPorts.Load().(*executionSlot)
	if slot == nil {
		return ExecutionPorts{}, false
	}
	return slot.ports, true
}

func requireExecutionPorts(w http.ResponseWriter, detail string) (ExecutionPorts, bool) {
	ports, ok := currentExecutionPorts()
	if !ok {
		writeExecutionPortsUnavailable(w, detail)
		return ExecutionPorts{}, false
	}
	return ports, true
}

func writeExecutionPortsUnavailable(w http.ResponseWriter, detail string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "execution_ports_not_configured", "detail": detail})
}

func registerExecutionCancel(id uuid.UUID, cancel context.CancelFunc) func() {
	if id == uuid.Nil || cancel == nil {
		return func() {}
	}
	executionCancels.Store(id, cancel)
	return func() { executionCancels.Delete(id) }
}

func cancelExecution(id uuid.UUID) bool {
	if id == uuid.Nil {
		return false
	}
	value, ok := executionCancels.Load(id)
	if !ok {
		return false
	}
	if cancel, ok := value.(context.CancelFunc); ok {
		cancel()
		return true
	}
	return false
}

type executePipelineRequest struct {
	BuildID     *uuid.UUID           `json:"build_id,omitempty"`
	BuildBranch string               `json:"build_branch,omitempty"`
	AbortPolicy string               `json:"abort_policy,omitempty"`
	Parallelism int                  `json:"parallelism,omitempty"`
	MaxAttempts int                  `json:"max_attempts,omitempty"`
	Nodes       []executeNodeRequest `json:"nodes,omitempty"`
}

type executeNodeRequest struct {
	ID                 string                       `json:"id"`
	JobID              *uuid.UUID                   `json:"job_id,omitempty"`
	DependsOn          []string                     `json:"depends_on,omitempty"`
	Outputs            []executor.OutputTransaction `json:"outputs,omitempty"`
	LogicKind          string                       `json:"logic_kind,omitempty"`
	TransformType      string                       `json:"transform_type,omitempty"`
	LogicPayload       json.RawMessage              `json:"logic_payload,omitempty"`
	InputDatasetIDs    []string                     `json:"input_dataset_ids,omitempty"`
	OutputDatasetID    string                       `json:"output_dataset_id,omitempty"`
	ResolvedInputViews []models.ResolvedInputView   `json:"resolved_input_views,omitempty"`
	Metadata           map[string]any               `json:"metadata,omitempty"`
}

type executePipelineResponse struct {
	BuildID   uuid.UUID                     `json:"build_id"`
	State     string                        `json:"state"`
	Completed int                           `json:"completed"`
	Failed    int                           `json:"failed"`
	Aborted   int                           `json:"aborted"`
	Attempts  map[string]int                `json:"attempts"`
	Nodes     map[string]executor.NodeState `json:"nodes"`
	Reasons   map[string]string             `json:"reasons,omitempty"`
}

// ExecutePipeline builds an executor.Plan from inline JSON or persisted build
// state, runs the DAG executor and returns the observable Rust-compatible build
// terminal envelope.
func ExecutePipeline(w http.ResponseWriter, r *http.Request) {
	ports, ok := requireExecutionPorts(w, "ExecutePipeline requires executor ports")
	if !ok {
		return
	}
	var body executePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	plan, err := planFromExecuteRequest(r.Context(), body, ports)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	runner := ports.NodeRunner
	if runner == nil {
		runner = runtimeNodeRunner{JobRunner: ports.JobRunner, Python: ports.Python}
	}
	execCtx, cancel := context.WithCancel(r.Context())
	unregister := registerExecutionCancel(plan.BuildID, cancel)
	defer unregister()
	outcome, err := executor.Execute(execCtx, plan, runner, ports.Transactions, ports.Committer, ports.Audit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, executePipelineResponse{BuildID: plan.BuildID, State: string(outcome.FinalState), Completed: outcome.Completed, Failed: outcome.Failed, Aborted: outcome.Aborted, Attempts: outcome.Attempts, Nodes: outcome.Nodes, Reasons: outcome.Reasons})
}

// TriggerPipelineRun mirrors Rust trigger_run for the supported Go path: open a
// pipeline_run, convert the pipeline DAG into an executor.Plan, run it, persist
// terminal status through hooks, and return the created run envelope.
func TriggerPipelineRun(w http.ResponseWriter, r *http.Request) {
	ports, ok := requireExecutionPorts(w, "TriggerPipelineRun requires pipeline run repository and executor ports")
	if !ok {
		return
	}
	if ports.Runs == nil {
		writeExecutionPortsUnavailable(w, "TriggerPipelineRun requires pipeline run repository and executor ports")
		return
	}
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	var body models.TriggerPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	pipeline, err := ports.Runs.LoadPipeline(r.Context(), pipelineID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pipeline == nil {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	contextJSON := body.Context
	if len(contextJSON) == 0 {
		contextJSON, _ = json.Marshal(map[string]any{"trigger": map[string]any{"type": "manual", "started_at": time.Now().UTC()}})
	}
	var startedBy *uuid.UUID
	if user, ok := authmw.AuthUserFromContext(r.Context()); ok && user.Claims != nil {
		id := user.Claims.Sub
		startedBy = &id
	}
	run, err := ports.Runs.OpenPipelineRun(r.Context(), pipeline, body, startedBy, contextJSON)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	plan, err := planFromPipeline(pipeline, run.ID, body, ports)
	if err != nil {
		finishRunBestEffort(r.Context(), ports.Runs, run.ID, "failed", nil, err.Error())
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	runner := ports.NodeRunner
	if runner == nil {
		runner = runtimeNodeRunner{JobRunner: ports.JobRunner, Python: ports.Python}
	}
	execCtx, cancel := context.WithCancel(r.Context())
	unregister := registerExecutionCancel(plan.BuildID, cancel)
	defer unregister()
	outcome, err := executor.Execute(execCtx, plan, runner, ports.Transactions, ports.Committer, ports.Audit)
	if err != nil {
		finishRunBestEffort(r.Context(), ports.Runs, run.ID, "failed", nil, err.Error())
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	status, errMsg := pipelineRunStatus(outcome)
	nodeResults, _ := json.Marshal(outcome.Nodes)
	if err := ports.Runs.FinishPipelineRun(r.Context(), run.ID, status, nodeResults, errMsg); err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func planFromExecuteRequest(ctx context.Context, body executePipelineRequest, ports ExecutionPorts) (executor.Plan, error) {
	if len(body.Nodes) == 0 {
		if body.BuildID == nil {
			return executor.Plan{}, errors.New("either build_id or nodes is required")
		}
		if ports.Plans == nil {
			return executor.Plan{}, errors.New("BuildPlanRepository is not configured")
		}
		plan, err := ports.Plans.LoadPlan(ctx, *body.BuildID)
		if err != nil {
			return executor.Plan{}, err
		}
		return normalizePlan(plan, ports), nil
	}
	buildID := uuid.New()
	if body.BuildID != nil {
		buildID = *body.BuildID
	}
	plan := executor.Plan{BuildID: buildID, BuildBranch: body.BuildBranch, AbortPolicy: executor.AbortPolicy(body.AbortPolicy), Parallelism: body.Parallelism, MaxAttempts: body.MaxAttempts, Nodes: make([]executor.Node, 0, len(body.Nodes))}
	for _, node := range body.Nodes {
		plan.Nodes = append(plan.Nodes, executorNodeFromRequest(buildID, node))
	}
	return normalizePlan(plan, ports), nil
}

func normalizePlan(plan executor.Plan, ports ExecutionPorts) executor.Plan {
	if plan.Parallelism < 1 {
		plan.Parallelism = ports.Parallelism
	}
	if plan.Parallelism < 1 {
		plan.Parallelism = executor.DefaultParallelism
	}
	if plan.MaxAttempts < 1 {
		plan.MaxAttempts = 1
	}
	if plan.AbortPolicy == "" {
		plan.AbortPolicy = executor.AbortDependentOnly
	}
	return plan
}

func executorNodeFromRequest(buildID uuid.UUID, node executeNodeRequest) executor.Node {
	jobID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(buildID.String()+":"+node.ID))
	if node.JobID != nil {
		jobID = *node.JobID
	}
	metadata := map[string]any{}
	for k, v := range node.Metadata {
		metadata[k] = v
	}
	if node.LogicKind != "" {
		metadata["logic_kind"] = node.LogicKind
	}
	if node.TransformType != "" {
		metadata["transform_type"] = node.TransformType
	}
	if len(node.LogicPayload) > 0 {
		metadata["logic_payload"] = json.RawMessage(node.LogicPayload)
	}
	if len(node.InputDatasetIDs) > 0 {
		metadata["input_dataset_ids"] = node.InputDatasetIDs
	}
	if node.OutputDatasetID != "" {
		metadata["output_dataset_id"] = node.OutputDatasetID
	}
	return executor.Node{ID: node.ID, JobID: jobID, DependsOn: node.DependsOn, Outputs: node.Outputs, Metadata: metadata, ResolvedInputViews: node.ResolvedInputViews}
}

func planFromPipeline(pipeline *models.Pipeline, runID uuid.UUID, req models.TriggerPipelineRequest, ports ExecutionPorts) (executor.Plan, error) {
	nodes, err := pipeline.ParsedNodes()
	if err != nil {
		return executor.Plan{}, err
	}
	if len(nodes) == 0 {
		return executor.Plan{}, errors.New("pipeline must define at least one node")
	}
	reachable := map[string]struct{}{}
	if req.FromNodeID != nil && strings.TrimSpace(*req.FromNodeID) != "" {
		var ok bool
		reachable, ok = reachablePipelineNodes(nodes, *req.FromNodeID)
		if !ok {
			return executor.Plan{}, fmt.Errorf("start node '%s' not found", *req.FromNodeID)
		}
	}
	plan := executor.Plan{BuildID: runID, BuildBranch: "master", AbortPolicy: executor.AbortDependentOnly, Parallelism: ports.Parallelism, MaxAttempts: int(pipeline.ParsedRetryPolicy().MaxAttempts), Nodes: make([]executor.Node, 0, len(nodes))}
	for _, node := range nodes {
		if len(reachable) > 0 {
			if _, ok := reachable[node.ID]; !ok {
				continue
			}
		}
		outputs := []executor.OutputTransaction{}
		outputRID := ""
		if node.OutputDatasetID != nil {
			outputRID = node.OutputDatasetID.String()
			outputs = append(outputs, executor.OutputTransaction{DatasetRID: outputRID, TransactionRID: "pipeline-run:" + runID.String() + ":" + node.ID})
		}
		metadata := map[string]any{
			"logic_kind":        runners.LogicKindTransform,
			"transform_type":    node.TransformType,
			"logic_payload":     json.RawMessage(node.Config),
			"label":             node.Label,
			"input_dataset_ids": uuidStrings(node.InputDatasetIDs),
			"output_dataset_id": outputRID,
		}
		deps := node.DependsOn
		if len(reachable) > 0 {
			deps = filterDeps(deps, reachable)
		}
		plan.Nodes = append(plan.Nodes, executor.Node{ID: node.ID, JobID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID.String()+":"+node.ID)), DependsOn: deps, Outputs: outputs, Metadata: metadata})
	}
	return normalizePlan(plan, ports), nil
}

type runtimeNodeRunner struct {
	JobRunner runners.JobRunner
	Python    runtimepkg.TransformExecutor
}

func (r runtimeNodeRunner) Run(ctx context.Context, node executor.NodeContext) (executor.NodeResult, error) {
	logicKind := metadataString(node.Node.Metadata, "logic_kind")
	if logicKind == "" {
		logicKind = runners.LogicKindTransform
	}
	transformType := metadataString(node.Node.Metadata, "transform_type")
	payload := metadataRaw(node.Node.Metadata, "logic_payload")
	if transformType == "python" {
		if r.Python == nil {
			return executor.NodeResult{}, errors.New("python_sidecar_not_configured: set PYTHON_SIDECAR_BINARY to execute Python transforms")
		}
		return r.runPython(ctx, node, payload)
	}
	if r.JobRunner == nil {
		return executor.NodeResult{}, fmt.Errorf("runner_not_wired:%s", strings.ToLower(logicKind))
	}
	outcome := r.JobRunner.Run(ctx, &runners.JobContext{BuildID: node.BuildID, BuildBranch: node.BuildBranch, JobID: node.Node.JobID, JobSpec: runners.JobSpec{JobSpecRID: node.Node.ID, LogicKind: logicKind, OutputDatasetRIDs: outputDatasetRIDs(node.Node.Outputs), Config: payload}})
	if outcome.Kind == runners.JobOutcomeFailed {
		return executor.NodeResult{}, errors.New(outcome.Reason)
	}
	return executor.NodeResult{OutputContentHash: outcome.OutputContentHash}, nil
}

func (r runtimeNodeRunner) runPython(ctx context.Context, node executor.NodeContext, payload json.RawMessage) (executor.NodeResult, error) {
	// Go -> sidecar contract for pipeline nodes:
	//   - source: logic_payload.source or logic_payload.code (required by runtime executor)
	//   - config_json: logic_payload.config when present, otherwise the complete logic_payload object
	//   - prepared_inputs_json: logic_payload.prepared_inputs or [] until dataset materialization is wired
	//   - input_dataset_ids/output_dataset_id: copied from node metadata/outputs
	//   - timeout_seconds: logic_payload.timeout_seconds or the sidecar executor default
	var cfg map[string]json.RawMessage
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &cfg)
	}
	source := firstString(cfg, "source", "code")
	configJSON := cfg["config"]
	if len(configJSON) == 0 && len(payload) > 0 {
		configJSON = payload
	}
	preparedInputsJSON := cfg["prepared_inputs"]
	if len(preparedInputsJSON) == 0 {
		preparedInputsJSON = []byte("[]")
	}
	inputIDs := metadataStringSlice(node.Node.Metadata, "input_dataset_ids")
	outputID := metadataString(node.Node.Metadata, "output_dataset_id")
	if outputID == "" && len(node.Node.Outputs) > 0 {
		outputID = node.Node.Outputs[0].DatasetRID
	}
	result, err := r.Python.ExecutePythonTransform(ctx, runtimepkg.TransformRequest{Source: source, ConfigJSON: configJSON, PreparedInputsJSON: preparedInputsJSON, InputDatasetIDs: inputIDs, OutputDatasetID: outputID, TimeoutSeconds: firstUint32(cfg, "timeout_seconds")})
	if err != nil {
		return executor.NodeResult{}, err
	}
	hashInput := append([]byte(source), result.Output...)
	hash := sha256.Sum256(hashInput)
	meta := map[string]any{"runtime": "python"}
	if result.RowsAffected != nil {
		meta["rows_affected"] = *result.RowsAffected
	}
	return executor.NodeResult{OutputContentHash: "sha256:" + hex.EncodeToString(hash[:]), Metadata: meta}, nil
}

func pipelineIDFromRequest(r *http.Request) (uuid.UUID, error) {
	for _, key := range []string{"id", "pipeline_id"} {
		if raw := chi.URLParam(r, key); raw != "" {
			return uuid.Parse(raw)
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "pipelines" && i+1 < len(parts) {
			return uuid.Parse(parts[i+1])
		}
	}
	return uuid.Nil, errors.New("pipeline id is required")
}

func pipelineRunStatus(outcome executor.Outcome) (string, *string) {
	switch outcome.FinalState {
	case models.BuildCompleted:
		return "completed", nil
	case models.BuildAborted:
		msg := "aborted"
		return "aborted", &msg
	default:
		msg := "failed"
		return "failed", &msg
	}
}

func finishRunBestEffort(ctx context.Context, repo PipelineRunRepository, runID uuid.UUID, status string, results json.RawMessage, errMsg string) {
	msg := errMsg
	_ = repo.FinishPipelineRun(ctx, runID, status, results, &msg)
}

func reachablePipelineNodes(nodes []models.PipelineNode, start string) (map[string]struct{}, bool) {
	adj := map[string][]string{}
	found := false
	for _, node := range nodes {
		if node.ID == start {
			found = true
		}
		for _, dep := range node.DependsOn {
			adj[dep] = append(adj[dep], node.ID)
		}
	}
	if !found {
		return nil, false
	}
	seen := map[string]struct{}{}
	stack := []string{start}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		stack = append(stack, adj[id]...)
	}
	return seen, true
}

func filterDeps(deps []string, reachable map[string]struct{}) []string {
	out := make([]string, 0, len(deps))
	for _, dep := range deps {
		if _, ok := reachable[dep]; ok {
			out = append(out, dep)
		}
	}
	return out
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func outputDatasetRIDs(outputs []executor.OutputTransaction) []string {
	out := make([]string, len(outputs))
	for i, output := range outputs {
		out[i] = output.DatasetRID
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	v, ok := metadata[key]
	if !ok || v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	case json.RawMessage:
		var s string
		_ = json.Unmarshal(typed, &s)
		return s
	default:
		return fmt.Sprint(typed)
	}
}

func metadataRaw(metadata map[string]any, key string) json.RawMessage {
	v, ok := metadata[key]
	if !ok || v == nil {
		return nil
	}
	switch typed := v.(type) {
	case json.RawMessage:
		return typed
	case []byte:
		return json.RawMessage(typed)
	default:
		out, _ := json.Marshal(typed)
		return out
	}
}

func metadataStringSlice(metadata map[string]any, key string) []string {
	v, ok := metadata[key]
	if !ok || v == nil {
		return nil
	}
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case json.RawMessage:
		var out []string
		_ = json.Unmarshal(typed, &out)
		return out
	default:
		return nil
	}
}

func firstString(cfg map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var s string
		if len(cfg[key]) > 0 && json.Unmarshal(cfg[key], &s) == nil {
			return s
		}
	}
	return ""
}

func firstUint32(cfg map[string]json.RawMessage, keys ...string) uint32 {
	for _, key := range keys {
		var n uint32
		if len(cfg[key]) > 0 && json.Unmarshal(cfg[key], &n) == nil {
			return n
		}
	}
	return 0
}
