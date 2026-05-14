package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	schedulecore "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/schedule"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

type scheduleDispatchInput struct {
	Actor       string
	TriggerType string
	Now         time.Time
	Snapshot    map[string]string
	Diagnostics map[string]string
	Event       *schedulecore.JSONTriggerEvent
}

type scheduleBuildTarget struct {
	Kind              string
	PipelineRID       string
	OutputDatasetRIDs []string
	BuildBranch       string
	JobSpecFallback   []string
	ForceBuild        bool
	AbortPolicy       string
	Diagnostics       map[string]string
}

func (r *Repository) DispatchDueSchedules(ctx context.Context, req models.RunDueSchedulesRequest, actor string) (models.ScheduleDispatchResponse, error) {
	if actor == "" {
		actor = "scheduler"
	}
	now := time.Now().UTC()
	if req.Now != nil {
		now = req.Now.UTC()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}

	rows, err := r.db.Query(ctx, `SELECT `+scheduleSelectColumns+`
FROM schedules
WHERE paused=FALSE
ORDER BY COALESCE(last_triggered_at, last_run_at, created_at), created_at
LIMIT $1`, limit)
	if err != nil {
		return models.ScheduleDispatchResponse{}, err
	}
	defer rows.Close()

	response := models.ScheduleDispatchResponse{Results: []models.ScheduleDispatchResult{}}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		result, ok, err := r.dispatchPendingOrDueSchedule(ctx, item, now, actor)
		if err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		if !ok {
			continue
		}
		addScheduleDispatchResult(&response, result)
	}
	return response, rows.Err()
}

func (r *Repository) RecordScheduleTriggerEvent(ctx context.Context, req models.ScheduleTriggerEventRequest, actor string) (models.ScheduleDispatchResponse, error) {
	if actor == "" {
		actor = "scheduler"
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = req.Type
	}
	occurredAt := time.Now().UTC()
	if req.OccurredAt != nil {
		occurredAt = req.OccurredAt.UTC()
	}
	event := schedulecore.JSONTriggerEvent{
		EventType:  schedulecore.NormalizeEventType(eventType),
		TargetRID:  strings.TrimSpace(req.TargetRID),
		Branch:     strings.TrimSpace(req.Branch),
		OccurredAt: occurredAt,
	}
	if event.TargetRID == "" {
		return models.ScheduleDispatchResponse{}, errors.New("target_rid is required")
	}

	rows, err := r.db.Query(ctx, `SELECT `+scheduleSelectColumns+`
FROM schedules
WHERE paused=FALSE
  AND (target_rids && ARRAY[$1]::text[] OR trigger_json::text LIKE '%' || $1 || '%')
ORDER BY updated_at DESC`, event.TargetRID)
	if err != nil {
		return models.ScheduleDispatchResponse{}, err
	}
	defer rows.Close()

	response := models.ScheduleDispatchResponse{Results: []models.ScheduleDispatchResult{}}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		observations, err := schedulecore.MatchingJSONEventObservations(item.Trigger, event)
		if err != nil {
			addScheduleDispatchResult(&response, models.ScheduleDispatchResult{
				ScheduleRID: item.RID,
				Outcome:     "FAILED",
				Diagnostics: map[string]string{"reason": "trigger_evaluation_failed", "detail": err.Error()},
			})
			continue
		}
		if len(observations) == 0 {
			continue
		}
		if err := r.recordScheduleObservations(ctx, item.ID, observations); err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		observed, err := r.loadScheduleObservedPaths(ctx, r.db, item.ID)
		if err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		evaluation, err := schedulecore.EvaluateJSONTrigger(item.Trigger, schedulecore.JSONTriggerContext{
			Now:       occurredAt,
			CreatedAt: item.CreatedAt,
			LastRunAt: item.LastRunAt,
			Event:     &event,
			Observed:  observed,
		})
		if err != nil {
			addScheduleDispatchResult(&response, models.ScheduleDispatchResult{
				ScheduleRID: item.RID,
				Outcome:     "FAILED",
				Diagnostics: map[string]string{"reason": "trigger_evaluation_failed", "detail": err.Error()},
			})
			continue
		}
		if !evaluation.Fires {
			continue
		}
		snapshot := mergeStringMaps(evaluation.Snapshot, req.Diagnostics)
		snapshot["event_type"] = string(event.EventType)
		snapshot["target_rid"] = event.TargetRID
		if event.Branch != "" {
			snapshot["branch"] = event.Branch
		}
		result, err := r.dispatchSchedule(ctx, item, scheduleDispatchInput{
			Actor:       actor,
			TriggerType: evaluation.TriggerType,
			Now:         occurredAt,
			Snapshot:    snapshot,
			Diagnostics: map[string]string{"source": "event", "event_type": string(event.EventType)},
			Event:       &event,
		})
		if err != nil {
			return models.ScheduleDispatchResponse{}, err
		}
		addScheduleDispatchResult(&response, result)
	}
	return response, rows.Err()
}

func (r *Repository) dispatchPendingOrDueSchedule(ctx context.Context, item *models.Schedule, now time.Time, actor string) (models.ScheduleDispatchResult, bool, error) {
	result, dispatched, err := r.dispatchPendingIfReady(ctx, item, now, actor)
	if err != nil || dispatched {
		return result, dispatched, err
	}
	observed, err := r.loadScheduleObservedPaths(ctx, r.db, item.ID)
	if err != nil {
		return models.ScheduleDispatchResult{}, false, err
	}
	evaluation, err := schedulecore.EvaluateJSONTrigger(item.Trigger, schedulecore.JSONTriggerContext{
		Now:       now,
		CreatedAt: item.CreatedAt,
		LastRunAt: item.LastRunAt,
		Observed:  observed,
	})
	if err != nil {
		return models.ScheduleDispatchResult{
			ScheduleRID: item.RID,
			Outcome:     "FAILED",
			Diagnostics: map[string]string{"reason": "trigger_evaluation_failed", "detail": err.Error()},
		}, true, nil
	}
	if !evaluation.Fires {
		return models.ScheduleDispatchResult{}, false, nil
	}
	result, err = r.dispatchSchedule(ctx, item, scheduleDispatchInput{
		Actor:       actor,
		TriggerType: evaluation.TriggerType,
		Now:         now,
		Snapshot:    evaluation.Snapshot,
		Diagnostics: map[string]string{"source": "scheduler_due"},
	})
	return result, true, err
}

func (r *Repository) dispatchPendingIfReady(ctx context.Context, item *models.Schedule, now time.Time, actor string) (models.ScheduleDispatchResult, bool, error) {
	if item.ActiveRunID == nil {
		return models.ScheduleDispatchResult{}, false, nil
	}
	finalized, err := r.finalizeActiveScheduleRun(ctx, item)
	if err != nil || !finalized {
		return models.ScheduleDispatchResult{}, false, err
	}
	refreshed, err := r.GetSchedule(ctx, item.RID)
	if err != nil || refreshed == nil || !refreshed.PendingReRun {
		return models.ScheduleDispatchResult{}, false, err
	}
	snapshot := map[string]string{"type": "pending", "reason": "previous_run_completed"}
	for key, value := range refreshed.PendingTrigger {
		snapshot[key] = value
	}
	result, err := r.dispatchSchedule(ctx, refreshed, scheduleDispatchInput{
		Actor:       actor,
		TriggerType: firstNonEmptyString(snapshot["trigger_type"], "PENDING"),
		Now:         now,
		Snapshot:    snapshot,
		Diagnostics: map[string]string{"source": "pending_rerun"},
	})
	return result, true, err
}

func (r *Repository) dispatchSchedule(ctx context.Context, item *models.Schedule, input scheduleDispatchInput) (models.ScheduleDispatchResult, error) {
	if input.Actor == "" {
		input.Actor = "scheduler"
	}
	if input.TriggerType == "" {
		input.TriggerType = "UNKNOWN"
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}
	if input.Snapshot == nil {
		input.Snapshot = map[string]string{}
	}
	if input.Diagnostics == nil {
		input.Diagnostics = map[string]string{}
	}
	input.Snapshot["trigger_type"] = input.TriggerType
	input.Snapshot["triggered_at"] = input.Now.Format(time.RFC3339Nano)

	if beginner, ok := r.db.(txBeginner); ok {
		tx, err := beginner.Begin(ctx)
		if err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		locked, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleSelectColumns+` FROM schedules WHERE id=$1 FOR UPDATE`, item.ID))
		if err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		result, err := r.dispatchScheduleWithDB(ctx, tx, locked, input)
		if err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		return result, nil
	}
	return r.dispatchScheduleWithDB(ctx, r.db, item, input)
}

func (r *Repository) dispatchScheduleWithDB(ctx context.Context, db DB, item *models.Schedule, input scheduleDispatchInput) (models.ScheduleDispatchResult, error) {
	if item.ActiveRunID != nil {
		result, err := insertIgnoredScheduleRun(ctx, db, item, input, "previous run active; trigger preserved for rerun")
		if err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		if err := updateSchedulePendingTrigger(ctx, db, item.ID, input); err != nil {
			return models.ScheduleDispatchResult{}, err
		}
		return result, nil
	}

	target, ok, err := parseScheduleBuildTarget(item.Target, item.Branch, item.BuildStrategy)
	if err != nil {
		result, insertErr := insertFailedScheduleRun(ctx, db, item, input, err.Error(), map[string]string{"reason": "invalid_target"})
		if insertErr != nil {
			return models.ScheduleDispatchResult{}, insertErr
		}
		_ = clearScheduleObservations(ctx, db, item.ID)
		return result, nil
	}
	if !ok {
		result, insertErr := insertIgnoredScheduleRun(ctx, db, item, input, "target kind does not enqueue a build")
		if insertErr != nil {
			return models.ScheduleDispatchResult{}, insertErr
		}
		_ = clearScheduleObservations(ctx, db, item.ID)
		return result, nil
	}

	buildID := uuid.New()
	buildRID := "ri.foundry.main.build." + buildID.String()
	triggerKind := "SCHEDULED"
	if target.ForceBuild {
		triggerKind = "FORCE"
	}
	if input.TriggerType == "MANUAL" && !target.ForceBuild {
		triggerKind = "SCHEDULED"
	}
	abortPolicy := target.AbortPolicy
	if abortPolicy == "" {
		abortPolicy = string(models.AbortDependentOnly)
	}
	_, err = db.Exec(ctx, `INSERT INTO builds (
    id, pipeline_rid, build_branch, job_spec_fallback, target_dataset_rids,
    state, trigger_kind, force_build, requested_by, abort_policy, queued_at
) VALUES (
    $1,$2,$3,$4,$5,
    'BUILD_QUEUED',$6,$7,$8,$9,NOW()
)`, buildID, target.PipelineRID, target.BuildBranch, uniqueStrings(target.JobSpecFallback), uniqueStrings(target.OutputDatasetRIDs), triggerKind, target.ForceBuild, input.Actor, abortPolicy)
	if err != nil {
		result, insertErr := insertFailedScheduleRun(ctx, db, item, input, err.Error(), map[string]string{"reason": "create_build_failed"})
		if insertErr != nil {
			return models.ScheduleDispatchResult{}, insertErr
		}
		return result, nil
	}

	diagnostics := mergeStringMaps(input.Diagnostics, target.Diagnostics)
	diagnostics["build_state"] = string(models.BuildQueued)
	diagnostics["target_kind"] = target.Kind
	runID := uuid.New()
	runRID := "ri.foundry.main.schedule_run." + runID.String()
	if err := insertScheduleRun(ctx, db, runID, item.ID, "SUCCEEDED", &buildRID, nil, input, diagnostics, false, item.Version); err != nil {
		return models.ScheduleDispatchResult{}, err
	}
	_, err = db.Exec(ctx, `UPDATE schedules
SET active_run_id=$2,
    pending_re_run=FALSE,
    pending_trigger_snapshot=NULL,
    last_run_at=$3,
    last_triggered_at=$3,
    updated_at=NOW()
WHERE id=$1`, item.ID, runID, input.Now)
	if err != nil {
		return models.ScheduleDispatchResult{}, err
	}
	_ = clearScheduleObservations(ctx, db, item.ID)
	return models.ScheduleDispatchResult{
		ScheduleRID: item.RID,
		RunRID:      runRID,
		Outcome:     "SUCCEEDED",
		BuildRID:    buildRID,
		Diagnostics: diagnostics,
	}, nil
}

func insertIgnoredScheduleRun(ctx context.Context, db DB, item *models.Schedule, input scheduleDispatchInput, reason string) (models.ScheduleDispatchResult, error) {
	runID := uuid.New()
	runRID := "ri.foundry.main.schedule_run." + runID.String()
	diagnostics := mergeStringMaps(input.Diagnostics, map[string]string{"reason": reason})
	if err := insertScheduleRun(ctx, db, runID, item.ID, "IGNORED", nil, &reason, input, diagnostics, true, item.Version); err != nil {
		return models.ScheduleDispatchResult{}, err
	}
	_, _ = db.Exec(ctx, `UPDATE schedules
SET last_run_at=$2,
    last_triggered_at=$2,
    updated_at=NOW()
WHERE id=$1`, item.ID, input.Now)
	return models.ScheduleDispatchResult{
		ScheduleRID:  item.RID,
		RunRID:       runRID,
		Outcome:      "IGNORED",
		PendingReRun: item.ActiveRunID != nil,
		Diagnostics:  diagnostics,
	}, nil
}

func insertFailedScheduleRun(ctx context.Context, db DB, item *models.Schedule, input scheduleDispatchInput, reason string, diagnostics map[string]string) (models.ScheduleDispatchResult, error) {
	runID := uuid.New()
	runRID := "ri.foundry.main.schedule_run." + runID.String()
	merged := mergeStringMaps(input.Diagnostics, diagnostics)
	if err := insertScheduleRun(ctx, db, runID, item.ID, "FAILED", nil, &reason, input, merged, true, item.Version); err != nil {
		return models.ScheduleDispatchResult{}, err
	}
	_, _ = db.Exec(ctx, `UPDATE schedules
SET last_run_at=$2,
    last_triggered_at=$2,
    updated_at=NOW()
WHERE id=$1`, item.ID, input.Now)
	return models.ScheduleDispatchResult{
		ScheduleRID: item.RID,
		RunRID:      runRID,
		Outcome:     "FAILED",
		Diagnostics: merged,
	}, nil
}

func insertScheduleRun(ctx context.Context, db DB, runID, scheduleID uuid.UUID, outcome string, buildRID *string, failureReason *string, input scheduleDispatchInput, diagnostics map[string]string, finished bool, scheduleVersion int) error {
	snapshotRaw, _ := json.Marshal(input.Snapshot)
	diagnosticsRaw, _ := json.Marshal(diagnostics)
	var finishedAt any
	if finished {
		finishedAt = input.Now
	}
	_, err := db.Exec(ctx, `INSERT INTO schedule_runs (
    id, schedule_id, outcome, build_rid, failure_reason,
    triggered_at, finished_at, trigger_snapshot, schedule_version,
    trigger_type, diagnostics
) VALUES (
    $1,$2,$3,$4,$5,
    $6,$7,$8::jsonb,$9,
    $10,$11::jsonb
)`, runID, scheduleID, outcome, buildRID, failureReason, input.Now, finishedAt, snapshotRaw, scheduleVersion, input.TriggerType, diagnosticsRaw)
	return err
}

func updateSchedulePendingTrigger(ctx context.Context, db DB, scheduleID uuid.UUID, input scheduleDispatchInput) error {
	raw, _ := json.Marshal(input.Snapshot)
	_, err := db.Exec(ctx, `UPDATE schedules
SET pending_re_run=TRUE,
    pending_trigger_snapshot=$2::jsonb,
    last_run_at=$3,
    last_triggered_at=$3,
    updated_at=NOW()
WHERE id=$1`, scheduleID, raw, input.Now)
	return err
}

func (r *Repository) finalizeActiveScheduleRun(ctx context.Context, item *models.Schedule) (bool, error) {
	if item.ActiveRunID == nil {
		return false, nil
	}
	var buildRID sql.NullString
	if err := r.db.QueryRow(ctx, `SELECT build_rid FROM schedule_runs WHERE id=$1`, *item.ActiveRunID).Scan(&buildRID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = r.db.Exec(ctx, `UPDATE schedules SET active_run_id=NULL, updated_at=NOW() WHERE id=$1`, item.ID)
			return true, err
		}
		return false, err
	}
	if !buildRID.Valid || buildRID.String == "" {
		_, err := r.db.Exec(ctx, `UPDATE schedules SET active_run_id=NULL, updated_at=NOW() WHERE id=$1`, item.ID)
		return true, err
	}
	var state string
	var errMsg sql.NullString
	err := r.db.QueryRow(ctx, `SELECT state, error_message FROM builds WHERE rid=$1`, buildRID.String).Scan(&state, &errMsg)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parsed, err := models.ParseBuildState(state)
	if err != nil || !parsed.IsTerminal() {
		return false, nil
	}

	outcome := "SUCCEEDED"
	reason := sql.NullString{}
	if parsed == models.BuildFailed || parsed == models.BuildAborted {
		outcome = "FAILED"
		reason = errMsg
	}
	if parsed == models.BuildCompleted {
		ignored, err := r.completedBuildWasIgnored(ctx, buildRID.String)
		if err != nil {
			return false, err
		}
		if ignored {
			outcome = "IGNORED"
			reason = sql.NullString{String: "Build completed with all jobs ignored because fresh", Valid: true}
		}
	}
	diagnosticsRaw, _ := json.Marshal(map[string]string{"build_state": state})
	_, err = r.db.Exec(ctx, `UPDATE schedule_runs
SET outcome=$2,
    failure_reason=COALESCE($3, failure_reason),
    finished_at=COALESCE(finished_at, NOW()),
    diagnostics=diagnostics || $4::jsonb
WHERE id=$1`, *item.ActiveRunID, outcome, nullStringPtr(reason), diagnosticsRaw)
	if err != nil {
		return false, err
	}
	_, err = r.db.Exec(ctx, `UPDATE schedules SET active_run_id=NULL, updated_at=NOW() WHERE id=$1`, item.ID)
	return true, err
}

func (r *Repository) completedBuildWasIgnored(ctx context.Context, buildRID string) (bool, error) {
	var total, ignored int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*)::int, COUNT(*) FILTER (WHERE stale_skipped)::int
FROM jobs j
JOIN builds b ON b.id=j.build_id
WHERE b.rid=$1`, buildRID).Scan(&total, &ignored)
	if err != nil {
		return false, err
	}
	return total > 0 && total == ignored, nil
}

func (r *Repository) recordScheduleObservations(ctx context.Context, scheduleID uuid.UUID, observations []schedulecore.JSONObservedTrigger) error {
	for _, observation := range observations {
		_, err := r.db.Exec(ctx, `INSERT INTO schedule_event_observations (
    schedule_id, trigger_path, observed_event_type, observed_target_rid, observed_at
) VALUES ($1,$2,$3,$4,NOW())
ON CONFLICT DO NOTHING`, scheduleID, observation.Path, string(observation.EventType), observation.TargetRID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) loadScheduleObservedPaths(ctx context.Context, db DB, scheduleID uuid.UUID) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT trigger_path FROM schedule_event_observations WHERE schedule_id=$1`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		out[path] = true
	}
	return out, rows.Err()
}

func clearScheduleObservations(ctx context.Context, db DB, scheduleID uuid.UUID) error {
	_, err := db.Exec(ctx, `DELETE FROM schedule_event_observations WHERE schedule_id=$1`, scheduleID)
	return err
}

func parseScheduleBuildTarget(raw json.RawMessage, fallbackBranch, buildStrategy string) (scheduleBuildTarget, bool, error) {
	var envelope struct {
		Kind map[string]json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return scheduleBuildTarget{}, false, err
	}
	for kind, data := range envelope.Kind {
		switch normalizeScheduleTargetKind(kind) {
		case "pipeline_build":
			var target struct {
				PipelineRID       string   `json:"pipeline_rid"`
				BuildBranch       string   `json:"build_branch"`
				JobSpecFallback   []string `json:"job_spec_fallback"`
				OutputDatasetRIDs []string `json:"output_dataset_rids"`
				TargetDatasetRIDs []string `json:"target_dataset_rids"`
				ForceBuild        bool     `json:"force_build"`
				AbortPolicy       *string  `json:"abort_policy"`
			}
			if err := json.Unmarshal(data, &target); err != nil {
				return scheduleBuildTarget{}, false, err
			}
			outputs := append([]string{}, target.OutputDatasetRIDs...)
			outputs = append(outputs, target.TargetDatasetRIDs...)
			if strings.TrimSpace(target.PipelineRID) == "" {
				return scheduleBuildTarget{}, false, errors.New("pipeline_build target requires pipeline_rid")
			}
			if len(uniqueStrings(outputs)) == 0 {
				return scheduleBuildTarget{}, false, errors.New("pipeline_build target requires output_dataset_rids")
			}
			return scheduleBuildTarget{
				Kind:              "pipeline_build",
				PipelineRID:       strings.TrimSpace(target.PipelineRID),
				OutputDatasetRIDs: uniqueStrings(outputs),
				BuildBranch:       firstNonEmptyString(target.BuildBranch, fallbackBranch, "master"),
				JobSpecFallback:   target.JobSpecFallback,
				ForceBuild:        target.ForceBuild || strings.EqualFold(buildStrategy, "FORCE"),
				AbortPolicy:       cleanStringValue(target.AbortPolicy),
			}, true, nil
		case "dataset_build":
			var target struct {
				DatasetRID        string   `json:"dataset_rid"`
				PipelineRID       string   `json:"pipeline_rid"`
				BuildBranch       string   `json:"build_branch"`
				JobSpecFallback   []string `json:"job_spec_fallback"`
				OutputDatasetRIDs []string `json:"output_dataset_rids"`
				ForceBuild        bool     `json:"force_build"`
				AbortPolicy       *string  `json:"abort_policy"`
			}
			if err := json.Unmarshal(data, &target); err != nil {
				return scheduleBuildTarget{}, false, err
			}
			outputs := append([]string{}, target.OutputDatasetRIDs...)
			if strings.TrimSpace(target.DatasetRID) != "" {
				outputs = append(outputs, strings.TrimSpace(target.DatasetRID))
			}
			if len(uniqueStrings(outputs)) == 0 {
				return scheduleBuildTarget{}, false, errors.New("dataset_build target requires dataset_rid")
			}
			pipelineRID := firstNonEmptyString(target.PipelineRID, "ri.foundry.main.pipeline.dataset-build")
			diagnostics := map[string]string{}
			if target.PipelineRID == "" {
				diagnostics["pipeline_resolution"] = "dataset_build target did not include pipeline_rid; queued with dataset-build placeholder"
			}
			return scheduleBuildTarget{
				Kind:              "dataset_build",
				PipelineRID:       pipelineRID,
				OutputDatasetRIDs: uniqueStrings(outputs),
				BuildBranch:       firstNonEmptyString(target.BuildBranch, fallbackBranch, "master"),
				JobSpecFallback:   target.JobSpecFallback,
				ForceBuild:        target.ForceBuild || strings.EqualFold(buildStrategy, "FORCE"),
				AbortPolicy:       cleanStringValue(target.AbortPolicy),
				Diagnostics:       diagnostics,
			}, true, nil
		default:
			return scheduleBuildTarget{Kind: normalizeScheduleTargetKind(kind)}, false, nil
		}
	}
	return scheduleBuildTarget{}, false, errors.New("schedule target kind is required")
}

func addScheduleDispatchResult(response *models.ScheduleDispatchResponse, result models.ScheduleDispatchResult) {
	response.Results = append(response.Results, result)
	switch result.Outcome {
	case "SUCCEEDED":
		response.Triggered++
	case "IGNORED":
		if result.PendingReRun {
			response.Queued++
		} else {
			response.Ignored++
		}
	case "FAILED":
		response.Failed++
	}
}

func normalizeScheduleTargetKind(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "pipelineBuild":
		return "pipeline_build"
	case "datasetBuild":
		return "dataset_build"
	case "syncRun":
		return "sync_run"
	case "healthCheck":
		return "health_check"
	default:
		return strings.ToLower(normalized)
	}
}

func mergeStringMaps(values ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, value := range values {
		for key, entry := range value {
			if strings.TrimSpace(key) == "" {
				continue
			}
			out[key] = entry
		}
	}
	return out
}

func cleanStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
