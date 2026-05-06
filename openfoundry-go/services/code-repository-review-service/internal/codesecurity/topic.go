// Package codesecurity carries the cross-service constants absorbed
// from the retired `code-security-scanning-service` (S8 / ADR-0030).
//
// The scanner integration is not yet wired; future work emits findings
// to FindingsTopic. The Postgres tables (`code_security_scans`,
// `code_security_findings`) live in the consolidated migration set
// under internal/repo/migrations/.
package codesecurity

// FindingsTopic is the canonical Kafka topic the scanner emits to.
// Preserved verbatim from the retired Rust binary so downstream
// consumers continue subscribing with the same name.
const FindingsTopic = "code.security.findings"
