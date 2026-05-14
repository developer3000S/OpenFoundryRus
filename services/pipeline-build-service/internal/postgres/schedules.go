package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

const scheduleSelectColumns = `id, rid, project_rid, folder_rid, name, description,
trigger_json, target_json, target_rids, branch, build_strategy,
paused, paused_reason, paused_at, auto_pause_exempt, pending_re_run, pending_trigger_snapshot, active_run_id::text, last_triggered_at,
version, created_by, created_at, updated_at, last_run_at,
scope_kind, project_scope_rids, run_as_user_id::text, service_principal_id::text,
run_as_identity, last_updated_by`

type scheduleRow struct {
	item               models.Schedule
	folderRID          sql.NullString
	pausedReason       sql.NullString
	pausedAt           sql.NullTime
	pendingTriggerRaw  []byte
	activeRunID        sql.NullString
	lastTriggeredAt    sql.NullTime
	lastRunAt          sql.NullTime
	scopeKind          string
	runAsUserID        sql.NullString
	servicePrincipalID sql.NullString
	runAsIdentity      sql.NullString
}

func (row *scheduleRow) dest() []any {
	return []any{
		&row.item.ID, &row.item.RID, &row.item.ProjectRID, &row.folderRID, &row.item.Name, &row.item.Description,
		&row.item.Trigger, &row.item.Target, &row.item.TargetRIDs, &row.item.Branch, &row.item.BuildStrategy,
		&row.item.Paused, &row.pausedReason, &row.pausedAt, &row.item.AutoPauseExempt, &row.item.PendingReRun, &row.pendingTriggerRaw, &row.activeRunID, &row.lastTriggeredAt,
		&row.item.Version, &row.item.CreatedBy, &row.item.CreatedAt, &row.item.UpdatedAt, &row.lastRunAt,
		&row.scopeKind, &row.item.ProjectScopeRIDs, &row.runAsUserID, &row.servicePrincipalID,
		&row.runAsIdentity, &row.item.LastUpdatedBy,
	}
}

func (row *scheduleRow) model() (*models.Schedule, error) {
	if row.folderRID.Valid {
		row.item.FolderRID = &row.folderRID.String
	}
	if row.pausedReason.Valid {
		row.item.PausedReason = &row.pausedReason.String
	}
	if row.pausedAt.Valid {
		t := row.pausedAt.Time
		row.item.PausedAt = &t
	}
	if row.activeRunID.Valid {
		id, err := uuid.Parse(row.activeRunID.String)
		if err != nil {
			return nil, fmt.Errorf("decode schedule active_run_id: %w", err)
		}
		row.item.ActiveRunID = &id
	}
	if len(row.pendingTriggerRaw) > 0 && string(row.pendingTriggerRaw) != "null" {
		row.item.PendingTrigger = map[string]string{}
		_ = json.Unmarshal(row.pendingTriggerRaw, &row.item.PendingTrigger)
	}
	if row.lastTriggeredAt.Valid {
		t := row.lastTriggeredAt.Time
		row.item.LastTriggeredAt = &t
	}
	if row.lastRunAt.Valid {
		t := row.lastRunAt.Time
		row.item.LastRunAt = &t
	}
	if row.scopeKind == "" {
		row.scopeKind = string(models.ScheduleScopeUser)
	}
	row.item.ScopeKind = models.ScheduleScopeKind(row.scopeKind)
	if row.runAsUserID.Valid {
		id, err := uuid.Parse(row.runAsUserID.String)
		if err != nil {
			return nil, fmt.Errorf("decode schedule run_as_user_id: %w", err)
		}
		row.item.RunAsUserID = &id
	}
	if row.servicePrincipalID.Valid {
		id, err := uuid.Parse(row.servicePrincipalID.String)
		if err != nil {
			return nil, fmt.Errorf("decode schedule service_principal_id: %w", err)
		}
		row.item.ServicePrincipalID = &id
	}
	if row.runAsIdentity.Valid {
		row.item.RunAsIdentity = &row.runAsIdentity.String
	}
	if row.item.TargetRIDs == nil {
		row.item.TargetRIDs = []string{}
	}
	if row.item.ProjectScopeRIDs == nil {
		row.item.ProjectScopeRIDs = []string{}
	}
	if len(row.item.Trigger) == 0 {
		row.item.Trigger = json.RawMessage(`{}`)
	}
	if len(row.item.Target) == 0 {
		row.item.Target = json.RawMessage(`{}`)
	}
	if row.item.Owner == "" {
		row.item.Owner = row.item.CreatedBy
	}
	return &row.item, nil
}

func scanSchedule(scanner interface{ Scan(dest ...any) error }) (*models.Schedule, error) {
	var row scheduleRow
	if err := scanner.Scan(row.dest()...); err != nil {
		return nil, err
	}
	return row.model()
}

func (r *Repository) ListSchedules(ctx context.Context, query models.ListSchedulesQuery) (models.ListSchedulesResponse, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	paused := ""
	if query.Paused != nil {
		paused = fmt.Sprintf("%t", *query.Paused)
	}
	orderBy := scheduleOrderBy(query.Sort)
	q := `SELECT ` + scheduleSelectColumns + `, COUNT(*) OVER()
FROM schedules
WHERE ($1 = '' OR project_rid = $1)
  AND ($2 = '' OR paused = $2::boolean)
  AND ($3 = '' OR created_by = $3)
  AND (cardinality($4::text[]) = 0 OR created_by = ANY($4::text[]) OR last_updated_by = ANY($4::text[]))
  AND (cardinality($5::text[]) = 0 OR project_rid = ANY($5::text[]) OR project_scope_rids && $5::text[])
  AND (cardinality($6::text[]) = 0 OR target_rids && $6::text[])
  AND ($7 = '' OR name ILIKE '%' || $7 || '%' OR rid ILIKE '%' || $7 || '%')
ORDER BY ` + orderBy + `
LIMIT $8 OFFSET $9`
	rows, err := r.db.Query(ctx, q, query.Project, paused, query.Owner, uniqueStrings(query.Users), uniqueStrings(query.Projects), uniqueStrings(query.Files), query.Q, query.Limit, query.Offset)
	if err != nil {
		return models.ListSchedulesResponse{}, err
	}
	defer rows.Close()
	out := models.ListSchedulesResponse{Data: []models.Schedule{}}
	for rows.Next() {
		var row scheduleRow
		var total int
		dest := append(row.dest(), &total)
		if err := rows.Scan(dest...); err != nil {
			return models.ListSchedulesResponse{}, err
		}
		item, err := row.model()
		if err != nil {
			return models.ListSchedulesResponse{}, err
		}
		out.Data = append(out.Data, *item)
		out.Total = total
	}
	return out, rows.Err()
}

func scheduleOrderBy(sortKey string) string {
	switch sortKey {
	case "name":
		return "name ASC, updated_at DESC"
	case "created_at":
		return "created_at DESC"
	case "last_run_at":
		return "last_run_at DESC NULLS LAST, updated_at DESC"
	default:
		return "updated_at DESC"
	}
}

func (r *Repository) CreateSchedule(ctx context.Context, req models.CreateScheduleRequest, actor string) (*models.Schedule, error) {
	id := uuid.New()
	if actor == "" {
		actor = "system"
	}
	trigger, err := normalizeScheduleJSON(req.Trigger, defaultScheduleTrigger())
	if err != nil {
		return nil, fmt.Errorf("invalid trigger: %w", err)
	}
	target, err := normalizeScheduleJSON(req.Target, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid target: %w", err)
	}
	if len(target) == 0 {
		return nil, errors.New("target is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Untitled schedule"
	}
	projectRID := strings.TrimSpace(req.ProjectRID)
	if projectRID == "" {
		projectRID = "ri.foundry.main.project.default"
	}
	scopeKind := req.ScopeKind
	if scopeKind == "" {
		scopeKind = models.ScheduleScopeUser
	}
	branch := firstNonEmptyString(req.Branch, deriveScheduleBranch(target), "master")
	buildStrategy := firstNonEmptyString(req.BuildStrategy, deriveScheduleBuildStrategy(target), "STALE_ONLY")
	targetRIDs := models.ScheduleResourceRIDs(trigger, target)
	runAsIdentity := req.RunAsIdentity
	if runAsIdentity == nil && req.RunAsUserID != nil {
		value := req.RunAsUserID.String()
		runAsIdentity = &value
	}
	var pausedReason *string
	var pausedAt *time.Time
	if req.Paused {
		reason := "MANUAL"
		now := time.Now().UTC()
		pausedReason = &reason
		pausedAt = &now
	}

	return scanSchedule(r.db.QueryRow(ctx, `INSERT INTO schedules (
    id, project_rid, folder_rid, name, description,
    trigger_json, target_json, target_rids, branch, build_strategy,
    paused, paused_reason, paused_at, created_by, last_updated_by,
    scope_kind, project_scope_rids, run_as_user_id, service_principal_id, run_as_identity
) VALUES (
    $1,$2,$3,$4,$5,
    $6,$7,$8,$9,$10,
    $11,$12,$13,$14,$14,
    $15,$16,$17,$18,$19
)
RETURNING `+scheduleSelectColumns,
		id, projectRID, cleanStringPtr(req.FolderRID), name, req.Description,
		trigger, target, targetRIDs, branch, buildStrategy,
		req.Paused, pausedReason, pausedAt, actor,
		string(scopeKind), uniqueStrings(req.ProjectScopeRIDs), req.RunAsUserID, req.ServicePrincipalID, runAsIdentity,
	))
}

func (r *Repository) GetSchedule(ctx context.Context, rid string) (*models.Schedule, error) {
	item, err := scanSchedule(r.db.QueryRow(ctx, `SELECT `+scheduleSelectColumns+` FROM schedules WHERE rid=$1 OR id::text=$1`, rid))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) PatchSchedule(ctx context.Context, rid string, req models.PatchScheduleRequest, actor string) (*models.Schedule, error) {
	current, err := r.GetSchedule(ctx, rid)
	if err != nil || current == nil {
		return current, err
	}
	if actor == "" {
		actor = "system"
	}
	next := *current
	if req.ProjectRID != nil && strings.TrimSpace(*req.ProjectRID) != "" {
		next.ProjectRID = strings.TrimSpace(*req.ProjectRID)
	}
	if req.FolderRID != nil {
		next.FolderRID = cleanStringPtr(req.FolderRID)
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		next.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		next.Description = *req.Description
	}
	if len(req.Trigger) > 0 && string(req.Trigger) != "null" {
		next.Trigger, err = normalizeScheduleJSON(req.Trigger, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid trigger: %w", err)
		}
	}
	if len(req.Target) > 0 && string(req.Target) != "null" {
		next.Target, err = normalizeScheduleJSON(req.Target, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid target: %w", err)
		}
	}
	if req.Branch != nil && strings.TrimSpace(*req.Branch) != "" {
		next.Branch = strings.TrimSpace(*req.Branch)
	} else {
		next.Branch = firstNonEmptyString(next.Branch, deriveScheduleBranch(next.Target), "master")
	}
	if req.BuildStrategy != nil && strings.TrimSpace(*req.BuildStrategy) != "" {
		next.BuildStrategy = strings.TrimSpace(*req.BuildStrategy)
	} else {
		next.BuildStrategy = firstNonEmptyString(next.BuildStrategy, deriveScheduleBuildStrategy(next.Target), "STALE_ONLY")
	}
	if req.ScopeKind != nil && *req.ScopeKind != "" {
		next.ScopeKind = *req.ScopeKind
	}
	if req.ProjectScopeRIDs != nil {
		next.ProjectScopeRIDs = uniqueStrings(req.ProjectScopeRIDs)
	}
	if req.RunAsUserID != nil {
		next.RunAsUserID = req.RunAsUserID
	}
	if req.ServicePrincipalID != nil {
		next.ServicePrincipalID = req.ServicePrincipalID
	}
	if req.RunAsIdentity != nil {
		next.RunAsIdentity = cleanStringPtr(req.RunAsIdentity)
	}
	if req.Paused != nil {
		next.Paused = *req.Paused
		if next.Paused {
			if next.PausedReason == nil {
				reason := "MANUAL"
				next.PausedReason = &reason
			}
			if next.PausedAt == nil {
				now := time.Now().UTC()
				next.PausedAt = &now
			}
		} else {
			next.PausedReason = nil
			next.PausedAt = nil
		}
	}
	next.TargetRIDs = models.ScheduleResourceRIDs(next.Trigger, next.Target)
	defChanged := !jsonEqual(current.Trigger, next.Trigger) ||
		!jsonEqual(current.Target, next.Target) ||
		current.Name != next.Name ||
		current.Description != next.Description
	if defChanged {
		next.Version = current.Version + 1
	}
	return r.updateSchedule(ctx, &next, actor, req.ChangeComment)
}

func (r *Repository) updateSchedule(ctx context.Context, next *models.Schedule, actor, comment string) (*models.Schedule, error) {
	db := r.db
	var tx pgx.Tx
	if beginner, ok := r.db.(txBeginner); ok {
		var err error
		tx, err = beginner.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		db = tx
		_, _ = db.Exec(ctx, `SELECT set_config('app.editor', $1, true), set_config('app.change_comment', $2, true)`, actor, comment)
	}
	updated, err := scanSchedule(db.QueryRow(ctx, `UPDATE schedules
SET project_rid=$2,
    folder_rid=$3,
    name=$4,
    description=$5,
    trigger_json=$6,
    target_json=$7,
    target_rids=$8,
    branch=$9,
    build_strategy=$10,
    paused=$11,
    paused_reason=$12,
    paused_at=$13,
    version=$14,
    scope_kind=$15,
    project_scope_rids=$16,
    run_as_user_id=$17,
    service_principal_id=$18,
    run_as_identity=$19,
    last_updated_by=$20,
    updated_at=NOW()
WHERE rid=$1 OR id::text=$1
RETURNING `+scheduleSelectColumns,
		next.RID, next.ProjectRID, next.FolderRID, next.Name, next.Description,
		next.Trigger, next.Target, uniqueStrings(next.TargetRIDs), next.Branch, next.BuildStrategy,
		next.Paused, next.PausedReason, next.PausedAt, next.Version,
		string(next.ScopeKind), uniqueStrings(next.ProjectScopeRIDs), next.RunAsUserID,
		next.ServicePrincipalID, next.RunAsIdentity, actor,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func (r *Repository) PauseSchedule(ctx context.Context, rid, reason, actor string) (*models.Schedule, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "MANUAL"
	}
	if actor == "" {
		actor = "system"
	}
	item, err := scanSchedule(r.db.QueryRow(ctx, `UPDATE schedules
SET paused=TRUE, paused_reason=$2, paused_at=COALESCE(paused_at, NOW()), last_updated_by=$3, updated_at=NOW()
WHERE rid=$1 OR id::text=$1
RETURNING `+scheduleSelectColumns, rid, reason, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ResumeSchedule(ctx context.Context, rid, actor string) (*models.Schedule, error) {
	if actor == "" {
		actor = "system"
	}
	item, err := scanSchedule(r.db.QueryRow(ctx, `UPDATE schedules
SET paused=FALSE, paused_reason=NULL, paused_at=NULL, last_updated_by=$2, updated_at=NOW()
WHERE rid=$1 OR id::text=$1
RETURNING `+scheduleSelectColumns, rid, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) DeleteSchedule(ctx context.Context, rid string) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM schedules WHERE rid=$1 OR id::text=$1`, rid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) SetScheduleAutoPauseExempt(ctx context.Context, rid string, exempt bool, actor string) (*models.Schedule, error) {
	if actor == "" {
		actor = "system"
	}
	item, err := scanSchedule(r.db.QueryRow(ctx, `UPDATE schedules
SET auto_pause_exempt=$2, last_updated_by=$3, updated_at=NOW()
WHERE rid=$1 OR id::text=$1
RETURNING `+scheduleSelectColumns, rid, exempt, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) RunScheduleNow(ctx context.Context, rid, actor string) (models.ScheduleRunNowResponse, error) {
	if actor == "" {
		actor = "system"
	}
	item, err := r.GetSchedule(ctx, rid)
	if err != nil {
		return models.ScheduleRunNowResponse{}, err
	}
	if item == nil {
		return models.ScheduleRunNowResponse{}, pgx.ErrNoRows
	}
	result, err := r.dispatchSchedule(ctx, item, scheduleDispatchInput{
		Actor:       actor,
		TriggerType: "MANUAL",
		Now:         time.Now().UTC(),
		Snapshot: map[string]string{
			"type":         "manual",
			"requested_by": actor,
		},
		Diagnostics: map[string]string{"source": "run_now"},
	})
	if err != nil {
		return models.ScheduleRunNowResponse{}, err
	}
	return models.ScheduleRunNowResponse{RunID: result.RunRID, ScheduleRID: item.RID, RequestedBy: actor}, nil
}

func (r *Repository) ListScheduleRuns(ctx context.Context, rid string, query models.ListScheduleRunsQuery) (models.ListScheduleRunsResponse, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	rows, err := r.db.Query(ctx, `SELECT sr.id, sr.rid, sr.schedule_id, sr.outcome, sr.build_rid, sr.failure_reason,
       sr.triggered_at, sr.finished_at, sr.trigger_snapshot, sr.trigger_type, sr.diagnostics, sr.schedule_version, COUNT(*) OVER()
FROM schedule_runs sr
JOIN schedules s ON s.id=sr.schedule_id
WHERE (s.rid=$1 OR s.id::text=$1)
  AND ($2='' OR sr.outcome=$2)
ORDER BY sr.triggered_at DESC
LIMIT $3 OFFSET $4`, rid, query.Outcome, query.Limit, query.Offset)
	if err != nil {
		return models.ListScheduleRunsResponse{}, err
	}
	defer rows.Close()
	out := models.ListScheduleRunsResponse{ScheduleRID: rid, Data: []models.ScheduleRun{}}
	for rows.Next() {
		var item models.ScheduleRun
		var raw, diagnosticsRaw []byte
		var total int
		if err := rows.Scan(&item.ID, &item.RID, &item.ScheduleID, &item.Outcome, &item.BuildRID, &item.FailureReason, &item.TriggeredAt, &item.FinishedAt, &raw, &item.TriggerType, &diagnosticsRaw, &item.ScheduleVersion, &total); err != nil {
			return models.ListScheduleRunsResponse{}, err
		}
		item.TriggerSnapshot = map[string]string{}
		_ = json.Unmarshal(raw, &item.TriggerSnapshot)
		item.Diagnostics = map[string]string{}
		_ = json.Unmarshal(diagnosticsRaw, &item.Diagnostics)
		out.Data = append(out.Data, item)
		out.Total = total
	}
	return out, rows.Err()
}

func (r *Repository) ListScheduleVersions(ctx context.Context, rid string, limit, offset int64) (models.ListScheduleVersionsResponse, error) {
	current, err := r.GetSchedule(ctx, rid)
	if err != nil || current == nil {
		return models.ListScheduleVersionsResponse{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.Query(ctx, scheduleVersionsSQL()+` ORDER BY version DESC LIMIT $2 OFFSET $3`, rid, limit, offset)
	if err != nil {
		return models.ListScheduleVersionsResponse{}, err
	}
	defer rows.Close()
	out := models.ListScheduleVersionsResponse{ScheduleRID: current.RID, CurrentVersion: current.Version, Data: []models.ScheduleVersion{}}
	for rows.Next() {
		item, err := scanScheduleVersion(rows)
		if err != nil {
			return models.ListScheduleVersionsResponse{}, err
		}
		out.Data = append(out.Data, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetScheduleVersion(ctx context.Context, rid string, version int) (*models.ScheduleVersion, error) {
	item, err := scanScheduleVersion(r.db.QueryRow(ctx, scheduleVersionsSQL()+` AND version=$2 LIMIT 1`, rid, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func scheduleVersionsSQL() string {
	return `WITH selected AS (
    SELECT * FROM schedules WHERE rid=$1 OR id::text=$1
)
SELECT id, schedule_id, version, name, description, trigger_json, target_json, edited_by, edited_at, comment
FROM (
    SELECT id, id AS schedule_id, version, name, description, trigger_json, target_json,
           last_updated_by AS edited_by, updated_at AS edited_at, 'Current definition' AS comment
    FROM selected
    UNION ALL
    SELECT sv.id, sv.schedule_id, sv.version, sv.name, sv.description, sv.trigger_json, sv.target_json,
           sv.edited_by, sv.edited_at, sv.comment
    FROM schedule_versions sv
    JOIN selected s ON s.id=sv.schedule_id
) versions
WHERE TRUE`
}

func scanScheduleVersion(scanner interface{ Scan(dest ...any) error }) (*models.ScheduleVersion, error) {
	var item models.ScheduleVersion
	if err := scanner.Scan(&item.ID, &item.ScheduleID, &item.Version, &item.Name, &item.Description, &item.TriggerJSON, &item.TargetJSON, &item.EditedBy, &item.EditedAt, &item.Comment); err != nil {
		return nil, err
	}
	if len(item.TriggerJSON) == 0 {
		item.TriggerJSON = json.RawMessage(`{}`)
	}
	if len(item.TargetJSON) == 0 {
		item.TargetJSON = json.RawMessage(`{}`)
	}
	return &item, nil
}

func (r *Repository) ConvertScheduleToProjectScope(ctx context.Context, rid string, req models.ConvertScheduleToProjectScopeRequest, actor string) (*models.ConvertScheduleToProjectScopeResponse, error) {
	current, err := r.GetSchedule(ctx, rid)
	if err != nil || current == nil {
		return nil, err
	}
	if actor == "" {
		actor = "system"
	}
	if len(req.ProjectScopeRIDs) == 0 {
		return nil, errors.New("project_scope_rids is required")
	}
	serviceID := uuid.New()
	displayName := "Schedule run-as: " + current.Name
	var principal models.ScheduleServicePrincipal
	err = r.db.QueryRow(ctx, `INSERT INTO service_principals (id, display_name, project_scope_rids, clearances, created_by)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, rid, display_name, project_scope_rids, clearances, created_by, created_at`,
		serviceID, displayName, uniqueStrings(req.ProjectScopeRIDs), uniqueStrings(req.Clearances), actor,
	).Scan(&principal.ID, &principal.RID, &principal.DisplayName, &principal.ProjectScopeRIDs, &principal.Clearances, &principal.CreatedBy, &principal.CreatedAt)
	if err != nil {
		return nil, err
	}
	current.ScopeKind = models.ScheduleScopeProjectScoped
	current.ProjectScopeRIDs = uniqueStrings(req.ProjectScopeRIDs)
	current.ServicePrincipalID = &principal.ID
	current.RunAsUserID = nil
	runAs := principal.RID
	current.RunAsIdentity = &runAs
	updated, err := r.updateSchedule(ctx, current, actor, "Converted schedule to project-scoped run-as identity")
	if err != nil {
		return nil, err
	}
	return &models.ConvertScheduleToProjectScopeResponse{Schedule: updated, ServicePrincipal: principal}, nil
}

func normalizeScheduleJSON(raw json.RawMessage, fallback json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if len(fallback) == 0 {
			return nil, nil
		}
		raw = fallback
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}

func defaultScheduleTrigger() json.RawMessage {
	return json.RawMessage(`{"kind":{"time":{"cron":"0 * * * *","time_zone":"UTC","flavor":"UNIX_5"}}}`)
}

func cleanStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func deriveScheduleBranch(target json.RawMessage) string {
	var body struct {
		Kind map[string]map[string]any `json:"kind"`
	}
	if err := json.Unmarshal(target, &body); err != nil {
		return ""
	}
	for _, value := range body.Kind {
		if branch, ok := value["build_branch"].(string); ok && strings.TrimSpace(branch) != "" {
			return strings.TrimSpace(branch)
		}
	}
	return ""
}

func deriveScheduleBuildStrategy(target json.RawMessage) string {
	var body struct {
		Kind map[string]map[string]any `json:"kind"`
	}
	if err := json.Unmarshal(target, &body); err != nil {
		return ""
	}
	for _, value := range body.Kind {
		if force, ok := value["force_build"].(bool); ok && force {
			return "FORCE"
		}
	}
	return "STALE_ONLY"
}

func jsonEqual(a, b json.RawMessage) bool {
	var ca, cb bytes.Buffer
	if err := json.Compact(&ca, a); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Compact(&cb, b); err != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}
