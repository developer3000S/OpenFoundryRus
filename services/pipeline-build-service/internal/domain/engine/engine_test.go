// Tests for the legacy engine package — DAG sort, cycle detection, execution
// stages, skip-unchanged + retry semantics, distributed worker annotation, and
// the lightweight passthrough runtime path used by compatibility callers.
package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func n(id string, deps ...string) PipelineNode {
	return models.PipelineNode{
		ID:            id,
		Label:         id,
		TransformType: "passthrough",
		Config:        json.RawMessage(`{}`),
		DependsOn:     deps,
	}
}

// ── Topological sort ───────────────────────────────────────────────

func TestTopologicalSortStraightLine(t *testing.T) {
	t.Parallel()
	nodes := []PipelineNode{n("a"), n("b", "a"), n("c", "b")}
	order, err := topologicalSort(nodes)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	idx := map[string]int{}
	for i, id := range order {
		idx[id] = i
	}
	if !(idx["a"] < idx["b"] && idx["b"] < idx["c"]) {
		t.Errorf("topo violated: %v", order)
	}
}

func TestTopologicalSortDetectsCycle(t *testing.T) {
	t.Parallel()
	nodes := []PipelineNode{n("a", "b"), n("b", "a")}
	if _, err := topologicalSort(nodes); err == nil {
		t.Fatal("expected cycle detected error")
	} else if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestExecutionOrderRespectsStartFromNode(t *testing.T) {
	t.Parallel()
	nodes := []PipelineNode{n("a"), n("b", "a"), n("c", "a"), n("d", "b", "c")}
	startB := "b"
	order, err := executionOrder(nodes, &startB)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, id := range order {
		if id == "a" || id == "c" {
			t.Errorf("start_from=b should exclude %s, got order=%v", id, order)
		}
	}
}

func TestExecutionOrderUnknownStartNodeErrors(t *testing.T) {
	t.Parallel()
	missing := "nope"
	if _, err := executionOrder([]PipelineNode{n("a")}, &missing); err == nil {
		t.Fatal("expected error for unknown start node")
	}
}

// ── Execution stages (distributed worker planner) ──────────────────

func TestExecutionStagesGroupsIndependentNodes(t *testing.T) {
	t.Parallel()
	nodes := []PipelineNode{
		n("a"),
		n("b", "a"),
		n("c", "a"),
		n("d", "b", "c"),
	}
	stages, err := executionStages(nodes, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Expected waves: [a], [b, c], [d]
	if len(stages) != 3 {
		t.Fatalf("expected 3 stages, got %d: %v", len(stages), stages)
	}
	if !sliceEq(stages[0], []string{"a"}) {
		t.Errorf("stage 0 drift: %v", stages[0])
	}
	got := append([]string(nil), stages[1]...)
	sort.Strings(got)
	if !sliceEq(got, []string{"b", "c"}) {
		t.Errorf("stage 1 drift: %v", got)
	}
	if !sliceEq(stages[2], []string{"d"}) {
		t.Errorf("stage 2 drift: %v", stages[2])
	}
}

func TestExecutionStagesCycleErrors(t *testing.T) {
	t.Parallel()
	if _, err := executionStages([]PipelineNode{n("a", "b"), n("b", "a")}, nil); err == nil {
		t.Fatal("expected cycle error")
	}
}

// ── Reachability ───────────────────────────────────────────────────

func TestReachableNodesTransitiveClosure(t *testing.T) {
	t.Parallel()
	nodes := []PipelineNode{n("a"), n("b", "a"), n("c", "b"), n("d", "c"), n("e")}
	reach := reachableNodes(nodes, "a")
	if len(reach) != 4 {
		t.Errorf("expected {a,b,c,d}, got %v", reach)
	}
	if _, ok := reach["e"]; ok {
		t.Errorf("e is independent, must not be reachable")
	}
}

// ── Fingerprint stability ──────────────────────────────────────────

func TestNodeFingerprintIsStableAcrossEqualInputs(t *testing.T) {
	t.Parallel()
	node := n("x")
	a := nodeFingerprint(&node, nil, nil)
	b := nodeFingerprint(&node, nil, nil)
	if a != b {
		t.Errorf("fingerprint must be deterministic, got %s vs %s", a, b)
	}
}

func TestNodeFingerprintChangesWhenConfigChanges(t *testing.T) {
	t.Parallel()
	a := n("x")
	a.Config = json.RawMessage(`{"sql":"SELECT 1"}`)
	b := n("x")
	b.Config = json.RawMessage(`{"sql":"SELECT 2"}`)
	if nodeFingerprint(&a, nil, nil) == nodeFingerprint(&b, nil, nil) {
		t.Fatal("fingerprint must reflect config changes")
	}
}

func TestNodeFingerprintIncludesDependencyFingerprints(t *testing.T) {
	t.Parallel()
	node := n("x", "dep")
	deps1 := map[string]string{"dep": "v1"}
	deps2 := map[string]string{"dep": "v2"}
	if nodeFingerprint(&node, nil, deps1) == nodeFingerprint(&node, nil, deps2) {
		t.Fatal("fingerprint must reflect dependency fingerprint changes")
	}
}

// ── End-to-end ExecutePipeline runtime dispatch ────────────────────

func TestExecutePipelinePassthroughCompletes(t *testing.T) {
	t.Parallel()
	env := &ExecutionEnvironment{ActorID: uuid.New()}
	results, err := ExecutePipeline(context.Background(), env, []PipelineNode{n("a")}, nil)
	if err != nil {
		t.Fatalf("orchestrator err: %v", err)
	}
	if len(results) != 1 || results[0].Status != "completed" {
		t.Fatalf("expected single completed result, got %+v", results)
	}
	if results[0].Error != nil {
		t.Errorf("passthrough should not fail, got %+v", results[0].Error)
	}
	if !strings.Contains(string(results[0].Output), "lightweight_table") {
		t.Errorf("output must identify lightweight runtime, got %s", results[0].Output)
	}
}

func TestExecutePipelineHonoursSkipUnchanged(t *testing.T) {
	t.Parallel()
	node := n("a")
	node.TransformType = "sql"
	node.Config = json.RawMessage(`{"sql":"SELECT 1"}`)

	// Compute the fingerprint the engine would derive then feed it
	// back as a prior result so the skip-unchanged branch fires.
	fp := nodeFingerprint(&node, nil, map[string]string{})
	priorMeta, _ := json.Marshal(NodeExecutionMetadata{Fingerprint: fp})

	priors := map[string]NodeResult{
		"a": {NodeID: "a", Metadata: priorMeta},
	}
	req := DefaultExecutionRequest()
	req.PriorNodeResults = priors
	results, err := ExecutePipeline(context.Background(), &ExecutionEnvironment{}, []PipelineNode{node}, &req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 || results[0].Status != "skipped" {
		t.Fatalf("expected skipped, got %+v", results)
	}
}

func TestExecutePipelineFailureCutsRunShort(t *testing.T) {
	t.Parallel()
	env := &ExecutionEnvironment{}
	a := n("a")
	a.TransformType = "llm"
	results, err := ExecutePipeline(context.Background(), env,
		[]PipelineNode{a, n("b", "a"), n("c", "b")}, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result before failure cut-off, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Errorf("expected first result to be failed, got %s", results[0].Status)
	}
}

// ── Distributed annotation ─────────────────────────────────────────

func TestExecutePipelineDistributedAnnotatesWorkerAndStage(t *testing.T) {
	t.Parallel()
	req := DefaultExecutionRequest()
	req.DistributedWorkerCount = 4
	results, err := ExecutePipeline(context.Background(), &ExecutionEnvironment{},
		[]PipelineNode{n("a")}, &req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	r := results[0]
	if r.WorkerID == nil || *r.WorkerID != "pipeline-worker-1" {
		t.Errorf("worker_id drift: %+v", r.WorkerID)
	}
	if r.StageIndex == nil || *r.StageIndex != 0 {
		t.Errorf("stage_index drift: %+v", r.StageIndex)
	}
}

func TestAnnotateOutputMergesIntoJSONObject(t *testing.T) {
	t.Parallel()
	r := &NodeResult{Output: json.RawMessage(`{"rows":42}`)}
	annotateOutput(r, 1, 2)
	var asMap map[string]any
	if err := json.Unmarshal(r.Output, &asMap); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if asMap["rows"].(float64) != 42 {
		t.Errorf("existing keys preserved drift: %v", asMap)
	}
	exec, ok := asMap["execution"].(map[string]any)
	if !ok {
		t.Fatalf("execution envelope missing: %v", asMap)
	}
	if exec["worker_id"] != "pipeline-worker-3" {
		t.Errorf("worker_id drift: %v", exec)
	}
	if exec["stage_index"].(float64) != 1 {
		t.Errorf("stage_index drift: %v", exec)
	}
}

func TestAnnotateOutputWrapsPrimitives(t *testing.T) {
	t.Parallel()
	r := &NodeResult{Output: json.RawMessage(`42`)}
	annotateOutput(r, 0, 0)
	var asMap map[string]any
	if err := json.Unmarshal(r.Output, &asMap); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if asMap["result"].(float64) != 42 {
		t.Errorf("primitive must wrap under result key, got %v", asMap)
	}
}

func TestAnnotateOutputCreatesEnvelopeWhenEmpty(t *testing.T) {
	t.Parallel()
	r := &NodeResult{}
	annotateOutput(r, 2, 3)
	var asMap map[string]any
	_ = json.Unmarshal(r.Output, &asMap)
	exec, _ := asMap["execution"].(map[string]any)
	if exec["worker_id"] != "pipeline-worker-4" {
		t.Errorf("worker_id drift: %v", exec)
	}
}

// ── helpers ────────────────────────────────────────────────────────

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
