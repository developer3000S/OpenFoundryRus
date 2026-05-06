package agents

import (
	"fmt"
	"strings"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// BuildPlan returns a deterministic plan-act-observe sequence.
// Mirrors Rust src/domain/agents/planner.rs verbatim:
//   1. analyze-request — always first.
//   2. retrieve-context — only when knowledge_hits is non-empty.
//   3. tool-{name} — first N tools where N = max(max_iterations, 1).
//   4. synthesize-answer — always last.
func BuildPlan(
	agent models.AgentDefinition,
	objective string,
	tools []models.ToolDefinition,
	knowledgeHits []models.KnowledgeSearchResult,
) []models.AgentPlanStep {
	steps := []models.AgentPlanStep{
		{
			ID:          "analyze-request",
			Title:       "Analyze user intent",
			Description: fmt.Sprintf("Align the request with agent objective '%s'.", objective),
			Status:      "planned",
		},
	}

	if len(knowledgeHits) > 0 {
		steps = append(steps, models.AgentPlanStep{
			ID:          "retrieve-context",
			Title:       "Retrieve supporting context",
			Description: fmt.Sprintf("Use %d retrieved passage(s) before drafting the answer.", len(knowledgeHits)),
			Status:      "planned",
		})
	}

	maxTools := int(agent.MaxIterations)
	if maxTools < 1 {
		maxTools = 1
	}
	if maxTools > len(tools) {
		maxTools = len(tools)
	}
	for i := 0; i < maxTools; i++ {
		t := tools[i]
		toolName := t.Name
		steps = append(steps, models.AgentPlanStep{
			ID:          fmt.Sprintf("tool-%s", strings.ReplaceAll(strings.ToLower(t.Name), " ", "-")),
			Title:       fmt.Sprintf("Invoke %s", t.Name),
			Description: t.Description,
			ToolName:    &toolName,
			Status:      "planned",
		})
	}

	steps = append(steps, models.AgentPlanStep{
		ID:          "synthesize-answer",
		Title:       "Synthesize final answer",
		Description: fmt.Sprintf("Use planning strategy '%s' and produce an operator-friendly summary.", agent.PlanningStrategy),
		Status:      "planned",
	})

	return steps
}
