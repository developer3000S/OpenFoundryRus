// Package codesecurity carries the cross-service constants and scanner
// integration absorbed from the retired `code-security-scanning-service`
// (S8 / ADR-0030).
//
// The basic runtime scanner writes scan/finding rows to the consolidated
// Postgres tables (`code_security_scans`, `code_security_findings`) under
// internal/repo/migrations/. FindingsTopic is preserved for downstream event
// consumers when publish-on-scan is added.
package codesecurity

// FindingsTopic is the canonical Kafka topic the scanner emits to.
// Preserved verbatim from the retired Rust binary so downstream
// consumers continue subscribing with the same name.
const FindingsTopic = "code.security.findings"
