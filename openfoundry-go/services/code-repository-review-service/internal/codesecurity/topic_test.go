package codesecurity

import "testing"

func TestFindingsTopicVerbatim(t *testing.T) {
	t.Parallel()
	if FindingsTopic != "code.security.findings" {
		t.Fatalf("FindingsTopic must remain 'code.security.findings' (preserved from Rust); got %q", FindingsTopic)
	}
}
