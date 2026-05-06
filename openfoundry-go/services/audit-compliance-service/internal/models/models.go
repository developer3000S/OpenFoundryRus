// Package models holds wire types for audit-compliance-service.
//
// The Rust crate is `fn main() {}` (S8 / B15) so the Go port becomes
// canonical. Three absorbed sub-modules: audit ledger + retention
// policies + sds (sensitive-data scanning) + lineage_deletion + saga
// audit log.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ListResponse is the canonical envelope.
type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// ─── Audit ledger (audit_events, hash-chained, append-only) ────────

type AuditEvent struct {
	ID             uuid.UUID       `json:"id"`
	Sequence       int64           `json:"sequence"`
	PreviousHash   string          `json:"previous_hash"`
	EntryHash      string          `json:"entry_hash"`
	SourceService  string          `json:"source_service"`
	Channel        string          `json:"channel"`
	Actor          string          `json:"actor"`
	Action         string          `json:"action"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	Status         string          `json:"status"`
	Severity       string          `json:"severity"`
	Classification string          `json:"classification"`
	SubjectID      *string         `json:"subject_id"`
	IPAddress      *string         `json:"ip_address"`
	Location       *string         `json:"location"`
	Metadata       json.RawMessage `json:"metadata"`
	Labels         json.RawMessage `json:"labels"`
	RetentionUntil time.Time       `json:"retention_until"`
	OccurredAt     time.Time       `json:"occurred_at"`
	IngestedAt     time.Time       `json:"ingested_at"`
}

// ─── Audit policies (per-classification retention rules) ────────────

type AuditPolicy struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Scope          string          `json:"scope"`
	Classification string          `json:"classification"`
	RetentionDays  int32           `json:"retention_days"`
	LegalHold      bool            `json:"legal_hold"`
	PurgeMode      string          `json:"purge_mode"`
	Active         bool            `json:"active"`
	Rules          json.RawMessage `json:"rules"`
	UpdatedBy      string          `json:"updated_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ─── Compliance reports ────────────────────────────────────────────

type ComplianceReport struct {
	ID                 uuid.UUID       `json:"id"`
	Standard           string          `json:"standard"`
	Title              string          `json:"title"`
	Scope              string          `json:"scope"`
	WindowStart        time.Time       `json:"window_start"`
	WindowEnd          time.Time       `json:"window_end"`
	GeneratedAt        time.Time       `json:"generated_at"`
	Status             string          `json:"status"`
	Findings           json.RawMessage `json:"findings"`
	Artifact           json.RawMessage `json:"artifact"`
	RelevantEventCount int64           `json:"relevant_event_count"`
	PolicyCount        int64           `json:"policy_count"`
	ControlSummary     string          `json:"control_summary"`
	ExpiresAt          time.Time       `json:"expires_at"`
}

// ─── Retention policies + jobs ─────────────────────────────────────

type RetentionPolicy struct {
	ID                  uuid.UUID       `json:"id"`
	Name                string          `json:"name"`
	Scope               string          `json:"scope"`
	TargetKind          string          `json:"target_kind"`
	RetentionDays       int32           `json:"retention_days"`
	LegalHold           bool            `json:"legal_hold"`
	PurgeMode           string          `json:"purge_mode"`
	Rules               json.RawMessage `json:"rules"`
	UpdatedBy           string          `json:"updated_by"`
	Active              bool            `json:"active"`
	IsSystem            bool            `json:"is_system"`
	Selector            json.RawMessage `json:"selector"`
	Criteria            json.RawMessage `json:"criteria"`
	GracePeriodMinutes  int32           `json:"grace_period_minutes"`
	LastAppliedAt       *time.Time      `json:"last_applied_at"`
	NextRunAt           *time.Time      `json:"next_run_at"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type CreateRetentionPolicyRequest struct {
	Name                string          `json:"name"`
	Scope               string          `json:"scope,omitempty"`
	TargetKind          string          `json:"target_kind"`
	RetentionDays       int32           `json:"retention_days"`
	LegalHold           *bool           `json:"legal_hold,omitempty"`
	PurgeMode           string          `json:"purge_mode"`
	Rules               json.RawMessage `json:"rules,omitempty"`
	Selector            json.RawMessage `json:"selector,omitempty"`
	Criteria            json.RawMessage `json:"criteria,omitempty"`
	GracePeriodMinutes  *int32          `json:"grace_period_minutes,omitempty"`
}

type UpdateRetentionPolicyRequest struct {
	RetentionDays      *int32          `json:"retention_days,omitempty"`
	LegalHold          *bool           `json:"legal_hold,omitempty"`
	PurgeMode          *string         `json:"purge_mode,omitempty"`
	Rules              json.RawMessage `json:"rules,omitempty"`
	Selector           json.RawMessage `json:"selector,omitempty"`
	Criteria           json.RawMessage `json:"criteria,omitempty"`
	GracePeriodMinutes *int32          `json:"grace_period_minutes,omitempty"`
	Active             *bool           `json:"active,omitempty"`
}

type RetentionJob struct {
	ID                   uuid.UUID  `json:"id"`
	PolicyID             uuid.UUID  `json:"policy_id"`
	TargetDatasetID      *uuid.UUID `json:"target_dataset_id"`
	TargetTransactionID  *uuid.UUID `json:"target_transaction_id"`
	Status               string     `json:"status"`
	ActionSummary        string     `json:"action_summary"`
	AffectedRecordCount  int32      `json:"affected_record_count"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at"`
}

// ─── SDS (sensitive data scanning) ──────────────────────────────────

type SDSScanJob struct {
	ID              uuid.UUID       `json:"id"`
	TargetName      string          `json:"target_name"`
	Scope           string          `json:"scope"`
	Status          string          `json:"status"`
	RiskScore       int32           `json:"risk_score"`
	Findings        json.RawMessage `json:"findings"`
	IssueCount      int32           `json:"issue_count"`
	RedactedContent string          `json:"redacted_content"`
	Remediations    json.RawMessage `json:"remediations"`
	RequestedBy     *uuid.UUID      `json:"requested_by"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type SDSIssue struct {
	ID                 uuid.UUID       `json:"id"`
	JobID              uuid.UUID       `json:"job_id"`
	Kind               string          `json:"kind"`
	Severity           string          `json:"severity"`
	Status             string          `json:"status"`
	MatchedValue       string          `json:"matched_value"`
	RedactedValue      string          `json:"redacted_value"`
	MatchCount         int32           `json:"match_count"`
	Markings           json.RawMessage `json:"markings"`
	RemediationActions json.RawMessage `json:"remediation_actions"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type SDSRemediationRule struct {
	ID                 uuid.UUID       `json:"id"`
	Name               string          `json:"name"`
	Scope              string          `json:"scope"`
	MatchConditions    json.RawMessage `json:"match_conditions"`
	RemediationActions json.RawMessage `json:"remediation_actions"`
	UpdatedBy          *uuid.UUID      `json:"updated_by"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// ─── Lineage deletion (GDPR / right-to-be-forgotten) ────────────────

type LineageDeletionRequest struct {
	ID            uuid.UUID       `json:"id"`
	DatasetID     uuid.UUID       `json:"dataset_id"`
	SubjectID     *string         `json:"subject_id"`
	HardDelete    bool            `json:"hard_delete"`
	LegalHold     bool            `json:"legal_hold"`
	Impact        json.RawMessage `json:"impact"`
	Status        string          `json:"status"`
	DeletedPaths  json.RawMessage `json:"deleted_paths"`
	AuditTrace    json.RawMessage `json:"audit_trace"`
	RequestedAt   time.Time       `json:"requested_at"`
	CompletedAt   *time.Time      `json:"completed_at"`
}

type CreateLineageDeletionRequest struct {
	DatasetID  uuid.UUID `json:"dataset_id"`
	SubjectID  *string   `json:"subject_id,omitempty"`
	HardDelete *bool     `json:"hard_delete,omitempty"`
	LegalHold  *bool     `json:"legal_hold,omitempty"`
}

// ─── Saga audit log ─────────────────────────────────────────────────

type SagaAuditEvent struct {
	EventID       uuid.UUID       `json:"event_id"`
	SagaID        uuid.UUID       `json:"saga_id"`
	SagaName      string          `json:"saga_name"`
	SourceService string          `json:"source_service"`
	Kind          string          `json:"kind"`
	StepName      *string         `json:"step_name"`
	Payload       json.RawMessage `json:"payload"`
	CorrelationID *uuid.UUID      `json:"correlation_id"`
	TenantID      *string         `json:"tenant_id"`
	ObservedAt    time.Time       `json:"observed_at"`
}
