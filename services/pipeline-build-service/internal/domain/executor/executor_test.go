package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestExecuteLinearDAG(t *testing.T) {
	plan := Plan{BuildBranch: "master", Parallelism: 2, Nodes: []Node{
		node("a", nil),
		node("b", []string{"a"}),
		node("c", []string{"b"}),
	}}
	runner := &scriptedRunner{outcomes: map[string][]error{"a": {nil}, "b": {nil}, "c": {nil}}}
	audit := &recordingAudit{}

	outcome, err := Execute(context.Background(), plan, runner, &recordingTransactions{}, &recordingCommitter{}, audit)
	require.NoError(t, err)
	require.Equal(t, models.BuildCompleted, outcome.FinalState)
	require.Equal(t, 3, outcome.Completed)
	require.Equal(t, []string{"a", "b", "c"}, runner.started)
	require.Equal(t, NodeCompleted, outcome.Nodes["c"])
}

func TestExecuteBranchedDAG(t *testing.T) {
	plan := Plan{BuildBranch: "master", Parallelism: 3, Nodes: []Node{
		node("root", nil),
		node("left", []string{"root"}),
		node("right", []string{"root"}),
		node("join", []string{"left", "right"}),
	}}
	runner := &scriptedRunner{outcomes: map[string][]error{"root": {nil}, "left": {nil}, "right": {nil}, "join": {nil}}}

	outcome, err := Execute(context.Background(), plan, runner, &recordingTransactions{}, &recordingCommitter{}, nil)
	require.NoError(t, err)
	require.Equal(t, models.BuildCompleted, outcome.FinalState)
	require.Equal(t, 4, outcome.Completed)
	require.Less(t, indexOfString(runner.started, "root"), indexOfString(runner.started, "left"))
	require.Less(t, indexOfString(runner.started, "root"), indexOfString(runner.started, "right"))
	require.Less(t, indexOfString(runner.started, "left"), indexOfString(runner.started, "join"))
	require.Less(t, indexOfString(runner.started, "right"), indexOfString(runner.started, "join"))
}

func TestExecuteFailureInIntermediateNodeCascadesDependents(t *testing.T) {
	plan := Plan{BuildBranch: "master", AbortPolicy: AbortDependentOnly, Parallelism: 2, Nodes: []Node{
		node("a", nil),
		node("b", []string{"a"}),
		node("c", []string{"b"}),
	}}
	runner := &scriptedRunner{outcomes: map[string][]error{"a": {nil}, "b": {errors.New("boom")}}}
	tx := &recordingTransactions{}

	outcome, err := Execute(context.Background(), plan, runner, tx, &recordingCommitter{}, nil)
	require.NoError(t, err)
	require.Equal(t, models.BuildFailed, outcome.FinalState)
	require.Equal(t, 1, outcome.Completed)
	require.Equal(t, 1, outcome.Failed)
	require.Equal(t, 1, outcome.Aborted)
	require.Equal(t, NodeFailed, outcome.Nodes["b"])
	require.Equal(t, NodeAborted, outcome.Nodes["c"])
	require.NotContains(t, runner.started, "c")
	require.Contains(t, tx.abortedDatasets(), "out.c")
}

func TestExecuteCancellationAbortsRemainingAndInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plan := Plan{BuildBranch: "master", Parallelism: 1, Nodes: []Node{
		node("a", nil),
		node("b", []string{"a"}),
	}}
	runner := &cancelingRunner{cancel: cancel}
	tx := &recordingTransactions{}

	outcome, err := Execute(ctx, plan, runner, tx, &recordingCommitter{}, nil)
	require.NoError(t, err)
	require.Equal(t, models.BuildAborted, outcome.FinalState)
	require.Equal(t, 0, outcome.Completed)
	require.Equal(t, 2, outcome.Aborted)
	require.Equal(t, NodeAborted, outcome.Nodes["a"])
	require.Equal(t, NodeAborted, outcome.Nodes["b"])
	require.ElementsMatch(t, []string{"out.a", "out.b"}, tx.abortedDatasets())
}

func TestExecuteRollbackMultiOutputOnPartialCommitFailure(t *testing.T) {
	plan := Plan{BuildBranch: "master", Nodes: []Node{{
		ID: "multi",
		Outputs: []OutputTransaction{
			{DatasetRID: "out.alpha", TransactionRID: "txn-alpha"},
			{DatasetRID: "out.beta", TransactionRID: "txn-beta"},
		},
	}}}
	runner := &scriptedRunner{outcomes: map[string][]error{"multi": {nil}}}
	committer := &recordingCommitter{failDataset: "out.beta"}
	tx := &recordingTransactions{}

	outcome, err := Execute(context.Background(), plan, runner, tx, committer, nil)
	require.NoError(t, err)
	require.Equal(t, models.BuildFailed, outcome.FinalState)
	require.Equal(t, 1, outcome.Failed)
	require.Equal(t, []string{"out.alpha", "out.beta"}, committer.committedDatasets())
	require.Equal(t, []string{"out.alpha", "out.beta"}, tx.abortedDatasets(), "partial commit failure rolls back committed outputs and aborts the failed output")
}

func TestExecuteRetriesUntilSuccess(t *testing.T) {
	plan := Plan{BuildBranch: "master", MaxAttempts: 2, Nodes: []Node{node("flaky", nil)}}
	runner := &scriptedRunner{outcomes: map[string][]error{"flaky": {errors.New("transient"), nil}}}

	outcome, err := Execute(context.Background(), plan, runner, &recordingTransactions{}, &recordingCommitter{}, nil)
	require.NoError(t, err)
	require.Equal(t, models.BuildCompleted, outcome.FinalState)
	require.Equal(t, 1, outcome.Completed)
	require.Equal(t, 2, outcome.Attempts["flaky"])
}

func TestExecuteIgnoresFreshNodeWithoutRunningOrCommitting(t *testing.T) {
	plan := Plan{BuildBranch: "master", Nodes: []Node{{
		ID:           "fresh",
		StaleSkipped: true,
		Outputs:      []OutputTransaction{{DatasetRID: "out.fresh", TransactionRID: "txn.fresh"}},
		Metadata:     map[string]any{"staleness_signature": "sig-fresh"},
	}}}
	runner := &scriptedRunner{}
	tx := &recordingTransactions{}
	committer := &recordingCommitter{}
	audit := &recordingAudit{}

	outcome, err := Execute(context.Background(), plan, runner, tx, committer, audit)
	require.NoError(t, err)
	require.Equal(t, models.BuildCompleted, outcome.FinalState)
	require.Equal(t, 0, outcome.Completed)
	require.Equal(t, 1, outcome.Ignored)
	require.Empty(t, runner.started)
	require.Empty(t, committer.committedDatasets())
	require.Equal(t, []string{"out.fresh"}, tx.abortedDatasets())
	require.Equal(t, "ignored because fresh", outcome.Reasons["fresh"])
	require.Equal(t, "sig-fresh", outcome.Results["fresh"].Metadata["staleness_signature"])
	require.True(t, audit.containsTransition(NodeWaiting, NodeCompleted, "ignored because fresh"))
}

func TestExecuteRejectsInvalidGraphCycle(t *testing.T) {
	_, err := Execute(context.Background(), Plan{Nodes: []Node{
		node("a", []string{"b"}),
		node("b", []string{"a"}),
	}}, &scriptedRunner{}, nil, nil, nil)
	require.Error(t, err)
	var cycle *CycleDetectedError
	require.True(t, errors.As(err, &cycle), "got %T: %v", err, err)
	require.NotEmpty(t, cycle.CyclePath)
}

func node(id string, deps []string) Node {
	return Node{ID: id, DependsOn: deps, Outputs: []OutputTransaction{{DatasetRID: "out." + id, TransactionRID: "txn." + id}}}
}

type scriptedRunner struct {
	mu       sync.Mutex
	outcomes map[string][]error
	started  []string
}

func (r *scriptedRunner) Run(_ context.Context, ctx NodeContext) (NodeResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, ctx.Node.ID)
	outcomes := r.outcomes[ctx.Node.ID]
	if len(outcomes) >= ctx.Attempt {
		return NodeResult{OutputContentHash: "hash-" + ctx.Node.ID}, outcomes[ctx.Attempt-1]
	}
	return NodeResult{OutputContentHash: "hash-" + ctx.Node.ID}, nil
}

type cancelingRunner struct{ cancel context.CancelFunc }

func (r *cancelingRunner) Run(ctx context.Context, _ NodeContext) (NodeResult, error) {
	r.cancel()
	<-ctx.Done()
	return NodeResult{}, ctx.Err()
}

type recordingTransactions struct {
	mu      sync.Mutex
	aborted []OutputTransaction
}

func (r *recordingTransactions) Abort(_ context.Context, tx OutputTransaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aborted = append(r.aborted, tx)
	return nil
}

func (r *recordingTransactions) abortedDatasets() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.aborted))
	for i, tx := range r.aborted {
		out[i] = tx.DatasetRID
	}
	return out
}

type recordingCommitter struct {
	mu          sync.Mutex
	committed   []OutputTransaction
	failDataset string
}

func (r *recordingCommitter) Commit(_ context.Context, tx OutputTransaction, _ NodeResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.committed = append(r.committed, tx)
	if tx.DatasetRID == r.failDataset {
		return fmt.Errorf("simulated commit failure")
	}
	return nil
}

func (r *recordingCommitter) committedDatasets() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.committed))
	for i, tx := range r.committed {
		out[i] = tx.DatasetRID
	}
	return out
}

type recordingAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (r *recordingAudit) Record(_ context.Context, event AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *recordingAudit) containsTransition(from, to NodeState, reason string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.From == from && event.To == to && event.Reason == reason {
			return true
		}
	}
	return false
}

func indexOfString(items []string, needle string) int {
	for i, item := range items {
		if item == needle {
			return i
		}
	}
	return -1
}
