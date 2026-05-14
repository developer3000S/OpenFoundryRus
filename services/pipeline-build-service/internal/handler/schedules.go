package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

var scheduleRepository atomic.Value // stores *scheduleRepositorySlot

type ScheduleRepository interface {
	ListSchedules(ctx context.Context, query models.ListSchedulesQuery) (models.ListSchedulesResponse, error)
	CreateSchedule(ctx context.Context, req models.CreateScheduleRequest, actor string) (*models.Schedule, error)
	GetSchedule(ctx context.Context, rid string) (*models.Schedule, error)
	PatchSchedule(ctx context.Context, rid string, req models.PatchScheduleRequest, actor string) (*models.Schedule, error)
	PauseSchedule(ctx context.Context, rid, reason, actor string) (*models.Schedule, error)
	ResumeSchedule(ctx context.Context, rid, actor string) (*models.Schedule, error)
	DeleteSchedule(ctx context.Context, rid string) (bool, error)
	SetScheduleAutoPauseExempt(ctx context.Context, rid string, exempt bool, actor string) (*models.Schedule, error)
	RunScheduleNow(ctx context.Context, rid, actor string) (models.ScheduleRunNowResponse, error)
	DispatchDueSchedules(ctx context.Context, req models.RunDueSchedulesRequest, actor string) (models.ScheduleDispatchResponse, error)
	RecordScheduleTriggerEvent(ctx context.Context, req models.ScheduleTriggerEventRequest, actor string) (models.ScheduleDispatchResponse, error)
	ListScheduleRuns(ctx context.Context, rid string, query models.ListScheduleRunsQuery) (models.ListScheduleRunsResponse, error)
	ListScheduleVersions(ctx context.Context, rid string, limit, offset int64) (models.ListScheduleVersionsResponse, error)
	GetScheduleVersion(ctx context.Context, rid string, version int) (*models.ScheduleVersion, error)
	ConvertScheduleToProjectScope(ctx context.Context, rid string, req models.ConvertScheduleToProjectScopeRequest, actor string) (*models.ConvertScheduleToProjectScopeResponse, error)
}

type scheduleRepositorySlot struct {
	repo ScheduleRepository
}

func SetScheduleRepository(repo ScheduleRepository) func() {
	previous, _ := scheduleRepository.Load().(*scheduleRepositorySlot)
	scheduleRepository.Store(&scheduleRepositorySlot{repo: repo})
	return func() { scheduleRepository.Store(previous) }
}

func currentScheduleRepository() (ScheduleRepository, bool) {
	slot, _ := scheduleRepository.Load().(*scheduleRepositorySlot)
	if slot == nil || slot.repo == nil {
		return nil, false
	}
	return slot.repo, true
}

func requireScheduleRepository(w http.ResponseWriter, detail string) (ScheduleRepository, bool) {
	repo, ok := currentScheduleRepository()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedule_repository_not_configured", "detail": detail})
		return nil, false
	}
	return repo, true
}

func ListSchedules(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "ListSchedules requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	query := parseListSchedulesQuery(r)
	response, err := repo.ListSchedules(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_schedules_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func CreateSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "CreateSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var req models.CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	item, err := repo.CreateSchedule(r.Context(), req, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "create_schedule_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func GetSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "GetSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	item, err := repo.GetSchedule(r.Context(), scheduleRIDParam(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_schedule_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func PatchSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "PatchSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var req models.PatchScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	item, err := repo.PatchSchedule(r.Context(), scheduleRIDParam(r), req, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "patch_schedule_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func PauseSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "PauseSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	item, err := repo.PauseSchedule(r.Context(), scheduleRIDParam(r), body.Reason, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pause_schedule_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func ResumeSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "ResumeSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	item, err := repo.ResumeSchedule(r.Context(), scheduleRIDParam(r), scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resume_schedule_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "DeleteSchedule requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	deleted, err := repo.DeleteSchedule(r.Context(), scheduleRIDParam(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete_schedule_failed", "detail": err.Error()})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func SetScheduleAutoPauseExempt(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "SetScheduleAutoPauseExempt requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var body struct {
		Exempt bool `json:"exempt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	item, err := repo.SetScheduleAutoPauseExempt(r.Context(), scheduleRIDParam(r), body.Exempt, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "set_auto_pause_exempt_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func RunScheduleNow(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "RunScheduleNow requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	response, err := repo.RunScheduleNow(r.Context(), scheduleRIDParam(r), scheduleActor(r))
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run_schedule_now_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func DispatchDueSchedules(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "DispatchDueSchedules requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var req models.RunDueSchedulesRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	response, err := repo.DispatchDueSchedules(r.Context(), req, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch_due_schedules_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func RecordScheduleTriggerEvent(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "RecordScheduleTriggerEvent requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var req models.ScheduleTriggerEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	response, err := repo.RecordScheduleTriggerEvent(r.Context(), req, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "record_schedule_trigger_event_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func ListScheduleRuns(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "ListScheduleRuns requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	response, err := repo.ListScheduleRuns(r.Context(), scheduleRIDParam(r), models.ListScheduleRunsQuery{
		Limit:   int64(firstPositiveQueryInt(r, "limit", 50)),
		Offset:  int64(queryInt(r, "offset")),
		Outcome: strings.TrimSpace(r.URL.Query().Get("outcome")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_schedule_runs_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func ListScheduleVersions(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "ListScheduleVersions requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	response, err := repo.ListScheduleVersions(r.Context(), scheduleRIDParam(r), int64(firstPositiveQueryInt(r, "limit", 50)), int64(queryInt(r, "offset")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_schedule_versions_failed", "detail": err.Error()})
		return
	}
	if response.ScheduleRID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func GetScheduleVersion(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "GetScheduleVersion requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_version"})
		return
	}
	item, err := repo.GetScheduleVersion(r.Context(), scheduleRIDParam(r), version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_schedule_version_failed", "detail": err.Error()})
		return
	}
	if item == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_version_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func GetScheduleVersionDiff(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "GetScheduleVersionDiff requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	from := queryInt(r, "from")
	to := queryInt(r, "to")
	if from < 1 || to < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_version_range"})
		return
	}
	before, err := repo.GetScheduleVersion(r.Context(), scheduleRIDParam(r), from)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_schedule_version_failed", "detail": err.Error()})
		return
	}
	after, err := repo.GetScheduleVersion(r.Context(), scheduleRIDParam(r), to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get_schedule_version_failed", "detail": err.Error()})
		return
	}
	if before == nil || after == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_version_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, buildScheduleVersionDiff(before, after))
}

func ConvertScheduleToProjectScope(w http.ResponseWriter, r *http.Request) {
	repo, ok := requireScheduleRepository(w, "ConvertScheduleToProjectScope requires DATABASE_URL-backed schedule repository wiring")
	if !ok {
		return
	}
	var req models.ConvertScheduleToProjectScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "detail": err.Error()})
		return
	}
	response, err := repo.ConvertScheduleToProjectScope(r.Context(), scheduleRIDParam(r), req, scheduleActor(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "convert_schedule_scope_failed", "detail": err.Error()})
		return
	}
	if response == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func parseListSchedulesQuery(r *http.Request) models.ListSchedulesQuery {
	q := r.URL.Query()
	var paused *bool
	if raw := strings.TrimSpace(q.Get("paused")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			paused = &parsed
		}
	}
	return models.ListSchedulesQuery{
		Project:  strings.TrimSpace(q.Get("project")),
		Paused:   paused,
		Owner:    strings.TrimSpace(q.Get("owner")),
		Q:        strings.TrimSpace(q.Get("q")),
		Files:    q["files"],
		Users:    q["users"],
		Projects: q["projects"],
		Sort:     strings.TrimSpace(q.Get("sort")),
		Limit:    int64(firstPositiveQueryInt(r, "limit", 50)),
		Offset:   int64(queryInt(r, "offset")),
	}
}

func scheduleRIDParam(r *http.Request) string {
	if rid := chi.URLParam(r, "rid"); rid != "" {
		return rid
	}
	return chi.URLParam(r, "id")
}

func scheduleActor(r *http.Request) string {
	if actor := actorIDFromRequest(r); actor != nil {
		return actor.String()
	}
	return "system"
}

func firstPositiveQueryInt(r *http.Request, key string, fallback int) int {
	value := queryInt(r, key)
	if value > 0 {
		return value
	}
	return fallback
}

type scheduleFieldChange[T any] struct {
	Before T `json:"before"`
	After  T `json:"after"`
}

type scheduleJSONDiffEntry struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type scheduleVersionDiff struct {
	ScheduleID      string                       `json:"schedule_id"`
	FromVersion     int                          `json:"from_version"`
	ToVersion       int                          `json:"to_version"`
	NameDiff        *scheduleFieldChange[string] `json:"name_diff"`
	DescriptionDiff *scheduleFieldChange[string] `json:"description_diff"`
	TriggerDiff     []scheduleJSONDiffEntry      `json:"trigger_diff"`
	TargetDiff      []scheduleJSONDiffEntry      `json:"target_diff"`
}

func buildScheduleVersionDiff(before, after *models.ScheduleVersion) scheduleVersionDiff {
	out := scheduleVersionDiff{
		ScheduleID:  after.ScheduleID.String(),
		FromVersion: before.Version,
		ToVersion:   after.Version,
		TriggerDiff: []scheduleJSONDiffEntry{},
		TargetDiff:  []scheduleJSONDiffEntry{},
	}
	if before.Name != after.Name {
		out.NameDiff = &scheduleFieldChange[string]{Before: before.Name, After: after.Name}
	}
	if before.Description != after.Description {
		out.DescriptionDiff = &scheduleFieldChange[string]{Before: before.Description, After: after.Description}
	}
	out.TriggerDiff = diffScheduleJSON("$", before.TriggerJSON, after.TriggerJSON)
	out.TargetDiff = diffScheduleJSON("$", before.TargetJSON, after.TargetJSON)
	return out
}

func diffScheduleJSON(path string, beforeRaw, afterRaw json.RawMessage) []scheduleJSONDiffEntry {
	var before any
	var after any
	_ = json.Unmarshal(beforeRaw, &before)
	_ = json.Unmarshal(afterRaw, &after)
	return diffScheduleValues(path, before, after)
}

func diffScheduleValues(path string, before, after any) []scheduleJSONDiffEntry {
	if fmt.Sprintf("%#v", before) == fmt.Sprintf("%#v", after) {
		return nil
	}
	beforeMap, beforeMapOK := before.(map[string]any)
	afterMap, afterMapOK := after.(map[string]any)
	if beforeMapOK && afterMapOK {
		keys := map[string]struct{}{}
		for key := range beforeMap {
			keys[key] = struct{}{}
		}
		for key := range afterMap {
			keys[key] = struct{}{}
		}
		out := []scheduleJSONDiffEntry{}
		for key := range keys {
			out = append(out, diffScheduleValues(path+"."+key, beforeMap[key], afterMap[key])...)
		}
		return out
	}
	return []scheduleJSONDiffEntry{{Path: path, Before: before, After: after}}
}
