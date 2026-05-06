package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// --- memory --------------------------------------------------------------

func TestUpdateMemoryAppendsAndCaps(t *testing.T) {
	t.Parallel()
	// Below cap: new note appended.
	current := models.AgentMemorySnapshot{ShortTermNotes: []string{"a", "b", "c"}}
	got := UpdateMemory(current, "new question", "the answer", nil)
	assert.Equal(t, []string{"a", "b", "c", "new question"}, got.ShortTermNotes)
	require.NotNil(t, got.LastRunSummary)
	assert.Equal(t, "the answer", *got.LastRunSummary)

	// At cap: Vec::truncate(6) in Rust drops trailing entries, so the
	// newest note IS dropped when buffer is already full. Verbatim Rust.
	full := models.AgentMemorySnapshot{ShortTermNotes: []string{"a", "b", "c", "d", "e", "f"}}
	gotFull := UpdateMemory(full, "new", "ans", nil)
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f"}, gotFull.ShortTermNotes,
		"cap drops trailing entries (Vec::truncate semantics from Rust)")
}

func TestUpdateMemoryDedupsLongTerm(t *testing.T) {
	t.Parallel()
	current := models.AgentMemorySnapshot{
		LongTermReferences: []string{"doc-1"},
	}
	hits := []models.KnowledgeSearchResult{
		{DocumentTitle: "doc-1"}, // duplicate, should be ignored
		{DocumentTitle: "doc-2"},
	}
	got := UpdateMemory(current, "q", "a", hits)
	assert.Equal(t, []string{"doc-1", "doc-2"}, got.LongTermReferences)
}

func TestTruncateRuneAware(t *testing.T) {
	t.Parallel()
	short := truncate("hello", 10)
	assert.Equal(t, "hello", short, "no ellipsis when under limit")
	long := truncate(strings.Repeat("x", 200), 120)
	assert.Equal(t, 123, len(long), "120 chars + 3-char ellipsis")
	assert.True(t, strings.HasSuffix(long, "..."))
}

// --- planner -------------------------------------------------------------

func TestBuildPlanAlwaysHasAnalyzeAndSynthesize(t *testing.T) {
	t.Parallel()
	agent := models.AgentDefinition{MaxIterations: 1, PlanningStrategy: "plan-act-observe"}
	steps := BuildPlan(agent, "test", nil, nil)
	require.NotEmpty(t, steps)
	assert.Equal(t, "analyze-request", steps[0].ID)
	assert.Equal(t, "synthesize-answer", steps[len(steps)-1].ID)
}

func TestBuildPlanRetrievesContextWhenHits(t *testing.T) {
	t.Parallel()
	agent := models.AgentDefinition{MaxIterations: 0, PlanningStrategy: "plan-act-observe"}
	hits := []models.KnowledgeSearchResult{{DocumentTitle: "x"}}
	steps := BuildPlan(agent, "test", nil, hits)
	found := false
	for _, s := range steps {
		if s.ID == "retrieve-context" {
			found = true
			assert.Contains(t, s.Description, "1 retrieved passage")
			break
		}
	}
	assert.True(t, found)
}

func TestBuildPlanInvokesToolsCappedByIterations(t *testing.T) {
	t.Parallel()
	agent := models.AgentDefinition{MaxIterations: 2, PlanningStrategy: "react"}
	tools := []models.ToolDefinition{
		{Name: "Search Web"},
		{Name: "Calculator"},
		{Name: "Database"}, // should NOT appear; cap is 2
	}
	steps := BuildPlan(agent, "research", tools, nil)
	toolSteps := []models.AgentPlanStep{}
	for _, s := range steps {
		if s.ToolName != nil {
			toolSteps = append(toolSteps, s)
		}
	}
	assert.Len(t, toolSteps, 2)
	assert.Equal(t, "tool-search-web", toolSteps[0].ID, "spaces become hyphens, lowercase")
	assert.Equal(t, "tool-calculator", toolSteps[1].ID)
}

func TestBuildPlanMaxIterationsZeroFallsBackToOne(t *testing.T) {
	t.Parallel()
	agent := models.AgentDefinition{MaxIterations: 0, PlanningStrategy: "react"}
	tools := []models.ToolDefinition{{Name: "OnlyTool"}}
	steps := BuildPlan(agent, "x", tools, nil)
	count := 0
	for _, s := range steps {
		if s.ToolName != nil {
			count++
		}
	}
	assert.Equal(t, 1, count, "max(MaxIterations, 1) gives at least 1 tool slot")
}
