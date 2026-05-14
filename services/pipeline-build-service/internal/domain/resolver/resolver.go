package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

// ClientError mirrors Rust build_resolution::ClientError.
type ClientError struct{ Message string }

func (e *ClientError) Error() string { return "dataset client error: " + e.Message }

// JobSpecRepository loads pipeline-authoring JobSpecs by declared output.
type JobSpecRepository interface {
	Lookup(ctx context.Context, pipelineRID, outputDatasetRID, buildBranch string, fallbackChain []string) (*models.JobSpec, error)
}

// DatasetVersioningRepository is the resolver-facing dataset-versioning API.
type DatasetVersioningRepository interface {
	ListBranches(ctx context.Context, datasetRID string) ([]models.BranchSnapshot, error)
	OpenTransaction(ctx context.Context, datasetRID, branch string) (string, error)
	ViewSchema(ctx context.Context, datasetRID, branch string) (json.RawMessage, error)
}

// BranchLockRepository stores output-dataset build locks. ok=false means the
// lock is already held by holderBuildID; the repository is the injected lock primitive.
type BranchLockRepository interface {
	TryAcquire(ctx context.Context, outputDatasetRID string, buildID uuid.UUID, transactionRID string) (holderBuildID uuid.UUID, ok bool, err error)
	HasUpstreamInProgress(ctx context.Context, inputDatasetRIDs []string, selfBuildID uuid.UUID) (bool, error)
}

// ResolveBuildArgs mirrors Rust build_resolution::ResolveBuildArgs.
type ResolveBuildArgs struct {
	// BuildID lets HTTP/repo adapters open the build record before resolution,
	// matching the Rust lifecycle transaction. When nil, ResolveBuild creates one.
	BuildID           *uuid.UUID
	PipelineRID       string
	BuildBranch       string
	JobSpecFallback   []string
	OutputDatasetRIDs []string
	ForceBuild        bool
	RequestedBy       string
	TriggerKind       string
	AbortPolicy       string
}

// MissingJobSpecError matches Rust BuildResolutionError::MissingJobSpec.
type MissingJobSpecError struct {
	DatasetRID string
	Tried      []string
}

func (e *MissingJobSpecError) Error() string {
	return fmt.Sprintf("missing JobSpec for output dataset %s (tried branches: %s)", e.DatasetRID, formatQuotedList(e.Tried))
}

// CycleDetectedError matches Rust BuildResolutionError::CycleDetected.
type CycleDetectedError struct{ CyclePath []string }

func (e *CycleDetectedError) Error() string {
	return "cycle detected in JobSpec graph: " + strings.Join(e.CyclePath, " → ")
}

// InputNotFoundError matches Rust BuildResolutionError::InputNotFound.
type InputNotFoundError struct{ DatasetRID string }

func (e *InputNotFoundError) Error() string { return "input dataset " + e.DatasetRID + " not found" }

// InputBranchMissingError matches Rust BuildResolutionError::InputBranchMissing.
type InputBranchMissingError struct {
	DatasetRID  string
	BuildBranch string
	Chain       []string
}

func (e *InputBranchMissingError) Error() string {
	return fmt.Sprintf("input dataset %s has no branch matching build='%s' (chain: %s)", e.DatasetRID, e.BuildBranch, formatQuotedList(e.Chain))
}

// StaleInputError matches Rust BuildResolutionError::StaleInput.
type StaleInputError struct{ DatasetRID string }

func (e *StaleInputError) Error() string {
	return "input " + e.DatasetRID + " resolved to fallback branch but require_fresh=true"
}

// LockHeldError matches Rust BuildResolutionError::LockHeld.
type LockHeldError struct {
	DatasetRID    string
	HolderBuildID uuid.UUID
}

func (e *LockHeldError) Error() string {
	return fmt.Sprintf("output dataset %s already locked by build %s", e.DatasetRID, e.HolderBuildID)
}

// ClientResolutionError matches Rust BuildResolutionError::Client.
type ClientResolutionError struct{ Message string }

func (e *ClientResolutionError) Error() string {
	return "dataset versioning client error: " + e.Message
}

// InvalidLogicKindError matches Rust BuildResolutionError::InvalidLogicKind.
type InvalidLogicKindError struct {
	JobSpecRID string
	Reason     string
}

func (e *InvalidLogicKindError) Error() string {
	return fmt.Sprintf("invalid logic_kind on JobSpec %s: %s", e.JobSpecRID, e.Reason)
}

// ViewFilterResolutionError matches Rust BuildResolutionError::ViewFilterResolution.
type ViewFilterResolutionError struct {
	JobSpecRID string
	Errors     []string
}

func (e *ViewFilterResolutionError) Error() string {
	return fmt.Sprintf("view filter resolution failed for JobSpec %s: %s", e.JobSpecRID, formatQuotedList(e.Errors))
}

// ResolveBuild drives the resolver without HTTP or a DAG executor.
func ResolveBuild(ctx context.Context, args ResolveBuildArgs, jobSpecs JobSpecRepository, versioning DatasetVersioningRepository, locks BranchLockRepository) (*models.ResolvedBuild, error) {
	started := time.Now().UTC()
	if args.TriggerKind == "" {
		args.TriggerKind = "MANUAL"
	}
	if args.AbortPolicy == "" {
		args.AbortPolicy = string(models.AbortDependentOnly)
	}

	specs, err := LoadJobSpecs(ctx, args.PipelineRID, args.BuildBranch, args.JobSpecFallback, args.OutputDatasetRIDs, jobSpecs)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if reason := ValidateLogicKind(spec.LogicKind, len(spec.OutputDatasetRIDs)); reason != "" {
			return nil, &InvalidLogicKindError{JobSpecRID: spec.RID, Reason: reason}
		}
	}
	if err := DetectCycles(specs); err != nil {
		return nil, err
	}
	inputViews, err := ValidateInputs(ctx, args.BuildBranch, specs, versioning)
	if err != nil {
		return nil, err
	}

	var buildID uuid.UUID
	if args.BuildID != nil {
		buildID = *args.BuildID
	} else {
		var err error
		buildID, err = uuid.NewV7()
		if err != nil {
			buildID = uuid.New()
		}
	}
	if locks != nil {
		inputRIDs := externalInputDatasetRIDs(specs)
		upstream, err := locks.HasUpstreamInProgress(ctx, inputRIDs, buildID)
		if err != nil {
			return nil, mapClientErr(err)
		}
		if upstream {
			reason := "upstream build in progress"
			return &models.ResolvedBuild{BuildID: buildID, State: models.BuildQueued, ForceBuild: args.ForceBuild, JobSpecs: specs, InputViews: inputViews, QueuedReason: &reason, ResolvedAt: started}, nil
		}
	}

	opened, err := AcquireLocks(ctx, buildID, specs, args.BuildBranch, versioning, locks)
	if err != nil {
		return nil, err
	}
	jobs, stages := BuildFanOutPlan(buildID, specs, opened)
	return &models.ResolvedBuild{BuildID: buildID, State: models.BuildResolution, ForceBuild: args.ForceBuild, JobSpecs: specs, InputViews: inputViews, OpenedTransactions: opened, Jobs: jobs, FanOutStages: stages, ResolvedAt: started}, nil
}

// LoadJobSpecs performs Rust step a, including deterministic RID de-duplication.
func LoadJobSpecs(ctx context.Context, pipelineRID, buildBranch string, fallbackChain, outputDatasetRIDs []string, repo JobSpecRepository) ([]models.JobSpec, error) {
	specs := make([]models.JobSpec, 0, len(outputDatasetRIDs))
	for _, output := range outputDatasetRIDs {
		spec, err := repo.Lookup(ctx, pipelineRID, output, buildBranch, cloneStrings(fallbackChain))
		if err != nil {
			return nil, mapClientErr(err)
		}
		if spec == nil {
			tried := append([]string{buildBranch}, fallbackChain...)
			return nil, &MissingJobSpecError{DatasetRID: output, Tried: tried}
		}
		specs = append(specs, *spec)
	}
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].RID < specs[j].RID })
	out := specs[:0]
	var last string
	for i, spec := range specs {
		if i == 0 || spec.RID != last {
			out = append(out, spec)
			last = spec.RID
		}
	}
	return out, nil
}

// DetectCycles ports the Rust Kahn scan and representative cycle path recovery.
func DetectCycles(specs []models.JobSpec) error {
	producer := map[string]string{}
	for _, spec := range specs {
		for _, output := range spec.OutputDatasetRIDs {
			producer[output] = spec.RID
		}
	}
	graph := map[string][]string{}
	indegree := map[string]int{}
	for _, spec := range specs {
		graph[spec.RID] = graph[spec.RID]
		indegree[spec.RID] = indegree[spec.RID]
	}
	for _, spec := range specs {
		for _, input := range spec.Inputs {
			upstream, ok := producer[input.DatasetRID]
			if !ok {
				continue
			}
			if upstream == spec.RID {
				return &CycleDetectedError{CyclePath: []string{spec.RID, spec.RID}}
			}
			graph[spec.RID] = append(graph[spec.RID], upstream)
			indegree[upstream]++
		}
	}
	zero := make([]string, 0)
	for node, degree := range indegree {
		if degree == 0 {
			zero = append(zero, node)
		}
	}
	sort.Strings(zero)
	popped := 0
	for len(zero) > 0 {
		node := zero[0]
		zero = zero[1:]
		popped++
		neighbours := append([]string(nil), graph[node]...)
		sort.Strings(neighbours)
		for _, next := range neighbours {
			if indegree[next] > 0 {
				indegree[next]--
			}
			if indegree[next] == 0 {
				zero = append(zero, next)
				sort.Strings(zero)
			}
		}
	}
	if popped == len(specs) {
		return nil
	}
	if cycle := findCyclePath(graph, indegree); len(cycle) > 0 {
		return &CycleDetectedError{CyclePath: cycle}
	}
	fallback := make([]string, len(specs))
	for i, spec := range specs {
		fallback[i] = spec.RID
	}
	return &CycleDetectedError{CyclePath: fallback}
}

// ValidateInputs ports Rust step c and skips inputs produced inside this build.
func ValidateInputs(ctx context.Context, buildBranch string, specs []models.JobSpec, client DatasetVersioningRepository) ([]models.ResolvedInputView, error) {
	producerOutputs := outputSet(specs)
	seen := map[string]struct{}{}
	resolved := make([]models.ResolvedInputView, 0)
	for _, spec := range specs {
		for _, input := range spec.Inputs {
			if _, internal := producerOutputs[input.DatasetRID]; internal {
				continue
			}
			if _, ok := seen[input.DatasetRID]; ok {
				continue
			}
			seen[input.DatasetRID] = struct{}{}
			snapshots, err := client.ListBranches(ctx, input.DatasetRID)
			if err != nil {
				return nil, mapClientErr(err)
			}
			if len(snapshots) == 0 {
				return nil, &InputNotFoundError{DatasetRID: input.DatasetRID}
			}
			branches := make([]string, 0, len(snapshots))
			for _, snapshot := range snapshots {
				branches = append(branches, snapshot.Name)
			}
			outcome, err := ResolveInputDataset(buildBranch, input.FallbackChain, branches)
			if err != nil {
				var noMatch *NoMatchError
				var ancestry *IncompatibleAncestryError
				if errors.As(err, &noMatch) || errors.As(err, &ancestry) {
					return nil, &InputBranchMissingError{DatasetRID: input.DatasetRID, BuildBranch: buildBranch, Chain: cloneStrings(input.FallbackChain)}
				}
				return nil, err
			}
			if input.RequireFresh && outcome.FallbackIndex > 0 {
				return nil, &StaleInputError{DatasetRID: input.DatasetRID}
			}
			schema, err := client.ViewSchema(ctx, input.DatasetRID, outcome.Branch)
			if err != nil {
				return nil, mapClientErr(err)
			}
			if len(schema) == 0 {
				schema = json.RawMessage(`null`)
			}
			resolved = append(resolved, models.ResolvedInputView{DatasetRID: input.DatasetRID, Branch: outcome.Branch, HeadTransactionRID: headTransactionForBranch(snapshots, outcome.Branch), Schema: schema})
		}
	}
	return resolved, nil
}

func headTransactionForBranch(snapshots []models.BranchSnapshot, branch string) *string {
	for _, snapshot := range snapshots {
		if snapshot.Name == branch {
			return snapshot.HeadTransactionRID
		}
	}
	return nil
}

// AcquireLocks opens one transaction per output and delegates lock persistence.
func AcquireLocks(ctx context.Context, buildID uuid.UUID, specs []models.JobSpec, buildBranch string, client DatasetVersioningRepository, locks BranchLockRepository) ([]models.OpenedTransaction, error) {
	opened := make([]models.OpenedTransaction, 0)
	seenOutputs := map[string]struct{}{}
	for _, spec := range specs {
		for _, output := range spec.OutputDatasetRIDs {
			if _, seen := seenOutputs[output]; seen {
				continue
			}
			seenOutputs[output] = struct{}{}
			txnRID, err := client.OpenTransaction(ctx, output, buildBranch)
			if err != nil {
				return nil, mapClientErr(err)
			}
			if locks != nil {
				holder, ok, err := locks.TryAcquire(ctx, output, buildID, txnRID)
				if err != nil {
					return nil, mapClientErr(err)
				}
				if !ok {
					return nil, &LockHeldError{DatasetRID: output, HolderBuildID: holder}
				}
			}
			opened = append(opened, models.OpenedTransaction{DatasetRID: output, TransactionRID: txnRID})
		}
	}
	return opened, nil
}

// BuildFanOutPlan returns resolver-created jobs and topological execution stages.
func BuildFanOutPlan(buildID uuid.UUID, specs []models.JobSpec, opened []models.OpenedTransaction) ([]models.ResolvedJob, [][]string) {
	producer := map[string]string{}
	for _, spec := range specs {
		for _, output := range spec.OutputDatasetRIDs {
			producer[output] = spec.RID
		}
	}
	openedByDataset := map[string]string{}
	for _, txn := range opened {
		openedByDataset[txn.DatasetRID] = txn.TransactionRID
	}
	jobs := make([]models.ResolvedJob, 0, len(specs))
	dependencies := map[string][]string{}
	for _, spec := range specs {
		depSet := map[string]struct{}{}
		for _, input := range spec.Inputs {
			if upstream, ok := producer[input.DatasetRID]; ok && upstream != spec.RID {
				depSet[upstream] = struct{}{}
			}
		}
		deps := setKeys(depSet)
		dependencies[spec.RID] = deps
		txnRIDs := make([]string, 0, len(spec.OutputDatasetRIDs))
		for _, output := range spec.OutputDatasetRIDs {
			if txn, ok := openedByDataset[output]; ok {
				txnRIDs = append(txnRIDs, txn)
			}
		}
		jobID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(buildID.String()+":"+spec.RID))
		jobs = append(jobs, models.ResolvedJob{ID: jobID, JobSpecRID: spec.RID, OutputTransactionRIDs: txnRIDs, DependsOnJobSpecRIDs: deps})
	}
	return jobs, fanOutStages(dependencies)
}

// ValidateLogicKind mirrors the resolver-time runner arity check subset.
func ValidateLogicKind(kind string, outputCount int) string {
	switch strings.ToUpper(kind) {
	case "SYNC", "TRANSFORM", "HEALTH_CHECK", "ANALYTICAL", "EXPORT":
		if outputCount == 0 {
			return "at least one output dataset is required"
		}
		return ""
	default:
		return "unsupported logic_kind " + kind
	}
}

func findCyclePath(graph map[string][]string, indegree map[string]int) []string {
	starts := make([]string, 0)
	for node, degree := range indegree {
		if degree > 0 {
			starts = append(starts, node)
		}
	}
	sort.Strings(starts)
	visited := map[string]struct{}{}
	stack := []string{}
	var dfs func(string) []string
	dfs = func(node string) []string {
		if pos := indexOf(stack, node); pos >= 0 {
			cycle := append([]string(nil), stack[pos:]...)
			cycle = append(cycle, node)
			return cycle
		}
		if _, ok := visited[node]; ok {
			return nil
		}
		visited[node] = struct{}{}
		stack = append(stack, node)
		neighbours := append([]string(nil), graph[node]...)
		sort.Strings(neighbours)
		for _, next := range neighbours {
			if cycle := dfs(next); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		return nil
	}
	for _, start := range starts {
		if cycle := dfs(start); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

func fanOutStages(dependencies map[string][]string) [][]string {
	remaining := map[string]struct{}{}
	for rid := range dependencies {
		remaining[rid] = struct{}{}
	}
	completed := map[string]struct{}{}
	stages := [][]string{}
	for len(remaining) > 0 {
		stage := make([]string, 0)
		for rid := range remaining {
			ready := true
			for _, dep := range dependencies[rid] {
				if _, ok := completed[dep]; !ok {
					ready = false
					break
				}
			}
			if ready {
				stage = append(stage, rid)
			}
		}
		sort.Strings(stage)
		if len(stage) == 0 {
			break
		}
		stages = append(stages, stage)
		for _, rid := range stage {
			delete(remaining, rid)
			completed[rid] = struct{}{}
		}
	}
	return stages
}

func externalInputDatasetRIDs(specs []models.JobSpec) []string {
	outputs := outputSet(specs)
	inputSet := map[string]struct{}{}
	for _, spec := range specs {
		for _, input := range spec.Inputs {
			if _, internal := outputs[input.DatasetRID]; !internal {
				inputSet[input.DatasetRID] = struct{}{}
			}
		}
	}
	return setKeys(inputSet)
}

func outputSet(specs []models.JobSpec) map[string]struct{} {
	out := map[string]struct{}{}
	for _, spec := range specs {
		for _, output := range spec.OutputDatasetRIDs {
			out[output] = struct{}{}
		}
	}
	return out
}

func setKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func mapClientErr(err error) error {
	if err == nil {
		return nil
	}
	var client *ClientError
	if errors.As(err, &client) {
		return &ClientResolutionError{Message: client.Message}
	}
	return err
}

func formatQuotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
