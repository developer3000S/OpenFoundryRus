package codesecurity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeScannerProducesFindings(t *testing.T) {
	t.Parallel()
	scanner := FakeScanner{}
	result, err := scanner.Scan(context.Background(), ScanRequest{
		RepositoryRID: "ri.repo.1",
		BranchRID:     "ri.branch.1",
		CommitSHA:     "abc123",
		Files:         []ScanFile{{Path: "app.js", Content: "const x = 1\neval(userInput)\n// TODO_SECURITY tighten policy"}},
	})
	require.NoError(t, err)
	require.Len(t, result.Findings, 2)
	assert.Equal(t, "fake.dynamic_eval", result.Findings[0].RuleID)
	assert.Equal(t, "high", result.Findings[0].Severity)
	assert.Equal(t, 2, result.Findings[0].Line)
	assert.Equal(t, "fake.todo_security", result.Findings[1].RuleID)
}
