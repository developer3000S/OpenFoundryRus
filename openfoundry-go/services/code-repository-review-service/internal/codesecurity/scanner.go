package codesecurity

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// ScanRequest is the minimal repository snapshot the code-repository-review
// service can hand to a scanner without taking a dependency on a checkout
// implementation. Runtime adapters can populate Files from a clone, archive or
// diff; tests use it directly.
type ScanRequest struct {
	RepositoryRID string     `json:"repository_rid"`
	BranchRID     string     `json:"branch_rid"`
	CommitSHA     string     `json:"commit_sha"`
	Files         []ScanFile `json:"files"`
}

type ScanFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Finding is the structured finding shape stored inside the JSONB payload of
// code_security_findings.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Evidence string `json:"evidence,omitempty"`
}

type ScanResult struct {
	ScanID   uuid.UUID `json:"scan_id"`
	Findings []Finding `json:"findings"`
}

// Scanner is the integration boundary for code-security engines.
type Scanner interface {
	Scan(ctx context.Context, req ScanRequest) (ScanResult, error)
}

// FakeScanner is a deterministic scanner useful for local runtime smoke tests
// and unit tests. It is intentionally simple but real: it inspects file content
// and emits findings for known insecure markers.
type FakeScanner struct{}

func (FakeScanner) Scan(ctx context.Context, req ScanRequest) (ScanResult, error) {
	select {
	case <-ctx.Done():
		return ScanResult{}, ctx.Err()
	default:
	}
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	result := ScanResult{ScanID: id, Findings: []Finding{}}
	for _, file := range req.Files {
		lines := strings.Split(file.Content, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(line, "TODO_SECURITY") {
				result.Findings = append(result.Findings, Finding{
					RuleID: "fake.todo_security", Severity: "medium",
					Message: "security TODO marker requires review", Path: file.Path,
					Line: i + 1, Evidence: trimmed,
				})
			}
			if strings.Contains(line, "eval(") {
				result.Findings = append(result.Findings, Finding{
					RuleID: "fake.dynamic_eval", Severity: "high",
					Message: "dynamic eval usage requires security review", Path: file.Path,
					Line: i + 1, Evidence: trimmed,
				})
			}
		}
	}
	return result, nil
}

// ScanPayload serializes request metadata into the scan JSONB payload.
func ScanPayload(req ScanRequest) (json.RawMessage, error) {
	b, err := json.Marshal(map[string]any{
		"repository_rid": req.RepositoryRID,
		"branch_rid":     req.BranchRID,
		"commit_sha":     req.CommitSHA,
		"file_count":     len(req.Files),
	})
	return json.RawMessage(b), err
}

// FindingPayload serializes one finding into the finding JSONB payload.
func FindingPayload(f Finding) (json.RawMessage, error) {
	b, err := json.Marshal(f)
	return json.RawMessage(b), err
}
