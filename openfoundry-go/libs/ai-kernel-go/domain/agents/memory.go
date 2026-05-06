// Package agents holds the pure-logic agent runtime helpers
// (memory, planner). The full executor (1307 LOC in Rust) lands
// in its own follow-up slice.
package agents

import (
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// UpdateMemory rolls forward an AgentMemorySnapshot using the
// latest user message + agent response + retrieved knowledge
// citations. Mirrors Rust src/domain/agents/memory.rs verbatim:
//   - short_term_notes: append truncated user message (120 chars),
//     then truncate to 6 entries.
//   - long_term_references: append unique document_titles from
//     knowledge_hits (in order), then truncate to 8 entries.
//   - last_run_summary: truncated final response (180 chars).
func UpdateMemory(
	current models.AgentMemorySnapshot,
	userMessage, finalResponse string,
	knowledgeHits []models.KnowledgeSearchResult,
) models.AgentMemorySnapshot {
	shortTerm := append([]string(nil), current.ShortTermNotes...)
	shortTerm = append(shortTerm, truncate(userMessage, 120))
	if len(shortTerm) > 6 {
		shortTerm = shortTerm[:6]
	}

	longTerm := append([]string(nil), current.LongTermReferences...)
	for _, hit := range knowledgeHits {
		exists := false
		for _, existing := range longTerm {
			if existing == hit.DocumentTitle {
				exists = true
				break
			}
		}
		if !exists {
			longTerm = append(longTerm, hit.DocumentTitle)
		}
	}
	if len(longTerm) > 8 {
		longTerm = longTerm[:8]
	}

	summary := truncate(finalResponse, 180)
	return models.AgentMemorySnapshot{
		ShortTermNotes:     shortTerm,
		LongTermReferences: longTerm,
		LastRunSummary:     &summary,
	}
}

// truncate cuts a string to `limit` runes, appending "..." when
// truncated. Rune-aware to match Rust char iteration.
func truncate(content string, limit int) string {
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "..."
}
