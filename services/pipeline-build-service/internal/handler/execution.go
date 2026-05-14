package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
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
	MarkPipelineRunRunning(ctx context.Context, runID uuid.UUID) error
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
	LLM          LLMTransformRunner
	AIP          PipelineAIPGenerator
	Functions    PipelineFunctionRegistry
	Distributed  DistributedTransformRunner
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

func (c ConfigGatedOutputCommitter) Commit(ctx context.Context, tx executor.OutputTransaction, result executor.NodeResult) error {
	if iceberg.Handles(tx.DatasetRID) {
		if c.Iceberg == nil {
			if c.CatalogConfigured {
				return errors.New("foundry_iceberg_catalog_adapter_not_configured: FOUNDRY_ICEBERG_CATALOG_URL is set but the Iceberg transaction store is not wired")
			}
			return errors.New("foundry_iceberg_catalog_not_configured: set FOUNDRY_ICEBERG_CATALOG_URL to commit Iceberg outputs")
		}
		return c.Iceberg.Commit(ctx, tx, result)
	}
	if c.Metadata == nil {
		return nil
	}
	return c.Metadata.Commit(ctx, tx, result)
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
	Ignored   int                           `json:"ignored"`
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
		runner = newRuntimeNodeRunner(ports)
	}
	execCtx, cancel := context.WithCancel(r.Context())
	unregister := registerExecutionCancel(plan.BuildID, cancel)
	defer unregister()
	audit := newCapturingAuditSink(ports.Audit)
	outcome, err := executor.Execute(execCtx, plan, runner, ports.Transactions, ports.Committer, audit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, executePipelineResponse{BuildID: plan.BuildID, State: string(outcome.FinalState), Completed: outcome.Completed, Ignored: outcome.Ignored, Failed: outcome.Failed, Aborted: outcome.Aborted, Attempts: outcome.Attempts, Nodes: outcome.Nodes, Reasons: outcome.Reasons})
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
	validationFailure, err := validationFailureForRuntimePipeline(pipeline)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_graph", "detail": err.Error()})
		return
	}
	if validationFailure != nil {
		writePipelineSchemaValidationFailure(w, validationFailure.Report)
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
	plan, err := planFromPipeline(r.Context(), pipeline, run.ID, body, ports)
	if err != nil {
		finishRunBestEffort(r.Context(), ports.Runs, run.ID, "failed", nil, err.Error())
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ports.Runs.MarkPipelineRunRunning(r.Context(), run.ID); err != nil {
		finishRunBestEffort(r.Context(), ports.Runs, run.ID, "failed", nil, err.Error())
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
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
		finishRunBestEffort(r.Context(), ports.Runs, run.ID, "failed", nil, err.Error())
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	status, errMsg := pipelineRunStatus(outcome)
	nodeResults, _ := json.Marshal(pipelineRunNodeResults(plan, outcome, audit.Events()))
	if err := ports.Runs.FinishPipelineRun(r.Context(), run.ID, status, nodeResults, errMsg); err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	run.Status = status
	run.NodeResults = nodeResults
	run.ErrorMessage = errMsg
	run.FinishedAt = &now
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

func planFromPipeline(ctx context.Context, pipeline *models.Pipeline, runID uuid.UUID, req models.TriggerPipelineRequest, ports ExecutionPorts) (executor.Plan, error) {
	nodes, err := pipeline.RuntimeNodes()
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
	pipelineType := models.NormalizePipelineType(pipeline.PipelineType)
	plan := executor.Plan{BuildID: runID, BuildBranch: "master", AbortPolicy: executor.AbortDependentOnly, Parallelism: ports.Parallelism, MaxAttempts: int(pipeline.ParsedRetryPolicy().MaxAttempts), Nodes: make([]executor.Node, 0, len(nodes))}
	for _, node := range nodes {
		if len(reachable) > 0 {
			if _, ok := reachable[node.ID]; !ok {
				continue
			}
		}
		outputs, outputRID := outputTransactionsForPipelineNode(pipeline.ID, runID, node, nodes)
		metadata := map[string]any{
			"logic_kind":        runners.LogicKindTransform,
			"transform_type":    node.TransformType,
			"logic_payload":     json.RawMessage(node.Config),
			"label":             node.Label,
			"input_dataset_ids": uuidStrings(node.InputDatasetIDs),
			"output_dataset_id": outputRID,
			"pipeline_type":     pipelineType,
		}
		if models.IsLightweightPipelineType(pipelineType) {
			metadata["execution_mode"] = "lightweight"
			metadata["preferred_runtime"] = "lightweight_table"
		}
		if pipelineType == models.PipelineTypeDistributed {
			engine := distributedEngineFromConfig(pipeline.DistributedConfig)
			metadata["execution_mode"] = "distributed"
			metadata["preferred_runtime"] = "distributed"
			metadata["distributed_engine"] = engine
		}
		deps := node.DependsOn
		if len(reachable) > 0 {
			deps = filterDeps(deps, reachable)
		}
		plan.Nodes = append(plan.Nodes, executor.Node{ID: node.ID, JobID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID.String()+":"+node.ID)), DependsOn: deps, Outputs: outputs, Metadata: metadata})
	}
	plan = normalizePlan(plan, ports)
	applyPipelineStaleness(ctx, pipeline.ID, runID, &plan, req.SkipUnchanged, ports.Runs)
	return plan, nil
}

type pipelineRunHistoryLister interface {
	ListPipelineRuns(ctx context.Context, pipelineID uuid.UUID, page, perPage int64) ([]models.PipelineRun, error)
}

func applyPipelineStaleness(ctx context.Context, pipelineID, currentRunID uuid.UUID, plan *executor.Plan, skipUnchanged bool, runs any) {
	if plan == nil || len(plan.Nodes) == 0 {
		return
	}
	current := pipelineStalenessSignatures(plan.Nodes)
	for i := range plan.Nodes {
		if plan.Nodes[i].Metadata == nil {
			plan.Nodes[i].Metadata = map[string]any{}
		}
		if sig := current[plan.Nodes[i].ID]; sig != "" {
			plan.Nodes[i].Metadata["staleness_signature"] = sig
		}
	}
	if !skipUnchanged {
		return
	}
	history, ok := runs.(pipelineRunHistoryLister)
	if !ok || history == nil {
		return
	}
	previous := previousPipelineRunSignatures(ctx, history, pipelineID, currentRunID)
	if len(previous) == 0 {
		return
	}
	fresh := map[string]bool{}
	for _, node := range plan.Nodes {
		fresh[node.ID] = previous[node.ID] != "" && previous[node.ID] == current[node.ID]
	}
	changed := true
	for changed {
		changed = false
		for _, node := range plan.Nodes {
			if !fresh[node.ID] {
				continue
			}
			for _, dep := range node.DependsOn {
				if !fresh[dep] {
					fresh[node.ID] = false
					changed = true
					break
				}
			}
		}
	}
	for i := range plan.Nodes {
		if fresh[plan.Nodes[i].ID] {
			plan.Nodes[i].StaleSkipped = true
		}
	}
}

func pipelineStalenessSignatures(nodes []executor.Node) map[string]string {
	byID := map[string]executor.Node{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	memo := map[string]string{}
	var sign func(string) string
	sign = func(nodeID string) string {
		if value, ok := memo[nodeID]; ok {
			return value
		}
		node, ok := byID[nodeID]
		if !ok {
			return ""
		}
		depSigs := map[string]string{}
		for _, dep := range node.DependsOn {
			depSigs[dep] = sign(dep)
		}
		payload := map[string]any{
			"id":                node.ID,
			"depends_on":        sortedUniqueStrings(node.DependsOn),
			"dependency_hashes": depSigs,
			"logic_kind":        metadataString(node.Metadata, "logic_kind"),
			"transform_type":    metadataString(node.Metadata, "transform_type"),
			"logic_payload":     metadataRaw(node.Metadata, "logic_payload"),
			"input_dataset_ids": metadataStringSlice(node.Metadata, "input_dataset_ids"),
			"output_dataset_id": metadataString(node.Metadata, "output_dataset_id"),
		}
		raw, _ := json.Marshal(payload)
		sum := sha256.Sum256(raw)
		value := hex.EncodeToString(sum[:])
		memo[nodeID] = value
		return value
	}
	for _, node := range nodes {
		sign(node.ID)
	}
	return memo
}

func previousPipelineRunSignatures(ctx context.Context, repo pipelineRunHistoryLister, pipelineID, currentRunID uuid.UUID) map[string]string {
	runs, err := repo.ListPipelineRuns(ctx, pipelineID, 1, 25)
	if err != nil {
		return nil
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	for _, run := range runs {
		if run.ID == currentRunID {
			continue
		}
		if !runStatusCarriesFreshness(run.Status) {
			continue
		}
		signatures := signaturesFromPipelineNodeResults(run.NodeResults)
		if len(signatures) > 0 {
			return signatures
		}
	}
	return nil
}

func runStatusCarriesFreshness(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed", "ignored", strings.ToLower(string(models.BuildCompleted)):
		return true
	default:
		return false
	}
}

func signaturesFromPipelineNodeResults(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var results []models.PipelineNodeResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil
	}
	out := map[string]string{}
	for _, result := range results {
		if result.Output == nil {
			continue
		}
		if sig, ok := result.Output["staleness_signature"].(string); ok && strings.TrimSpace(sig) != "" {
			out[result.NodeID] = strings.TrimSpace(sig)
		}
	}
	return out
}

type outputDatasetConfig struct {
	Kind                string                 `json:"kind,omitempty"`
	Name                string                 `json:"name,omitempty"`
	DatasetName         string                 `json:"dataset_name,omitempty"`
	DisplayName         string                 `json:"display_name,omitempty"`
	DatasetID           string                 `json:"dataset_id,omitempty"`
	DatasetRID          string                 `json:"dataset_rid,omitempty"`
	OutputDatasetRID    string                 `json:"output_dataset_rid,omitempty"`
	Branch              string                 `json:"branch,omitempty"`
	WriteMode           string                 `json:"write_mode,omitempty"`
	FileFormat          string                 `json:"file_format,omitempty"`
	LogicalPath         string                 `json:"logical_path,omitempty"`
	ObjectTypeID        string                 `json:"object_type_id,omitempty"`
	ObjectTypeRID       string                 `json:"object_type_rid,omitempty"`
	ObjectTypeName      string                 `json:"object_type_name,omitempty"`
	PluralName          string                 `json:"plural_name,omitempty"`
	PluralDisplay       string                 `json:"plural_display_name,omitempty"`
	PrimaryKey          string                 `json:"primary_key,omitempty"`
	PrimaryKeys         []string               `json:"primary_keys,omitempty"`
	Icon                string                 `json:"icon,omitempty"`
	Color               string                 `json:"color,omitempty"`
	AllowEdits          bool                   `json:"allow_edits,omitempty"`
	Editable            bool                   `json:"editable,omitempty"`
	PropertyMapping     []outputPropertyConfig `json:"property_mapping,omitempty"`
	Properties          []outputPropertyConfig `json:"properties,omitempty"`
	LinkTypeID          string                 `json:"link_type_id,omitempty"`
	LinkTypeRID         string                 `json:"link_type_rid,omitempty"`
	LinkTypeName        string                 `json:"link_type_name,omitempty"`
	LinkDisplayName     string                 `json:"link_display_name,omitempty"`
	LinkDescription     string                 `json:"link_description,omitempty"`
	Cardinality         string                 `json:"cardinality,omitempty"`
	SourceObjectTypeID  string                 `json:"source_object_type_id,omitempty"`
	SourceObjectTypeRID string                 `json:"source_object_type_rid,omitempty"`
	SourceObjectNodeID  string                 `json:"source_object_node_id,omitempty"`
	SourcePrimaryKey    string                 `json:"source_primary_key,omitempty"`
	SourceKeyColumn     string                 `json:"source_key_column,omitempty"`
	TargetObjectTypeID  string                 `json:"target_object_type_id,omitempty"`
	TargetObjectTypeRID string                 `json:"target_object_type_rid,omitempty"`
	TargetObjectNodeID  string                 `json:"target_object_node_id,omitempty"`
	TargetPrimaryKey    string                 `json:"target_primary_key,omitempty"`
	TargetKeyColumn     string                 `json:"target_key_column,omitempty"`
	ForeignKeyColumn    string                 `json:"foreign_key_column,omitempty"`
	Tenant              string                 `json:"tenant,omitempty"`
	VirtualTableRID     string                 `json:"virtual_table_rid,omitempty"`
	SourceRID           string                 `json:"source_rid,omitempty"`
	Provider            string                 `json:"provider,omitempty"`
	TableType           string                 `json:"table_type,omitempty"`
	Locator             json.RawMessage        `json:"locator,omitempty"`
	ExternalReference   json.RawMessage        `json:"external_reference,omitempty"`
	Orchestration       string                 `json:"orchestration,omitempty"`
	Storage             string                 `json:"storage,omitempty"`
}

type outputPropertyConfig struct {
	SourceField      string `json:"source_field,omitempty"`
	TargetProperty   string `json:"target_property,omitempty"`
	Name             string `json:"name,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	PropertyType     string `json:"property_type,omitempty"`
	Type             string `json:"type,omitempty"`
	Required         bool   `json:"required,omitempty"`
	UniqueConstraint bool   `json:"unique_constraint,omitempty"`
}

func outputTransactionsForPipelineNode(pipelineID uuid.UUID, runID uuid.UUID, node models.PipelineNode, nodes []models.PipelineNode) ([]executor.OutputTransaction, string) {
	cfg := parseOutputDatasetConfig(node.Config)
	if node.OutputDatasetID == nil && normaliseTableTransform(node.TransformType) != "output" {
		return nil, ""
	}
	if isLogicalViewOutputKind(outputKindForNode(node, cfg)) {
		return nil, ""
	}
	outputRID := outputDatasetRIDForPipelineNode(pipelineID, node, cfg)
	if outputRID == "" {
		return nil, ""
	}
	sourceNodeID := ""
	if len(node.DependsOn) > 0 {
		sourceNodeID = node.DependsOn[0]
	}
	tx := executor.OutputTransaction{
		DatasetRID:                outputRID,
		TransactionRID:            "pipeline-run:" + runID.String() + ":" + node.ID,
		DatasetName:               firstNonEmpty(cfg.DatasetName, cfg.DisplayName, cfg.Name, node.Label),
		Branch:                    firstNonEmpty(cfg.Branch, "main"),
		WriteMode:                 firstNonEmpty(cfg.WriteMode, "SNAPSHOT"),
		FileFormat:                firstNonEmpty(cfg.FileFormat, "PARQUET"),
		LogicalPath:               cfg.LogicalPath,
		OutputKind:                outputKindForNode(node, cfg),
		OutputNodeID:              node.ID,
		SourceNodeID:              sourceNodeID,
		PipelineRID:               pipelineID.String(),
		InputDatasetRIDs:          upstreamInputDatasetRIDs(nodes, node.ID),
		CreateIfMissing:           true,
		ObjectTypeID:              outputObjectTypeIDForPipelineNode(pipelineID, node, cfg),
		ObjectTypeName:            firstNonEmpty(cfg.ObjectTypeName, cfg.Name, cfg.DisplayName, node.Label),
		ObjectTypeDisplayName:     firstNonEmpty(cfg.DisplayName, cfg.ObjectTypeName, cfg.Name, node.Label),
		ObjectTypePluralName:      firstNonEmpty(cfg.PluralDisplay, cfg.PluralName),
		ObjectTypePrimaryKey:      outputPrimaryKeyFromConfig(cfg),
		ObjectTypeIcon:            cfg.Icon,
		ObjectTypeColor:           cfg.Color,
		ObjectTypeEditable:        cfg.AllowEdits || cfg.Editable,
		ObjectPropertyMappings:    outputPropertyMappingsFromConfig(cfg),
		LinkTypeID:                outputLinkTypeIDForPipelineNode(pipelineID, node, cfg),
		LinkTypeName:              firstNonEmpty(cfg.LinkTypeName, cfg.Name, cfg.DisplayName, node.Label),
		LinkTypeDisplayName:       firstNonEmpty(cfg.LinkDisplayName, cfg.DisplayName, cfg.LinkTypeName, cfg.Name, node.Label),
		LinkTypeDescription:       cfg.LinkDescription,
		LinkTypeCardinality:       normaliseLinkCardinality(cfg.Cardinality),
		LinkSourceObjectTypeID:    outputLinkObjectTypeIDForPipelineNode(pipelineID, nodes, cfg, true),
		LinkTargetObjectTypeID:    outputLinkObjectTypeIDForPipelineNode(pipelineID, nodes, cfg, false),
		LinkSourceObjectNodeID:    firstNonEmpty(cfg.SourceObjectNodeID),
		LinkTargetObjectNodeID:    firstNonEmpty(cfg.TargetObjectNodeID),
		LinkSourcePrimaryKey:      outputLinkPrimaryKeyForPipelineNode(nodes, cfg, true),
		LinkTargetPrimaryKey:      outputLinkPrimaryKeyForPipelineNode(nodes, cfg, false),
		LinkSourceKeyColumn:       outputLinkSourceKeyColumn(cfg),
		LinkTargetKeyColumn:       outputLinkTargetKeyColumn(cfg),
		LinkTenant:                firstNonEmpty(cfg.Tenant, "default"),
		VirtualTableRID:           firstNonEmpty(cfg.VirtualTableRID, outputRID),
		VirtualTableSourceRID:     cfg.SourceRID,
		VirtualTableProvider:      cfg.Provider,
		VirtualTableType:          cfg.TableType,
		VirtualTableLocator:       string(cfg.Locator),
		VirtualTableReference:     string(cfg.ExternalReference),
		VirtualTableStorage:       firstNonEmpty(cfg.Storage, "external"),
		VirtualTableOrchestration: firstNonEmpty(cfg.Orchestration, "openfoundry"),
	}
	return []executor.OutputTransaction{tx}, outputRID
}

func outputKindForNode(node models.PipelineNode, cfg outputDatasetConfig) string {
	if strings.TrimSpace(cfg.Kind) != "" {
		return strings.ToLower(strings.TrimSpace(cfg.Kind))
	}
	if strings.Contains(strings.ToLower(node.TransformType), "virtual_table") {
		return "virtual_table"
	}
	if strings.Contains(strings.ToLower(node.TransformType), "link") {
		return "link_type"
	}
	if strings.Contains(strings.ToLower(node.TransformType), "object") || strings.Contains(strings.ToLower(node.TransformType), "ontology") {
		return "object_type"
	}
	return "dataset"
}

func parseOutputDatasetConfig(raw json.RawMessage) outputDatasetConfig {
	if len(raw) == 0 {
		return outputDatasetConfig{}
	}
	var nested struct {
		Output outputDatasetConfig `json:"_output"`
	}
	if json.Unmarshal(raw, &nested) == nil && !nested.Output.empty() {
		return nested.Output
	}
	var direct outputDatasetConfig
	_ = json.Unmarshal(raw, &direct)
	return direct
}

func (c outputDatasetConfig) empty() bool {
	return strings.TrimSpace(c.Kind+c.Name+c.DatasetName+c.DisplayName+c.DatasetID+c.DatasetRID+c.OutputDatasetRID+c.Branch+c.WriteMode+c.FileFormat+c.LogicalPath+c.ObjectTypeID+c.ObjectTypeRID+c.ObjectTypeName+c.PrimaryKey+c.Icon+c.Color+c.LinkTypeID+c.LinkTypeRID+c.LinkTypeName+c.LinkDisplayName+c.Cardinality+c.SourceObjectTypeID+c.SourceObjectTypeRID+c.SourceObjectNodeID+c.TargetObjectTypeID+c.TargetObjectTypeRID+c.TargetObjectNodeID+c.SourceKeyColumn+c.TargetKeyColumn+c.ForeignKeyColumn+c.VirtualTableRID+c.SourceRID+c.Provider+c.TableType+c.Orchestration+c.Storage) == "" && len(c.PrimaryKeys) == 0 && len(c.PropertyMapping) == 0 && len(c.Properties) == 0 && len(c.Locator) == 0 && len(c.ExternalReference) == 0
}

func outputDatasetRIDForPipelineNode(pipelineID uuid.UUID, node models.PipelineNode, cfg outputDatasetConfig) string {
	if isLogicalViewOutputKind(outputKindForNode(node, cfg)) {
		return ""
	}
	if outputKindForNode(node, cfg) == "virtual_table" {
		if strings.TrimSpace(cfg.VirtualTableRID) != "" {
			return strings.TrimSpace(cfg.VirtualTableRID)
		}
		return "ri.foundry.main.virtual-table." + uuid.NewSHA1(uuid.NameSpaceOID, []byte(pipelineID.String()+":"+node.ID+":virtual-table-output")).String()
	}
	if strings.TrimSpace(cfg.DatasetRID) != "" {
		return strings.TrimSpace(cfg.DatasetRID)
	}
	if strings.TrimSpace(cfg.OutputDatasetRID) != "" {
		return strings.TrimSpace(cfg.OutputDatasetRID)
	}
	if strings.TrimSpace(cfg.DatasetID) != "" {
		return strings.TrimSpace(cfg.DatasetID)
	}
	if node.OutputDatasetID != nil {
		return node.OutputDatasetID.String()
	}
	if normaliseTableTransform(node.TransformType) == "output" {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(pipelineID.String()+":"+node.ID+":dataset-output")).String()
	}
	return ""
}

func outputObjectTypeIDForPipelineNode(pipelineID uuid.UUID, node models.PipelineNode, cfg outputDatasetConfig) string {
	for _, value := range []string{cfg.ObjectTypeID, strings.TrimPrefix(cfg.ObjectTypeRID, "ri.ontology.main.object-type.")} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if outputKindForNode(node, cfg) == "object_type" {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(pipelineID.String()+":"+node.ID+":object-type")).String()
	}
	return ""
}

func outputLinkTypeIDForPipelineNode(pipelineID uuid.UUID, node models.PipelineNode, cfg outputDatasetConfig) string {
	for _, value := range []string{cfg.LinkTypeID, strings.TrimPrefix(cfg.LinkTypeRID, "ri.ontology.main.link-type.")} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if outputKindForNode(node, cfg) == "link_type" {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(pipelineID.String()+":"+node.ID+":link-type")).String()
	}
	return ""
}

func outputPrimaryKeyFromConfig(cfg outputDatasetConfig) string {
	if strings.TrimSpace(cfg.PrimaryKey) != "" {
		return strings.TrimSpace(cfg.PrimaryKey)
	}
	for _, key := range cfg.PrimaryKeys {
		if strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key)
		}
	}
	return ""
}

func outputPropertyMappingsFromConfig(cfg outputDatasetConfig) []executor.OutputPropertyMapping {
	props := cfg.PropertyMapping
	if len(props) == 0 {
		props = cfg.Properties
	}
	out := make([]executor.OutputPropertyMapping, 0, len(props))
	for _, prop := range props {
		source := strings.TrimSpace(prop.SourceField)
		target := strings.TrimSpace(firstNonEmpty(prop.TargetProperty, prop.Name, source))
		if source == "" {
			source = target
		}
		if target == "" {
			continue
		}
		out = append(out, executor.OutputPropertyMapping{
			SourceField:      source,
			TargetProperty:   target,
			PropertyType:     firstNonEmpty(prop.PropertyType, prop.Type),
			DisplayName:      firstNonEmpty(prop.DisplayName, target),
			Required:         prop.Required,
			UniqueConstraint: prop.UniqueConstraint,
		})
	}
	return out
}

func outputLinkObjectTypeIDForPipelineNode(pipelineID uuid.UUID, nodes []models.PipelineNode, cfg outputDatasetConfig, source bool) string {
	if source {
		for _, value := range []string{cfg.SourceObjectTypeID, strings.TrimPrefix(cfg.SourceObjectTypeRID, "ri.ontology.main.object-type.")} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return objectTypeIDForOutputNodeRef(pipelineID, nodes, cfg.SourceObjectNodeID)
	}
	for _, value := range []string{cfg.TargetObjectTypeID, strings.TrimPrefix(cfg.TargetObjectTypeRID, "ri.ontology.main.object-type.")} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return objectTypeIDForOutputNodeRef(pipelineID, nodes, cfg.TargetObjectNodeID)
}

func outputLinkPrimaryKeyForPipelineNode(nodes []models.PipelineNode, cfg outputDatasetConfig, source bool) string {
	if source {
		if strings.TrimSpace(cfg.SourcePrimaryKey) != "" {
			return strings.TrimSpace(cfg.SourcePrimaryKey)
		}
		return primaryKeyForOutputNodeRef(nodes, cfg.SourceObjectNodeID)
	}
	if strings.TrimSpace(cfg.TargetPrimaryKey) != "" {
		return strings.TrimSpace(cfg.TargetPrimaryKey)
	}
	return primaryKeyForOutputNodeRef(nodes, cfg.TargetObjectNodeID)
}

func outputLinkSourceKeyColumn(cfg outputDatasetConfig) string {
	if strings.TrimSpace(cfg.SourceKeyColumn) != "" {
		return strings.TrimSpace(cfg.SourceKeyColumn)
	}
	if strings.TrimSpace(cfg.ForeignKeyColumn) != "" && normaliseLinkCardinality(cfg.Cardinality) == "one_to_many" {
		return strings.TrimSpace(cfg.ForeignKeyColumn)
	}
	return firstNonEmpty(cfg.SourcePrimaryKey, "source_id")
}

func outputLinkTargetKeyColumn(cfg outputDatasetConfig) string {
	if strings.TrimSpace(cfg.TargetKeyColumn) != "" {
		return strings.TrimSpace(cfg.TargetKeyColumn)
	}
	return firstNonEmpty(cfg.TargetPrimaryKey, "target_id")
}

func objectTypeIDForOutputNodeRef(pipelineID uuid.UUID, nodes []models.PipelineNode, nodeID string) string {
	node, ok := pipelineNodeByID(nodes, nodeID)
	if !ok {
		return ""
	}
	return outputObjectTypeIDForPipelineNode(pipelineID, node, parseOutputDatasetConfig(node.Config))
}

func primaryKeyForOutputNodeRef(nodes []models.PipelineNode, nodeID string) string {
	node, ok := pipelineNodeByID(nodes, nodeID)
	if !ok {
		return ""
	}
	return outputPrimaryKeyFromConfig(parseOutputDatasetConfig(node.Config))
}

func pipelineNodeByID(nodes []models.PipelineNode, nodeID string) (models.PipelineNode, bool) {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return models.PipelineNode{}, false
}

func normaliseLinkCardinality(value string) string {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_")) {
	case "", "many_to_many":
		return "many_to_many"
	case "one_to_many", "1_to_many":
		return "one_to_many"
	case "many_to_one", "many_to_1":
		return "many_to_one"
	case "one_to_one", "1_to_1":
		return "one_to_one"
	default:
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	}
}

func upstreamInputDatasetRIDs(nodes []models.PipelineNode, nodeID string) []string {
	byID := make(map[string]models.PipelineNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	seenNodes := map[string]struct{}{}
	seenDatasets := map[string]struct{}{}
	var walk func(string)
	walk = func(id string) {
		if _, ok := seenNodes[id]; ok {
			return
		}
		seenNodes[id] = struct{}{}
		node, ok := byID[id]
		if !ok {
			return
		}
		for _, datasetID := range uuidStrings(node.InputDatasetIDs) {
			seenDatasets[datasetID] = struct{}{}
		}
		for _, dep := range node.DependsOn {
			walk(dep)
		}
	}
	walk(nodeID)
	out := make([]string, 0, len(seenDatasets))
	for datasetID := range seenDatasets {
		out = append(out, datasetID)
	}
	sort.Strings(out)
	return out
}

type runtimeNodeRunner struct {
	JobRunner runners.JobRunner
	Python    runtimepkg.TransformExecutor
	LLM       LLMTransformRunner
	Table     *lightweightTableRuntime
	Dist      DistributedTransformRunner
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
	if isLLMTransform(transformType) {
		return r.runLLM(ctx, node, payload)
	}
	distributedEngine := distributedEngineForNode(transformType, payload, node.Node.Metadata)
	if shouldUseDistributedRuntime(transformType, metadataString(node.Node.Metadata, "pipeline_type"), distributedEngine, payload, node.Node.Metadata) {
		if r.Dist == nil {
			return executor.NodeResult{}, errors.New("distributed_runtime_not_configured: inject a DistributedTransformRunner to execute Spark/Flink pipeline nodes")
		}
		return r.Dist.RunDistributedTransform(ctx, DistributedTransformRequest{
			Node:          node,
			Payload:       payload,
			TransformType: transformType,
			Engine:        distributedEngine,
		})
	}
	if r.Table != nil && r.Table.Supports(transformType) {
		return r.Table.Run(ctx, node, payload, transformType)
	}
	if models.IsLightweightPipelineType(metadataString(node.Node.Metadata, "pipeline_type")) {
		return executor.NodeResult{}, fmt.Errorf("lightweight_runtime_unsupported:%s", strings.ToLower(transformType))
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
	//   - prepared_inputs_json: logic_payload.prepared_inputs or upstream lightweight rows
	//   - input_dataset_ids/output_dataset_id: copied from node metadata/outputs
	//   - timeout_seconds: logic_payload.timeout_seconds clamped to a safe per-node maximum
	var cfg map[string]json.RawMessage
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &cfg)
	}
	source := pythonSourceFromConfig(cfg)
	if err := validatePythonPackageConstraints(source, cfg); err != nil {
		return executor.NodeResult{}, err
	}
	configJSON := cfg["config"]
	if len(configJSON) == 0 && len(payload) > 0 {
		configJSON = payload
	}
	preparedInputsJSON, err := pythonPreparedInputsJSON(r.Table, node, cfg)
	if err != nil {
		return executor.NodeResult{}, err
	}
	inputIDs := metadataStringSlice(node.Node.Metadata, "input_dataset_ids")
	if len(inputIDs) == 0 {
		inputIDs = append([]string(nil), node.Node.DependsOn...)
	}
	outputID := metadataString(node.Node.Metadata, "output_dataset_id")
	if outputID == "" && len(node.Node.Outputs) > 0 {
		outputID = node.Node.Outputs[0].DatasetRID
	}
	timeoutSeconds := pythonTransformTimeoutSeconds(cfg)
	result, err := r.Python.ExecutePythonTransform(ctx, runtimepkg.TransformRequest{Source: source, ConfigJSON: configJSON, PreparedInputsJSON: preparedInputsJSON, InputDatasetIDs: inputIDs, OutputDatasetID: outputID, TimeoutSeconds: timeoutSeconds})
	if err != nil {
		return executor.NodeResult{}, err
	}
	rows, err := pythonRowsFromResult(result)
	if err != nil {
		return executor.NodeResult{}, err
	}
	if r.Table != nil {
		r.Table.storeRows(node.Node.ID, pythonRuntimeRows(rows))
	}
	hashInput := append([]byte(source), result.Output...)
	hashInput = append(hashInput, result.ResultRowsJSON...)
	hash := sha256.Sum256(hashInput)
	metaRows := rows
	if len(metaRows) > 5 {
		metaRows = metaRows[:5]
	}
	meta := map[string]any{
		"runtime":         "python",
		"engine":          "python_sidecar",
		"transform_type":  "python",
		"timeout_seconds": timeoutSeconds,
		"stdout":          result.Stdout,
		"stderr":          result.Stderr,
		"columns":         inferResultColumns(rows),
		"sample_rows":     metaRows,
		"data_rows":       rows,
	}
	if result.RowsAffected != nil {
		meta["rows_affected"] = *result.RowsAffected
	} else {
		meta["rows_affected"] = len(rows)
	}
	if len(result.Output) > 0 {
		var output any
		if json.Unmarshal(result.Output, &output) == nil {
			meta["output"] = output
		}
	}
	if len(pythonConfigRaw(cfg, "input_schema")) > 0 {
		meta["input_contract"] = json.RawMessage(pythonConfigRaw(cfg, "input_schema"))
	}
	if len(pythonConfigRaw(cfg, "output_schema")) > 0 {
		meta["output_contract"] = json.RawMessage(pythonConfigRaw(cfg, "output_schema"))
	}
	if packages := pythonConfigStringSlice(cfg, "packages", "requirements"); len(packages) > 0 {
		meta["packages"] = packages
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
		if outcome.Ignored > 0 && outcome.Completed == 0 {
			return "ignored", nil
		}
		return "succeeded", nil
	case models.BuildAborted:
		msg := "cancelled"
		return "cancelled", &msg
	default:
		msg := "failed"
		return "failed", &msg
	}
}

type capturingAuditSink struct {
	delegate executor.AuditSink
	mu       sync.Mutex
	events   []executor.AuditEvent
}

func newCapturingAuditSink(delegate executor.AuditSink) *capturingAuditSink {
	return &capturingAuditSink{delegate: delegate}
}

func (s *capturingAuditSink) Record(ctx context.Context, event executor.AuditEvent) error {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	emitJobAuditLog(ctx, event)
	if s.delegate != nil {
		return s.delegate.Record(ctx, event)
	}
	return nil
}

func (s *capturingAuditSink) Events() []executor.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]executor.AuditEvent(nil), s.events...)
}

func emitJobAuditLog(ctx context.Context, event executor.AuditEvent) {
	if event.JobID == uuid.Nil {
		return
	}
	service, _ := jobLogService.Load().(*livellogs.Service)
	if service == nil {
		return
	}
	jobRID := "ri.foundry.main.job." + event.JobID.String()
	level := auditLogLevel(event)
	message := auditLogMessage(event)
	params, _ := json.Marshal(map[string]any{
		"build_id":    event.BuildID,
		"job_id":      event.JobID,
		"node_id":     event.NodeID,
		"from":        event.From,
		"to":          event.To,
		"attempt":     event.Attempt,
		"reason":      event.Reason,
		"dataset_rid": event.DatasetRID,
	})
	if appender, ok := service.Store.(LogAppendStore); ok && appender != nil {
		_, _ = appender.AppendLogByRID(ctx, jobRID, level, message, params)
	}
	if mem, ok := service.Subscriber.(*livellogs.MemoryService); ok && mem != nil {
		if storeMem, ok := service.Store.(*livellogs.MemoryService); !ok || storeMem != mem {
			mem.Emit(jobRID, level, message, params)
		}
	}
}

func auditLogLevel(event executor.AuditEvent) livellogs.LogLevel {
	switch {
	case event.To == executor.NodeFailed || event.To == executor.NodeAborted || event.To == executor.NodeAbortPending:
		return livellogs.LogError
	case strings.Contains(strings.ToLower(event.Reason), "warn"):
		return livellogs.LogWarn
	default:
		return livellogs.LogInfo
	}
}

func auditLogMessage(event executor.AuditEvent) string {
	switch {
	case event.From != "" || event.To != "":
		return fmt.Sprintf("job state changed from %s to %s", event.From, event.To)
	case event.DatasetRID != "":
		return "output event: " + event.Reason
	case strings.TrimSpace(event.Reason) != "":
		return event.Reason
	default:
		return "job event"
	}
}

func pipelineRunNodeResults(plan executor.Plan, outcome executor.Outcome, events []executor.AuditEvent) []models.PipelineNodeResult {
	eventsByNode := map[string][]models.PipelineRunEvent{}
	for _, event := range events {
		converted := pipelineRunEventFromAudit(event)
		eventsByNode[event.NodeID] = append(eventsByNode[event.NodeID], converted)
	}

	columnsByNode := map[string][]string{}
	for _, node := range plan.Nodes {
		columnsByNode[node.ID] = metadataColumns(outcome.Results[node.ID].Metadata)
	}

	out := make([]models.PipelineNodeResult, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		result := outcome.Results[node.ID]
		state := outcome.Nodes[node.ID]
		status := nodeResultStatus(node, state)
		reason := strings.TrimSpace(outcome.Reasons[node.ID])
		var errPtr *string
		if reason != "" && (status == "failed" || status == "cancelled") {
			errPtr = &reason
		}
		before := dependencyColumns(node, columnsByNode)
		after := columnsByNode[node.ID]
		out = append(out, models.PipelineNodeResult{
			NodeID:          node.ID,
			Label:           firstNonEmpty(metadataString(node.Metadata, "label"), node.ID),
			TransformType:   metadataString(node.Metadata, "transform_type"),
			Status:          status,
			RowsAffected:    metadataInt64(result.Metadata, "rows_affected"),
			Attempts:        outcome.Attempts[node.ID],
			Output:          nodeResultOutput(node, result),
			Error:           errPtr,
			SchemaDelta:     schemaDelta(before, after),
			OutputResources: outputResourcesForNode(node, status),
			Events:          eventsByNode[node.ID],
			LogRID:          "ri.foundry.main.job." + node.JobID.String(),
		})
	}
	return out
}

func pipelineRunEventFromAudit(event executor.AuditEvent) models.PipelineRunEvent {
	eventType := "audit"
	switch {
	case event.NodeID == "":
		eventType = "build_terminal"
	case event.From != "" || event.To != "":
		eventType = "state_transition"
	case event.DatasetRID != "":
		eventType = "output"
	case strings.HasPrefix(event.Reason, "retry:"):
		eventType = "retry"
	}
	return models.PipelineRunEvent{
		At:         event.At,
		NodeID:     event.NodeID,
		EventType:  eventType,
		From:       string(event.From),
		To:         string(event.To),
		Attempt:    event.Attempt,
		Reason:     event.Reason,
		DatasetRID: event.DatasetRID,
	}
}

func nodeResultStatus(node executor.Node, state executor.NodeState) string {
	switch state {
	case executor.NodeCompleted:
		if node.StaleSkipped {
			return "ignored"
		}
		return "succeeded"
	case executor.NodeFailed:
		return "failed"
	case executor.NodeAborted, executor.NodeAbortPending:
		return "cancelled"
	case executor.NodeRunning:
		return "running"
	default:
		return "queued"
	}
}

func nodeResultOutput(node executor.Node, result executor.NodeResult) map[string]any {
	out := map[string]any{}
	if result.OutputContentHash != "" {
		out["content_hash"] = result.OutputContentHash
	}
	if signature := metadataString(node.Metadata, "staleness_signature"); signature != "" {
		out["staleness_signature"] = signature
	}
	if node.StaleSkipped {
		out["ignored_reason"] = "fresh"
	}
	for key, value := range result.Metadata {
		if key == "data_rows" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metadataInt64(metadata map[string]any, key string) *int64 {
	if metadata == nil {
		return nil
	}
	switch value := metadata[key].(type) {
	case int:
		v := int64(value)
		return &v
	case int64:
		v := value
		return &v
	case int32:
		v := int64(value)
		return &v
	case uint64:
		v := int64(value)
		return &v
	case uint32:
		v := int64(value)
		return &v
	case float64:
		v := int64(value)
		return &v
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataColumns(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	switch raw := metadata["columns"].(type) {
	case []string:
		return sortedUniqueStrings(raw)
	case []any:
		out := []string{}
		for _, entry := range raw {
			if value, ok := entry.(string); ok && strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		}
		return sortedUniqueStrings(out)
	}
	return nil
}

func dependencyColumns(node executor.Node, columnsByNode map[string][]string) []string {
	out := []string{}
	for _, dep := range node.DependsOn {
		out = append(out, columnsByNode[dep]...)
	}
	return sortedUniqueStrings(out)
}

func schemaDelta(before, after []string) *models.PipelineRunSchemaDelta {
	before = sortedUniqueStrings(before)
	after = sortedUniqueStrings(after)
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	beforeSet := stringSet(before)
	afterSet := stringSet(after)
	added := []string{}
	removed := []string{}
	for _, col := range after {
		if _, ok := beforeSet[col]; !ok {
			added = append(added, col)
		}
	}
	for _, col := range before {
		if _, ok := afterSet[col]; !ok {
			removed = append(removed, col)
		}
	}
	return &models.PipelineRunSchemaDelta{ColumnsBefore: before, ColumnsAfter: after, AddedColumns: added, RemovedColumns: removed}
}

func outputResourcesForNode(node executor.Node, status string) []models.PipelineRunOutputResource {
	out := make([]models.PipelineRunOutputResource, 0, len(node.Outputs))
	outputStatus := "pending"
	if status == "succeeded" {
		outputStatus = "committed"
	} else if status == "ignored" {
		outputStatus = "unchanged"
	} else if status == "failed" || status == "cancelled" {
		outputStatus = "aborted"
	}
	for _, tx := range node.Outputs {
		out = append(out, models.PipelineRunOutputResource{
			Kind:           firstNonEmpty(tx.OutputKind, "dataset"),
			RID:            tx.DatasetRID,
			Name:           tx.DatasetName,
			Branch:         tx.Branch,
			TransactionRID: tx.TransactionRID,
			Status:         outputStatus,
		})
	}
	return out
}

func sortedUniqueStrings(values []string) []string {
	set := stringSet(values)
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
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
