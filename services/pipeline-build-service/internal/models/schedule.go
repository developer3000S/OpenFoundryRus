package models

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ScheduleScopeKind string

const (
	ScheduleScopeUser          ScheduleScopeKind = "USER"
	ScheduleScopeProjectScoped ScheduleScopeKind = "PROJECT_SCOPED"
)

type Schedule struct {
	ID                 uuid.UUID         `json:"id"`
	RID                string            `json:"rid"`
	ProjectRID         string            `json:"project_rid"`
	FolderRID          *string           `json:"folder_rid"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Trigger            json.RawMessage   `json:"trigger"`
	Target             json.RawMessage   `json:"target"`
	TargetRIDs         []string          `json:"target_rids"`
	Branch             string            `json:"branch"`
	BuildStrategy      string            `json:"build_strategy"`
	Paused             bool              `json:"paused"`
	PausedReason       *string           `json:"paused_reason"`
	PausedAt           *time.Time        `json:"paused_at"`
	AutoPauseExempt    bool              `json:"auto_pause_exempt"`
	PendingReRun       bool              `json:"pending_re_run"`
	PendingTrigger     map[string]string `json:"pending_trigger_snapshot,omitempty"`
	ActiveRunID        *uuid.UUID        `json:"active_run_id"`
	LastTriggeredAt    *time.Time        `json:"last_triggered_at,omitempty"`
	Version            int               `json:"version"`
	CreatedBy          string            `json:"created_by"`
	Owner              string            `json:"owner"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	LastRunAt          *time.Time        `json:"last_run_at"`
	ScopeKind          ScheduleScopeKind `json:"scope_kind"`
	ProjectScopeRIDs   []string          `json:"project_scope_rids"`
	RunAsUserID        *uuid.UUID        `json:"run_as_user_id"`
	ServicePrincipalID *uuid.UUID        `json:"service_principal_id"`
	RunAsIdentity      *string           `json:"run_as_identity"`
	LastUpdatedBy      string            `json:"last_updated_by"`
}

type CreateScheduleRequest struct {
	ProjectRID         string            `json:"project_rid"`
	FolderRID          *string           `json:"folder_rid,omitempty"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Trigger            json.RawMessage   `json:"trigger"`
	Target             json.RawMessage   `json:"target"`
	Paused             bool              `json:"paused"`
	Branch             string            `json:"branch"`
	BuildStrategy      string            `json:"build_strategy"`
	ScopeKind          ScheduleScopeKind `json:"scope_kind"`
	ProjectScopeRIDs   []string          `json:"project_scope_rids"`
	RunAsUserID        *uuid.UUID        `json:"run_as_user_id,omitempty"`
	ServicePrincipalID *uuid.UUID        `json:"service_principal_id,omitempty"`
	RunAsIdentity      *string           `json:"run_as_identity,omitempty"`
	ChangeComment      string            `json:"change_comment,omitempty"`
}

type PatchScheduleRequest struct {
	ProjectRID         *string            `json:"project_rid,omitempty"`
	FolderRID          *string            `json:"folder_rid,omitempty"`
	Name               *string            `json:"name,omitempty"`
	Description        *string            `json:"description,omitempty"`
	Trigger            json.RawMessage    `json:"trigger,omitempty"`
	Target             json.RawMessage    `json:"target,omitempty"`
	Paused             *bool              `json:"paused,omitempty"`
	Branch             *string            `json:"branch,omitempty"`
	BuildStrategy      *string            `json:"build_strategy,omitempty"`
	ScopeKind          *ScheduleScopeKind `json:"scope_kind,omitempty"`
	ProjectScopeRIDs   []string           `json:"project_scope_rids,omitempty"`
	RunAsUserID        *uuid.UUID         `json:"run_as_user_id,omitempty"`
	ServicePrincipalID *uuid.UUID         `json:"service_principal_id,omitempty"`
	RunAsIdentity      *string            `json:"run_as_identity,omitempty"`
	ChangeComment      string             `json:"change_comment,omitempty"`
}

type ListSchedulesQuery struct {
	Project  string
	Paused   *bool
	Owner    string
	Q        string
	Files    []string
	Users    []string
	Projects []string
	Sort     string
	Limit    int64
	Offset   int64
}

type ListSchedulesResponse struct {
	Data  []Schedule `json:"data"`
	Total int        `json:"total"`
}

type ScheduleRun struct {
	ID              uuid.UUID         `json:"id"`
	RID             string            `json:"rid"`
	ScheduleID      uuid.UUID         `json:"schedule_id"`
	Outcome         string            `json:"outcome"`
	BuildRID        *string           `json:"build_rid"`
	FailureReason   *string           `json:"failure_reason"`
	TriggeredAt     time.Time         `json:"triggered_at"`
	FinishedAt      *time.Time        `json:"finished_at"`
	TriggerSnapshot map[string]string `json:"trigger_snapshot"`
	TriggerType     string            `json:"trigger_type"`
	Diagnostics     map[string]string `json:"diagnostics"`
	ScheduleVersion int               `json:"schedule_version"`
}

type ListScheduleRunsQuery struct {
	Limit   int64
	Offset  int64
	Outcome string
}

type ListScheduleRunsResponse struct {
	ScheduleRID string        `json:"schedule_rid"`
	Data        []ScheduleRun `json:"data"`
	Total       int           `json:"total"`
}

type ScheduleVersion struct {
	ID          uuid.UUID       `json:"id"`
	ScheduleID  uuid.UUID       `json:"schedule_id"`
	Version     int             `json:"version"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	TriggerJSON json.RawMessage `json:"trigger_json"`
	TargetJSON  json.RawMessage `json:"target_json"`
	EditedBy    string          `json:"edited_by"`
	EditedAt    time.Time       `json:"edited_at"`
	Comment     string          `json:"comment"`
}

type ListScheduleVersionsResponse struct {
	ScheduleRID    string            `json:"schedule_rid"`
	CurrentVersion int               `json:"current_version"`
	Data           []ScheduleVersion `json:"data"`
}

type ScheduleRunNowResponse struct {
	RunID       string `json:"run_id"`
	ScheduleRID string `json:"schedule_rid"`
	RequestedBy string `json:"requested_by"`
}

type RunDueSchedulesRequest struct {
	Now   *time.Time `json:"now,omitempty"`
	Limit int        `json:"limit,omitempty"`
}

type ScheduleTriggerEventRequest struct {
	Type        string            `json:"type"`
	EventType   string            `json:"event_type,omitempty"`
	TargetRID   string            `json:"target_rid"`
	Branch      string            `json:"branch,omitempty"`
	OccurredAt  *time.Time        `json:"occurred_at,omitempty"`
	Diagnostics map[string]string `json:"diagnostics,omitempty"`
}

type ScheduleDispatchResult struct {
	ScheduleRID  string            `json:"schedule_rid"`
	RunRID       string            `json:"run_rid,omitempty"`
	Outcome      string            `json:"outcome"`
	BuildRID     string            `json:"build_rid,omitempty"`
	PendingReRun bool              `json:"pending_re_run,omitempty"`
	Diagnostics  map[string]string `json:"diagnostics,omitempty"`
}

type ScheduleDispatchResponse struct {
	Triggered int                      `json:"triggered"`
	Queued    int                      `json:"queued"`
	Ignored   int                      `json:"ignored"`
	Failed    int                      `json:"failed"`
	Results   []ScheduleDispatchResult `json:"results"`
}

type ConvertScheduleToProjectScopeRequest struct {
	ProjectScopeRIDs []string `json:"project_scope_rids"`
	Clearances       []string `json:"clearances,omitempty"`
}

type ScheduleServicePrincipal struct {
	ID               uuid.UUID `json:"id"`
	RID              string    `json:"rid"`
	DisplayName      string    `json:"display_name"`
	ProjectScopeRIDs []string  `json:"project_scope_rids"`
	Clearances       []string  `json:"clearances"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

type ConvertScheduleToProjectScopeResponse struct {
	Schedule         *Schedule                `json:"schedule"`
	ServicePrincipal ScheduleServicePrincipal `json:"service_principal"`
}

func ScheduleResourceRIDs(values ...json.RawMessage) []string {
	seen := map[string]struct{}{}
	for _, raw := range values {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			continue
		}
		collectScheduleRIDs(decoded, seen)
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	return sortUniqueScheduleStrings(out)
}

func collectScheduleRIDs(value any, seen map[string]struct{}) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if s, ok := child.(string); ok && strings.HasSuffix(strings.ToLower(key), "rid") && strings.TrimSpace(s) != "" {
				seen[strings.TrimSpace(s)] = struct{}{}
			}
			collectScheduleRIDs(child, seen)
		}
	case []any:
		for _, child := range v {
			collectScheduleRIDs(child, seen)
		}
	}
}

func sortUniqueScheduleStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
