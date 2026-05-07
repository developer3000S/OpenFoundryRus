package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/resolver"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

// BuildRepository is the minimal persistence adapter needed by the HTTP build
// surface. Production wiring should persist builds/jobs/transactions in the
// Rust-compatible tables; tests inject fakes instead of silently stubbing IO.
type BuildRepository interface {
	OpenBuild(ctx context.Context, args resolver.ResolveBuildArgs, buildID uuid.UUID) error
	PersistResolvedBuild(ctx context.Context, resolved *models.ResolvedBuild) error
	MarkBuildFailed(ctx context.Context, buildID uuid.UUID, reason string) error
}

type BuildLifecyclePorts struct {
	JobSpecs   resolver.JobSpecRepository
	Versioning resolver.DatasetVersioningRepository
	Locks      resolver.BranchLockRepository
	Builds     BuildRepository
}

type buildLifecycleSlot struct{ ports BuildLifecyclePorts }

var buildLifecyclePorts atomic.Value // stores *buildLifecycleSlot

// SetBuildLifecyclePorts injects resolver and persistence ports for CreateBuild
// and DryRunResolve. It returns a restore function so tests remain isolated.
func SetBuildLifecyclePorts(ports BuildLifecyclePorts) func() {
	previous, _ := buildLifecyclePorts.Load().(*buildLifecycleSlot)
	buildLifecyclePorts.Store(&buildLifecycleSlot{ports: ports})
	return func() { buildLifecyclePorts.Store(previous) }
}

func currentBuildLifecyclePorts() (BuildLifecyclePorts, bool) {
	slot, _ := buildLifecyclePorts.Load().(*buildLifecycleSlot)
	if slot == nil || slot.ports.JobSpecs == nil || slot.ports.Versioning == nil {
		return BuildLifecyclePorts{}, false
	}
	return slot.ports, true
}

func requireBuildLifecyclePorts(w http.ResponseWriter, detail string, needBuilds bool) (BuildLifecyclePorts, bool) {
	ports, ok := currentBuildLifecyclePorts()
	if !ok || (needBuilds && ports.Builds == nil) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "build_lifecycle_ports_not_configured", "detail": detail})
		return BuildLifecyclePorts{}, false
	}
	return ports, true
}

// CreateBuild resolves a Rust-compatible build plan, persists the opened build
// and job plan through injected ports, and returns the same accepted envelope as
// the Rust v1 handler.
func CreateBuild(w http.ResponseWriter, r *http.Request) {
	ports, ok := requireBuildLifecyclePorts(w, "CreateBuild requires JobSpec, dataset versioning and build persistence ports", true)
	if !ok {
		return
	}

	var body models.CreateBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	args, err := createBuildArgs(r.Context(), body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	buildID, err := uuid.NewV7()
	if err != nil {
		buildID = uuid.New()
	}
	args.BuildID = &buildID

	if err := ports.Builds.OpenBuild(r.Context(), args, buildID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build_open_failed", "detail": err.Error()})
		return
	}

	resolved, err := resolver.ResolveBuild(r.Context(), args, ports.JobSpecs, ports.Versioning, ports.Locks)
	if err != nil {
		_ = ports.Builds.MarkBuildFailed(r.Context(), buildID, err.Error())
		writeResolutionError(w, err)
		return
	}
	if err := ports.Builds.PersistResolvedBuild(r.Context(), resolved); err != nil {
		_ = ports.Builds.MarkBuildFailed(r.Context(), buildID, err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build_persist_failed", "detail": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"build_id":            resolved.BuildID,
		"state":               string(resolved.State),
		"queued_reason":       resolved.QueuedReason,
		"job_count":           len(resolved.JobSpecs),
		"output_transactions": resolved.OpenedTransactions,
	})
}

type dryRunResolveRequest struct {
	PipelineRID       string           `json:"pipeline_rid"`
	BuildBranch       string           `json:"build_branch"`
	JobSpecFallback   []string         `json:"job_spec_fallback,omitempty"`
	OutputDatasetRIDs []string         `json:"output_dataset_rids"`
	InlineSpecs       []models.JobSpec `json:"inline_specs,omitempty"`
}

type dryRunResolveResponse struct {
	Jobs   []dryRunJob   `json:"jobs"`
	Errors []dryRunError `json:"errors"`
}

type dryRunJob struct {
	JobSpecRID            string         `json:"job_spec_rid"`
	ResolvedJobSpecBranch string         `json:"resolved_jobspec_branch"`
	OutputDatasetRIDs     []string       `json:"output_dataset_rids"`
	ResolvedOutputs       []dryRunOutput `json:"resolved_outputs"`
	ResolvedInputs        []dryRunInput  `json:"resolved_inputs"`
	DependsOnJobSpecRIDs  []string       `json:"depends_on_job_spec_rids,omitempty"`
	OutputTransactionRIDs []string       `json:"output_transaction_rids,omitempty"`
}

type dryRunInput struct {
	DatasetRID          string   `json:"dataset_rid"`
	ResolvedInputBranch *string  `json:"resolved_input_branch"`
	FallbackIndex       *int     `json:"fallback_index"`
	FallbackChain       []string `json:"fallback_chain"`
}

type dryRunOutput struct {
	DatasetRID     string  `json:"dataset_rid"`
	ResolvedOutput string  `json:"resolved_output"`
	CreatesBranch  bool    `json:"creates_branch"`
	FromBranch     *string `json:"from_branch"`
}

type dryRunError struct {
	DatasetRID *string `json:"dataset_rid"`
	Kind       string  `json:"kind"`
	Message    string  `json:"message"`
}

// DryRunResolve uses the resolver's load/validate/branch-resolution steps but
// intentionally avoids OpenBuild, OpenTransaction, lock acquisition and final persistence.
func DryRunResolve(w http.ResponseWriter, r *http.Request) {
	ports, ok := requireBuildLifecyclePorts(w, "DryRunResolve requires JobSpec and dataset versioning ports", false)
	if !ok {
		return
	}
	var body dryRunResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	for _, key := range []string{"id", "pipeline_rid"} {
		if rid := chi.URLParam(r, key); body.PipelineRID == "" && rid != "" {
			body.PipelineRID = rid
		}
	}
	if err := validateResolveRequest(body.PipelineRID, body.BuildBranch, body.OutputDatasetRIDs); err != nil {
		writeJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	jobRepo := ports.JobSpecs
	if len(body.InlineSpecs) > 0 {
		jobRepo = newInlineJobSpecRepo(body.InlineSpecs)
	}
	specs, err := resolver.LoadJobSpecs(r.Context(), body.PipelineRID, body.BuildBranch, body.JobSpecFallback, body.OutputDatasetRIDs, jobRepo)
	if err != nil {
		writeJSON(w, http.StatusOK, dryRunResolveResponse{Errors: []dryRunError{dryRunErrorFromErr(err)}})
		return
	}
	if err := resolver.DetectCycles(specs); err != nil {
		writeJSON(w, http.StatusOK, dryRunResolveResponse{Errors: []dryRunError{dryRunErrorFromErr(err)}})
		return
	}

	jobs := make([]dryRunJob, 0, len(specs))
	dryErrors := make([]dryRunError, 0)
	planJobs, _ := resolver.BuildFanOutPlan(uuid.Nil, specs, nil)
	depsBySpec := map[string][]string{}
	for _, job := range planJobs {
		depsBySpec[job.JobSpecRID] = job.DependsOnJobSpecRIDs
	}
	for _, spec := range specs {
		inputs := make([]dryRunInput, 0, len(spec.Inputs))
		for _, input := range spec.Inputs {
			resolvedInput := dryRunInput{DatasetRID: input.DatasetRID, FallbackChain: input.FallbackChain}
			branches, err := ports.Versioning.ListBranches(r.Context(), input.DatasetRID)
			if err != nil {
				dryErrors = append(dryErrors, dryRunError{DatasetRID: strPtr(input.DatasetRID), Kind: "VERSIONING_CLIENT_ERROR", Message: err.Error()})
				inputs = append(inputs, resolvedInput)
				continue
			}
			branchNames := branchNames(branches)
			outcome, err := resolver.ResolveInputDataset(body.BuildBranch, input.FallbackChain, branchNames)
			if err != nil {
				dryErrors = append(dryErrors, dryRunError{DatasetRID: strPtr(input.DatasetRID), Kind: "INPUT_NOT_RESOLVABLE", Message: err.Error()})
				inputs = append(inputs, resolvedInput)
				continue
			}
			resolvedInput.ResolvedInputBranch = strPtr(outcome.Branch)
			resolvedInput.FallbackIndex = intPtr(outcome.FallbackIndex)
			inputs = append(inputs, resolvedInput)
		}

		outputs := make([]dryRunOutput, 0, len(spec.OutputDatasetRIDs))
		for _, outputRID := range spec.OutputDatasetRIDs {
			resolvedOutput := dryRunOutput{DatasetRID: outputRID, ResolvedOutput: body.BuildBranch}
			branches, err := ports.Versioning.ListBranches(r.Context(), outputRID)
			if err != nil {
				dryErrors = append(dryErrors, dryRunError{DatasetRID: strPtr(outputRID), Kind: "VERSIONING_CLIENT_ERROR", Message: err.Error()})
				outputs = append(outputs, resolvedOutput)
				continue
			}
			outcome, err := resolver.ResolveOutputDataset(body.BuildBranch, body.JobSpecFallback, branchNames(branches))
			if err != nil {
				dryErrors = append(dryErrors, dryRunError{DatasetRID: strPtr(outputRID), Kind: "OUTPUT_NOT_RESOLVABLE", Message: err.Error()})
				outputs = append(outputs, resolvedOutput)
				continue
			}
			if outcome.Kind == resolver.ResolvedOutputCreateFrom {
				resolvedOutput.ResolvedOutput = outcome.NewBranch
				resolvedOutput.CreatesBranch = true
				resolvedOutput.FromBranch = strPtr(outcome.From)
			} else {
				resolvedOutput.ResolvedOutput = outcome.Branch
			}
			outputs = append(outputs, resolvedOutput)
		}
		jobs = append(jobs, dryRunJob{JobSpecRID: spec.RID, ResolvedJobSpecBranch: spec.BranchName, OutputDatasetRIDs: spec.OutputDatasetRIDs, ResolvedOutputs: outputs, ResolvedInputs: inputs, DependsOnJobSpecRIDs: depsBySpec[spec.RID]})
	}
	writeJSON(w, http.StatusOK, dryRunResolveResponse{Jobs: jobs, Errors: dryErrors})
}

func createBuildArgs(ctx context.Context, body models.CreateBuildRequest) (resolver.ResolveBuildArgs, error) {
	if err := validateResolveRequest(body.PipelineRID, body.BuildBranch, body.OutputDatasetRIDs); err != nil {
		return resolver.ResolveBuildArgs{}, err
	}
	triggerKind := "MANUAL"
	if body.TriggerKind != nil && strings.TrimSpace(*body.TriggerKind) != "" {
		triggerKind = strings.ToUpper(strings.TrimSpace(*body.TriggerKind))
	}
	abortPolicy := string(models.AbortDependentOnly)
	if body.AbortPolicy != nil {
		abortPolicy = string(*body.AbortPolicy)
	}
	requestedBy := ""
	if user, ok := authmw.AuthUserFromContext(ctx); ok && user.Claims != nil {
		requestedBy = user.Claims.Sub.String()
	}
	return resolver.ResolveBuildArgs{PipelineRID: body.PipelineRID, BuildBranch: body.BuildBranch, JobSpecFallback: body.JobSpecFallback, OutputDatasetRIDs: body.OutputDatasetRIDs, ForceBuild: body.ForceBuild, RequestedBy: requestedBy, TriggerKind: triggerKind, AbortPolicy: abortPolicy}, nil
}

func validateResolveRequest(pipelineRID, buildBranch string, outputs []string) error {
	if strings.TrimSpace(pipelineRID) == "" {
		return errors.New("pipeline_rid is required")
	}
	if strings.TrimSpace(buildBranch) == "" {
		return errors.New("build_branch is required")
	}
	if len(outputs) == 0 {
		return errors.New("output_dataset_rids must declare at least one dataset")
	}
	return nil
}

func writeResolutionError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, err.Error())
}

func dryRunErrorFromErr(err error) dryRunError {
	var missing *resolver.MissingJobSpecError
	var cycle *resolver.CycleDetectedError
	var inputMissing *resolver.InputBranchMissingError
	var notFound *resolver.InputNotFoundError
	switch {
	case errors.As(err, &missing):
		return dryRunError{DatasetRID: strPtr(missing.DatasetRID), Kind: "MISSING_JOB_SPEC", Message: err.Error()}
	case errors.As(err, &cycle):
		return dryRunError{Kind: "CYCLE_DETECTED", Message: err.Error()}
	case errors.As(err, &inputMissing):
		return dryRunError{DatasetRID: strPtr(inputMissing.DatasetRID), Kind: "INPUT_NOT_RESOLVABLE", Message: err.Error()}
	case errors.As(err, &notFound):
		return dryRunError{DatasetRID: strPtr(notFound.DatasetRID), Kind: "INPUT_NOT_FOUND", Message: err.Error()}
	default:
		return dryRunError{Kind: "RESOLUTION_ERROR", Message: err.Error()}
	}
}

type inlineJobSpecRepo struct{ specs map[string]models.JobSpec }

func newInlineJobSpecRepo(specs []models.JobSpec) *inlineJobSpecRepo {
	repo := &inlineJobSpecRepo{specs: map[string]models.JobSpec{}}
	for _, spec := range specs {
		for _, output := range spec.OutputDatasetRIDs {
			repo.specs[output] = spec
		}
	}
	return repo
}

func (r *inlineJobSpecRepo) Lookup(_ context.Context, _, outputDatasetRID, _ string, _ []string) (*models.JobSpec, error) {
	spec, ok := r.specs[outputDatasetRID]
	if !ok {
		return nil, nil
	}
	return &spec, nil
}

func branchNames(branches []models.BranchSnapshot) []string {
	out := make([]string, 0, len(branches))
	for _, branch := range branches {
		out = append(out, branch.Name)
	}
	return out
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
