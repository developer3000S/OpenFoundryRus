// Package repo holds SQL queries + embedded migrations for
// audit-compliance-service.
package repo

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded migration in lex order.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

type Repo struct{ Pool *pgxpool.Pool }

type rowLikeT interface{ Scan(...any) error }

// ─── Audit ledger ──────────────────────────────────────────────────

const auditEventSelect = `SELECT id, sequence, previous_hash, entry_hash,
	source_service, channel, actor, action, resource_type, resource_id,
	status, severity, classification, subject_id, ip_address, location,
	metadata, labels, retention_until, occurred_at, ingested_at
	FROM audit_events`

// ListAuditEvents returns the most-recent N events. Append-only — no
// write path on this slice; producers feed the table via the Kafka
// consumer (FASE 6.5+).
func (r *Repo) ListAuditEvents(ctx context.Context, limit int) ([]models.AuditEvent, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.Pool.Query(ctx, auditEventSelect+` ORDER BY sequence DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AuditEvent, 0)
	for rows.Next() {
		var e models.AuditEvent
		if err := rows.Scan(&e.ID, &e.Sequence, &e.PreviousHash, &e.EntryHash,
			&e.SourceService, &e.Channel, &e.Actor, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.Status, &e.Severity,
			&e.Classification, &e.SubjectID, &e.IPAddress, &e.Location,
			&e.Metadata, &e.Labels, &e.RetentionUntil, &e.OccurredAt,
			&e.IngestedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ─── Audit policies (read-only catalog) ────────────────────────────

func (r *Repo) ListAuditPolicies(ctx context.Context) ([]models.AuditPolicy, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, name, description, scope, classification, retention_days,
		        legal_hold, purge_mode, active, rules, updated_by,
		        created_at, updated_at
		   FROM audit_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AuditPolicy, 0)
	for rows.Next() {
		var v models.AuditPolicy
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.Scope,
			&v.Classification, &v.RetentionDays, &v.LegalHold, &v.PurgeMode,
			&v.Active, &v.Rules, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ─── Compliance reports ────────────────────────────────────────────

func (r *Repo) ListComplianceReports(ctx context.Context) ([]models.ComplianceReport, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, standard, title, scope, window_start, window_end,
		        generated_at, status, findings, artifact, relevant_event_count,
		        policy_count, control_summary, expires_at
		   FROM compliance_reports ORDER BY generated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ComplianceReport, 0)
	for rows.Next() {
		var v models.ComplianceReport
		if err := rows.Scan(&v.ID, &v.Standard, &v.Title, &v.Scope,
			&v.WindowStart, &v.WindowEnd, &v.GeneratedAt, &v.Status,
			&v.Findings, &v.Artifact, &v.RelevantEventCount, &v.PolicyCount,
			&v.ControlSummary, &v.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ─── Retention policies ────────────────────────────────────────────

const retentionPolicySelect = `SELECT id, name, scope, target_kind,
	retention_days, legal_hold, purge_mode, rules, updated_by, active,
	is_system, selector, criteria, grace_period_minutes,
	last_applied_at, next_run_at, created_at, updated_at
	FROM retention_policies`

func (r *Repo) ListRetentionPolicies(ctx context.Context) ([]models.RetentionPolicy, error) {
	rows, err := r.Pool.Query(ctx, retentionPolicySelect+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.RetentionPolicy, 0)
	for rows.Next() {
		v, err := scanRetentionPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetRetentionPolicy(ctx context.Context, id uuid.UUID) (*models.RetentionPolicy, error) {
	row := r.Pool.QueryRow(ctx, retentionPolicySelect+` WHERE id = $1`, id)
	v, err := scanRetentionPolicy(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateRetentionPolicy(ctx context.Context, body *models.CreateRetentionPolicyRequest, updatedBy string) (*models.RetentionPolicy, error) {
	id := uuid.New()
	legal := false
	if body.LegalHold != nil {
		legal = *body.LegalHold
	}
	grace := int32(60)
	if body.GracePeriodMinutes != nil {
		grace = *body.GracePeriodMinutes
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO retention_policies
		    (id, name, scope, target_kind, retention_days, legal_hold,
		     purge_mode, rules, updated_by, active, is_system, selector,
		     criteria, grace_period_minutes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, FALSE, $10, $11, $12)
		 RETURNING id, name, scope, target_kind, retention_days, legal_hold,
		           purge_mode, rules, updated_by, active, is_system, selector,
		           criteria, grace_period_minutes, last_applied_at, next_run_at,
		           created_at, updated_at`,
		id, body.Name, body.Scope, body.TargetKind, body.RetentionDays, legal,
		body.PurgeMode, defaultJSON(body.Rules, "[]"), updatedBy,
		defaultJSON(body.Selector, "{}"), defaultJSON(body.Criteria, "{}"), grace,
	)
	return scanRetentionPolicy(row)
}

func (r *Repo) UpdateRetentionPolicy(ctx context.Context, id uuid.UUID, body *models.UpdateRetentionPolicyRequest, updatedBy string) (*models.RetentionPolicy, error) {
	current, err := r.GetRetentionPolicy(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	rd := current.RetentionDays
	if body.RetentionDays != nil {
		rd = *body.RetentionDays
	}
	legal := current.LegalHold
	if body.LegalHold != nil {
		legal = *body.LegalHold
	}
	pm := current.PurgeMode
	if body.PurgeMode != nil {
		pm = *body.PurgeMode
	}
	rules := current.Rules
	if len(body.Rules) > 0 {
		rules = body.Rules
	}
	sel := current.Selector
	if len(body.Selector) > 0 {
		sel = body.Selector
	}
	crit := current.Criteria
	if len(body.Criteria) > 0 {
		crit = body.Criteria
	}
	grace := current.GracePeriodMinutes
	if body.GracePeriodMinutes != nil {
		grace = *body.GracePeriodMinutes
	}
	active := current.Active
	if body.Active != nil {
		active = *body.Active
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE retention_policies SET
		    retention_days = $2, legal_hold = $3, purge_mode = $4,
		    rules = $5, updated_by = $6, selector = $7, criteria = $8,
		    grace_period_minutes = $9, active = $10, updated_at = $11
		  WHERE id = $1
		  RETURNING id, name, scope, target_kind, retention_days, legal_hold,
		            purge_mode, rules, updated_by, active, is_system, selector,
		            criteria, grace_period_minutes, last_applied_at, next_run_at,
		            created_at, updated_at`,
		id, rd, legal, pm, rules, updatedBy, sel, crit, grace, active,
		time.Now().UTC(),
	)
	return scanRetentionPolicy(row)
}

func scanRetentionPolicy(r rowLikeT) (*models.RetentionPolicy, error) {
	v := &models.RetentionPolicy{}
	if err := r.Scan(&v.ID, &v.Name, &v.Scope, &v.TargetKind,
		&v.RetentionDays, &v.LegalHold, &v.PurgeMode, &v.Rules,
		&v.UpdatedBy, &v.Active, &v.IsSystem, &v.Selector, &v.Criteria,
		&v.GracePeriodMinutes, &v.LastAppliedAt, &v.NextRunAt,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func (r *Repo) ListRetentionJobs(ctx context.Context, policyID *uuid.UUID, limit int) ([]models.RetentionJob, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	const sel = `SELECT id, policy_id, target_dataset_id, target_transaction_id,
		status, action_summary, affected_record_count, created_at, completed_at
		FROM retention_jobs`
	var (
		rows pgx.Rows
		err  error
	)
	if policyID != nil {
		rows, err = r.Pool.Query(ctx, sel+` WHERE policy_id = $1 ORDER BY created_at DESC LIMIT $2`, *policyID, limit)
	} else {
		rows, err = r.Pool.Query(ctx, sel+` ORDER BY created_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.RetentionJob, 0)
	for rows.Next() {
		var v models.RetentionJob
		if err := rows.Scan(&v.ID, &v.PolicyID, &v.TargetDatasetID,
			&v.TargetTransactionID, &v.Status, &v.ActionSummary,
			&v.AffectedRecordCount, &v.CreatedAt, &v.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ─── SDS ───────────────────────────────────────────────────────────

func (r *Repo) ListSDSScanJobs(ctx context.Context) ([]models.SDSScanJob, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, target_name, scope, status, risk_score, findings,
		        issue_count, redacted_content, remediations, requested_by,
		        created_at, updated_at
		   FROM sds_scan_jobs ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SDSScanJob, 0)
	for rows.Next() {
		var v models.SDSScanJob
		if err := rows.Scan(&v.ID, &v.TargetName, &v.Scope, &v.Status,
			&v.RiskScore, &v.Findings, &v.IssueCount, &v.RedactedContent,
			&v.Remediations, &v.RequestedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListSDSIssuesByJob(ctx context.Context, jobID uuid.UUID) ([]models.SDSIssue, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, job_id, kind, severity, status, matched_value, redacted_value,
		        match_count, markings, remediation_actions, created_at, updated_at
		   FROM sds_issues WHERE job_id = $1 ORDER BY created_at DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SDSIssue, 0)
	for rows.Next() {
		var v models.SDSIssue
		if err := rows.Scan(&v.ID, &v.JobID, &v.Kind, &v.Severity, &v.Status,
			&v.MatchedValue, &v.RedactedValue, &v.MatchCount, &v.Markings,
			&v.RemediationActions, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) ListSDSRemediationRules(ctx context.Context) ([]models.SDSRemediationRule, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, name, scope, match_conditions, remediation_actions,
		        updated_by, created_at, updated_at
		   FROM sds_remediation_rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SDSRemediationRule, 0)
	for rows.Next() {
		var v models.SDSRemediationRule
		if err := rows.Scan(&v.ID, &v.Name, &v.Scope, &v.MatchConditions,
			&v.RemediationActions, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ─── Lineage deletion ──────────────────────────────────────────────

const ldrSelect = `SELECT id, dataset_id, subject_id, hard_delete, legal_hold,
	impact, status, deleted_paths, audit_trace, requested_at, completed_at
	FROM lineage_deletion_requests`

func (r *Repo) ListLineageDeletionRequests(ctx context.Context, datasetID *uuid.UUID, limit int) ([]models.LineageDeletionRequest, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var (
		rows pgx.Rows
		err  error
	)
	if datasetID != nil {
		rows, err = r.Pool.Query(ctx, ldrSelect+` WHERE dataset_id = $1 ORDER BY requested_at DESC LIMIT $2`, *datasetID, limit)
	} else {
		rows, err = r.Pool.Query(ctx, ldrSelect+` ORDER BY requested_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.LineageDeletionRequest, 0)
	for rows.Next() {
		var v models.LineageDeletionRequest
		if err := rows.Scan(&v.ID, &v.DatasetID, &v.SubjectID, &v.HardDelete,
			&v.LegalHold, &v.Impact, &v.Status, &v.DeletedPaths, &v.AuditTrace,
			&v.RequestedAt, &v.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repo) CreateLineageDeletionRequest(ctx context.Context, body *models.CreateLineageDeletionRequest) (*models.LineageDeletionRequest, error) {
	id := uuid.New()
	hard := false
	if body.HardDelete != nil {
		hard = *body.HardDelete
	}
	legal := false
	if body.LegalHold != nil {
		legal = *body.LegalHold
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO lineage_deletion_requests
		    (id, dataset_id, subject_id, hard_delete, legal_hold, status)
		 VALUES ($1, $2, $3, $4, $5, 'requested')
		 RETURNING id, dataset_id, subject_id, hard_delete, legal_hold, impact,
		           status, deleted_paths, audit_trace, requested_at, completed_at`,
		id, body.DatasetID, body.SubjectID, hard, legal,
	)
	v := &models.LineageDeletionRequest{}
	if err := row.Scan(&v.ID, &v.DatasetID, &v.SubjectID, &v.HardDelete,
		&v.LegalHold, &v.Impact, &v.Status, &v.DeletedPaths, &v.AuditTrace,
		&v.RequestedAt, &v.CompletedAt); err != nil {
		return nil, err
	}
	return v, nil
}

// ─── Saga audit log ────────────────────────────────────────────────

func (r *Repo) ListSagaAuditEvents(ctx context.Context, sagaID *uuid.UUID, limit int) ([]models.SagaAuditEvent, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	const sel = `SELECT event_id, saga_id, saga_name, source_service, kind,
		step_name, payload, correlation_id, tenant_id, observed_at
		FROM audit_compliance.saga_audit_log`
	var (
		rows pgx.Rows
		err  error
	)
	if sagaID != nil {
		rows, err = r.Pool.Query(ctx, sel+` WHERE saga_id = $1 ORDER BY observed_at DESC LIMIT $2`, *sagaID, limit)
	} else {
		rows, err = r.Pool.Query(ctx, sel+` ORDER BY observed_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SagaAuditEvent, 0)
	for rows.Next() {
		var v models.SagaAuditEvent
		if err := rows.Scan(&v.EventID, &v.SagaID, &v.SagaName, &v.SourceService,
			&v.Kind, &v.StepName, &v.Payload, &v.CorrelationID, &v.TenantID,
			&v.ObservedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ─── helpers ───────────────────────────────────────────────────────

func defaultJSON(body json.RawMessage, fallback string) json.RawMessage {
	if len(body) > 0 {
		return body
	}
	return json.RawMessage(fallback)
}
