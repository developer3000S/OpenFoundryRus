package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/joblifecycle"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

// AbortBuildRepository is the persistence port needed to make AbortBuild real.
// Implementations should lock the build/jobs rows while loading the snapshot and
// persist the requested build/job state transitions atomically where possible.
type AbortBuildRepository interface {
	LoadBuildForAbort(ctx context.Context, id string) (*AbortBuildSnapshot, error)
	MarkBuildAborting(ctx context.Context, buildID uuid.UUID, reason string) error
	TransitionJob(ctx context.Context, jobID uuid.UUID, from, to models.JobState, reason string) error
	MarkBuildAborted(ctx context.Context, buildID uuid.UUID, reason string) error
}

type AbortBuildSnapshot struct {
	ID    uuid.UUID          `json:"id"`
	RID   string             `json:"rid"`
	State models.BuildState  `json:"state"`
	Jobs  []AbortJobSnapshot `json:"jobs,omitempty"`
}

type AbortJobSnapshot struct {
	ID      uuid.UUID                    `json:"id"`
	State   models.JobState              `json:"state"`
	Outputs []executor.OutputTransaction `json:"outputs,omitempty"`
}

type abortBuildResponse struct {
	RID                 string    `json:"rid,omitempty"`
	BuildID             uuid.UUID `json:"build_id"`
	State               string    `json:"state"`
	CancelledExecution  bool      `json:"cancelled_execution"`
	AbortedTransactions int       `json:"aborted_transactions"`
	TransactionErrors   []string  `json:"transaction_errors,omitempty"`
}

// AbortBuild mirrors the Rust v1 abort flow for build rows and extends it with
// executor cancellation + best-effort open-output transaction aborts.
func AbortBuild(w http.ResponseWriter, r *http.Request) {
	ports, abortRepo, ok := requireAbortPorts(w)
	if !ok {
		return
	}
	id := buildIDParam(r)
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, "build id is required")
		return
	}

	snapshot, err := abortRepo.LoadBuildForAbort(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if snapshot == nil {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if snapshot.State == models.BuildCompleted || snapshot.State == models.BuildFailed {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Idempotent retries for already-aborting/aborted builds are accepted. We
	// still try to cancel any registered execution and abort any open outputs.
	if snapshot.State != models.BuildAborting && snapshot.State != models.BuildAborted {
		if err := abortRepo.MarkBuildAborting(r.Context(), snapshot.ID, "aborted by user"); err != nil {
			writeJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	cancelled := cancelExecution(snapshot.ID)
	transitionAbortJobs(r.Context(), abortRepo, snapshot.Jobs)
	aborted, txErrors := abortOpenOutputs(r.Context(), ports.Transactions, snapshot.Jobs)
	if len(txErrors) == 0 {
		_ = abortRepo.MarkBuildAborted(r.Context(), snapshot.ID, "aborted by user")
	}

	writeAbortBuildResponse(w, abortBuildResponse{RID: snapshot.RID, BuildID: snapshot.ID, State: string(models.BuildAborting), CancelledExecution: cancelled, AbortedTransactions: aborted, TransactionErrors: txErrors})
}

func requireAbortPorts(w http.ResponseWriter) (ExecutionPorts, AbortBuildRepository, bool) {
	ports, ok := currentExecutionPorts()
	if !ok || ports.Transactions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "execution_ports_not_configured", "detail": "AbortBuild requires transaction and abort persistence ports"})
		return ExecutionPorts{}, nil, false
	}
	abortRepo, ok := abortRepositoryFromPorts(ports)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "abort_ports_not_configured", "detail": "AbortBuild requires an AbortBuildRepository"})
		return ExecutionPorts{}, nil, false
	}
	return ports, abortRepo, true
}

func writeAbortBuildResponse(w http.ResponseWriter, response abortBuildResponse) {
	status := http.StatusOK
	if len(response.TransactionErrors) > 0 {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, response)
}

func abortRepositoryFromPorts(ports ExecutionPorts) (AbortBuildRepository, bool) {
	if repo, ok := ports.Plans.(AbortBuildRepository); ok && repo != nil {
		return repo, true
	}
	if repo, ok := ports.Runs.(AbortBuildRepository); ok && repo != nil {
		return repo, true
	}
	return nil, false
}

func buildIDParam(r *http.Request) string {
	for _, key := range []string{"id", "rid", "build_id", "run_id"} {
		if value := chi.URLParam(r, key); value != "" {
			return value
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "builds" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func transitionAbortJobs(ctx context.Context, repo AbortBuildRepository, jobs []AbortJobSnapshot) {
	for _, job := range jobs {
		targets := abortTargets(job.State)
		from := job.State
		for _, to := range targets {
			if !joblifecycle.IsValidTransition(from, to) && from != to {
				continue
			}
			_ = repo.TransitionJob(ctx, job.ID, from, to, "build aborted by user")
			from = to
		}
	}
}

func abortTargets(state models.JobState) []models.JobState {
	switch state {
	case models.JobRunning, models.JobRunPending:
		return []models.JobState{models.JobAbortPending, models.JobAborted}
	case models.JobWaiting:
		return []models.JobState{models.JobAborted}
	case models.JobAbortPending:
		return []models.JobState{models.JobAborted}
	default:
		return nil
	}
}

func abortOpenOutputs(ctx context.Context, txManager executor.TransactionManager, jobs []AbortJobSnapshot) (int, []string) {
	seen := map[string]struct{}{}
	aborted := 0
	txErrors := []string{}
	for _, job := range jobs {
		for _, output := range job.Outputs {
			key := output.DatasetRID + "\x00" + output.TransactionRID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if err := txManager.Abort(ctx, output); err != nil {
				txErrors = append(txErrors, fmt.Sprintf("%s: %s", output.DatasetRID, err.Error()))
				continue
			}
			aborted++
		}
	}
	return aborted, txErrors
}
