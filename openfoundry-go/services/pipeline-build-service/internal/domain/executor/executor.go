// Package executor ports the pipeline-build-service DAG job executor without
// binding it to Postgres, Spark, Iceberg, or HTTP. Production adapters can map
// resolver output rows into Plan/Node and persist StateChange events through the
// interfaces below; tests can drive the executor entirely with fakes.
package executor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

const DefaultParallelism = 4

// AbortPolicy controls the Rust-compatible failure cascade.
type AbortPolicy string

const (
	AbortDependentOnly   AbortPolicy = "DEPENDENT_ONLY"
	AbortAllNonDependent AbortPolicy = "ALL_NON_DEPENDENT"
)

// NodeState is the executor-local state vocabulary. It mirrors the persisted
// job states without requiring a database in this package.
type NodeState string

const (
	NodeWaiting      NodeState = "WAITING"
	NodeRunPending   NodeState = "RUN_PENDING"
	NodeRunning      NodeState = "RUNNING"
	NodeAbortPending NodeState = "ABORT_PENDING"
	NodeAborted      NodeState = "ABORTED"
	NodeFailed       NodeState = "FAILED"
	NodeCompleted    NodeState = "COMPLETED"
)

func (s NodeState) terminal() bool {
	return s == NodeAborted || s == NodeFailed || s == NodeCompleted
}

// OutputTransaction is the open write transaction for one output dataset.
type OutputTransaction struct {
	DatasetRID     string
	TransactionRID string
}

// Node is one executable JobSpec in the DAG.
type Node struct {
	ID                 string
	JobID              uuid.UUID
	DependsOn          []string
	Outputs            []OutputTransaction
	MaxAttempts        int
	Metadata           map[string]any
	ResolvedInputViews []models.ResolvedInputView
}

// Plan is the full build execution graph.
type Plan struct {
	BuildID     uuid.UUID
	BuildBranch string
	AbortPolicy AbortPolicy
	Parallelism int
	MaxAttempts int
	Nodes       []Node
}

// NodeContext is handed to NodeRunner on every attempt.
type NodeContext struct {
	BuildID     uuid.UUID
	BuildBranch string
	Node        Node
	Attempt     int
}

// NodeResult describes runner output before transaction commit.
type NodeResult struct {
	OutputContentHash string
	Metadata          map[string]any
}

// NodeRunner executes node logic. It must respect ctx cancellation where
// possible; the executor converts returned errors into FAILED/CANCELLED flow.
type NodeRunner interface {
	Run(ctx context.Context, node NodeContext) (NodeResult, error)
}

// TransactionManager aborts open output transactions during runner failure,
// cancellation, and multi-output partial commit rollback.
type TransactionManager interface {
	Abort(ctx context.Context, tx OutputTransaction) error
}

// OutputCommitter commits output transactions once runner logic succeeds.
type OutputCommitter interface {
	Commit(ctx context.Context, tx OutputTransaction) error
}

// AuditSink observes lifecycle transitions and transaction events. Adapters can
// persist these to job_state_transitions/build_events; fakes can assert them.
type AuditSink interface {
	Record(ctx context.Context, event AuditEvent) error
}

// AuditEvent is emitted for state transitions, commit/abort operations, retries
// and build terminal state. JobID is uuid.Nil for build-terminal events that do
// not correspond to a single job row.
type AuditEvent struct {
	At         time.Time
	BuildID    uuid.UUID
	JobID      uuid.UUID
	NodeID     string
	From       NodeState
	To         NodeState
	Attempt    int
	Reason     string
	DatasetRID string
}

// Outcome aggregates terminal node counts and final build state.
type Outcome struct {
	FinalState models.BuildState
	Completed  int
	Failed     int
	Aborted    int
	Attempts   map[string]int
	Nodes      map[string]NodeState
	Reasons    map[string]string
}

// Error contracts exposed by Execute.
type CycleDetectedError struct{ CyclePath []string }

func (e *CycleDetectedError) Error() string {
	return "cycle detected in executor DAG: " + joinPath(e.CyclePath)
}

type DuplicateNodeError struct{ NodeID string }

func (e *DuplicateNodeError) Error() string { return "duplicate executor node: " + e.NodeID }

type MissingDependencyError struct {
	NodeID       string
	DependencyID string
}

func (e *MissingDependencyError) Error() string {
	return fmt.Sprintf("executor node %s depends on missing node %s", e.NodeID, e.DependencyID)
}

// Execute runs the DAG topologically. Independent ready nodes run in parallel up
// to Plan.Parallelism. The function returns only after all nodes are terminal or
// the context is cancelled.
func Execute(ctx context.Context, plan Plan, runner NodeRunner, txManager TransactionManager, committer OutputCommitter, audit AuditSink) (Outcome, error) {
	if runner == nil {
		return Outcome{}, errors.New("executor requires a NodeRunner")
	}
	if txManager == nil {
		txManager = noopTransactions{}
	}
	if committer == nil {
		committer = noopCommitter{}
	}
	if audit == nil {
		audit = noopAudit{}
	}

	graph, err := newExecutionGraph(plan)
	if err != nil {
		return Outcome{}, err
	}
	parallelism := plan.Parallelism
	if parallelism < 1 {
		parallelism = DefaultParallelism
	}
	maxAttempts := plan.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	policy := plan.AbortPolicy
	if policy == "" {
		policy = AbortDependentOnly
	}

	state := map[string]NodeState{}
	attempts := map[string]int{}
	remaining := map[string]struct{}{}
	completed := map[string]struct{}{}
	failed := map[string]struct{}{}
	aborted := map[string]struct{}{}
	reasons := map[string]string{}
	inFlight := map[string]struct{}{}
	for _, node := range graph.nodes {
		state[node.ID] = NodeWaiting
		remaining[node.ID] = struct{}{}
	}

	results := make(chan nodeRunResult)

	launch := func(node Node) {
		delete(remaining, node.ID)
		inFlight[node.ID] = struct{}{}
		go func() {
			results <- driveNode(ctx, plan, node, maxAttempts, runner, txManager, committer, audit)
		}()
	}

	for len(remaining) > 0 || len(inFlight) > 0 {
		if err := ctx.Err(); err != nil {
			for id := range remaining {
				abortNode(context.Background(), plan.BuildID, graph.byID[id], state, aborted, txManager, audit, "build cancelled")
				delete(remaining, id)
			}
			for id := range inFlight {
				// The in-flight runner observes ctx cancellation. Pre-marking keeps
				// dependents from being scheduled while we wait for its result.
				state[id] = NodeAbortPending
			}
		}

		ready := readyNodes(remaining, completed, graph)
		if len(ready) == 0 && len(inFlight) == 0 {
			// Defensive: if a failed or aborted ancestor made every remaining
			// node unreachable, abort the rest instead of deadlocking.
			ids := sortedSetKeys(remaining)
			for _, id := range ids {
				abortNode(context.Background(), plan.BuildID, graph.byID[id], state, aborted, txManager, audit, "dependency failed")
				delete(remaining, id)
			}
			break
		}
		for _, id := range ready {
			if ctx.Err() != nil || len(inFlight) >= parallelism {
				break
			}
			launch(graph.byID[id])
		}
		if len(inFlight) == 0 {
			continue
		}
		result := <-results
		delete(inFlight, result.nodeID)
		attempts[result.nodeID] = result.attempts
		if result.reason != "" {
			reasons[result.nodeID] = result.reason
		}
		state[result.nodeID] = result.state
		switch result.state {
		case NodeCompleted:
			completed[result.nodeID] = struct{}{}
		case NodeFailed:
			failed[result.nodeID] = struct{}{}
			cascade := computeCascade(graph, result.nodeID, policy, completed)
			for _, dep := range cascade.dependents {
				if _, ok := remaining[dep]; ok {
					abortNode(context.Background(), plan.BuildID, graph.byID[dep], state, aborted, txManager, audit, "dependency failed")
					delete(remaining, dep)
				}
			}
			for _, independent := range cascade.independents {
				if _, ok := remaining[independent]; ok {
					abortNode(context.Background(), plan.BuildID, graph.byID[independent], state, aborted, txManager, audit, "abort_policy=ALL_NON_DEPENDENT triggered by upstream failure")
					delete(remaining, independent)
				}
			}
		case NodeAborted:
			aborted[result.nodeID] = struct{}{}
		}
	}

	out := Outcome{Attempts: attempts, Nodes: state, Reasons: reasons}
	for _, s := range state {
		switch s {
		case NodeCompleted:
			out.Completed++
		case NodeFailed:
			out.Failed++
		case NodeAborted, NodeAbortPending:
			out.Aborted++
		}
	}
	switch {
	case out.Failed > 0:
		out.FinalState = models.BuildFailed
	case out.Aborted > 0:
		out.FinalState = models.BuildAborted
	default:
		out.FinalState = models.BuildCompleted
	}
	_ = audit.Record(context.Background(), AuditEvent{At: time.Now().UTC(), BuildID: plan.BuildID, To: NodeState(out.FinalState), Reason: "build terminal"})
	return out, nil
}

type executionGraph struct {
	nodes      []Node
	byID       map[string]Node
	dependsOn  map[string][]string
	dependents map[string][]string
}

func newExecutionGraph(plan Plan) (executionGraph, error) {
	graph := executionGraph{byID: map[string]Node{}, dependsOn: map[string][]string{}, dependents: map[string][]string{}}
	for _, node := range plan.Nodes {
		if node.ID == "" {
			return graph, errors.New("executor node id cannot be empty")
		}
		if _, exists := graph.byID[node.ID]; exists {
			return graph, &DuplicateNodeError{NodeID: node.ID}
		}
		node.DependsOn = cloneAndSort(node.DependsOn)
		graph.nodes = append(graph.nodes, node)
		graph.byID[node.ID] = node
		graph.dependsOn[node.ID] = node.DependsOn
	}
	sort.SliceStable(graph.nodes, func(i, j int) bool { return graph.nodes[i].ID < graph.nodes[j].ID })
	for _, node := range graph.nodes {
		for _, dep := range node.DependsOn {
			if _, ok := graph.byID[dep]; !ok {
				return graph, &MissingDependencyError{NodeID: node.ID, DependencyID: dep}
			}
			graph.dependents[dep] = append(graph.dependents[dep], node.ID)
		}
	}
	for id := range graph.dependents {
		sort.Strings(graph.dependents[id])
	}
	if cycle := detectCycle(graph); len(cycle) > 0 {
		return graph, &CycleDetectedError{CyclePath: cycle}
	}
	return graph, nil
}

type nodeRunResult struct {
	nodeID   string
	state    NodeState
	attempts int
	reason   string
}

func driveNode(ctx context.Context, plan Plan, node Node, defaultMaxAttempts int, runner NodeRunner, txManager TransactionManager, committer OutputCommitter, audit AuditSink) nodeRunResult {
	_ = transition(ctx, audit, plan.BuildID, node.ID, NodeWaiting, NodeRunPending, 0, "dispatching")
	_ = transition(ctx, audit, plan.BuildID, node.ID, NodeRunPending, NodeRunning, 0, "running")
	maxAttempts := node.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = defaultMaxAttempts
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			abortOutputs(context.Background(), txManager, audit, plan.BuildID, node.ID, node.Outputs, "build cancelled")
			_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeRunning, NodeAbortPending, attempt, "build cancelled")
			_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeAbortPending, NodeAborted, attempt, "build cancelled")
			return nodeRunResult{nodeID: node.ID, state: NodeAborted, attempts: attempt, reason: err.Error()}
		}
		_, err := runner.Run(ctx, NodeContext{BuildID: plan.BuildID, BuildBranch: plan.BuildBranch, Node: node, Attempt: attempt})
		if err == nil {
			if err := commitAllOutputs(ctx, committer, txManager, audit, plan.BuildID, node.ID, node.Outputs); err != nil {
				_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeRunning, NodeFailed, attempt, err.Error())
				return nodeRunResult{nodeID: node.ID, state: NodeFailed, attempts: attempt, reason: err.Error()}
			}
			_ = transition(ctx, audit, plan.BuildID, node.ID, NodeRunning, NodeCompleted, attempt, "all outputs committed")
			return nodeRunResult{nodeID: node.ID, state: NodeCompleted, attempts: attempt}
		}
		lastErr = err
		if attempt < maxAttempts && ctx.Err() == nil {
			_ = audit.Record(ctx, AuditEvent{At: time.Now().UTC(), BuildID: plan.BuildID, NodeID: node.ID, Attempt: attempt, Reason: "retry: " + err.Error()})
			continue
		}
	}
	reason := "job failed"
	if lastErr != nil {
		reason = lastErr.Error()
	}
	if ctx.Err() != nil {
		reason = "build cancelled"
		abortOutputs(context.Background(), txManager, audit, plan.BuildID, node.ID, node.Outputs, reason)
		_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeRunning, NodeAbortPending, maxAttempts, reason)
		_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeAbortPending, NodeAborted, maxAttempts, reason)
		return nodeRunResult{nodeID: node.ID, state: NodeAborted, attempts: maxAttempts, reason: reason}
	}
	abortOutputs(context.Background(), txManager, audit, plan.BuildID, node.ID, node.Outputs, reason)
	_ = transition(context.Background(), audit, plan.BuildID, node.ID, NodeRunning, NodeFailed, maxAttempts, reason)
	return nodeRunResult{nodeID: node.ID, state: NodeFailed, attempts: maxAttempts, reason: reason}
}

func commitAllOutputs(ctx context.Context, committer OutputCommitter, txManager TransactionManager, audit AuditSink, buildID uuid.UUID, nodeID string, outputs []OutputTransaction) error {
	committed := map[string]struct{}{}
	var commitErrors []string
	for _, output := range outputs {
		if err := committer.Commit(ctx, output); err != nil {
			commitErrors = append(commitErrors, fmt.Sprintf("%s: %s", output.DatasetRID, err.Error()))
			break
		}
		committed[output.DatasetRID] = struct{}{}
		_ = audit.Record(ctx, AuditEvent{At: time.Now().UTC(), BuildID: buildID, NodeID: nodeID, Reason: "output committed", DatasetRID: output.DatasetRID})
	}
	if len(commitErrors) == 0 {
		return nil
	}
	var rollbackErrors []string
	for _, output := range outputs {
		reason := "output aborted"
		if _, alreadyCommitted := committed[output.DatasetRID]; alreadyCommitted {
			reason = "output rolled back"
		}
		if err := txManager.Abort(context.Background(), output); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Sprintf("%s: %s", output.DatasetRID, err.Error()))
		}
		_ = audit.Record(context.Background(), AuditEvent{At: time.Now().UTC(), BuildID: buildID, NodeID: nodeID, Reason: reason, DatasetRID: output.DatasetRID})
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("multi-output commit failed: %s; rollback failed: %s", joinStrings(commitErrors, "; "), joinStrings(rollbackErrors, "; "))
	}
	return fmt.Errorf("multi-output commit failed: %s", joinStrings(commitErrors, "; "))
}

func abortOutputs(ctx context.Context, txManager TransactionManager, audit AuditSink, buildID uuid.UUID, nodeID string, outputs []OutputTransaction, reason string) {
	for _, output := range outputs {
		_ = txManager.Abort(ctx, output)
		_ = audit.Record(ctx, AuditEvent{At: time.Now().UTC(), BuildID: buildID, NodeID: nodeID, Reason: "output aborted: " + reason, DatasetRID: output.DatasetRID})
	}
}

func abortNode(ctx context.Context, buildID uuid.UUID, node Node, state map[string]NodeState, aborted map[string]struct{}, txManager TransactionManager, audit AuditSink, reason string) {
	from := state[node.ID]
	if from.terminal() {
		return
	}
	if from == NodeWaiting {
		_ = transition(ctx, audit, buildID, node.ID, NodeWaiting, NodeAborted, 0, reason)
	} else {
		_ = transition(ctx, audit, buildID, node.ID, from, NodeAbortPending, 0, reason)
		_ = transition(ctx, audit, buildID, node.ID, NodeAbortPending, NodeAborted, 0, reason)
	}
	abortOutputs(ctx, txManager, audit, buildID, node.ID, node.Outputs, reason)
	state[node.ID] = NodeAborted
	aborted[node.ID] = struct{}{}
}

func transition(ctx context.Context, audit AuditSink, buildID uuid.UUID, nodeID string, from, to NodeState, attempt int, reason string) error {
	return audit.Record(ctx, AuditEvent{At: time.Now().UTC(), BuildID: buildID, NodeID: nodeID, From: from, To: to, Attempt: attempt, Reason: reason})
}

type cascadePlan struct {
	dependents   []string
	independents []string
}

func computeCascade(graph executionGraph, failed string, policy AbortPolicy, completed map[string]struct{}) cascadePlan {
	cascade := cascadePlan{}
	stack := []string{failed}
	visited := map[string]struct{}{}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range graph.dependents[node] {
			if _, ok := visited[child]; !ok {
				visited[child] = struct{}{}
				cascade.dependents = append(cascade.dependents, child)
				stack = append(stack, child)
			}
		}
	}
	sort.Strings(cascade.dependents)
	if policy == AbortAllNonDependent {
		dependentSet := map[string]struct{}{}
		for _, dep := range cascade.dependents {
			dependentSet[dep] = struct{}{}
		}
		for _, node := range graph.nodes {
			if node.ID == failed {
				continue
			}
			if _, isDependent := dependentSet[node.ID]; isDependent {
				continue
			}
			if _, isCompleted := completed[node.ID]; isCompleted {
				continue
			}
			cascade.independents = append(cascade.independents, node.ID)
		}
	}
	return cascade
}

func readyNodes(remaining, completed map[string]struct{}, graph executionGraph) []string {
	ready := make([]string, 0)
	for id := range remaining {
		depsDone := true
		for _, dep := range graph.dependsOn[id] {
			if _, ok := completed[dep]; !ok {
				depsDone = false
				break
			}
		}
		if depsDone {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	return ready
}

func detectCycle(graph executionGraph) []string {
	visited := map[string]struct{}{}
	stack := []string{}
	var dfs func(string) []string
	dfs = func(id string) []string {
		if pos := indexOf(stack, id); pos >= 0 {
			cycle := append([]string(nil), stack[pos:]...)
			cycle = append(cycle, id)
			return cycle
		}
		if _, ok := visited[id]; ok {
			return nil
		}
		visited[id] = struct{}{}
		stack = append(stack, id)
		deps := append([]string(nil), graph.dependsOn[id]...)
		sort.Strings(deps)
		for _, dep := range deps {
			if cycle := dfs(dep); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		return nil
	}
	for _, node := range graph.nodes {
		if cycle := dfs(node.ID); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

func cloneAndSort(items []string) []string {
	if items == nil {
		return nil
	}
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

func sortedSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func indexOf(items []string, needle string) int {
	for i, item := range items {
		if item == needle {
			return i
		}
	}
	return -1
}

func joinPath(items []string) string { return joinStrings(items, " → ") }

func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	out := items[0]
	for _, item := range items[1:] {
		out += sep + item
	}
	return out
}

type noopTransactions struct{}

func (noopTransactions) Abort(context.Context, OutputTransaction) error { return nil }

type noopCommitter struct{}

func (noopCommitter) Commit(context.Context, OutputTransaction) error { return nil }

type noopAudit struct{}

func (noopAudit) Record(context.Context, AuditEvent) error { return nil }
